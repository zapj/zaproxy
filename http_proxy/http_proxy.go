package http_proxy

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
)

var onExitFlushLoop func()

const (
	defaultTimeout = time.Minute * 10 // 增加默认超时时间到10分钟
)

// ReverseProxy is an HTTP Handler that takes an incoming request and
// sends it to another server, proxying the response back to the
// client, support http, also support https tunnel using http.hijacker
type ReverseProxy struct {
	// Set the timeout of the proxy server, default is 10 minutes
	Timeout time.Duration

	// Director must be a function which modifies
	// the request into a new request to be sent
	// using Transport. Its response is then copied
	// back to the original client unmodified.
	// Director must not access the provided Request
	// after returning.
	Director func(*http.Request)

	// The transport used to perform proxy requests.
	// default is http.DefaultTransport.
	Transport http.RoundTripper

	// FlushInterval specifies the flush interval
	// to flush to the client while copying the
	// response body. If zero, no periodic flushing is done.
	FlushInterval time.Duration

	// ErrorLog specifies an optional logger for errors
	// that occur when attempting to proxy the request.
	// If nil, logging goes to os.Stderr via the log package's
	// standard logger.
	ErrorLog *log.Logger

	// ModifyResponse is an optional function that
	// modifies the Response from the backend.
	// If it returns an error, the proxy returns a StatusBadGateway error.
	ModifyResponse func(*http.Response) error

	// DisableHTTPS controls whether HTTPS proxying is disabled
	DisableHTTPS bool

	// BufferSize specifies the size of the buffers used for copying data
	// If zero, a default size of 64KB is used
	BufferSize int

	// OnProxyConnect is an optional function that is called when a proxy connection is established
	OnProxyConnect func(*http.Request)

	// OnProxyError is an optional function that is called when a proxy error occurs
	OnProxyError func(*http.Request, error)
}

type requestCanceler interface {
	CancelRequest(req *http.Request)
}

// NewReverseProxy returns a new ReverseProxy that routes
// URLs to the scheme, host, and base path provided in target. If the
// target's path is "/base" and the incoming request was for "/dir",
// the target request will be for /base/dir. if the target's query is a=10
// and the incoming request's query is b=100, the target's request's query
// will be a=10&b=100.
// NewReverseProxy does not rewrite the Host header.
// To rewrite Host headers, use ReverseProxy directly with a custom
// Director policy.
func NewReverseProxy(target *url.URL) *ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)

		// If Host is empty, the Request.Write method uses
		// the value of URL.Host.
		// force use URL.Host
		req.Host = req.URL.Host
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}

		if _, ok := req.Header["User-Agent"]; !ok {
			req.Header.Set("User-Agent", "")
		}
	}

	return &ReverseProxy{Director: director}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	"Connection",
	"Proxy-Connection", // non-standard but still sent by libcurl and rejected by e.g. google
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",      // canonicalized version of "TE"
	"Trailer", // not Trailers per URL above; http://www.rfc-editor.org/errata_search.php?eid=4522
	"Transfer-Encoding",
	"Upgrade",
}

func (p *ReverseProxy) copyResponse(dst io.Writer, src io.Reader) {
	if p.FlushInterval != 0 {
		if wf, ok := dst.(writeFlusher); ok {
			mlw := &maxLatencyWriter{
				dst:     wf,
				latency: p.FlushInterval,
				done:    make(chan bool),
			}

			go mlw.flushLoop()
			defer mlw.stop()
			dst = mlw
		}
	}

	io.Copy(dst, src)
}

type writeFlusher interface {
	io.Writer
	http.Flusher
}

type maxLatencyWriter struct {
	dst     writeFlusher
	latency time.Duration
	mu      sync.Mutex
	done    chan bool
}

func (m *maxLatencyWriter) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dst.Write(b)
}

func (m *maxLatencyWriter) flushLoop() {
	t := time.NewTicker(m.latency)
	defer t.Stop()
	for {
		select {
		case <-m.done:
			if onExitFlushLoop != nil {
				onExitFlushLoop()
			}
			return
		case <-t.C:
			m.mu.Lock()
			m.dst.Flush()
			m.mu.Unlock()
		}
	}
}

func (m *maxLatencyWriter) stop() {
	m.done <- true
}

func (p *ReverseProxy) logf(format string, args ...interface{}) {
	if p.ErrorLog != nil {
		p.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func removeHeaders(header http.Header) {
	// Remove hop-by-hop headers listed in the "Connection" header.
	if c := header.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				header.Del(f)
			}
		}
	}

	// Remove hop-by-hop headers
	for _, h := range hopHeaders {
		if header.Get(h) != "" {
			header.Del(h)
		}
	}
}

func addXForwardedForHeader(req *http.Request) {
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
	}
}

func (p *ReverseProxy) ProxyHTTP(rw http.ResponseWriter, req *http.Request) {
	// 通知连接建立（如果设置了回调）
	if p.OnProxyConnect != nil {
		p.OnProxyConnect(req)
	}

	transport := p.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	outreq := new(http.Request)
	// Shallow copies of maps, like header
	*outreq = *req

	// 使用请求上下文，确保请求可以被取消
	if cn, ok := rw.(http.CloseNotifier); ok {
		if requestCanceler, ok := transport.(requestCanceler); ok {
			reqDone := make(chan struct{})
			defer close(reqDone)
			clientGone := cn.CloseNotify()

			go func() {
				select {
				case <-clientGone:
					requestCanceler.CancelRequest(outreq)
					if p.OnProxyError != nil {
						p.OnProxyError(req, fmt.Errorf("client connection closed"))
					}
				case <-reqDone:
				}
			}()
		}
	}

	// 应用Director函数修改请求
	p.Director(outreq)
	outreq.Close = false

	// 复制并修改请求头
	outreq.Header = make(http.Header)
	copyHeader(outreq.Header, req.Header)
	removeHeaders(outreq.Header)
	addXForwardedForHeader(outreq)

	// 记录代理请求信息
	if p.ErrorLog != nil {
		p.logf("http: proxy request: %s %s", outreq.Method, outreq.URL)
	}

	// 发送请求到目标服务器
	res, err := transport.RoundTrip(outreq)
	if err != nil {
		p.logf("http: proxy error: %v", err)
		if p.OnProxyError != nil {
			p.OnProxyError(req, err)
		}
		http.Error(rw, "Bad Gateway", http.StatusBadGateway)
		return
	}

	// 处理响应
	removeHeaders(res.Header)

	// 应用ModifyResponse函数
	if p.ModifyResponse != nil {
		if err := p.ModifyResponse(res); err != nil {
			p.logf("http: proxy modify response error: %v", err)
			if p.OnProxyError != nil {
				p.OnProxyError(req, err)
			}
			http.Error(rw, "Bad Gateway", http.StatusBadGateway)
			return
		}
	}

	// 复制响应头
	copyHeader(rw.Header(), res.Header)

	// 处理Trailer头
	if len(res.Trailer) > 0 {
		trailerKeys := make([]string, 0, len(res.Trailer))
		for k := range res.Trailer {
			trailerKeys = append(trailerKeys, k)
		}
		rw.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	// 写入状态码
	rw.WriteHeader(res.StatusCode)

	// 强制分块传输（如果有Trailer）
	if len(res.Trailer) > 0 {
		if fl, ok := rw.(http.Flusher); ok {
			fl.Flush()
		}
	}

	// 使用配置的缓冲区大小复制响应体
	if res.Body != nil {
		var dst io.Writer = rw

		// 使用FlushInterval进行定期刷新
		if p.FlushInterval != 0 {
			if wf, ok := dst.(writeFlusher); ok {
				mlw := &maxLatencyWriter{
					dst:     wf,
					latency: p.FlushInterval,
					done:    make(chan bool),
				}
				go mlw.flushLoop()
				defer mlw.stop()
				dst = mlw
			}
		}

		// 使用配置的缓冲区大小
		bufSize := 32 * 1024 // 默认32KB
		if p.BufferSize > 0 {
			bufSize = p.BufferSize
		}

		buf := make([]byte, bufSize)
		_, err := io.CopyBuffer(dst, res.Body, buf)
		if err != nil && !isClosedConnError(err) {
			p.logf("http: proxy error copying response: %v", err)
			if p.OnProxyError != nil {
				p.OnProxyError(req, err)
			}
		}

		// 关闭响应体
		res.Body.Close()
	}

	// 复制Trailer头
	copyHeader(rw.Header(), res.Trailer)

	// 记录成功完成的请求
	if p.ErrorLog != nil {
		p.logf("http: proxy response complete: %d %s", res.StatusCode, http.StatusText(res.StatusCode))
	}
}

func (p *ReverseProxy) ProxyHTTPS(rw http.ResponseWriter, req *http.Request) {
	// 如果禁用了HTTPS代理，返回错误
	if p.DisableHTTPS {
		p.logf("https: proxy disabled by configuration")
		http.Error(rw, "HTTPS Proxy Disabled", http.StatusServiceUnavailable)
		return
	}

	// 通知连接建立（如果设置了回调）
	if p.OnProxyConnect != nil {
		p.OnProxyConnect(req)
	}

	hij, ok := rw.(http.Hijacker)
	if !ok {
		p.logf("http server does not support hijacker")
		http.Error(rw, "Proxy Server Error", http.StatusServiceUnavailable)
		return
	}

	clientConn, _, err := hij.Hijack()
	if err != nil {
		p.logf("http: proxy hijack error: %v", err)
		http.Error(rw, "Proxy Server Error", http.StatusServiceUnavailable)
		if p.OnProxyError != nil {
			p.OnProxyError(req, err)
		}
		return
	}

	// 使用defer和recover来确保连接总是被关闭
	defer func() {
		if r := recover(); r != nil {
			p.logf("http: proxy panic: %v", r)
			if p.OnProxyError != nil {
				var err error
				switch e := r.(type) {
				case error:
					err = e
				default:
					err = fmt.Errorf("%v", r)
				}
				p.OnProxyError(req, err)
			}
		}
		clientConn.Close()
	}()

	// 设置合理的超时时间
	timeout := defaultTimeout
	if p.Timeout > 0 {
		timeout = p.Timeout
	}

	// 使用带超时的拨号器，增加超时时间
	dialer := &net.Dialer{
		Timeout:   60 * time.Second, // 增加连接超时到60秒
		KeepAlive: 60 * time.Second, // 增加保活时间到60秒
		DualStack: true,             // 启用双栈支持
	}

	// 尝试建立到目标服务器的连接
	proxyConn, err := dialer.Dial("tcp", req.URL.Host)
	if err != nil {
		p.logf("http: proxy dial error: %v", err)
		clientConn.Write([]byte("HTTP/1.1 504 Gateway Timeout\r\n\r\n"))
		if p.OnProxyError != nil {
			p.OnProxyError(req, err)
		}
		return
	}
	defer proxyConn.Close()

	// 设置较长的读写超时
	deadline := time.Now().Add(timeout)
	if err = clientConn.SetDeadline(deadline); err != nil {
		p.logf("http: proxy error setting client deadline: %v", err)
		if p.OnProxyError != nil {
			p.OnProxyError(req, err)
		}
		return
	}
	if err = proxyConn.SetDeadline(deadline); err != nil {
		p.logf("http: proxy error setting server deadline: %v", err)
		if p.OnProxyError != nil {
			p.OnProxyError(req, err)
		}
		return
	}

	// 发送连接成功响应
	if _, err = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		p.logf("http: proxy error writing response: %v", err)
		if p.OnProxyError != nil {
			p.OnProxyError(req, err)
		}
		return
	}

	// 使用配置的缓冲区大小或默认值
	bufSize := 64 * 1024 // 默认64KB缓冲区
	if p.BufferSize > 0 {
		bufSize = p.BufferSize
	}

	// 使用错误通道
	errChan := make(chan error, 2)

	// 客户端到服务器
	go func() {
		buf := make([]byte, bufSize)
		_, err := io.CopyBuffer(proxyConn, clientConn, buf)
		if err != nil && !isClosedConnError(err) {
			p.logf("http: proxy error copying to server: %v", err)
			if p.OnProxyError != nil {
				p.OnProxyError(req, err)
			}
			errChan <- err
		} else {
			// 尝试优雅关闭连接
			if tcpConn, ok := proxyConn.(*net.TCPConn); ok {
				tcpConn.CloseWrite()
			}
			errChan <- nil
		}
	}()

	// 服务器到客户端
	go func() {
		buf := make([]byte, bufSize)
		_, err := io.CopyBuffer(clientConn, proxyConn, buf)
		if err != nil && !isClosedConnError(err) {
			p.logf("http: proxy error copying to client: %v", err)
			if p.OnProxyError != nil {
				p.OnProxyError(req, err)
			}
			errChan <- err
		} else {
			// 尝试优雅关闭连接
			if tcpConn, ok := clientConn.(*net.TCPConn); ok {
				tcpConn.CloseWrite()
			}
			errChan <- nil
		}
	}()

	// 等待两个goroutine完成或出错
	for i := 0; i < 2; i++ {
		if err := <-errChan; err != nil {
			return // 如果有错误发生，立即返回
		}
	}
}

// 判断是否是连接关闭错误
func isClosedConnError(err error) bool {
	if err == nil {
		return false
	}

	// 检查常见的连接关闭错误
	if err == io.EOF || err == io.ErrClosedPipe || err == io.ErrUnexpectedEOF {
		return true
	}

	// 检查网络特定错误
	if ne, ok := err.(*net.OpError); ok {
		if ne.Timeout() || ne.Temporary() {
			return true
		}
		// 检查底层错误
		if ne.Err != nil {
			if syscallErr, ok := ne.Err.(*os.SyscallError); ok {
				if syscallErr.Err == syscall.ECONNRESET || syscallErr.Err == syscall.EPIPE {
					return true
				}
			}
		}
	}

	// 检查HTTP2特定错误
	if strings.Contains(err.Error(), "http2: client conn not usable") ||
		strings.Contains(err.Error(), "http2: server sent GOAWAY") {
		return true
	}

	// 检查TLS错误
	if strings.Contains(err.Error(), "tls: use of closed connection") ||
		strings.Contains(err.Error(), "tls: protocol is shutdown") {
		return true
	}

	// 检查错误字符串
	str := err.Error()
	return strings.Contains(str, "use of closed network connection") ||
		strings.Contains(str, "connection reset by peer") ||
		strings.Contains(str, "broken pipe") ||
		strings.Contains(str, "i/o timeout") ||
		strings.Contains(str, "connection refused") ||
		strings.Contains(str, "connection timed out") ||
		strings.Contains(str, "EOF") ||
		strings.Contains(str, "write: broken pipe") ||
		strings.Contains(str, "protocol wrong type for socket") ||
		strings.Contains(str, "network connection closed") ||
		strings.Contains(str, "wsarecv: An existing connection was forcibly closed") ||
		strings.Contains(str, "readfrom: connection refused")
}

func (p *ReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// 基本请求验证
	if req.URL == nil {
		p.logf("http: proxy received invalid request: nil URL")
		http.Error(rw, "Bad Request", http.StatusBadRequest)
		return
	}

	// 设置请求开始时间（用于记录请求处理时间）
	start := time.Now()

	// 设置请求上下文超时
	timeout := defaultTimeout
	if p.Timeout > 0 {
		timeout = p.Timeout
	}
	ctx, cancel := context.WithTimeout(req.Context(), timeout)
	defer cancel()
	req = req.WithContext(ctx)

	// 记录请求信息（如果启用了调试日志）
	if p.ErrorLog != nil {
		p.logf("http: proxy received request: %s %s %s", req.Method, req.URL, req.Proto)
	}

	// 根据请求方法选择处理方式
	var err error
	switch req.Method {
	case "CONNECT":
		p.ProxyHTTPS(rw, req)
	case "GET", "POST", "PUT", "DELETE", "HEAD", "OPTIONS", "PATCH":
		p.ProxyHTTP(rw, req)
	default:
		p.logf("http: proxy received unsupported method: %s", req.Method)
		http.Error(rw, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// 记录请求处理时间和结果
	if p.ErrorLog != nil {
		duration := time.Since(start)
		if err != nil {
			p.logf("http: proxy completed request with error: %s %s %v (took %v)",
				req.Method, req.URL, err, duration)
		} else {
			p.logf("http: proxy completed request successfully: %s %s (took %v)",
				req.Method, req.URL, duration)
		}
	}
}

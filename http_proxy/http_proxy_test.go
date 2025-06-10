package http_proxy

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestReverseProxy_ServeHTTP(t *testing.T) {
	// 创建一个后端服务器
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("Failed to read request body: %v", err)
				return
			}
			w.Write(body) // 回显POST数据
		} else {
			fmt.Fprintf(w, "Hello from backend, path: %s", r.URL.Path)
		}
	}))
	defer backend.Close()

	// 解析后端URL
	backendURL, err := url.Parse(backend.URL)
	if err != nil {
		t.Fatal(err)
	}

	// 创建代理服务器
	proxy := NewReverseProxy(backendURL)
	proxy.BufferSize = 1024
	proxy.Timeout = 5 * time.Second

	// 测试GET请求
	t.Run("GET request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		expected := "Hello from backend, path: /test"
		if !strings.Contains(w.Body.String(), expected) {
			t.Errorf("Expected body to contain %q, got %q", expected, w.Body.String())
		}
	})

	// 测试POST请求
	t.Run("POST request", func(t *testing.T) {
		body := []byte("test data")
		req := httptest.NewRequest("POST", "http://example.com/test", bytes.NewReader(body))
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
		}

		if !bytes.Equal(w.Body.Bytes(), body) {
			t.Errorf("Expected body %q, got %q", body, w.Body.Bytes())
		}
	})

	// 测试无效请求
	t.Run("Invalid request", func(t *testing.T) {
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		req.URL = nil // 使请求无效
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
		}
	})

	// 测试回调函数
	t.Run("Callback functions", func(t *testing.T) {
		var connectCalled, errorCalled bool
		proxy.OnProxyConnect = func(req *http.Request) {
			connectCalled = true
		}
		proxy.OnProxyError = func(req *http.Request, err error) {
			errorCalled = true
		}

		// 正常请求应该触发connect回调
		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		w := httptest.NewRecorder()
		proxy.ServeHTTP(w, req)

		if !connectCalled {
			t.Error("OnProxyConnect was not called")
		}

		// 无效请求应该触发error回调
		req = httptest.NewRequest("GET", "http://invalid.example.com", nil)
		w = httptest.NewRecorder()
		proxy.Transport = &http.Transport{
			Dial: func(network, addr string) (net.Conn, error) {
				return nil, fmt.Errorf("simulated error")
			},
		}
		proxy.ServeHTTP(w, req)

		if !errorCalled {
			t.Error("OnProxyError was not called")
		}
	})

	// 测试超时
	t.Run("Timeout", func(t *testing.T) {
		slowBackend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(2 * time.Second)
			fmt.Fprintln(w, "Hello")
		}))
		defer slowBackend.Close()

		slowURL, err := url.Parse(slowBackend.URL)
		if err != nil {
			t.Fatal(err)
		}

		proxy := NewReverseProxy(slowURL)
		proxy.Timeout = 1 * time.Second

		req := httptest.NewRequest("GET", "http://example.com/test", nil)
		w := httptest.NewRecorder()

		proxy.ServeHTTP(w, req)

		if w.Code != http.StatusGatewayTimeout && w.Code != http.StatusBadGateway {
			t.Errorf("Expected timeout status (504 or 502), got %d", w.Code)
		}
	})
}

func TestIsClosedConnError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "EOF error",
			err:  io.EOF,
			want: true,
		},
		{
			name: "closed pipe error",
			err:  io.ErrClosedPipe,
			want: true,
		},
		{
			name: "unexpected EOF error",
			err:  io.ErrUnexpectedEOF,
			want: true,
		},
		{
			name: "use of closed network connection error",
			err:  fmt.Errorf("use of closed network connection"),
			want: true,
		},
		{
			name: "connection reset error",
			err:  fmt.Errorf("connection reset by peer"),
			want: true,
		},
		{
			name: "other error",
			err:  fmt.Errorf("some other error"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isClosedConnError(tt.err); got != tt.want {
				t.Errorf("isClosedConnError() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewReverseProxy(t *testing.T) {
	tests := []struct {
		name       string
		targetURL  string
		wantScheme string
		wantHost   string
		wantPath   string
	}{
		{
			name:       "simple URL",
			targetURL:  "http://example.com",
			wantScheme: "http",
			wantHost:   "example.com",
			wantPath:   "",
		},
		{
			name:       "URL with path",
			targetURL:  "https://example.com/base",
			wantScheme: "https",
			wantHost:   "example.com",
			wantPath:   "/base",
		},
		{
			name:       "URL with query",
			targetURL:  "http://example.com?key=value",
			wantScheme: "http",
			wantHost:   "example.com",
			wantPath:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := url.Parse(tt.targetURL)
			if err != nil {
				t.Fatal(err)
			}

			proxy := NewReverseProxy(target)

			// 创建测试请求
			req := httptest.NewRequest("GET", "http://localhost/test", nil)
			proxy.Director(req)

			if req.URL.Scheme != tt.wantScheme {
				t.Errorf("Scheme = %v, want %v", req.URL.Scheme, tt.wantScheme)
			}
			if req.URL.Host != tt.wantHost {
				t.Errorf("Host = %v, want %v", req.URL.Host, tt.wantHost)
			}
			if !strings.HasPrefix(req.URL.Path, tt.wantPath) {
				t.Errorf("Path = %v, want prefix %v", req.URL.Path, tt.wantPath)
			}
		})
	}
}

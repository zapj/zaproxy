package http_proxy

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthCache 用于缓存认证结果
type AuthCache struct {
	cache map[string]authEntry
	mu    sync.RWMutex
}

type authEntry struct {
	valid    bool
	username string
	password string
	expireAt time.Time
}

var (
	authCache = &AuthCache{
		cache: make(map[string]authEntry),
	}
	cacheDuration = 5 * time.Minute // 缓存有效期
)

// GetBasicAuth 从请求中提取基本认证信息
func GetBasicAuth(r *http.Request) (username, password string, ok bool) {
	auth := r.Header.Get("Proxy-Authorization")
	if auth == "" {
		return "", "", false
	}

	// 检查缓存
	authCache.mu.RLock()
	if entry, exists := authCache.cache[auth]; exists && time.Now().Before(entry.expireAt) {
		authCache.mu.RUnlock()
		if entry.valid {
			return entry.username, entry.password, true
		}
		return "", "", false
	}
	authCache.mu.RUnlock()

	// 解析认证信息
	username, password, ok = parseBasicAuth(auth)

	// 缓存结果
	if ok {
		cacheAuthResult(auth, username, password, true)
	} else {
		cacheAuthResult(auth, "", "", false)
	}

	return username, password, ok
}

// BasicAuth 生成基本认证字符串
func BasicAuth(username, password string) string {
	auth := username + ":" + password
	return base64.StdEncoding.EncodeToString([]byte(auth))
}

// parseBasicAuth 解析基本认证头
func parseBasicAuth(auth string) (username, password string, ok bool) {
	const prefix = "Basic "

	if len(auth) < len(prefix) || !strings.EqualFold(auth[:len(prefix)], prefix) {
		return "", "", false
	}

	c, err := base64.StdEncoding.DecodeString(auth[len(prefix):])
	if err != nil {
		return "", "", false
	}

	cs := string(c)
	username, password, ok = strings.Cut(cs, ":")
	if !ok {
		return "", "", false
	}

	// 验证凭证格式
	if !isValidCredentials(username, password) {
		return "", "", false
	}

	return username, password, true
}

// isValidCredentials 验证凭证格式是否合法
func isValidCredentials(username, password string) bool {
	// 检查用户名和密码是否包含非法字符
	return len(username) > 0 && len(username) <= 64 &&
		len(password) <= 128 &&
		!strings.ContainsAny(username, "\x00\n\r") &&
		!strings.ContainsAny(password, "\x00\n\r")
}

// cacheAuthResult 缓存认证结果
func cacheAuthResult(auth, username, password string, valid bool) {
	authCache.mu.Lock()
	defer authCache.mu.Unlock()

	authCache.cache[auth] = authEntry{
		valid:    valid,
		username: username,
		password: password,
		expireAt: time.Now().Add(cacheDuration),
	}
}

// CompareCredentials 安全地比较用户名和密码
func CompareCredentials(inputUser, inputPass, expectedUser, expectedPass string) bool {
	// 使用 subtle.ConstantTimeCompare 来防止时序攻击
	userMatch := subtle.ConstantTimeCompare([]byte(inputUser), []byte(expectedUser)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(inputPass), []byte(expectedPass)) == 1
	return userMatch && passMatch
}

// InitAuthCache 初始化认证缓存并启动清理任务
func InitAuthCache() {
	// 启动定期清理过期缓存的goroutine
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			now := time.Now()
			authCache.mu.Lock()
			for k, v := range authCache.cache {
				if now.After(v.expireAt) {
					delete(authCache.cache, k)
				}
			}
			authCache.mu.Unlock()
		}
	}()
}

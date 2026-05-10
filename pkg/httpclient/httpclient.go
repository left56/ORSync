package httpclient

import (
	"net"
	"net/http"
	"time"
)

// DefaultTimeout 为对外部 API（Outline / RAGFlow）请求的默认总超时时间。
const DefaultTimeout = 30 * time.Second

// New 返回配置了连接池与总超时的 *http.Client，适用于上传较大正文或等待解析的场景。
func New() *http.Client {
	return NewWithTimeout(DefaultTimeout)
}

// NewWithTimeout 与 New 相同，但可自定义总超时（须大于 0）。
func NewWithTimeout(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		MaxIdleConnsPerHost:   8,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}

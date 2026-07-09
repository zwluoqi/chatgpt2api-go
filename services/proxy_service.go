package services

import (
	"fmt"
	"strings"
	"time"

	"chatgpt2api-go/config"
)

type ProxySettings struct {
	Enabled bool     `json:"enabled"`
	URL     string   `json:"url"`  // 兼容旧前端：第一条代理
	URLs    []string `json:"urls"` // 全部代理地址
}

type ProxyTestResult struct {
	OK        bool    `json:"ok"`
	Status    int     `json:"status"`
	LatencyMS int     `json:"latency_ms"`
	Error     *string `json:"error"`
}

func GetProxySettings() ProxySettings {
	urls := config.GetProxyURLs()
	first := ""
	if len(urls) > 0 {
		first = urls[0]
	}
	return ProxySettings{
		Enabled: len(urls) > 0,
		URL:     first,
		URLs:    urls,
	}
}

// UpdateProxySettings 更新代理。urls 为空时回退到 url（单条/多行）；enabled=false 表示清空。
func UpdateProxySettings(enabled *bool, urls []string, urlFallback string) (ProxySettings, error) {
	if enabled != nil && !*enabled {
		urls = nil
		urlFallback = ""
	}
	if len(urls) == 0 && strings.TrimSpace(urlFallback) != "" {
		// 兼容旧前端只传单个 url（可能是多行）
		if err := config.UpdateProxyURL(urlFallback); err != nil {
			return ProxySettings{}, err
		}
		return GetProxySettings(), nil
	}
	if err := config.UpdateProxyURLs(urls); err != nil {
		return ProxySettings{}, err
	}
	return GetProxySettings(), nil
}

func TestProxy(proxyURL string, timeout time.Duration) (result ProxyTestResult) {
	started := time.Now()
	defer func() {
		if r := recover(); r != nil {
			result = ProxyTestResult{
				OK:        false,
				Status:    0,
				LatencyMS: int(time.Since(started).Milliseconds()),
				Error:     strPtr(fmt.Sprintf("proxy test failed: %v", r)),
			}
		}
	}()

	tc, err := NewTLSClientWithProxyURLAndTimeout(proxyURL, timeout)
	if err != nil {
		return ProxyTestResult{
			OK:        false,
			Status:    0,
			LatencyMS: int(time.Since(started).Milliseconds()),
			Error:     strPtr(err.Error()),
		}
	}

	resp, err := tc.Get("https://chatgpt.com/api/auth/csrf", map[string]string{
		"user-agent": "Mozilla/5.0 (chatgpt2api-go proxy test)",
	})
	if err != nil {
		return ProxyTestResult{
			OK:        false,
			Status:    0,
			LatencyMS: int(time.Since(started).Milliseconds()),
			Error:     strPtr(err.Error()),
		}
	}
	defer resp.Body.Close()

	status := resp.StatusCode
	var errorText *string
	ok := status >= 200 && status < 400
	if !ok {
		errorText = strPtr(fmt.Sprintf("HTTP %d", status))
	}
	return ProxyTestResult{
		OK:        ok,
		Status:    status,
		LatencyMS: int(time.Since(started).Milliseconds()),
		Error:     errorText,
	}
}

func strPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

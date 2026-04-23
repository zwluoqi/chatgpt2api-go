package services

import (
	"fmt"
	"time"

	"chatgpt2api-go/config"
)

type ProxySettings struct {
	Enabled bool   `json:"enabled"`
	URL     string `json:"url"`
}

type ProxyTestResult struct {
	OK        bool    `json:"ok"`
	Status    int     `json:"status"`
	LatencyMS int     `json:"latency_ms"`
	Error     *string `json:"error"`
}

func GetProxySettings() ProxySettings {
	enabled, proxyURL := config.GetProxySettings()
	return ProxySettings{
		Enabled: enabled,
		URL:     proxyURL,
	}
}

func UpdateProxySettings(enabled *bool, proxyURL string) (ProxySettings, error) {
	if enabled != nil && !*enabled {
		proxyURL = ""
	}
	if err := config.UpdateProxyURL(proxyURL); err != nil {
		return ProxySettings{}, err
	}
	return GetProxySettings(), nil
}

func TestProxy(proxyURL string, timeout time.Duration) ProxyTestResult {
	_ = timeout
	started := time.Now()

	tc, err := NewTLSClientWithProxyURL(proxyURL)
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
	ok := status < 500
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

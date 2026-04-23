package services

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"strings"

	"chatgpt2api-go/config"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	tls_profiles "github.com/bogdanfinn/tls-client/profiles"
)

var tlsProfiles = []tls_profiles.ClientProfile{
	tls_profiles.Chrome_131,
	tls_profiles.Chrome_131_PSK,
	tls_profiles.Chrome_124,
	tls_profiles.Chrome_120,
}

var defaultUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36",
}

var chromeGetHeaderOrder = []string{
	"host", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform",
	"upgrade-insecure-requests", "user-agent", "accept",
	"sec-fetch-site", "sec-fetch-mode", "sec-fetch-user", "sec-fetch-dest",
	"accept-encoding", "accept-language",
}

var chromePostHeaderOrder = []string{
	"host", "content-length", "sec-ch-ua", "sec-ch-ua-mobile",
	"sec-ch-ua-platform", "content-type", "user-agent", "accept",
	"origin", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-dest",
	"referer", "accept-encoding", "accept-language",
}

type TLSClient struct {
	client    tls_client.HttpClient
	UserAgent string
}

func NewTLSClient() (*TLSClient, error) {
	profile := tlsProfiles[rand.Intn(len(tlsProfiles))]
	ua := defaultUserAgents[rand.Intn(len(defaultUserAgents))]

	jar := tls_client.NewCookieJar()
	options := []tls_client.HttpClientOption{
		tls_client.WithClientProfile(profile),
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithCookieJar(jar),
		tls_client.WithRandomTLSExtensionOrder(),
	}
	if proxyURL := getConfiguredProxyURL(); proxyURL != "" {
		options = append(options, tls_client.WithProxyUrl(proxyURL))
	}

	c, err := tls_client.NewHttpClient(nil, options...)
	if err != nil {
		return nil, err
	}

	return &TLSClient{client: c, UserAgent: ua}, nil
}

func NewTLSClientWithUA(userAgent string) (*TLSClient, error) {
	tc, err := NewTLSClient()
	if err != nil {
		return nil, err
	}
	if userAgent != "" {
		tc.UserAgent = userAgent
	}
	return tc, nil
}

func (tc *TLSClient) setCommonHeaders(req *fhttp.Request, isPost bool) {
	if isPost {
		req.Header[fhttp.HeaderOrderKey] = chromePostHeaderOrder
	} else {
		req.Header[fhttp.HeaderOrderKey] = chromeGetHeaderOrder
	}

	if req.Header.Get("user-agent") == "" {
		req.Header.Set("user-agent", tc.UserAgent)
	}
	if req.Header.Get("accept-language") == "" {
		req.Header.Set("accept-language", "en-US,en;q=0.9")
	}
	if req.Header.Get("accept-encoding") == "" {
		req.Header.Set("accept-encoding", "gzip, deflate, br, zstd")
	}
	if req.Header.Get("sec-ch-ua") == "" {
		req.Header.Set("sec-ch-ua", `"Chromium";v="131", "Not_A Brand";v="24"`)
	}
	if req.Header.Get("sec-ch-ua-mobile") == "" {
		req.Header.Set("sec-ch-ua-mobile", "?0")
	}
	if req.Header.Get("sec-ch-ua-platform") == "" {
		req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	}
	if req.Header.Get("dnt") == "" {
		req.Header.Set("dnt", "1")
	}
}

func (tc *TLSClient) Do(req *fhttp.Request) (*fhttp.Response, error) {
	isPost := req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH"
	tc.setCommonHeaders(req, isPost)
	return tc.client.Do(req)
}

func (tc *TLSClient) Get(urlStr string, headers map[string]string) (*fhttp.Response, error) {
	req, err := fhttp.NewRequest("GET", urlStr, nil)
	if err != nil {
		return nil, err
	}
	req.Header = fhttp.Header{}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("accept") == "" {
		req.Header.Set("accept", "*/*")
	}
	if req.Header.Get("sec-fetch-site") == "" {
		req.Header.Set("sec-fetch-site", "same-origin")
	}
	if req.Header.Get("sec-fetch-mode") == "" {
		req.Header.Set("sec-fetch-mode", "cors")
	}
	if req.Header.Get("sec-fetch-dest") == "" {
		req.Header.Set("sec-fetch-dest", "empty")
	}
	return tc.Do(req)
}

func (tc *TLSClient) PostJSON(urlStr string, headers map[string]string, body any) (*fhttp.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := fhttp.NewRequest("POST", urlStr, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header = fhttp.Header{}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("content-type") == "" {
		req.Header.Set("content-type", "application/json")
	}
	if req.Header.Get("accept") == "" {
		req.Header.Set("accept", "application/json")
	}
	if req.Header.Get("sec-fetch-site") == "" {
		req.Header.Set("sec-fetch-site", "same-origin")
	}
	if req.Header.Get("sec-fetch-mode") == "" {
		req.Header.Set("sec-fetch-mode", "cors")
	}
	if req.Header.Get("sec-fetch-dest") == "" {
		req.Header.Set("sec-fetch-dest", "empty")
	}
	return tc.Do(req)
}

func (tc *TLSClient) PutData(urlStr string, headers map[string]string, data []byte) (*fhttp.Response, error) {
	req, err := fhttp.NewRequest("PUT", urlStr, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header = fhttp.Header{}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return tc.Do(req)
}

func (tc *TLSClient) GetCookieValue(rawURL, name string) string {
	u, err := fhttp.NewRequest("GET", rawURL, nil)
	if err != nil {
		return ""
	}
	cookies := tc.client.GetCookies(u.URL)
	for _, c := range cookies {
		if c.Name == name {
			return c.Value
		}
	}
	return ""
}

func ReadResponseJSON(resp *fhttp.Response) (map[string]any, error) {
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("invalid JSON: %s", truncateStr(string(data), 200))
	}
	return result, nil
}

func truncateStr(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

func getConfiguredProxyURL() string {
	if config.Config == nil {
		return ""
	}
	return strings.TrimSpace(config.Config.ProxyURL)
}

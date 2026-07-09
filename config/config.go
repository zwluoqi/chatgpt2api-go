package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

type AppSettings struct {
	AuthKey                      string
	ProxyURL                     string   // 兼容旧字段：等于 ProxyURLs 的第一条
	ProxyURLs                    []string // 支持配置多条代理地址，按轮询使用
	ChatCompletionsEnabled       bool
	InsecureSkipVerify           bool
	Host                         string
	Port                         int
	ConfigFile                   string
	AccountsFile                 string
	RefreshAccountIntervalMinute int
	ImagePollTimeoutSecs         int
	LogMaxEntries                int
	BaseDir                      string
	DataDir                      string
}

var Config *AppSettings
var configMu sync.Mutex

func Init(baseDir string) error {
	cfg, err := loadSettings(baseDir)
	if err != nil {
		return err
	}
	Config = cfg
	return nil
}

func loadSettings(baseDir string) (*AppSettings, error) {
	dataDir := filepath.Join(baseDir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	configFile := filepath.Join(baseDir, "config.json")
	rawConfig := make(map[string]any)

	if info, err := os.Stat(configFile); err == nil && !info.IsDir() {
		data, err := os.ReadFile(configFile)
		if err == nil {
			text := strings.TrimSpace(string(data))
			if text != "" {
				if err := json.Unmarshal([]byte(text), &rawConfig); err != nil {
					return nil, fmt.Errorf("invalid config.json: %w", err)
				}
			}
		}
	}

	authKey := strings.TrimSpace(os.Getenv("CHATGPT2API_AUTH_KEY"))
	if authKey == "" {
		if v, ok := rawConfig["auth-key"]; ok {
			authKey = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	if authKey == "" {
		return nil, fmt.Errorf(
			"auth-key is not set!\n" +
				"Please set it via:\n" +
				"1. Environment variable: CHATGPT2API_AUTH_KEY=your_auth_key\n" +
				"2. Or in config.json: \"auth-key\": \"your_auth_key\"",
		)
	}

	rawProxy := strings.TrimSpace(os.Getenv("CHATGPT2API_PROXY_URL"))
	if rawProxy == "" {
		if v, ok := rawConfig["proxy-url"]; ok {
			rawProxy = proxyRawToString(v)
		}
	}
	proxyURLs, err := parseProxyList(rawProxy)
	if err != nil {
		return nil, fmt.Errorf(
			"invalid proxy-url: %w\n"+
				"Please set it via:\n"+
				"1. Environment variable: CHATGPT2API_PROXY_URL=http://host:port （多条用换行或逗号分隔）\n"+
				"2. Or in config.json: \"proxy-url\": \"http://host:port\" （多条用换行分隔或写成数组）",
			err,
		)
	}
	proxyURL := ""
	if len(proxyURLs) > 0 {
		proxyURL = proxyURLs[0]
	}

	chatCompletionsEnabled := true
	if v := strings.TrimSpace(os.Getenv("CHATGPT2API_ENABLE_CHAT_COMPLETIONS")); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CHATGPT2API_ENABLE_CHAT_COMPLETIONS: %w", err)
		}
		chatCompletionsEnabled = parsed
	} else if v, ok := rawConfig["enable-chat-completions"]; ok {
		parsed, err := parseBoolValue(v)
		if err != nil {
			return nil, fmt.Errorf("invalid enable-chat-completions: %w", err)
		}
		chatCompletionsEnabled = parsed
	}

	refreshInterval := 60
	if v, ok := rawConfig["refresh_account_interval_minute"]; ok {
		switch val := v.(type) {
		case float64:
			refreshInterval = int(val)
		case int:
			refreshInterval = val
		}
	}

	insecureSkipVerify := false
	if v := strings.TrimSpace(os.Getenv("CHATGPT2API_INSECURE_SKIP_VERIFY")); v != "" {
		parsed, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("invalid CHATGPT2API_INSECURE_SKIP_VERIFY: %w", err)
		}
		insecureSkipVerify = parsed
	} else if v, ok := rawConfig["insecure-skip-verify"]; ok {
		parsed, err := parseBoolValue(v)
		if err != nil {
			return nil, fmt.Errorf("invalid insecure-skip-verify: %w", err)
		}
		insecureSkipVerify = parsed
	}

	imagePollTimeoutSecs := 180
	if v, ok := rawConfig["image-poll-timeout-secs"]; ok {
		parsed, err := parseIntValue(v)
		if err != nil {
			return nil, fmt.Errorf("invalid image-poll-timeout-secs: %w", err)
		}
		if parsed > 0 {
			imagePollTimeoutSecs = parsed
		}
	}

	logMaxEntries := 200
	if v, ok := rawConfig["log-max-entries"]; ok {
		parsed, err := parseIntValue(v)
		if err != nil {
			return nil, fmt.Errorf("invalid log-max-entries: %w", err)
		}
		if parsed > 0 {
			logMaxEntries = parsed
		}
	}

	return &AppSettings{
		AuthKey:                      authKey,
		ProxyURL:                     proxyURL,
		ProxyURLs:                    proxyURLs,
		ChatCompletionsEnabled:       chatCompletionsEnabled,
		InsecureSkipVerify:           insecureSkipVerify,
		Host:                         "0.0.0.0",
		Port:                         8000,
		ConfigFile:                   configFile,
		AccountsFile:                 filepath.Join(dataDir, "accounts.json"),
		RefreshAccountIntervalMinute: refreshInterval,
		ImagePollTimeoutSecs:         imagePollTimeoutSecs,
		LogMaxEntries:                logMaxEntries,
		BaseDir:                      baseDir,
		DataDir:                      dataDir,
	}, nil
}

func parseIntValue(v any) (int, error) {
	switch val := v.(type) {
	case float64:
		return int(val), nil
	case int:
		return val, nil
	case string:
		text := strings.TrimSpace(val)
		if text == "" {
			return 0, fmt.Errorf("integer value is required")
		}
		return strconv.Atoi(text)
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", v))
		if text == "" {
			return 0, fmt.Errorf("integer value is required")
		}
		return strconv.Atoi(text)
	}
}

func parseBoolValue(v any) (bool, error) {
	switch val := v.(type) {
	case bool:
		return val, nil
	case string:
		return strconv.ParseBool(strings.TrimSpace(val))
	default:
		text := strings.TrimSpace(fmt.Sprintf("%v", v))
		if text == "" {
			return false, fmt.Errorf("boolean value is required")
		}
		return strconv.ParseBool(text)
	}
}

func validateProxyURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if parsed.Scheme == "" {
		return fmt.Errorf("scheme is required")
	}
	switch parsed.Scheme {
	case "http", "https", "socks4", "socks4a", "socks5", "socks5h":
	default:
		return fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("host is required")
	}
	return nil
}

// proxyRawToString 把 config.json 里的 proxy-url（可能是字符串或数组）归一为字符串，
// 多条以换行分隔，供 parseProxyList 统一解析。
func proxyRawToString(v any) string {
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val)
	case []any:
		var parts []string
		for _, item := range val {
			s := strings.TrimSpace(fmt.Sprintf("%v", item))
			if s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "\n")
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

// parseProxyList 把原始字符串按换行/逗号拆分为多条代理地址，逐条校验并去重。
func parseProxyList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	var urls []string
	seen := make(map[string]bool)
	for _, f := range fields {
		u := strings.TrimSpace(f)
		if u == "" || seen[u] {
			continue
		}
		if err := validateProxyURL(u); err != nil {
			return nil, fmt.Errorf("%q: %w", u, err)
		}
		seen[u] = true
		urls = append(urls, u)
	}
	return urls, nil
}

var proxyRotation uint64

func GetProxySettings() (bool, string) {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return false, ""
	}
	proxyURL := strings.TrimSpace(Config.ProxyURL)
	return proxyURL != "", proxyURL
}

// GetProxyURLs 返回配置的全部代理地址（副本）。
func GetProxyURLs() []string {
	configMu.Lock()
	defer configMu.Unlock()
	if Config == nil || len(Config.ProxyURLs) == 0 {
		return nil
	}
	out := make([]string, len(Config.ProxyURLs))
	copy(out, Config.ProxyURLs)
	return out
}

// GetNextProxyURL 轮询返回下一条代理地址；未配置时返回空串。
func GetNextProxyURL() string {
	configMu.Lock()
	if Config == nil || len(Config.ProxyURLs) == 0 {
		configMu.Unlock()
		return ""
	}
	urls := Config.ProxyURLs
	if len(urls) == 1 {
		u := urls[0]
		configMu.Unlock()
		return u
	}
	picked := make([]string, len(urls))
	copy(picked, urls)
	configMu.Unlock()
	i := atomic.AddUint64(&proxyRotation, 1)
	return picked[(i-1)%uint64(len(picked))]
}

func GetChatCompletionsEnabled() bool {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return true
	}
	return Config.ChatCompletionsEnabled
}

func GetImagePollTimeoutSecs() int {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil || Config.ImagePollTimeoutSecs <= 0 {
		return 180
	}
	return Config.ImagePollTimeoutSecs
}

func GetLogMaxEntries() int {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil || Config.LogMaxEntries <= 0 {
		return 200
	}
	return Config.LogMaxEntries
}

func GetInsecureSkipVerify() bool {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return false
	}
	return Config.InsecureSkipVerify
}

func UpdateImagePollTimeoutSecs(seconds int) error {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return fmt.Errorf("config is not initialized")
	}
	if seconds <= 0 {
		return fmt.Errorf("image poll timeout secs must be greater than 0")
	}

	rawConfig, err := readRawConfig(Config.ConfigFile)
	if err != nil {
		return err
	}
	rawConfig["image-poll-timeout-secs"] = seconds

	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config.json: %w", err)
	}
	if err := os.WriteFile(Config.ConfigFile, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}

	Config.ImagePollTimeoutSecs = seconds
	return nil
}

// UpdateProxyURL 更新代理地址；入参可为单条或多条（换行/逗号分隔）。
func UpdateProxyURL(proxyURL string) error {
	urls, err := parseProxyList(proxyURL)
	if err != nil {
		return err
	}
	return UpdateProxyURLs(urls)
}

// UpdateProxyURLs 用一组代理地址覆盖配置并持久化。config.json 里：0 条删除、1 条存
// 字符串、多条存数组，保持向后兼容。
func UpdateProxyURLs(urls []string) error {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return fmt.Errorf("config is not initialized")
	}

	// 逐条校验 + 去重
	cleaned := make([]string, 0, len(urls))
	seen := make(map[string]bool)
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		if err := validateProxyURL(u); err != nil {
			return fmt.Errorf("%q: %w", u, err)
		}
		seen[u] = true
		cleaned = append(cleaned, u)
	}

	rawConfig, err := readRawConfig(Config.ConfigFile)
	if err != nil {
		return err
	}
	switch len(cleaned) {
	case 0:
		delete(rawConfig, "proxy-url")
	case 1:
		rawConfig["proxy-url"] = cleaned[0]
	default:
		arr := make([]any, len(cleaned))
		for i, u := range cleaned {
			arr[i] = u
		}
		rawConfig["proxy-url"] = arr
	}

	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config.json: %w", err)
	}
	if err := os.WriteFile(Config.ConfigFile, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}

	Config.ProxyURLs = cleaned
	if len(cleaned) > 0 {
		Config.ProxyURL = cleaned[0]
	} else {
		Config.ProxyURL = ""
	}
	return nil
}

func UpdateChatCompletionsEnabled(enabled bool) error {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return fmt.Errorf("config is not initialized")
	}

	rawConfig, err := readRawConfig(Config.ConfigFile)
	if err != nil {
		return err
	}
	rawConfig["enable-chat-completions"] = enabled

	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config.json: %w", err)
	}
	if err := os.WriteFile(Config.ConfigFile, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}

	Config.ChatCompletionsEnabled = enabled
	return nil
}

func readRawConfig(configFile string) (map[string]any, error) {
	rawConfig := make(map[string]any)
	if strings.TrimSpace(configFile) == "" {
		return rawConfig, nil
	}

	info, err := os.Stat(configFile)
	if err != nil {
		if os.IsNotExist(err) {
			return rawConfig, nil
		}
		return nil, fmt.Errorf("failed to stat config.json: %w", err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("config.json path is a directory")
	}

	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config.json: %w", err)
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return rawConfig, nil
	}
	if err := json.Unmarshal([]byte(text), &rawConfig); err != nil {
		return nil, fmt.Errorf("invalid config.json: %w", err)
	}
	return rawConfig, nil
}

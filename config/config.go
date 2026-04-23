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
)

type AppSettings struct {
	AuthKey                      string
	ProxyURL                     string
	ChatCompletionsEnabled       bool
	InsecureSkipVerify           bool
	Host                         string
	Port                         int
	ConfigFile                   string
	AccountsFile                 string
	RefreshAccountIntervalMinute int
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

	proxyURL := strings.TrimSpace(os.Getenv("CHATGPT2API_PROXY_URL"))
	if proxyURL == "" {
		if v, ok := rawConfig["proxy-url"]; ok {
			proxyURL = strings.TrimSpace(fmt.Sprintf("%v", v))
		}
	}
	if proxyURL != "" {
		if err := validateProxyURL(proxyURL); err != nil {
			return nil, fmt.Errorf(
				"invalid proxy-url: %w\n"+
					"Please set it via:\n"+
					"1. Environment variable: CHATGPT2API_PROXY_URL=http://host:port\n"+
					"2. Or in config.json: \"proxy-url\": \"http://host:port\"",
				err,
			)
		}
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

	return &AppSettings{
		AuthKey:                      authKey,
		ProxyURL:                     proxyURL,
		ChatCompletionsEnabled:       chatCompletionsEnabled,
		InsecureSkipVerify:           insecureSkipVerify,
		Host:                         "0.0.0.0",
		Port:                         8000,
		ConfigFile:                   configFile,
		AccountsFile:                 filepath.Join(dataDir, "accounts.json"),
		RefreshAccountIntervalMinute: refreshInterval,
		BaseDir:                      baseDir,
		DataDir:                      dataDir,
	}, nil
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

func GetProxySettings() (bool, string) {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return false, ""
	}
	proxyURL := strings.TrimSpace(Config.ProxyURL)
	return proxyURL != "", proxyURL
}

func GetChatCompletionsEnabled() bool {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return true
	}
	return Config.ChatCompletionsEnabled
}

func GetInsecureSkipVerify() bool {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return false
	}
	return Config.InsecureSkipVerify
}

func UpdateProxyURL(proxyURL string) error {
	configMu.Lock()
	defer configMu.Unlock()

	if Config == nil {
		return fmt.Errorf("config is not initialized")
	}

	proxyURL = strings.TrimSpace(proxyURL)
	if err := validateProxyURL(proxyURL); err != nil {
		return err
	}

	rawConfig, err := readRawConfig(Config.ConfigFile)
	if err != nil {
		return err
	}
	if proxyURL == "" {
		delete(rawConfig, "proxy-url")
	} else {
		rawConfig["proxy-url"] = proxyURL
	}

	data, err := json.MarshalIndent(rawConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode config.json: %w", err)
	}
	if err := os.WriteFile(Config.ConfigFile, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("failed to write config.json: %w", err)
	}

	Config.ProxyURL = proxyURL
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

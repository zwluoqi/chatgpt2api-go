package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadSettingsFromFile(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test123", "proxy-url": "http://127.0.0.1:7890", "refresh_account_interval_minute": 30}`), 0o644)

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthKey != "test123" {
		t.Errorf("AuthKey = %q, want test123", cfg.AuthKey)
	}
	if cfg.RefreshAccountIntervalMinute != 30 {
		t.Errorf("RefreshAccountIntervalMinute = %d, want 30", cfg.RefreshAccountIntervalMinute)
	}
	if cfg.ProxyURL != "http://127.0.0.1:7890" {
		t.Errorf("ProxyURL = %q, want http://127.0.0.1:7890", cfg.ProxyURL)
	}
	if !cfg.ChatCompletionsEnabled {
		t.Error("ChatCompletionsEnabled = false, want true")
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want 0.0.0.0", cfg.Host)
	}
	if cfg.Port != 8000 {
		t.Errorf("Port = %d, want 8000", cfg.Port)
	}
}

func TestLoadSettingsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "file_key", "proxy-url": "http://127.0.0.1:7890"}`), 0o644)

	t.Setenv("CHATGPT2API_AUTH_KEY", "env_key")
	t.Setenv("CHATGPT2API_PROXY_URL", "socks5://127.0.0.1:1080")

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthKey != "env_key" {
		t.Errorf("AuthKey = %q, want env_key (env should override file)", cfg.AuthKey)
	}
	if cfg.ProxyURL != "socks5://127.0.0.1:1080" {
		t.Errorf("ProxyURL = %q, want socks5://127.0.0.1:1080 (env should override file)", cfg.ProxyURL)
	}
	if !cfg.ChatCompletionsEnabled {
		t.Error("ChatCompletionsEnabled = false, want true")
	}
}

func TestLoadSettingsNoAuthKey(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{}`), 0o644)

	t.Setenv("CHATGPT2API_AUTH_KEY", "")
	t.Setenv("CHATGPT2API_PROXY_URL", "")

	_, err := loadSettings(dir)
	if err == nil {
		t.Error("expected error for missing auth-key")
	}
}

func TestLoadSettingsDefaultRefreshInterval(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test"}`), 0o644)

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RefreshAccountIntervalMinute != 60 {
		t.Errorf("RefreshAccountIntervalMinute = %d, want 60 (default)", cfg.RefreshAccountIntervalMinute)
	}
	if !cfg.ChatCompletionsEnabled {
		t.Error("ChatCompletionsEnabled = false, want true")
	}
}

func TestLoadSettingsNoConfigFile(t *testing.T) {
	dir := t.TempDir()

	t.Setenv("CHATGPT2API_AUTH_KEY", "env_only_key")
	t.Setenv("CHATGPT2API_PROXY_URL", "https://127.0.0.1:8443")

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AuthKey != "env_only_key" {
		t.Errorf("AuthKey = %q, want env_only_key", cfg.AuthKey)
	}
	if cfg.ProxyURL != "https://127.0.0.1:8443" {
		t.Errorf("ProxyURL = %q, want https://127.0.0.1:8443", cfg.ProxyURL)
	}
	if !cfg.ChatCompletionsEnabled {
		t.Error("ChatCompletionsEnabled = false, want true")
	}
}

func TestDataDirCreation(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test"}`), 0o644)

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}

	dataDir := filepath.Join(dir, "data")
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("data directory should be created")
	}
	if cfg.AccountsFile != filepath.Join(dataDir, "accounts.json") {
		t.Errorf("AccountsFile = %q, want %q", cfg.AccountsFile, filepath.Join(dataDir, "accounts.json"))
	}
}

func TestLoadSettingsInvalidProxyURL(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test", "proxy-url": "127.0.0.1:7890"}`), 0o644)

	_, err := loadSettings(dir)
	if err == nil {
		t.Fatal("expected error for invalid proxy-url")
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "invalid proxy-url") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadSettingsChatCompletionsOverride(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test", "enable-chat-completions": false}`), 0o644)

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChatCompletionsEnabled {
		t.Error("ChatCompletionsEnabled = true, want false")
	}
}

func TestLoadSettingsChatCompletionsEnvOverride(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test", "enable-chat-completions": false}`), 0o644)

	t.Setenv("CHATGPT2API_ENABLE_CHAT_COMPLETIONS", "true")

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ChatCompletionsEnabled {
		t.Error("ChatCompletionsEnabled = false, want true")
	}
}

func TestLoadSettingsInsecureSkipVerifyOverride(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test", "insecure-skip-verify": true}`), 0o644)

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestLoadSettingsInsecureSkipVerifyEnvOverride(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key": "test", "insecure-skip-verify": false}`), 0o644)

	t.Setenv("CHATGPT2API_INSECURE_SKIP_VERIFY", "true")

	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
}

func TestParseProxyList(t *testing.T) {
	// 单条
	urls, err := parseProxyList("http://127.0.0.1:7890")
	if err != nil || len(urls) != 1 || urls[0] != "http://127.0.0.1:7890" {
		t.Fatalf("单条解析错误: %v %v", urls, err)
	}
	// 多条（换行+逗号+去重）
	urls, err = parseProxyList("http://a:1\nsocks5://b:2, http://a:1\n;https://c:3")
	if err != nil {
		t.Fatalf("多条解析错误: %v", err)
	}
	if len(urls) != 3 {
		t.Fatalf("应为3条(去重后), got %v", urls)
	}
	// 非法条目报错
	if _, err := parseProxyList("http://ok:1\nnotaurl"); err == nil {
		t.Error("非法代理应报错")
	}
	// 空
	if urls, _ := parseProxyList("  \n , "); len(urls) != 0 {
		t.Errorf("空输入应为0条, got %v", urls)
	}
}

func TestLoadSettingsMultiProxyArray(t *testing.T) {
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.json")
	os.WriteFile(configFile, []byte(`{"auth-key":"k","proxy-url":["http://a:1","socks5://b:2"]}`), 0o644)
	cfg, err := loadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.ProxyURLs) != 2 || cfg.ProxyURL != "http://a:1" {
		t.Fatalf("数组多代理解析错误: %+v", cfg.ProxyURLs)
	}
}

func TestGetNextProxyURLRoundRobin(t *testing.T) {
	Config = &AppSettings{ProxyURLs: []string{"http://a:1", "http://b:2", "http://c:3"}}
	defer func() { Config = nil }()
	got := map[string]int{}
	for i := 0; i < 6; i++ {
		got[GetNextProxyURL()]++
	}
	if len(got) != 3 || got["http://a:1"] != 2 || got["http://b:2"] != 2 || got["http://c:3"] != 2 {
		t.Errorf("轮询分布不均: %v", got)
	}
}

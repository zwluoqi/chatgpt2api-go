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

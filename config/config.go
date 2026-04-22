package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type AppSettings struct {
	AuthKey                      string
	Host                         string
	Port                         int
	AccountsFile                 string
	RefreshAccountIntervalMinute int
	BaseDir                      string
	DataDir                      string
}

var Config *AppSettings

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

	refreshInterval := 60
	if v, ok := rawConfig["refresh_account_interval_minute"]; ok {
		switch val := v.(type) {
		case float64:
			refreshInterval = int(val)
		case int:
			refreshInterval = val
		}
	}

	return &AppSettings{
		AuthKey:                      authKey,
		Host:                         "0.0.0.0",
		Port:                         8000,
		AccountsFile:                 filepath.Join(dataDir, "accounts.json"),
		RefreshAccountIntervalMinute: refreshInterval,
		BaseDir:                      baseDir,
		DataDir:                      dataDir,
	}, nil
}

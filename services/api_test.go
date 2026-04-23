package services

import (
	"bytes"
	"chatgpt2api-go/config"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupTestApp(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	storeFile := filepath.Join(dir, "accounts.json")
	cpaFile := filepath.Join(dir, "cpa_config.json")
	configFile := filepath.Join(dir, "config.json")
	dataDir := filepath.Join(dir, "data")
	webDistDir := filepath.Join(dir, "web_dist")
	os.MkdirAll(dataDir, 0o755)
	os.MkdirAll(webDistDir, 0o755)
	os.WriteFile(filepath.Join(webDistDir, "index.html"), []byte("<html>test</html>"), 0o644)
	os.WriteFile(configFile, []byte("{\n  \"auth-key\": \"test-auth-key\"\n}\n"), 0o644)

	authKey := "test-auth-key"
	prevConfig := config.Config
	config.Config = &config.AppSettings{
		AuthKey:                      authKey,
		Host:                         "0.0.0.0",
		Port:                         8000,
		ChatCompletionsEnabled:       true,
		ConfigFile:                   configFile,
		AccountsFile:                 storeFile,
		RefreshAccountIntervalMinute: 60,
		BaseDir:                      dir,
		DataDir:                      dataDir,
	}
	t.Cleanup(func() {
		config.Config = prevConfig
	})

	accountService := NewAccountService(storeFile)
	cpaConfig := NewCPAConfig(cpaFile)
	cpaImportService := NewCPAImportService(cpaConfig, accountService)
	chatGPTService := NewChatGPTService(accountService)

	router := CreateApp(authKey, "0.1.0", webDistDir, accountService, cpaConfig, cpaImportService, chatGPTService)
	srv := httptest.NewServer(router)
	return srv, authKey
}

func authHeader(key string) string {
	return "Bearer " + key
}

func TestVersionEndpoint(t *testing.T) {
	srv, _ := setupTestApp(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["version"] != "0.1.0" {
		t.Errorf("version = %v, want 0.1.0", body["version"])
	}
}

func TestLoginSuccess(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/auth/login", nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["ok"] != true {
		t.Errorf("ok = %v, want true", body["ok"])
	}
}

func TestLoginUnauthorized(t *testing.T) {
	srv, _ := setupTestApp(t)
	defer srv.Close()

	req, _ := http.NewRequest("POST", srv.URL+"/auth/login", nil)
	req.Header.Set("Authorization", "Bearer wrong-key")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestListModels(t *testing.T) {
	srv, _ := setupTestApp(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["object"] != "list" {
		t.Errorf("object = %v, want list", body["object"])
	}
	data, ok := body["data"].([]any)
	if !ok || len(data) != 2 {
		t.Errorf("expected 2 models")
	}
}

func TestProxySettingsCRUD(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	req, _ := http.NewRequest("GET", srv.URL+"/api/proxy", nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var getBody map[string]any
	json.NewDecoder(resp.Body).Decode(&getBody)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	proxy, _ := getBody["proxy"].(map[string]any)
	if proxy["enabled"] != false {
		t.Fatalf("enabled = %v, want false", proxy["enabled"])
	}

	updateBody, _ := json.Marshal(map[string]any{
		"enabled": true,
		"url":     "http://127.0.0.1:7890",
	})
	req, _ = http.NewRequest("POST", srv.URL+"/api/proxy", bytes.NewReader(updateBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var updateResp map[string]any
	json.NewDecoder(resp.Body).Decode(&updateResp)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	proxy, _ = updateResp["proxy"].(map[string]any)
	if proxy["url"] != "http://127.0.0.1:7890" {
		t.Fatalf("url = %v, want http://127.0.0.1:7890", proxy["url"])
	}

	req, _ = http.NewRequest("GET", srv.URL+"/api/proxy", nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	json.NewDecoder(resp.Body).Decode(&getBody)
	resp.Body.Close()
	proxy, _ = getBody["proxy"].(map[string]any)
	if proxy["enabled"] != true {
		t.Fatalf("enabled = %v, want true", proxy["enabled"])
	}
	if proxy["url"] != "http://127.0.0.1:7890" {
		t.Fatalf("url = %v, want http://127.0.0.1:7890", proxy["url"])
	}
}

func TestProxySettingsInvalidURL(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"enabled": true, "url": "127.0.0.1:7890"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/proxy", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestProxyTestRequiresURL(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"url": ""})
	req, _ := http.NewRequest("POST", srv.URL+"/api/proxy/test", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}
}

func TestChatCompletionsSettingsCRUD(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	req, _ := http.NewRequest("GET", srv.URL+"/api/chat-completions", nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var getBody map[string]any
	json.NewDecoder(resp.Body).Decode(&getBody)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if getBody["enabled"] != true {
		t.Fatalf("enabled = %v, want true", getBody["enabled"])
	}

	updateBody, _ := json.Marshal(map[string]any{"enabled": false})
	req, _ = http.NewRequest("POST", srv.URL+"/api/chat-completions", bytes.NewReader(updateBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var updateResp map[string]any
	json.NewDecoder(resp.Body).Decode(&updateResp)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if updateResp["enabled"] != false {
		t.Fatalf("enabled = %v, want false", updateResp["enabled"])
	}
}

func TestChatCompletionsDisabled(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()

	updateBody, _ := json.Marshal(map[string]any{"enabled": false})
	req, _ := http.NewRequest("POST", srv.URL+"/api/chat-completions", bytes.NewReader(updateBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	body, _ := json.Marshal(map[string]any{"model": "gpt-image-1", "messages": []any{}})
	req, _ = http.NewRequest("POST", srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestAccountCRUD(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	// Create accounts
	createBody, _ := json.Marshal(map[string]any{"tokens": []string{"tok_1", "tok_2"}})
	req, _ := http.NewRequest("POST", srv.URL+"/api/accounts", bytes.NewReader(createBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	resp.Body.Close()

	// List accounts
	req, _ = http.NewRequest("GET", srv.URL+"/api/accounts", nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, _ = client.Do(req)
	var listBody map[string]any
	json.NewDecoder(resp.Body).Decode(&listBody)
	resp.Body.Close()
	items, _ := listBody["items"].([]any)
	if len(items) != 2 {
		t.Errorf("len(items) = %d, want 2", len(items))
	}

	// Delete one account
	deleteBody, _ := json.Marshal(map[string]any{"tokens": []string{"tok_1"}})
	req, _ = http.NewRequest("DELETE", srv.URL+"/api/accounts", bytes.NewReader(deleteBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var delResp map[string]any
	json.NewDecoder(resp.Body).Decode(&delResp)
	resp.Body.Close()
	if toInt(delResp["removed"]) != 1 {
		t.Errorf("removed = %v, want 1", delResp["removed"])
	}
}

func TestAccountUpdate(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	// Create account
	createBody, _ := json.Marshal(map[string]any{"tokens": []string{"tok_upd"}})
	req, _ := http.NewRequest("POST", srv.URL+"/api/accounts", bytes.NewReader(createBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	resp.Body.Close()

	// Update account
	updateBody, _ := json.Marshal(map[string]any{
		"access_token": "tok_upd",
		"type":         "Plus",
		"quota":        50,
	})
	req, _ = http.NewRequest("POST", srv.URL+"/api/accounts/update", bytes.NewReader(updateBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var updateResp map[string]any
	json.NewDecoder(resp.Body).Decode(&updateResp)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("update status = %d, want 200", resp.StatusCode)
	}
}

func TestAccountUpdateNotFound(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	updateBody, _ := json.Marshal(map[string]any{
		"access_token": "nonexistent",
		"type":         "Plus",
	})
	req, _ := http.NewRequest("POST", srv.URL+"/api/accounts/update", bytes.NewReader(updateBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestImageGenerationNoAuth(t *testing.T) {
	srv, _ := setupTestApp(t)
	defer srv.Close()

	body, _ := json.Marshal(map[string]any{"prompt": "draw a cat"})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 401 {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

func TestStaticFileServing(t *testing.T) {
	srv, _ := setupTestApp(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSPA404ForNext(t *testing.T) {
	srv, _ := setupTestApp(t)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/_next/nonexistent.js")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestCPAPoolCRUD(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	// List empty pools
	req, _ := http.NewRequest("GET", srv.URL+"/api/cpa/pools", nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, _ := client.Do(req)
	var listResp map[string]any
	json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()
	pools, _ := listResp["pools"].([]any)
	if len(pools) != 0 {
		t.Errorf("expected 0 pools, got %d", len(pools))
	}

	// Create pool
	createBody, _ := json.Marshal(map[string]any{
		"name":       "Test Pool",
		"base_url":   "https://example.com",
		"secret_key": "secret123",
	})
	req, _ = http.NewRequest("POST", srv.URL+"/api/cpa/pools", bytes.NewReader(createBody))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ = client.Do(req)
	var createResp map[string]any
	json.NewDecoder(resp.Body).Decode(&createResp)
	resp.Body.Close()

	pool, _ := createResp["pool"].(map[string]any)
	if pool["name"] != "Test Pool" {
		t.Errorf("name = %v, want Test Pool", pool["name"])
	}
	if pool["secret_key"] != nil {
		t.Error("secret_key should be removed from response")
	}

	poolID, _ := pool["id"].(string)
	if poolID == "" {
		t.Fatal("pool ID is empty")
	}

	// Delete pool
	req, _ = http.NewRequest("DELETE", srv.URL+"/api/cpa/pools/"+poolID, nil)
	req.Header.Set("Authorization", authHeader(authKey))
	resp, _ = client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("delete status = %d, want 200", resp.StatusCode)
	}
}

func TestCPAPoolCreateValidation(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()
	client := &http.Client{}

	body, _ := json.Marshal(map[string]any{"name": "pool", "base_url": "", "secret_key": "abc"})
	req, _ := http.NewRequest("POST", srv.URL+"/api/cpa/pools", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, _ := client.Do(req)
	resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Errorf("status = %d, want 400 for missing base_url", resp.StatusCode)
	}
}

func TestExtractBearerToken(t *testing.T) {
	tests := []struct {
		auth     string
		expected string
	}{
		{"Bearer abc123", "abc123"},
		{"bearer  xyz ", "xyz"},
		{"Basic abc123", ""},
		{"Bearer", ""},
		{"", ""},
	}
	for _, tt := range tests {
		result := extractBearerToken(tt.auth)
		if result != tt.expected {
			t.Errorf("extractBearerToken(%q) = %q, want %q", tt.auth, result, tt.expected)
		}
	}
}

func TestResolveWebAsset(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.html"), []byte("hello"), 0o644)
	os.MkdirAll(filepath.Join(dir, "about"), 0o755)
	os.WriteFile(filepath.Join(dir, "about", "index.html"), []byte("about"), 0o644)

	tests := []struct {
		path   string
		expect bool
	}{
		{"", true},
		{"/", true},
		{"about", true},
		{"nonexistent", false},
	}
	for _, tt := range tests {
		result := resolveWebAsset(dir, tt.path)
		if tt.expect && result == "" {
			t.Errorf("resolveWebAsset(%q) returned empty, want found", tt.path)
		}
		if !tt.expect && result != "" {
			t.Errorf("resolveWebAsset(%q) = %q, want empty", tt.path, result)
		}
	}
}

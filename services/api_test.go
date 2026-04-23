package services

import (
	"bytes"
	"chatgpt2api-go/config"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func setupTestAppWithAccountService(t *testing.T) (*httptest.Server, string, *AccountService) {
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
	return srv, authKey, accountService
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

func TestChatCompletionsStreamResponse(t *testing.T) {
	srv, authKey, accountService := setupTestAppWithAccountService(t)
	defer srv.Close()

	accountService.AddAccounts([]string{"token_a"})
	accountService.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := generateImageResultFunc
	generateImageResultFunc = func(_ *AccountService, accessToken, prompt, model string) (map[string]any, error) {
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		generateImageResultFunc = previous
	})

	body, _ := json.Marshal(map[string]any{
		"model":  "gpt-image-1",
		"stream": true,
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "draw a cat"},
				},
			},
		},
	})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("content-type = %q, want text/event-stream", resp.Header.Get("Content-Type"))
	}

	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	if !strings.Contains(text, "\"object\":\"chat.completion.chunk\"") {
		t.Fatalf("stream body missing chunk object: %s", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream body missing [DONE]: %s", text)
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

func TestImageGenerationQuotaExceededMarksAccountLimited(t *testing.T) {
	srv, authKey, accountService := setupTestAppWithAccountService(t)
	defer srv.Close()

	accountService.AddAccounts([]string{"token_a"})
	accountService.UpdateAccount("token_a", map[string]any{"quota": 1, "status": "正常"})

	previous := generateImageResultFunc
	generateImageResultFunc = func(_ *AccountService, accessToken, prompt, model string) (map[string]any, error) {
		return nil, &ImageGenerationError{
			Message: "You've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25 minutes.",
		}
	}
	t.Cleanup(func() {
		generateImageResultFunc = previous
	})

	body, _ := json.Marshal(map[string]any{
		"prompt": "draw a cat",
		"model":  "gpt-image-1",
	})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 502 {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}

	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}

	message := response["error"]
	if message == nil || !strings.Contains(message.(string), "free plan limit for image generation requests") {
		t.Fatalf("error = %v, want original quota exceeded message", response["error"])
	}

	account := accountService.GetAccount("token_a")
	if account == nil {
		t.Fatal("token_a account not found")
	}
	if account["status"] != "限流" {
		t.Fatalf("status = %v, want 限流", account["status"])
	}
	if toInt(account["quota"]) != 0 {
		t.Fatalf("quota = %v, want 0", account["quota"])
	}
	if account["restore_at"] == nil {
		t.Fatal("restore_at should be set")
	}
}

func TestImageGenerationAppendsSizeToPrompt(t *testing.T) {
	srv, authKey, accountService := setupTestAppWithAccountService(t)
	defer srv.Close()

	accountService.AddAccounts([]string{"token_a"})
	accountService.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := generateImageResultFunc
	generateImageResultFunc = func(_ *AccountService, accessToken, prompt, model string) (map[string]any, error) {
		expected := "draw a cat\n\nRequested output image size: 1024x1024."
		if prompt != expected {
			t.Fatalf("prompt = %q, want %q", prompt, expected)
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		generateImageResultFunc = previous
	})

	body, _ := json.Marshal(map[string]any{
		"prompt": "draw a cat",
		"model":  "gpt-image-1",
		"size":   "1024x1024",
	})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/images/generations", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestImageEditAppendsSizeToPrompt(t *testing.T) {
	srv, authKey, accountService := setupTestAppWithAccountService(t)
	defer srv.Close()

	accountService.AddAccounts([]string{"token_a"})
	accountService.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := editImageResultFunc
	editImageResultFunc = func(_ *AccountService, accessToken, prompt string, images []RequestImage, model string) (map[string]any, error) {
		expected := "edit this\n\nRequested output image size: 1536x1024."
		if prompt != expected {
			t.Fatalf("prompt = %q, want %q", prompt, expected)
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		editImageResultFunc = previous
	})

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("prompt", "edit this"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("size", "1536x1024"); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("image", "image.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte("png")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("POST", srv.URL+"/v1/images/edits", &body)
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestImageEditsRejectsMoreThan16InputImages(t *testing.T) {
	srv, authKey := setupTestApp(t)
	defer srv.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("prompt", "edit this"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxEditInputImages+1; i++ {
		part, err := writer.CreateFormFile("image", "image.png")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("png")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req, _ := http.NewRequest("POST", srv.URL+"/v1/images/edits", &body)
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 400 {
		t.Fatalf("status = %d, want 400", resp.StatusCode)
	}

	var response map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		t.Fatal(err)
	}
	if response["error"] != "image count must be between 1 and 16" {
		t.Fatalf("error = %v, want image count must be between 1 and 16", response["error"])
	}
}

func TestChatCompletionsPassesMultipleImagesToEdit(t *testing.T) {
	srv, authKey, accountService := setupTestAppWithAccountService(t)
	defer srv.Close()

	accountService.AddAccounts([]string{"token_a"})
	accountService.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := editImageResultFunc
	editImageResultFunc = func(_ *AccountService, accessToken, prompt string, images []RequestImage, model string) (map[string]any, error) {
		if len(images) != 2 {
			t.Fatalf("len(images) = %d, want 2", len(images))
		}
		if string(images[0].Data) != "test1" {
			t.Fatalf("images[0].Data = %q, want test1", string(images[0].Data))
		}
		if string(images[1].Data) != "test2" {
			t.Fatalf("images[1].Data = %q, want test2", string(images[1].Data))
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		editImageResultFunc = previous
	})

	body, _ := json.Marshal(map[string]any{
		"model": "gpt-image-1",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "edit this"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": "data:image/png;base64,dGVzdDE=",
						},
					},
					map[string]any{
						"type":      "input_image",
						"image_url": "data:image/png;base64,dGVzdDI=",
					},
				},
			},
		},
	})
	req, _ := http.NewRequest("POST", srv.URL+"/v1/chat/completions", bytes.NewReader(body))
	req.Header.Set("Authorization", authHeader(authKey))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
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

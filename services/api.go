package services

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"chatgpt2api-go/config"

	"github.com/gin-gonic/gin"
)

func buildModelItem(modelID string) map[string]any {
	return map[string]any{
		"id":       modelID,
		"object":   "model",
		"created":  0,
		"owned_by": "chatgpt2api-go",
	}
}

func sanitizeCPAPool(pool map[string]any) map[string]any {
	if pool == nil {
		return nil
	}
	result := make(map[string]any)
	for k, v := range pool {
		if k != "secret_key" {
			result[k] = v
		}
	}
	return result
}

func sanitizeCPAPools(pools []map[string]any) []map[string]any {
	var result []map[string]any
	for _, pool := range pools {
		sanitized := sanitizeCPAPool(pool)
		if sanitized != nil {
			result = append(result, sanitized)
		}
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func extractBearerToken(authorization string) string {
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") || strings.TrimSpace(parts[1]) == "" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func requireAuthKey(c *gin.Context, authKey string) bool {
	auth := c.GetHeader("Authorization")
	if extractBearerToken(auth) != strings.TrimSpace(authKey) {
		c.JSON(401, gin.H{"error": "authorization is invalid"})
		return false
	}
	return true
}

func resolveWebAsset(webDistDir, requestedPath string) string {
	if _, err := os.Stat(webDistDir); os.IsNotExist(err) {
		return ""
	}

	cleanPath := strings.Trim(requestedPath, "/")
	var candidates []string
	if cleanPath == "" {
		candidates = []string{filepath.Join(webDistDir, "index.html")}
	} else {
		candidates = []string{
			filepath.Join(webDistDir, cleanPath),
			filepath.Join(webDistDir, cleanPath, "index.html"),
			filepath.Join(webDistDir, cleanPath+".html"),
		}
	}

	for _, candidate := range candidates {
		absCandidate, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		absWebDist, err := filepath.Abs(webDistDir)
		if err != nil {
			continue
		}
		if !strings.HasPrefix(absCandidate, absWebDist) {
			continue
		}
		info, err := os.Stat(absCandidate)
		if err == nil && !info.IsDir() {
			return absCandidate
		}
	}
	return ""
}

func StartLimitedAccountWatcher(ctx context.Context, accountService *AccountService, intervalMinutes int) {
	intervalSeconds := intervalMinutes * 60
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Duration(intervalSeconds) * time.Second):
				limitedTokens := accountService.ListLimitedTokens()
				if len(limitedTokens) > 0 {
					fmt.Printf("[account-limited-watcher] checking %d limited accounts\n", len(limitedTokens))
					accountService.RefreshAccounts(limitedTokens)
				}
			}
		}
	}()
}

func CreateApp(
	authKey string,
	appVersion string,
	webDistDir string,
	accountService *AccountService,
	cpaConfig *CPAConfig,
	cpaImportService *CPAImportService,
	chatGPTService *ChatGPTService,
) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(gin.Logger())

	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "*")
		c.Header("Access-Control-Allow-Headers", "*")
		c.Header("Access-Control-Max-Age", "86400")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	r.GET("/v1/models", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"object": "list",
			"data": []any{
				buildModelItem("gpt-image-1"),
				buildModelItem("gpt-image-2"),
			},
		})
	})

	r.POST("/auth/login", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		c.JSON(200, gin.H{"ok": true, "version": appVersion})
	})

	r.GET("/version", func(c *gin.Context) {
		c.JSON(200, gin.H{"version": appVersion})
	})

	r.GET("/api/proxy", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		c.JSON(200, gin.H{"proxy": GetProxySettings()})
	})

	r.POST("/api/proxy", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			Enabled *bool  `json:"enabled"`
			URL     string `json:"url"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		proxy, err := UpdateProxySettings(body.Enabled, body.URL)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"proxy": proxy})
	})

	r.POST("/api/proxy/test", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			URL string `json:"url"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		candidate := strings.TrimSpace(body.URL)
		if candidate == "" {
			candidate = GetProxySettings().URL
		}
		if candidate == "" {
			c.JSON(400, gin.H{"error": "proxy url is required"})
			return
		}
		c.JSON(200, gin.H{"result": TestProxy(candidate, 15*time.Second)})
	})

	r.GET("/api/chat-completions", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		c.JSON(200, gin.H{"enabled": config.GetChatCompletionsEnabled()})
	})

	r.POST("/api/chat-completions", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			Enabled bool `json:"enabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		if err := config.UpdateChatCompletionsEnabled(body.Enabled); err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"enabled": config.GetChatCompletionsEnabled()})
	})

	r.GET("/api/accounts", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		c.JSON(200, gin.H{"items": accountService.ListAccounts()})
	})

	r.POST("/api/accounts", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			Tokens []string `json:"tokens"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		var tokens []string
		for _, token := range body.Tokens {
			t := strings.TrimSpace(token)
			if t != "" {
				tokens = append(tokens, t)
			}
		}
		if len(tokens) == 0 {
			c.JSON(400, gin.H{"error": "tokens is required"})
			return
		}
		result := accountService.AddAccounts(tokens)
		refreshResult := accountService.RefreshAccounts(tokens)

		response := make(map[string]any)
		for k, v := range result {
			response[k] = v
		}
		response["refreshed"] = refreshResult["refreshed"]
		response["errors"] = refreshResult["errors"]
		if items, ok := refreshResult["items"]; ok {
			response["items"] = items
		}
		c.JSON(200, response)
	})

	r.DELETE("/api/accounts", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			Tokens []string `json:"tokens"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		var tokens []string
		for _, token := range body.Tokens {
			t := strings.TrimSpace(token)
			if t != "" {
				tokens = append(tokens, t)
			}
		}
		if len(tokens) == 0 {
			c.JSON(400, gin.H{"error": "tokens is required"})
			return
		}
		c.JSON(200, accountService.DeleteAccounts(tokens))
	})

	r.POST("/api/accounts/refresh", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			AccessTokens []string `json:"access_tokens"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		var accessTokens []string
		for _, token := range body.AccessTokens {
			t := strings.TrimSpace(token)
			if t != "" {
				accessTokens = append(accessTokens, t)
			}
		}
		if len(accessTokens) == 0 {
			accessTokens = accountService.ListTokens()
		}
		if len(accessTokens) == 0 {
			c.JSON(400, gin.H{"error": "access_tokens is required"})
			return
		}
		c.JSON(200, accountService.RefreshAccounts(accessTokens))
	})

	r.POST("/api/accounts/update", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			AccessToken string  `json:"access_token"`
			Type        *string `json:"type"`
			Status      *string `json:"status"`
			Quota       *int    `json:"quota"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		accessToken := strings.TrimSpace(body.AccessToken)
		if accessToken == "" {
			c.JSON(400, gin.H{"error": "access_token is required"})
			return
		}
		updates := make(map[string]any)
		if body.Type != nil {
			updates["type"] = *body.Type
		}
		if body.Status != nil {
			updates["status"] = *body.Status
		}
		if body.Quota != nil {
			updates["quota"] = *body.Quota
		}
		if len(updates) == 0 {
			c.JSON(400, gin.H{"error": "no updates provided"})
			return
		}
		account := accountService.UpdateAccount(accessToken, updates)
		if account == nil {
			c.JSON(404, gin.H{"error": "account not found"})
			return
		}
		c.JSON(200, gin.H{"item": account, "items": accountService.ListAccounts()})
	})

	r.POST("/v1/images/generations", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			Prompt          string `json:"prompt" binding:"required,min=1"`
			Model           string `json:"model"`
			N               int    `json:"n"`
			ResponseFormat  string `json:"response_format"`
			HistoryDisabled bool   `json:"history_disabled"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "prompt is required"})
			return
		}
		if body.Model == "" {
			body.Model = "gpt-4o"
		}
		if body.N == 0 {
			body.N = 1
		}
		if body.N < 1 || body.N > 4 {
			c.JSON(400, gin.H{"error": "n must be between 1 and 4"})
			return
		}
		result, err := chatGPTService.GenerateWithPool(body.Prompt, body.Model, body.N)
		if err != nil {
			c.JSON(502, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, result)
	})

	r.POST("/v1/images/edits", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		prompt := c.PostForm("prompt")
		model := c.PostForm("model")
		if model == "" {
			model = "gpt-image-1"
		}
		nStr := c.PostForm("n")
		n := 1
		if nStr != "" {
			fmt.Sscanf(nStr, "%d", &n)
		}
		if n < 1 || n > 4 {
			c.JSON(400, gin.H{"error": "n must be between 1 and 4"})
			return
		}

		form, err := c.MultipartForm()
		if err != nil {
			c.JSON(400, gin.H{"error": "invalid multipart form"})
			return
		}
		files := form.File["image"]
		if len(files) == 0 {
			c.JSON(400, gin.H{"error": "image is required"})
			return
		}

		var images []struct {
			Data     []byte
			FileName string
			MimeType string
		}
		for _, file := range files {
			f, err := file.Open()
			if err != nil {
				c.JSON(400, gin.H{"error": "failed to read image"})
				return
			}
			data, err := io.ReadAll(f)
			f.Close()
			if err != nil || len(data) == 0 {
				c.JSON(400, gin.H{"error": "image file is empty"})
				return
			}
			fileName := file.Filename
			if fileName == "" {
				fileName = "image.png"
			}
			mimeType := file.Header.Get("Content-Type")
			if mimeType == "" {
				mimeType = "image/png"
			}
			images = append(images, struct {
				Data     []byte
				FileName string
				MimeType string
			}{Data: data, FileName: fileName, MimeType: mimeType})
		}

		result, genErr := chatGPTService.EditWithPool(prompt, images, model, n)
		if genErr != nil {
			c.JSON(502, gin.H{"error": genErr.Error()})
			return
		}
		c.JSON(200, result)
	})

	r.POST("/v1/chat/completions", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		if !config.GetChatCompletionsEnabled() {
			c.JSON(403, gin.H{"error": "/v1/chat/completions is disabled"})
			return
		}
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		result, httpErr := chatGPTService.CreateImageCompletion(body)
		if httpErr != nil {
			c.JSON(httpErr.StatusCode, httpErr.Detail)
			return
		}
		c.JSON(200, result)
	})

	r.POST("/v1/responses", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		result, httpErr := chatGPTService.CreateResponse(body)
		if httpErr != nil {
			c.JSON(httpErr.StatusCode, httpErr.Detail)
			return
		}
		c.JSON(200, result)
	})

	// CPA endpoints
	r.GET("/api/cpa/pools", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		c.JSON(200, gin.H{"pools": sanitizeCPAPools(cpaConfig.ListPools())})
	})

	r.POST("/api/cpa/pools", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		var body struct {
			Name      string `json:"name"`
			BaseURL   string `json:"base_url"`
			SecretKey string `json:"secret_key"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		if strings.TrimSpace(body.BaseURL) == "" {
			c.JSON(400, gin.H{"error": "base_url is required"})
			return
		}
		if strings.TrimSpace(body.SecretKey) == "" {
			c.JSON(400, gin.H{"error": "secret_key is required"})
			return
		}
		pool := cpaConfig.AddPool(body.Name, body.BaseURL, body.SecretKey)
		c.JSON(200, gin.H{
			"pool":  sanitizeCPAPool(pool),
			"pools": sanitizeCPAPools(cpaConfig.ListPools()),
		})
	})

	r.POST("/api/cpa/pools/:pool_id", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		poolID := c.Param("pool_id")
		var body map[string]any
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		updates := make(map[string]any)
		if v, ok := body["name"]; ok {
			updates["name"] = v
		}
		if v, ok := body["base_url"]; ok {
			updates["base_url"] = v
		}
		if v, ok := body["secret_key"]; ok {
			updates["secret_key"] = v
		}
		pool := cpaConfig.UpdatePool(poolID, updates)
		if pool == nil {
			c.JSON(404, gin.H{"error": "pool not found"})
			return
		}
		c.JSON(200, gin.H{
			"pool":  sanitizeCPAPool(pool),
			"pools": sanitizeCPAPools(cpaConfig.ListPools()),
		})
	})

	r.DELETE("/api/cpa/pools/:pool_id", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		poolID := c.Param("pool_id")
		if !cpaConfig.DeletePool(poolID) {
			c.JSON(404, gin.H{"error": "pool not found"})
			return
		}
		c.JSON(200, gin.H{"pools": sanitizeCPAPools(cpaConfig.ListPools())})
	})

	r.GET("/api/cpa/pools/:pool_id/files", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		poolID := c.Param("pool_id")
		pool := cpaConfig.GetPool(poolID)
		if pool == nil {
			c.JSON(404, gin.H{"error": "pool not found"})
			return
		}
		files, err := ListRemoteFiles(pool)
		if err != nil {
			c.JSON(502, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"pool_id": poolID, "files": files})
	})

	r.POST("/api/cpa/pools/:pool_id/import", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		poolID := c.Param("pool_id")
		pool := cpaConfig.GetPool(poolID)
		if pool == nil {
			c.JSON(404, gin.H{"error": "pool not found"})
			return
		}
		var body struct {
			Names []string `json:"names"`
		}
		if err := c.ShouldBindJSON(&body); err != nil {
			c.JSON(400, gin.H{"error": "invalid request body"})
			return
		}
		job, err := cpaImportService.StartImport(pool, body.Names)
		if err != nil {
			c.JSON(400, gin.H{"error": err.Error()})
			return
		}
		c.JSON(200, gin.H{"import_job": job})
	})

	r.GET("/api/cpa/pools/:pool_id/import", func(c *gin.Context) {
		if !requireAuthKey(c, authKey) {
			return
		}
		poolID := c.Param("pool_id")
		pool := cpaConfig.GetPool(poolID)
		if pool == nil {
			c.JSON(404, gin.H{"error": "pool not found"})
			return
		}
		c.JSON(200, gin.H{"import_job": pool["import_job"]})
	})

	// Static file serving (SPA fallback)
	r.NoRoute(func(c *gin.Context) {
		fullPath := c.Request.URL.Path

		asset := resolveWebAsset(webDistDir, fullPath)
		if asset != "" {
			c.File(asset)
			return
		}

		if strings.HasPrefix(strings.Trim(fullPath, "/"), "_next/") {
			c.JSON(404, gin.H{"detail": "Not Found"})
			return
		}

		fallback := resolveWebAsset(webDistDir, "")
		if fallback == "" {
			c.JSON(404, gin.H{"detail": "Not Found"})
			return
		}
		c.File(fallback)
	})

	return r
}

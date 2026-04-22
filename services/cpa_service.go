package services

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type CPAConfig struct {
	storeFile string
	mu        sync.Mutex
	pools     []map[string]any
}

func NewCPAConfig(storeFile string) *CPAConfig {
	c := &CPAConfig{storeFile: storeFile}
	c.pools = c.load()
	return c
}

func newPoolID() string {
	return uuid.New().String()[:12]
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func normalizeImportJob(raw any, failUnfinished bool) map[string]any {
	m, ok := raw.(map[string]any)
	if !ok || m == nil {
		return nil
	}
	status := strings.TrimSpace(fmt.Sprintf("%v", m["status"]))
	if status == "" || status == "<nil>" {
		status = "failed"
	}
	if failUnfinished && (status == "pending" || status == "running") {
		status = "failed"
	}

	jobID := strings.TrimSpace(fmt.Sprintf("%v", m["job_id"]))
	if jobID == "" || jobID == "<nil>" {
		jobID = uuid.New().String()
	}
	createdAt := strings.TrimSpace(fmt.Sprintf("%v", m["created_at"]))
	if createdAt == "" || createdAt == "<nil>" {
		createdAt = nowISO()
	}
	updatedAt := strings.TrimSpace(fmt.Sprintf("%v", m["updated_at"]))
	if updatedAt == "" || updatedAt == "<nil>" {
		updatedAt = createdAt
	}

	var errors []any
	if e, ok := m["errors"].([]any); ok {
		errors = e
	} else {
		errors = []any{}
	}

	return map[string]any{
		"job_id":     jobID,
		"status":     status,
		"created_at": createdAt,
		"updated_at": updatedAt,
		"total":      toInt(m["total"]),
		"completed":  toInt(m["completed"]),
		"added":      toInt(m["added"]),
		"skipped":    toInt(m["skipped"]),
		"refreshed":  toInt(m["refreshed"]),
		"failed":     toInt(m["failed"]),
		"errors":     errors,
	}
}

func strVal(v any) string {
	if v == nil {
		return ""
	}
	s := strings.TrimSpace(fmt.Sprintf("%v", v))
	if s == "<nil>" {
		return ""
	}
	return s
}

func normalizePool(raw map[string]any) map[string]any {
	id := strVal(raw["id"])
	if id == "" {
		id = newPoolID()
	}
	return map[string]any{
		"id":         id,
		"name":       strVal(raw["name"]),
		"base_url":   strVal(raw["base_url"]),
		"secret_key": strVal(raw["secret_key"]),
		"import_job": normalizeImportJob(raw["import_job"], true),
	}
}

func (c *CPAConfig) load() []map[string]any {
	data, err := os.ReadFile(c.storeFile)
	if err != nil {
		return nil
	}

	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}

	switch v := raw.(type) {
	case map[string]any:
		if _, ok := v["base_url"]; ok {
			pool := normalizePool(v)
			if fmt.Sprintf("%v", pool["base_url"]) != "" {
				return []map[string]any{pool}
			}
		}
		return nil
	case []any:
		var pools []map[string]any
		for _, item := range v {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			pools = append(pools, normalizePool(m))
		}
		return pools
	}
	return nil
}

func (c *CPAConfig) save() {
	dir := filepath.Dir(c.storeFile)
	os.MkdirAll(dir, 0o755)
	data, _ := json.MarshalIndent(c.pools, "", "  ")
	os.WriteFile(c.storeFile, append(data, '\n'), 0o644)
}

func (c *CPAConfig) ListPools() []map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	result := make([]map[string]any, len(c.pools))
	for i, pool := range c.pools {
		cp := make(map[string]any)
		for k, v := range pool {
			cp[k] = v
		}
		result[i] = cp
	}
	return result
}

func (c *CPAConfig) GetPool(poolID string) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, pool := range c.pools {
		if fmt.Sprintf("%v", pool["id"]) == poolID {
			cp := make(map[string]any)
			for k, v := range pool {
				cp[k] = v
			}
			return cp
		}
	}
	return nil
}

func (c *CPAConfig) AddPool(name, baseURL, secretKey string) map[string]any {
	pool := normalizePool(map[string]any{
		"id":         newPoolID(),
		"name":       name,
		"base_url":   baseURL,
		"secret_key": secretKey,
	})
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pools = append(c.pools, pool)
	c.save()
	cp := make(map[string]any)
	for k, v := range pool {
		cp[k] = v
	}
	return cp
}

func (c *CPAConfig) UpdatePool(poolID string, updates map[string]any) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, pool := range c.pools {
		if fmt.Sprintf("%v", pool["id"]) != poolID {
			continue
		}
		merged := make(map[string]any)
		for k, v := range pool {
			merged[k] = v
		}
		for k, v := range updates {
			if v != nil {
				merged[k] = v
			}
		}
		merged["id"] = poolID
		c.pools[i] = normalizePool(merged)
		c.save()
		cp := make(map[string]any)
		for k, v := range c.pools[i] {
			cp[k] = v
		}
		return cp
	}
	return nil
}

func (c *CPAConfig) DeletePool(poolID string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	before := len(c.pools)
	var newPools []map[string]any
	for _, pool := range c.pools {
		if fmt.Sprintf("%v", pool["id"]) != poolID {
			newPools = append(newPools, pool)
		}
	}
	c.pools = newPools
	if len(c.pools) < before {
		c.save()
		return true
	}
	return false
}

func (c *CPAConfig) SetImportJob(poolID string, importJob map[string]any) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i, pool := range c.pools {
		if fmt.Sprintf("%v", pool["id"]) != poolID {
			continue
		}
		nextPool := make(map[string]any)
		for k, v := range pool {
			nextPool[k] = v
		}
		nextPool["import_job"] = normalizeImportJob(importJob, false)
		c.pools[i] = nextPool
		c.save()
		cp := make(map[string]any)
		for k, v := range nextPool {
			cp[k] = v
		}
		return cp
	}
	return nil
}

func (c *CPAConfig) GetImportJob(poolID string) map[string]any {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, pool := range c.pools {
		if fmt.Sprintf("%v", pool["id"]) == poolID {
			job, ok := pool["import_job"].(map[string]any)
			if ok && job != nil {
				cp := make(map[string]any)
				for k, v := range job {
					cp[k] = v
				}
				return cp
			}
			return nil
		}
	}
	return nil
}

func managementHeaders(secretKey string) map[string]string {
	return map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", secretKey),
		"Accept":        "application/json",
	}
}

func ListRemoteFiles(pool map[string]any) ([]map[string]any, error) {
	baseURL := strings.TrimSpace(fmt.Sprintf("%v", pool["base_url"]))
	secretKey := strings.TrimSpace(fmt.Sprintf("%v", pool["secret_key"]))
	if baseURL == "" || secretKey == "" {
		return []map[string]any{}, nil
	}

	url := strings.TrimRight(baseURL, "/") + "/v0/management/auth-files"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	for k, v := range managementHeaders(secretKey) {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("remote list failed: HTTP %d", resp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("remote list payload is invalid")
	}

	files, ok := payload["files"].([]any)
	if !ok {
		return nil, fmt.Errorf("remote list payload is invalid")
	}

	var items []map[string]any
	for _, item := range files {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name := strings.TrimSpace(fmt.Sprintf("%v", m["name"]))
		if name == "" {
			continue
		}
		email := strings.TrimSpace(fmt.Sprintf("%v", m["email"]))
		if email == "" || email == "<nil>" {
			email = strings.TrimSpace(fmt.Sprintf("%v", m["account"]))
			if email == "<nil>" {
				email = ""
			}
		}
		items = append(items, map[string]any{"name": name, "email": email})
	}
	if items == nil {
		items = []map[string]any{}
	}
	return items, nil
}

func fetchRemoteAccessToken(pool map[string]any, fileName string) (string, error) {
	baseURL := strings.TrimSpace(fmt.Sprintf("%v", pool["base_url"]))
	secretKey := strings.TrimSpace(fmt.Sprintf("%v", pool["secret_key"]))
	fileName = strings.TrimSpace(fileName)
	if baseURL == "" || secretKey == "" || fileName == "" {
		return "", fmt.Errorf("invalid request")
	}

	url := strings.TrimRight(baseURL, "/") + "/v0/management/auth-files/download?name=" + fileName
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	for k, v := range managementHeaders(secretKey) {
		req.Header.Set(k, v)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("invalid payload")
	}

	accessToken := strings.TrimSpace(fmt.Sprintf("%v", payload["access_token"]))
	if accessToken == "" || accessToken == "<nil>" {
		return "", fmt.Errorf("missing access_token")
	}
	return accessToken, nil
}

type CPAImportService struct {
	config         *CPAConfig
	accountService *AccountService
}

func NewCPAImportService(config *CPAConfig, accountService *AccountService) *CPAImportService {
	return &CPAImportService{config: config, accountService: accountService}
}

func (s *CPAImportService) StartImport(pool map[string]any, selectedFiles []string) (map[string]any, error) {
	var names []string
	for _, name := range selectedFiles {
		n := strings.TrimSpace(name)
		if n != "" {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("selected files is required")
	}

	poolID := strings.TrimSpace(fmt.Sprintf("%v", pool["id"]))
	job := map[string]any{
		"job_id":     uuid.New().String(),
		"status":     "pending",
		"created_at": nowISO(),
		"updated_at": nowISO(),
		"total":      len(names),
		"completed":  0,
		"added":      0,
		"skipped":    0,
		"refreshed":  0,
		"failed":     0,
		"errors":     []any{},
	}

	savedPool := s.config.SetImportJob(poolID, job)
	if savedPool == nil {
		return nil, fmt.Errorf("pool not found")
	}

	go s.runImport(poolID, pool, names)

	if importJob, ok := savedPool["import_job"].(map[string]any); ok {
		return importJob, nil
	}
	return job, nil
}

func (s *CPAImportService) updateJob(poolID string, updates map[string]any) {
	current := s.config.GetImportJob(poolID)
	if current == nil {
		return
	}
	for k, v := range updates {
		current[k] = v
	}
	current["updated_at"] = nowISO()
	s.config.SetImportJob(poolID, current)
}

func (s *CPAImportService) appendError(poolID, fileName, message string) {
	current := s.config.GetImportJob(poolID)
	if current == nil {
		return
	}
	errors, _ := current["errors"].([]any)
	errors = append(errors, map[string]any{"name": fileName, "error": message})
	s.updateJob(poolID, map[string]any{"errors": errors, "failed": len(errors)})
}

func (s *CPAImportService) runImport(poolID string, pool map[string]any, names []string) {
	s.updateJob(poolID, map[string]any{"status": "running"})

	type fetchResult struct {
		fileName string
		token    string
		err      error
	}

	maxWorkers := min(16, max(1, len(names)))
	results := make(chan fetchResult, len(names))
	sem := make(chan struct{}, maxWorkers)

	for _, name := range names {
		sem <- struct{}{}
		go func(n string) {
			defer func() { <-sem }()
			token, err := fetchRemoteAccessToken(pool, n)
			results <- fetchResult{fileName: n, token: token, err: err}
		}(name)
	}

	var tokens []string
	for i := 0; i < len(names); i++ {
		r := <-results
		if r.token != "" {
			tokens = append(tokens, r.token)
		} else {
			errMsg := "unknown error"
			if r.err != nil {
				errMsg = r.err.Error()
			}
			s.appendError(poolID, r.fileName, errMsg)
		}

		current := s.config.GetImportJob(poolID)
		completed := 0
		failed := 0
		if current != nil {
			completed = toInt(current["completed"])
			errors, _ := current["errors"].([]any)
			failed = len(errors)
		}
		s.updateJob(poolID, map[string]any{"completed": completed + 1, "failed": failed})
	}

	if len(tokens) == 0 {
		current := s.config.GetImportJob(poolID)
		total := 0
		failed := 0
		if current != nil {
			total = toInt(current["total"])
			errors, _ := current["errors"].([]any)
			failed = len(errors)
		}
		s.updateJob(poolID, map[string]any{
			"status":    "failed",
			"completed": total,
			"failed":    failed,
		})
		return
	}

	addResult := s.accountService.AddAccounts(tokens)
	refreshResult := s.accountService.RefreshAccounts(tokens)
	current := s.config.GetImportJob(poolID)
	failed := 0
	if current != nil {
		errors, _ := current["errors"].([]any)
		failed = len(errors)
	}
	s.updateJob(poolID, map[string]any{
		"status":    "completed",
		"completed": len(names),
		"added":     toInt(addResult["added"]),
		"skipped":   toInt(addResult["skipped"]),
		"refreshed": toInt(refreshResult["refreshed"]),
		"failed":    failed,
	})
}

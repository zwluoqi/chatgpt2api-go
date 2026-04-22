package services

import (
	"path/filepath"
	"testing"
)

func tempCPAConfig(t *testing.T) *CPAConfig {
	t.Helper()
	dir := t.TempDir()
	return NewCPAConfig(filepath.Join(dir, "cpa_config.json"))
}

func TestCPAConfigAddAndList(t *testing.T) {
	cfg := tempCPAConfig(t)

	pool := cfg.AddPool("Pool A", "https://example.com", "secret123")
	if pool["name"] != "Pool A" {
		t.Errorf("name = %v, want Pool A", pool["name"])
	}
	if pool["base_url"] != "https://example.com" {
		t.Errorf("base_url = %v", pool["base_url"])
	}

	pools := cfg.ListPools()
	if len(pools) != 1 {
		t.Errorf("len(pools) = %d, want 1", len(pools))
	}
}

func TestCPAConfigGetPool(t *testing.T) {
	cfg := tempCPAConfig(t)
	pool := cfg.AddPool("Test", "https://example.com", "secret")
	poolID := pool["id"].(string)

	found := cfg.GetPool(poolID)
	if found == nil {
		t.Fatal("GetPool returned nil")
	}
	if found["name"] != "Test" {
		t.Errorf("name = %v, want Test", found["name"])
	}

	notFound := cfg.GetPool("nonexistent")
	if notFound != nil {
		t.Error("GetPool should return nil for nonexistent")
	}
}

func TestCPAConfigUpdate(t *testing.T) {
	cfg := tempCPAConfig(t)
	pool := cfg.AddPool("Old", "https://old.com", "secret")
	poolID := pool["id"].(string)

	updated := cfg.UpdatePool(poolID, map[string]any{"name": "New"})
	if updated == nil {
		t.Fatal("UpdatePool returned nil")
	}
	if updated["name"] != "New" {
		t.Errorf("name = %v, want New", updated["name"])
	}
	if updated["base_url"] != "https://old.com" {
		t.Errorf("base_url should not change: %v", updated["base_url"])
	}
}

func TestCPAConfigDelete(t *testing.T) {
	cfg := tempCPAConfig(t)
	pool := cfg.AddPool("Del", "https://del.com", "secret")
	poolID := pool["id"].(string)

	if !cfg.DeletePool(poolID) {
		t.Error("DeletePool should return true")
	}
	if cfg.DeletePool(poolID) {
		t.Error("DeletePool should return false for already deleted")
	}

	pools := cfg.ListPools()
	if len(pools) != 0 {
		t.Errorf("len(pools) = %d, want 0", len(pools))
	}
}

func TestCPAConfigImportJob(t *testing.T) {
	cfg := tempCPAConfig(t)
	pool := cfg.AddPool("Test", "https://example.com", "secret")
	poolID := pool["id"].(string)

	job := map[string]any{
		"job_id":  "job123",
		"status":  "running",
		"total":   5,
		"completed": 2,
	}
	result := cfg.SetImportJob(poolID, job)
	if result == nil {
		t.Fatal("SetImportJob returned nil")
	}

	importJob := cfg.GetImportJob(poolID)
	if importJob == nil {
		t.Fatal("GetImportJob returned nil")
	}
	if importJob["status"] != "running" {
		t.Errorf("status = %v, want running", importJob["status"])
	}
}

func TestCPAConfigPersistence(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "cpa.json")

	cfg1 := NewCPAConfig(file)
	cfg1.AddPool("Persist", "https://persist.com", "sec")

	cfg2 := NewCPAConfig(file)
	pools := cfg2.ListPools()
	if len(pools) != 1 {
		t.Errorf("len(pools) = %d, want 1 after reload", len(pools))
	}
	if pools[0]["name"] != "Persist" {
		t.Errorf("name = %v, want Persist", pools[0]["name"])
	}
}

func TestSanitizeCPAPool(t *testing.T) {
	pool := map[string]any{
		"id":         "abc",
		"name":       "Test",
		"base_url":   "https://example.com",
		"secret_key": "should-be-removed",
	}
	sanitized := sanitizeCPAPool(pool)
	if sanitized["secret_key"] != nil {
		t.Error("secret_key should be removed")
	}
	if sanitized["name"] != "Test" {
		t.Errorf("name = %v, want Test", sanitized["name"])
	}
}

func TestNormalizeImportJob(t *testing.T) {
	job := normalizeImportJob(map[string]any{
		"job_id": "j1",
		"status": "pending",
		"total":  float64(10),
	}, true)
	if job == nil {
		t.Fatal("normalizeImportJob returned nil")
	}
	if job["status"] != "failed" {
		t.Errorf("status = %v, want failed (failUnfinished=true)", job["status"])
	}

	job2 := normalizeImportJob(map[string]any{
		"job_id": "j2",
		"status": "completed",
	}, true)
	if job2["status"] != "completed" {
		t.Errorf("status = %v, want completed", job2["status"])
	}
}

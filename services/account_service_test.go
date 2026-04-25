package services

import (
	"os"
	"path/filepath"
	"testing"
)

func tempAccountService(t *testing.T) *AccountService {
	t.Helper()
	dir := t.TempDir()
	storeFile := filepath.Join(dir, "accounts.json")
	return NewAccountService(storeFile)
}

func TestAccountServiceAddAndList(t *testing.T) {
	as := tempAccountService(t)

	result := as.AddAccounts([]string{"token_a", "token_b"})
	if result["added"] != 2 {
		t.Errorf("added = %v, want 2", result["added"])
	}
	if result["skipped"] != 0 {
		t.Errorf("skipped = %v, want 0", result["skipped"])
	}

	accounts := as.ListAccounts()
	if len(accounts) != 2 {
		t.Errorf("len(accounts) = %d, want 2", len(accounts))
	}
}

func TestAccountServiceAddDuplicates(t *testing.T) {
	as := tempAccountService(t)

	as.AddAccounts([]string{"token_a", "token_b"})
	result := as.AddAccounts([]string{"token_b", "token_c"})
	if result["added"] != 1 {
		t.Errorf("added = %v, want 1", result["added"])
	}
	if result["skipped"] != 1 {
		t.Errorf("skipped = %v, want 1", result["skipped"])
	}

	accounts := as.ListAccounts()
	if len(accounts) != 3 {
		t.Errorf("len(accounts) = %d, want 3", len(accounts))
	}
}

func TestAccountServiceDelete(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a", "token_b", "token_c"})

	result := as.DeleteAccounts([]string{"token_b"})
	if toInt(result["removed"]) != 1 {
		t.Errorf("removed = %v, want 1", result["removed"])
	}

	accounts := as.ListAccounts()
	if len(accounts) != 2 {
		t.Errorf("len(accounts) = %d, want 2", len(accounts))
	}
}

func TestAccountServiceRemoveToken(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_x"})

	removed := as.RemoveToken("token_x")
	if !removed {
		t.Error("expected RemoveToken to return true")
	}

	removed = as.RemoveToken("nonexistent")
	if removed {
		t.Error("expected RemoveToken to return false for nonexistent")
	}
}

func TestAccountServiceUpdate(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})

	account := as.UpdateAccount("token_a", map[string]any{
		"type":   "Plus",
		"status": "正常",
		"quota":  50,
	})
	if account == nil {
		t.Fatal("UpdateAccount returned nil")
	}
	if account["type"] != "Plus" {
		t.Errorf("type = %v, want Plus", account["type"])
	}
	if toInt(account["quota"]) != 50 {
		t.Errorf("quota = %v, want 50", account["quota"])
	}
}

func TestAccountServiceMarkImageResult(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 3, "image_quota_unknown": false})

	account := as.MarkImageResult("token_a", true)
	if account == nil {
		t.Fatal("MarkImageResult returned nil")
	}
	if toInt(account["success"]) != 1 {
		t.Errorf("success = %v, want 1", account["success"])
	}
	if toInt(account["quota"]) != 2 {
		t.Errorf("quota = %v, want 2", account["quota"])
	}

	account = as.MarkImageResult("token_a", false)
	if toInt(account["fail"]) != 1 {
		t.Errorf("fail = %v, want 1", account["fail"])
	}
}

func TestAccountServiceMarkImageResultQuotaExhaust(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 1, "status": "正常", "image_quota_unknown": false})

	account := as.MarkImageResult("token_a", true)
	if toInt(account["quota"]) != 0 {
		t.Errorf("quota = %v, want 0", account["quota"])
	}
	if account["status"] != "限流" {
		t.Errorf("status = %v, want 限流", account["status"])
	}
}

func TestAccountServiceUpdateQuotaZeroSetsLimited(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 3, "status": "正常", "image_quota_unknown": false})

	account := as.UpdateAccount("token_a", map[string]any{"quota": 0})
	if account == nil {
		t.Fatal("UpdateAccount returned nil")
	}
	if toInt(account["quota"]) != 0 {
		t.Errorf("quota = %v, want 0", account["quota"])
	}
	if account["status"] != "限流" {
		t.Errorf("status = %v, want 限流", account["status"])
	}
}

func TestAccountServiceZeroQuotaIsNotAvailable(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 0, "image_quota_unknown": false})

	_, err := as.GetAvailableAccessToken()
	if err == nil {
		t.Fatal("expected no available token")
	}
}

func TestAccountServiceMarkImageResultUnknownQuota(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{
		"quota":               0,
		"status":              "正常",
		"image_quota_unknown": true,
	})

	account := as.MarkImageResult("token_a", true)
	if account == nil {
		t.Fatal("MarkImageResult returned nil")
	}
	if toInt(account["quota"]) != 0 {
		t.Errorf("quota = %v, want 0", account["quota"])
	}
	if account["status"] != "正常" {
		t.Errorf("status = %v, want 正常", account["status"])
	}
}

func TestAccountServiceListTokens(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_1", "token_2"})

	tokens := as.ListTokens()
	if len(tokens) != 2 {
		t.Errorf("len(tokens) = %d, want 2", len(tokens))
	}
}

func TestAccountServiceListLimitedTokens(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_1", "token_2"})
	as.UpdateAccount("token_1", map[string]any{"status": "限流", "quota": 0, "image_quota_unknown": false})

	limited := as.ListLimitedTokens()
	if len(limited) != 1 {
		t.Errorf("len(limited) = %d, want 1", len(limited))
	}
	if limited[0] != "token_1" {
		t.Errorf("limited[0] = %v, want token_1", limited[0])
	}
}

func TestNormalizeAccountImageQuotaUnknown(t *testing.T) {
	account := normalizeAccount(map[string]any{
		"access_token":        "test_token",
		"image_quota_unknown": true,
	})
	if account == nil {
		t.Fatal("normalizeAccount returned nil")
	}
	if account["image_quota_unknown"] != true {
		t.Errorf("image_quota_unknown = %v, want true", account["image_quota_unknown"])
	}
}

func TestAccountServicePersistence(t *testing.T) {
	dir := t.TempDir()
	storeFile := filepath.Join(dir, "accounts.json")

	as1 := NewAccountService(storeFile)
	as1.AddAccounts([]string{"persist_token"})
	as1.UpdateAccount("persist_token", map[string]any{"type": "Pro", "quota": 100})

	as2 := NewAccountService(storeFile)
	accounts := as2.ListAccounts()
	if len(accounts) != 1 {
		t.Fatalf("len(accounts) = %d, want 1", len(accounts))
	}
	if accounts[0]["access_token"] != "persist_token" {
		t.Errorf("access_token = %v, want persist_token", accounts[0]["access_token"])
	}
}

func TestAccountServiceGetAccount(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})

	account := as.GetAccount("token_a")
	if account == nil {
		t.Fatal("GetAccount returned nil")
	}
	if account["access_token"] != "token_a" {
		t.Errorf("access_token = %v, want token_a", account["access_token"])
	}

	account = as.GetAccount("nonexistent")
	if account != nil {
		t.Error("GetAccount should return nil for nonexistent token")
	}
}

func TestNormalizeAccount(t *testing.T) {
	account := normalizeAccount(map[string]any{
		"access_token": "test_token",
		"quota":        -5,
	})
	if account == nil {
		t.Fatal("normalizeAccount returned nil")
	}
	if toInt(account["quota"]) != 0 {
		t.Errorf("quota = %v, want 0 (negative should be clamped)", account["quota"])
	}
	if account["type"] != "Free" {
		t.Errorf("type = %v, want Free (default)", account["type"])
	}
	if account["status"] != "限流" {
		t.Errorf("status = %v, want 限流 (quota=0 with image_quota_unknown=false)", account["status"])
	}
}

func TestNormalizeAccountNilToken(t *testing.T) {
	account := normalizeAccount(map[string]any{})
	if account != nil {
		t.Error("normalizeAccount should return nil for empty access_token")
	}
}

func TestExtractQuotaAndRestoreAt(t *testing.T) {
	limits := []any{
		map[string]any{
			"feature_name": "image_gen",
			"remaining":    float64(42),
			"reset_after":  "2024-05-01T00:00:00Z",
		},
	}
	quota, restoreAt, unknown := extractQuotaAndRestoreAt(limits)
	if quota != 42 {
		t.Errorf("quota = %d, want 42", quota)
	}
	if restoreAt == nil || *restoreAt != "2024-05-01T00:00:00Z" {
		t.Errorf("restoreAt = %v, want 2024-05-01T00:00:00Z", restoreAt)
	}
	if unknown {
		t.Error("unknown = true, want false")
	}
}

func TestExtractQuotaNoImageGen(t *testing.T) {
	limits := []any{
		map[string]any{"feature_name": "other", "remaining": float64(100)},
	}
	quota, restoreAt, unknown := extractQuotaAndRestoreAt(limits)
	if quota != 0 {
		t.Errorf("quota = %d, want 0", quota)
	}
	if restoreAt != nil {
		t.Errorf("restoreAt = %v, want nil", restoreAt)
	}
	if !unknown {
		t.Error("unknown = false, want true")
	}
}

func TestAccountTypeDetection(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"free", "Free"},
		{"plus", "Plus"},
		{"Plus", "Plus"},
		{"team", "Team"},
		{"pro", "Pro"},
		{"personal", "Plus"},
		{"business", "Team"},
		{"enterprise", "Team"},
		{"unknown", ""},
	}
	for _, tt := range tests {
		result := normalizeAccountType(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeAccountType(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCleanTokens(t *testing.T) {
	result := cleanTokens([]string{" a ", "b", " a ", "", "  c  "})
	if len(result) != 3 {
		t.Errorf("len(result) = %d, want 3", len(result))
	}
	expected := []string{"a", "b", "c"}
	for i, v := range result {
		if v != expected[i] {
			t.Errorf("result[%d] = %q, want %q", i, v, expected[i])
		}
	}
}

func TestAccountServiceEmptyFile(t *testing.T) {
	dir := t.TempDir()
	storeFile := filepath.Join(dir, "accounts.json")
	os.WriteFile(storeFile, []byte("[]"), 0o644)

	as := NewAccountService(storeFile)
	accounts := as.ListAccounts()
	if len(accounts) != 0 {
		t.Errorf("len(accounts) = %d, want 0", len(accounts))
	}
}

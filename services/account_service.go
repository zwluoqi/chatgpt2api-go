package services

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

var AccountTypeMap = map[string]string{
	"free":       "Free",
	"plus":       "Plus",
	"prolite":    "ProLite",
	"pro_lite":   "ProLite",
	"team":       "Team",
	"pro":        "Pro",
	"personal":   "Plus",
	"business":   "Team",
	"enterprise": "Team",
}

type AccountService struct {
	storeFile string
	mu        sync.Mutex
	index     int
	accounts  []map[string]any
}

func NewAccountService(storeFile string) *AccountService {
	as := &AccountService{
		storeFile: storeFile,
	}
	as.accounts = as.loadAccounts()
	return as
}

func cleanToken(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func cleanTokens(tokens []string) []string {
	var cleaned []string
	seen := make(map[string]bool)
	for _, token := range tokens {
		v := strings.TrimSpace(token)
		if v != "" && !seen[v] {
			seen[v] = true
			cleaned = append(cleaned, v)
		}
	}
	return cleaned
}

func (as *AccountService) findAccountIndex(accessToken string) int {
	for i, item := range as.accounts {
		if cleanToken(item["access_token"]) == accessToken {
			return i
		}
	}
	return -1
}

func isImageAccountAvailable(account map[string]any) bool {
	if account == nil {
		return false
	}
	status := fmt.Sprintf("%v", account["status"])
	if status == "禁用" || status == "限流" || status == "异常" {
		return false
	}
	if imageQuotaUnknown, ok := account["image_quota_unknown"].(bool); ok && imageQuotaUnknown {
		return true
	}
	quota := toInt(account["quota"])
	return quota > 0
}

func toInt(v any) int {
	if v == nil {
		return 0
	}
	switch val := v.(type) {
	case float64:
		return int(val)
	case int:
		return val
	case int64:
		return int(val)
	case json.Number:
		n, _ := val.Int64()
		return int(n)
	case string:
		var n int
		fmt.Sscanf(val, "%d", &n)
		return n
	}
	return 0
}

func (as *AccountService) decodeAccessTokenPayload(accessToken string) map[string]any {
	parts := strings.Split(cleanToken(accessToken), ".")
	if len(parts) < 2 {
		return nil
	}
	payload := parts[1]
	for len(payload)%4 != 0 {
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		decoded, err = base64.RawURLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var data map[string]any
	if err := json.Unmarshal(decoded, &data); err != nil {
		return nil
	}
	return data
}

func normalizeAccountType(value any) string {
	v := strings.ToLower(cleanToken(value))
	if mapped, ok := AccountTypeMap[v]; ok {
		return mapped
	}
	return ""
}

func searchAccountType(value any) string {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			keyText := strings.ToLower(cleanToken(key))
			matched := normalizeAccountType(item)
			if matched != "" {
				flags := []string{"plan", "type", "subscription", "workspace", "tier"}
				for _, flag := range flags {
					if strings.Contains(keyText, flag) {
						return matched
					}
				}
			}
		}
		for _, item := range v {
			matched := searchAccountType(item)
			if matched != "" {
				return matched
			}
		}
		return ""
	case []any:
		for _, item := range v {
			matched := searchAccountType(item)
			if matched != "" {
				return matched
			}
		}
		return ""
	default:
		return normalizeAccountType(value)
	}
}

func (as *AccountService) detectAccountType(accessToken string, mePayload, initPayload any) string {
	tokenPayload := as.decodeAccessTokenPayload(accessToken)
	if tokenPayload != nil {
		if authPayload, ok := tokenPayload["https://api.openai.com/auth"].(map[string]any); ok {
			matched := normalizeAccountType(authPayload["chatgpt_plan_type"])
			if matched != "" {
				return matched
			}
		}
	}

	for _, payload := range []any{mePayload, initPayload, tokenPayload} {
		matched := searchAccountType(payload)
		if matched != "" {
			return matched
		}
	}
	return "Free"
}

func normalizeAccount(item map[string]any) map[string]any {
	if item == nil {
		return nil
	}
	accessToken := cleanToken(item["access_token"])
	if accessToken == "" {
		return nil
	}
	normalized := make(map[string]any)
	for k, v := range item {
		normalized[k] = v
	}
	normalized["access_token"] = accessToken
	if cleanToken(normalized["type"]) == "" {
		normalized["type"] = "Free"
	} else {
		normalized["type"] = cleanToken(normalized["type"])
	}
	if cleanToken(normalized["status"]) == "" {
		normalized["status"] = "正常"
	} else {
		normalized["status"] = cleanToken(normalized["status"])
	}
	quota := toInt(normalized["quota"])
	if quota < 0 {
		quota = 0
	}
	normalized["quota"] = quota
	if imageQuotaUnknown, ok := normalized["image_quota_unknown"].(bool); ok {
		normalized["image_quota_unknown"] = imageQuotaUnknown
	} else {
		normalized["image_quota_unknown"] = false
	}

	email := cleanToken(normalized["email"])
	if email == "" {
		normalized["email"] = nil
	} else {
		normalized["email"] = email
	}
	userID := cleanToken(normalized["user_id"])
	if userID == "" {
		normalized["user_id"] = nil
	} else {
		normalized["user_id"] = userID
	}

	if lp, ok := normalized["limits_progress"].([]any); ok {
		normalized["limits_progress"] = lp
	} else {
		normalized["limits_progress"] = []any{}
	}

	dms := cleanToken(normalized["default_model_slug"])
	if dms == "" {
		normalized["default_model_slug"] = nil
	} else {
		normalized["default_model_slug"] = dms
	}
	ra := cleanToken(normalized["restore_at"])
	if ra == "" {
		normalized["restore_at"] = nil
	} else {
		normalized["restore_at"] = ra
	}
	normalized["success"] = toInt(normalized["success"])
	normalized["fail"] = toInt(normalized["fail"])
	syncImageStatusByQuota(normalized)
	return normalized
}

func syncImageStatusByQuota(account map[string]any) {
	if account == nil {
		return
	}

	status := cleanToken(account["status"])
	if status == "禁用" || status == "异常" {
		return
	}

	imageQuotaUnknown, _ := account["image_quota_unknown"].(bool)
	quota := toInt(account["quota"])

	if imageQuotaUnknown {
		if status == "" || status == "限流" {
			account["status"] = "正常"
		}
		account["restore_at"] = nil
		return
	}

	if quota == 0 {
		account["status"] = "限流"
		return
	}

	if status == "" || status == "限流" {
		account["status"] = "正常"
	}
	account["restore_at"] = nil
}

func extractQuotaAndRestoreAt(limitsProgress []any) (int, *string, bool) {
	for _, item := range limitsProgress {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if fmt.Sprintf("%v", m["feature_name"]) != "image_gen" {
			continue
		}
		quota := toInt(m["remaining"])
		restoreAt := cleanToken(m["reset_after"])
		if restoreAt == "" {
			return quota, nil, false
		}
		return quota, &restoreAt, false
	}
	return 0, nil, true
}

func (as *AccountService) loadAccounts() []map[string]any {
	data, err := os.ReadFile(as.storeFile)
	if err != nil {
		return nil
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil
	}
	var result []map[string]any
	for _, item := range raw {
		normalized := normalizeAccount(item)
		if normalized != nil {
			result = append(result, normalized)
		}
	}
	return result
}

func (as *AccountService) saveAccounts() {
	dir := filepath.Dir(as.storeFile)
	os.MkdirAll(dir, 0o755)
	data, _ := json.MarshalIndent(as.accounts, "", "  ")
	os.WriteFile(as.storeFile, append(data, '\n'), 0o644)
}

func (as *AccountService) buildRemoteHeaders(accessToken string) (map[string]string, string) {
	account := as.GetAccount(accessToken)
	if account == nil {
		account = map[string]any{}
	}

	userAgent := cleanToken(account["user-agent"])
	if userAgent == "" {
		userAgent = cleanToken(account["user_agent"])
	}
	impersonate := cleanToken(account["impersonate"])
	if impersonate == "" {
		impersonate = "edge101"
	}
	if userAgent == "" {
		userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	}
	secChUa := cleanToken(account["sec-ch-ua"])
	if secChUa == "" {
		secChUa = `"Google Chrome";v="147", "Not.A/Brand";v="8", "Chromium";v="147"`
	}
	secChUaMobile := cleanToken(account["sec-ch-ua-mobile"])
	if secChUaMobile == "" {
		secChUaMobile = "?0"
	}
	secChUaPlatform := cleanToken(account["sec-ch-ua-platform"])
	if secChUaPlatform == "" {
		secChUaPlatform = `"Windows"`
	}

	headers := map[string]string{
		"authorization":      fmt.Sprintf("Bearer %s", accessToken),
		"accept":             "*/*",
		"accept-language":    "zh-CN,zh;q=0.9,en;q=0.8",
		"content-type":       "application/json",
		"oai-language":       "zh-CN",
		"origin":             "https://chatgpt.com",
		"referer":            "https://chatgpt.com/",
		"sec-fetch-dest":     "empty",
		"sec-fetch-mode":     "cors",
		"sec-fetch-site":     "same-origin",
		"user-agent":         userAgent,
		"sec-ch-ua":          secChUa,
		"sec-ch-ua-mobile":   secChUaMobile,
		"sec-ch-ua-platform": secChUaPlatform,
	}

	deviceID := cleanToken(account["oai-device-id"])
	if deviceID == "" {
		deviceID = cleanToken(account["oai_device_id"])
	}
	sessionID := cleanToken(account["oai-session-id"])
	if sessionID == "" {
		sessionID = cleanToken(account["oai_session_id"])
	}
	if deviceID != "" {
		headers["oai-device-id"] = deviceID
	}
	if sessionID != "" {
		headers["oai-session-id"] = sessionID
	}

	return headers, impersonate
}

func (as *AccountService) publicItems(accounts []map[string]any) []map[string]any {
	var result []map[string]any
	for _, account := range accounts {
		accessToken := cleanToken(account["access_token"])
		if accessToken == "" {
			continue
		}
		h := sha1.New()
		h.Write([]byte(accessToken))
		id := fmt.Sprintf("%x", h.Sum(nil))[:16]

		item := map[string]any{
			"id":                 id,
			"access_token":       accessToken,
			"type":               account["type"],
			"status":             account["status"],
			"quota":              toInt(account["quota"]),
			"imageQuotaUnknown":  account["image_quota_unknown"],
			"email":              account["email"],
			"user_id":            account["user_id"],
			"limits_progress":    account["limits_progress"],
			"default_model_slug": account["default_model_slug"],
			"restoreAt":          account["restore_at"],
			"success":            toInt(account["success"]),
			"fail":               toInt(account["fail"]),
			"lastUsedAt":         account["last_used_at"],
		}
		if item["limits_progress"] == nil {
			item["limits_progress"] = []any{}
		}
		result = append(result, item)
	}
	if result == nil {
		result = []map[string]any{}
	}
	return result
}

func (as *AccountService) ListTokens() []string {
	as.mu.Lock()
	defer as.mu.Unlock()
	var tokens []string
	for _, item := range as.accounts {
		token := cleanToken(item["access_token"])
		if token != "" {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func (as *AccountService) listAvailableCandidateTokens(excluded map[string]bool) []string {
	var tokens []string
	for _, item := range as.accounts {
		if !isImageAccountAvailable(item) {
			continue
		}
		token := cleanToken(item["access_token"])
		if token == "" || (excluded != nil && excluded[token]) {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func (as *AccountService) pickNextCandidateToken(excluded map[string]bool) (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	tokens := as.listAvailableCandidateTokens(excluded)
	if len(tokens) == 0 {
		return "", fmt.Errorf("no available tokens found in %s", as.storeFile)
	}
	accessToken := tokens[as.index%len(tokens)]
	as.index++
	return accessToken, nil
}

func (as *AccountService) RefreshAccountState(accessToken string) map[string]any {
	remoteInfo, err := as.FetchRemoteInfo(accessToken)
	if err != nil {
		message := err.Error()
		fmt.Printf("[account-available] refresh token=%s... fail %s\n", accessToken[:min(12, len(accessToken))], message)
		if strings.Contains(message, "/backend-api/me failed: HTTP 401") {
			return as.UpdateAccount(accessToken, map[string]any{
				"status": "异常",
				"quota":  0,
			})
		}
		return nil
	}
	return as.UpdateAccount(accessToken, remoteInfo)
}

func (as *AccountService) GetAvailableAccessToken() (string, error) {
	as.mu.Lock()
	defer as.mu.Unlock()
	tokens := as.listAvailableCandidateTokens(nil)
	if len(tokens) == 0 {
		return "", fmt.Errorf("no available tokens found in %s", as.storeFile)
	}
	accessToken := tokens[as.index%len(tokens)]
	as.index++
	return accessToken, nil
}

func (as *AccountService) NextToken() (string, error) {
	return as.GetAvailableAccessToken()
}

func (as *AccountService) GetAccount(accessToken string) map[string]any {
	accessToken = cleanToken(accessToken)
	if accessToken == "" {
		return nil
	}
	as.mu.Lock()
	defer as.mu.Unlock()
	idx := as.findAccountIndex(accessToken)
	if idx >= 0 {
		result := make(map[string]any)
		for k, v := range as.accounts[idx] {
			result[k] = v
		}
		return result
	}
	return nil
}

func (as *AccountService) ListAccounts() []map[string]any {
	as.mu.Lock()
	defer as.mu.Unlock()
	return as.publicItems(as.accounts)
}

func (as *AccountService) ListLimitedTokens() []string {
	as.mu.Lock()
	defer as.mu.Unlock()
	var tokens []string
	for _, item := range as.accounts {
		if fmt.Sprintf("%v", item["status"]) == "限流" {
			token := cleanToken(item["access_token"])
			if token != "" {
				tokens = append(tokens, token)
			}
		}
	}
	return tokens
}

func (as *AccountService) AddAccounts(tokens []string) map[string]any {
	cleanedTokens := cleanTokens(tokens)
	if len(cleanedTokens) == 0 {
		return map[string]any{"added": 0, "skipped": 0, "items": as.ListAccounts()}
	}

	as.mu.Lock()
	defer as.mu.Unlock()

	indexed := make(map[string]map[string]any)
	var order []string
	for _, item := range as.accounts {
		token := cleanToken(item["access_token"])
		cloned := make(map[string]any)
		for k, v := range item {
			cloned[k] = v
		}
		indexed[token] = cloned
		order = append(order, token)
	}

	added := 0
	skipped := 0
	for _, accessToken := range cleanedTokens {
		current, exists := indexed[accessToken]
		if !exists {
			added++
			current = map[string]any{}
		} else {
			skipped++
		}
		current["access_token"] = accessToken
		if current["type"] == nil {
			current["type"] = "Free"
		}
		account := normalizeAccount(current)
		if account != nil {
			indexed[accessToken] = account
			if !exists {
				order = append(order, accessToken)
			}
		}
	}

	var newAccounts []map[string]any
	for _, token := range order {
		if acc, ok := indexed[token]; ok {
			newAccounts = append(newAccounts, acc)
		}
	}
	as.accounts = newAccounts
	as.saveAccounts()
	items := as.publicItems(as.accounts)
	return map[string]any{"added": added, "skipped": skipped, "items": items}
}

func (as *AccountService) DeleteAccounts(tokens []string) map[string]any {
	targetSet := make(map[string]bool)
	for _, token := range cleanTokens(tokens) {
		targetSet[token] = true
	}
	if len(targetSet) == 0 {
		return map[string]any{"removed": 0, "items": as.ListAccounts()}
	}

	as.mu.Lock()
	defer as.mu.Unlock()

	before := len(as.accounts)
	var newAccounts []map[string]any
	for _, item := range as.accounts {
		if !targetSet[cleanToken(item["access_token"])] {
			newAccounts = append(newAccounts, item)
		}
	}
	as.accounts = newAccounts
	removed := before - len(as.accounts)

	if len(as.accounts) > 0 {
		as.index = as.index % len(as.accounts)
	} else {
		as.index = 0
	}
	if removed > 0 {
		as.saveAccounts()
	}
	items := as.publicItems(as.accounts)
	return map[string]any{"removed": removed, "items": items}
}

func (as *AccountService) RemoveToken(accessToken string) bool {
	result := as.DeleteAccounts([]string{accessToken})
	return toInt(result["removed"]) > 0
}

func (as *AccountService) UpdateAccount(accessToken string, updates map[string]any) map[string]any {
	accessToken = cleanToken(accessToken)
	if accessToken == "" {
		return nil
	}

	as.mu.Lock()
	defer as.mu.Unlock()

	idx := as.findAccountIndex(accessToken)
	if idx < 0 {
		return nil
	}

	merged := make(map[string]any)
	for k, v := range as.accounts[idx] {
		merged[k] = v
	}
	for k, v := range updates {
		merged[k] = v
	}
	merged["access_token"] = accessToken

	account := normalizeAccount(merged)
	if account == nil {
		return nil
	}
	if _, ok := updates["quota"]; ok {
		syncImageStatusByQuota(account)
	}
	if _, ok := updates["image_quota_unknown"]; ok {
		syncImageStatusByQuota(account)
	}
	as.accounts[idx] = account
	as.saveAccounts()
	result := make(map[string]any)
	for k, v := range account {
		result[k] = v
	}
	return result
}

func (as *AccountService) MarkImageResult(accessToken string, success bool) map[string]any {
	accessToken = cleanToken(accessToken)
	if accessToken == "" {
		return nil
	}

	as.mu.Lock()
	defer as.mu.Unlock()

	idx := as.findAccountIndex(accessToken)
	if idx < 0 {
		return nil
	}

	nextItem := make(map[string]any)
	for k, v := range as.accounts[idx] {
		nextItem[k] = v
	}
	nextItem["last_used_at"] = time.Now().Format("2006-01-02 15:04:05")
	imageQuotaUnknown, _ := nextItem["image_quota_unknown"].(bool)

	if success {
		nextItem["success"] = toInt(nextItem["success"]) + 1
		if !imageQuotaUnknown {
			quota := toInt(nextItem["quota"]) - 1
			if quota < 0 {
				quota = 0
			}
			nextItem["quota"] = quota
			if quota == 0 {
				nextItem["status"] = "限流"
			} else if fmt.Sprintf("%v", nextItem["status"]) == "限流" {
				nextItem["status"] = "正常"
			}
		} else if fmt.Sprintf("%v", nextItem["status"]) == "限流" {
			nextItem["status"] = "正常"
		}
	} else {
		nextItem["fail"] = toInt(nextItem["fail"]) + 1
	}

	account := normalizeAccount(nextItem)
	if account == nil {
		return nil
	}
	as.accounts[idx] = account
	as.saveAccounts()
	result := make(map[string]any)
	for k, v := range account {
		result[k] = v
	}
	return result
}

func doJSONRequest(tc *TLSClient, method, urlStr string, headers map[string]string, body any, _ time.Duration) (*fhttp.Response, error) {
	if method == "GET" {
		return tc.Get(urlStr, headers)
	}
	return tc.PostJSON(urlStr, headers, body)
}

func (as *AccountService) FetchRemoteInfo(accessToken string) (map[string]any, error) {
	accessToken = cleanToken(accessToken)
	if accessToken == "" {
		return nil, fmt.Errorf("access_token is required")
	}

	headers, _ := as.buildRemoteHeaders(accessToken)
	fmt.Printf("[account-refresh] start %s...\n", accessToken[:min(12, len(accessToken))])

	tc, err := NewTLSClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create TLS client: %v", err)
	}

	type result struct {
		payload map[string]any
		err     error
	}

	meCh := make(chan result, 1)
	initCh := make(chan result, 1)

	go func() {
		meHeaders := make(map[string]string)
		for k, v := range headers {
			meHeaders[k] = v
		}
		meHeaders["x-openai-target-path"] = "/backend-api/me"
		meHeaders["x-openai-target-route"] = "/backend-api/me"

		resp, err := doJSONRequest(tc, "GET", "https://chatgpt.com/backend-api/me", meHeaders, nil, 20*time.Second)
		if err != nil {
			meCh <- result{nil, err}
			return
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			meCh <- result{nil, fmt.Errorf("/backend-api/me failed: HTTP %d", resp.StatusCode)}
			return
		}
		payload, err := ReadResponseJSON(resp)
		meCh <- result{payload, err}
	}()

	go func() {
		resp, err := doJSONRequest(tc, "POST", "https://chatgpt.com/backend-api/conversation/init", headers, map[string]any{
			"gizmo_id":                nil,
			"requested_default_model": nil,
			"conversation_id":         nil,
			"timezone_offset_min":     -480,
		}, 20*time.Second)
		if err != nil {
			initCh <- result{nil, err}
			return
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			initCh <- result{nil, fmt.Errorf("/backend-api/conversation/init failed: HTTP %d", resp.StatusCode)}
			return
		}
		payload, err := ReadResponseJSON(resp)
		initCh <- result{payload, err}
	}()

	meResult := <-meCh
	initResult := <-initCh

	if meResult.err != nil {
		return nil, meResult.err
	}
	if initResult.err != nil {
		return nil, initResult.err
	}

	mePayload := meResult.payload
	initPayload := initResult.payload

	var limitsProgress []any
	if lp, ok := initPayload["limits_progress"].([]any); ok {
		limitsProgress = lp
	} else {
		limitsProgress = []any{}
	}

	accountType := as.detectAccountType(accessToken, mePayload, initPayload)
	quota, restoreAt, imageQuotaUnknown := extractQuotaAndRestoreAt(limitsProgress)
	status := "正常"
	if !(imageQuotaUnknown && accountType != "Free") && quota == 0 {
		status = "限流"
	}

	info := map[string]any{
		"email":               mePayload["email"],
		"user_id":             mePayload["id"],
		"type":                accountType,
		"quota":               quota,
		"image_quota_unknown": imageQuotaUnknown,
		"limits_progress":     limitsProgress,
		"default_model_slug":  initPayload["default_model_slug"],
		"status":              status,
	}
	if restoreAt != nil {
		info["restore_at"] = *restoreAt
	}

	fmt.Printf("[account-refresh] ok %v %v quota=%d restore_at=%v\n",
		info["user_id"], info["email"], quota, restoreAt)
	return info, nil
}

func (as *AccountService) RefreshAccounts(accessTokens []string) map[string]any {
	cleanedTokens := cleanTokens(accessTokens)
	if len(cleanedTokens) == 0 {
		return map[string]any{"refreshed": 0, "errors": []any{}, "items": as.ListAccounts()}
	}

	maxWorkers := min(64, len(cleanedTokens))
	type refreshResult struct {
		accessToken string
		info        map[string]any
		err         error
	}

	results := make(chan refreshResult, len(cleanedTokens))
	sem := make(chan struct{}, maxWorkers)

	for _, token := range cleanedTokens {
		sem <- struct{}{}
		go func(t string) {
			defer func() { <-sem }()
			info, err := as.FetchRemoteInfo(t)
			results <- refreshResult{t, info, err}
		}(token)
	}

	refreshed := 0
	var errors []any

	for i := 0; i < len(cleanedTokens); i++ {
		r := <-results
		if r.err != nil {
			message := r.err.Error()
			tokenPrefix := r.accessToken[:min(12, len(r.accessToken))]
			fmt.Printf("[account-refresh] fail %s... %s\n", tokenPrefix, message)
			if strings.Contains(message, "/backend-api/me failed: HTTP 401") {
				as.UpdateAccount(r.accessToken, map[string]any{
					"status": "异常",
					"quota":  0,
				})
				message = "检测到封号"
			}
			errors = append(errors, map[string]any{
				"access_token": r.accessToken,
				"error":        message,
			})
		} else {
			if as.UpdateAccount(r.accessToken, r.info) != nil {
				refreshed++
			}
		}
	}

	if errors == nil {
		errors = []any{}
	}
	fmt.Printf("[account-refresh] done refreshed=%d errors=%d workers=%d\n", refreshed, len(errors), maxWorkers)
	return map[string]any{
		"refreshed": refreshed,
		"errors":    errors,
		"items":     as.ListAccounts(),
	}
}

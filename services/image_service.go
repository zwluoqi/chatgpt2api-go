package services

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chatgpt2api-go/config"

	fhttp "github.com/bogdanfinn/fhttp"
	"github.com/google/uuid"
)

const (
	baseURL      = "https://chatgpt.com"
	userAgentStr = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	defaultModel = "gpt-4o"
)

type ImageGenerationError struct {
	Message    string
	StatusCode int
	ErrorType  string
	Code       string
	Reason     string
	Meta       map[string]any
}

func (e *ImageGenerationError) Error() string {
	return e.Message
}

func buildImageErrorMeta(state sseResult, waitedForResult, waitedWhileQueued bool) map[string]any {
	meta := map[string]any{
		"upstream_conversation_id": state.ConversationID,
		"waited_for_result":        waitedForResult,
		"waited_while_queued":      waitedWhileQueued,
		"upstream_text_present":    strings.TrimSpace(state.Text) != "",
		"upstream_file_count":      len(state.FileIDs),
	}
	if state.RejectCode != "" {
		meta["reject_code"] = state.RejectCode
	}
	return meta
}

type GeneratedImage struct {
	B64JSON       string
	RevisedPrompt string
	URL           string
}

type EditInputImage struct {
	FileID   string
	Data     []byte
	FileName string
	MimeType string
	Width    int
	Height   int
}

func buildFP(accountService *AccountService, accessToken string) map[string]string {
	account := accountService.GetAccount(accessToken)
	if account == nil {
		account = map[string]any{}
	}
	fp := make(map[string]string)

	if rawFP, ok := account["fp"].(map[string]any); ok {
		for k, v := range rawFP {
			fp[strings.ToLower(k)] = fmt.Sprintf("%v", v)
		}
	}
	for _, key := range []string{"user-agent", "impersonate", "oai-device-id", "sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform"} {
		if v, ok := account[key]; ok {
			fp[key] = fmt.Sprintf("%v", v)
		}
	}
	if fp["user-agent"] == "" {
		fp["user-agent"] = userAgentStr
	}
	if fp["impersonate"] == "" {
		fp["impersonate"] = "edge101"
	}
	if fp["oai-device-id"] == "" {
		fp["oai-device-id"] = uuid.New().String()
	}
	return fp
}

type session struct {
	tc *TLSClient
	fp map[string]string
}

func newSession(accountService *AccountService, accessToken string) *session {
	fp := buildFP(accountService, accessToken)
	tc, err := NewTLSClientWithUA(fp["user-agent"])
	if err != nil {
		tc, _ = NewTLSClient()
	}
	return &session{tc: tc, fp: fp}
}

func (s *session) get(urlStr string, headers map[string]string, _ time.Duration) (*fhttp.Response, error) {
	merged := make(map[string]string)
	for k, v := range headers {
		merged[k] = v
	}
	if merged["origin"] == "" {
		merged["origin"] = baseURL
	}
	if merged["referer"] == "" {
		merged["referer"] = baseURL + "/"
	}
	if merged["oai-device-id"] == "" && s.fp["oai-device-id"] != "" {
		merged["oai-device-id"] = s.fp["oai-device-id"]
	}
	return s.tc.Get(urlStr, merged)
}

func (s *session) postJSON(urlStr string, headers map[string]string, body any, _ time.Duration) (*fhttp.Response, error) {
	merged := make(map[string]string)
	for k, v := range headers {
		merged[k] = v
	}
	if merged["origin"] == "" {
		merged["origin"] = baseURL
	}
	if merged["referer"] == "" {
		merged["referer"] = baseURL + "/"
	}
	if merged["oai-device-id"] == "" && s.fp["oai-device-id"] != "" {
		merged["oai-device-id"] = s.fp["oai-device-id"]
	}
	return s.tc.PostJSON(urlStr, merged, body)
}

func (s *session) putData(urlStr string, headers map[string]string, data []byte, _ time.Duration) (*fhttp.Response, error) {
	return s.tc.PutData(urlStr, headers, data)
}

func retry(fn func() (*fhttp.Response, error), retries int, delay time.Duration, retryOnStatus ...int) (*fhttp.Response, error) {
	statusSet := make(map[int]bool)
	for _, s := range retryOnStatus {
		statusSet[s] = true
	}

	var lastErr error
	var lastResp *fhttp.Response
	for attempt := 0; attempt < retries; attempt++ {
		resp, err := fn()
		if err != nil {
			lastErr = err
			time.Sleep(delay)
			continue
		}
		if len(statusSet) > 0 && statusSet[resp.StatusCode] {
			lastResp = resp
			time.Sleep(delay * time.Duration(attempt+1))
			continue
		}
		return resp, nil
	}
	if lastResp != nil {
		return lastResp, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, &ImageGenerationError{Message: "request failed"}
}

func bootstrap(s *session) string {
	resp, err := retry(func() (*fhttp.Response, error) {
		return s.get(baseURL+"/", nil, 30*time.Second)
	}, 4, 2*time.Second)
	if err == nil {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		GetDataBuildFromHTML(string(body))

		if did := s.tc.GetCookieValue(baseURL, "oai-did"); did != "" {
			return did
		}
	}
	return s.fp["oai-device-id"]
}

func chatRequirements(s *session, accessToken, deviceID string) (string, map[string]any, error) {
	config := GetPowConfig(userAgentStr)
	reqToken := GetRequirementsToken(config)

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", accessToken),
		"oai-device-id": deviceID,
		"content-type":  "application/json",
	}

	resp, err := retry(func() (*fhttp.Response, error) {
		return s.postJSON(baseURL+"/backend-api/sentinel/chat-requirements", headers, map[string]any{
			"p": reqToken,
		}, 30*time.Second)
	}, 4, 2*time.Second)
	if err != nil {
		return "", nil, &ImageGenerationError{Message: fmt.Sprintf("chat-requirements request failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		msg := string(body)
		if len(msg) > 400 {
			msg = msg[:400]
		}
		if msg == "" {
			msg = fmt.Sprintf("chat-requirements failed: %d", resp.StatusCode)
		}
		return "", nil, &ImageGenerationError{Message: msg}
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", nil, &ImageGenerationError{Message: "failed to parse chat-requirements response"}
	}

	token, _ := payload["token"].(string)
	powInfo, _ := payload["proofofwork"].(map[string]any)
	if powInfo == nil {
		powInfo = map[string]any{}
	}
	return token, powInfo, nil
}

func IsTokenInvalidError(message string) bool {
	text := strings.ToLower(message)
	return strings.Contains(text, "token_invalidated") ||
		strings.Contains(text, "token_revoked") ||
		strings.Contains(text, "authentication token has been invalidated") ||
		strings.Contains(text, "invalidated oauth token") ||
		strings.Contains(text, "token_expired") ||
		strings.Contains(text, "token is expired")
}

func IsImageQuotaExceededError(message string) bool {
	text := strings.ToLower(message)
	return strings.Contains(text, "free plan limit for image generation requests") ||
		(strings.Contains(text, "image generation requests") && strings.Contains(text, "limit resets in"))
}

var imageQuotaDurationPattern = regexp.MustCompile(`(?i)(\d+)\s*(day|days|hour|hours|minute|minutes)`)
var conversationIDPattern = regexp.MustCompile(`"conversation_id"\s*:\s*"([^"]+)"`)

func parseImageQuotaDuration(text string) (time.Duration, int) {
	matches := imageQuotaDurationPattern.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return 0, 0
	}

	var duration time.Duration
	count := 0
	for _, match := range matches {
		value, err := strconv.Atoi(match[1])
		if err != nil || value <= 0 {
			continue
		}

		switch strings.ToLower(match[2]) {
		case "day", "days":
			duration += time.Duration(value) * 24 * time.Hour
		case "hour", "hours":
			duration += time.Duration(value) * time.Hour
		case "minute", "minutes":
			duration += time.Duration(value) * time.Minute
		default:
			continue
		}
		count++
	}
	return duration, count
}

func ExtractImageQuotaRestoreAt(message string, now time.Time) *time.Time {
	if !IsImageQuotaExceededError(message) {
		return nil
	}

	lowerMessage := strings.ToLower(message)
	const marker = "limit resets in"

	bestDuration := time.Duration(0)
	bestCount := 0
	searchStart := 0

	for {
		idx := strings.Index(lowerMessage[searchStart:], marker)
		if idx < 0 {
			break
		}

		start := searchStart + idx + len(marker)
		end := len(message)
		if next := strings.Index(lowerMessage[start:], marker); next >= 0 {
			end = start + next
		}

		duration, count := parseImageQuotaDuration(message[start:end])
		if count > bestCount || (count == bestCount && count > 0 && (bestDuration == 0 || duration < bestDuration)) {
			bestDuration = duration
			bestCount = count
		}

		searchStart = start
	}

	if bestCount == 0 || bestDuration <= 0 {
		return nil
	}

	restoreAt := now.Add(bestDuration).UTC().Truncate(time.Second)
	return &restoreAt
}

func extractConversationIDFromPayload(payload string) string {
	match := conversationIDPattern.FindStringSubmatch(payload)
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func uploadImage(s *session, accessToken, deviceID string, imageData []byte, fileName, mimeType string) (string, error) {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", accessToken),
		"oai-device-id": deviceID,
		"content-type":  "application/json",
	}

	resp, err := retry(func() (*fhttp.Response, error) {
		return s.postJSON(baseURL+"/backend-api/files", headers, map[string]any{
			"file_name":           fileName,
			"file_size":           len(imageData),
			"use_case":            "multimodal",
			"timezone_offset_min": -480,
			"reset_rate_limits":   false,
		}, 30*time.Second)
	}, 3, 2*time.Second)
	if err != nil {
		return "", &ImageGenerationError{Message: fmt.Sprintf("file upload init failed: %v", err)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", &ImageGenerationError{Message: fmt.Sprintf("file upload init failed: %d %s", resp.StatusCode, truncate(string(body), 200))}
	}

	var payload map[string]any
	json.NewDecoder(resp.Body).Decode(&payload)
	uploadURL, _ := payload["upload_url"].(string)
	fileID, _ := payload["file_id"].(string)
	if uploadURL == "" || fileID == "" {
		return "", &ImageGenerationError{Message: "file upload init returned no upload_url or file_id"}
	}

	putResp, err := retry(func() (*fhttp.Response, error) {
		return s.putData(uploadURL, map[string]string{
			"Content-Type":   mimeType,
			"x-ms-blob-type": "BlockBlob",
			"x-ms-version":   "2020-04-08",
		}, imageData, 60*time.Second)
	}, 3, 2*time.Second)
	if err != nil {
		return "", &ImageGenerationError{Message: fmt.Sprintf("file upload PUT failed: %v", err)}
	}
	putResp.Body.Close()
	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		return "", &ImageGenerationError{Message: fmt.Sprintf("file upload PUT failed: %d", putResp.StatusCode)}
	}

	processResp, err := retry(func() (*fhttp.Response, error) {
		return s.postJSON(baseURL+"/backend-api/files/process_upload_stream", headers, map[string]any{
			"file_id":             fileID,
			"use_case":            "multimodal",
			"index_for_retrieval": false,
			"file_name":           fileName,
		}, 30*time.Second)
	}, 3, 2*time.Second)
	if err != nil {
		return "", &ImageGenerationError{Message: fmt.Sprintf("file process failed: %v", err)}
	}
	processResp.Body.Close()
	if processResp.StatusCode != 200 {
		return "", &ImageGenerationError{Message: fmt.Sprintf("file process failed: %d", processResp.StatusCode)}
	}
	return fileID, nil
}

func truncate(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

func sendConversation(s *session, accessToken, deviceID, chatToken string, proofToken *string, parentMessageID, prompt, model string) (*fhttp.Response, error) {
	headers := map[string]string{
		"Authorization":           fmt.Sprintf("Bearer %s", accessToken),
		"accept":                  "text/event-stream",
		"accept-language":         "zh-CN,zh;q=0.9,en;q=0.8",
		"content-type":            "application/json",
		"oai-device-id":           deviceID,
		"oai-language":            "zh-CN",
		"oai-client-build-number": "5955942",
		"oai-client-version":      "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad",
		"origin":                  baseURL,
		"referer":                 baseURL + "/",
		"openai-sentinel-chat-requirements-token": chatToken,
	}
	if proofToken != nil {
		headers["openai-sentinel-proof-token"] = *proofToken
	}

	body := map[string]any{
		"action": "next",
		"messages": []any{
			map[string]any{
				"id":     uuid.New().String(),
				"author": map[string]any{"role": "user"},
				"content": map[string]any{
					"content_type": "text",
					"parts":        []any{prompt},
				},
				"metadata": map[string]any{
					"attachments": []any{},
				},
			},
		},
		"parent_message_id":                    parentMessageID,
		"model":                                model,
		"history_and_training_disabled":        false,
		"timezone_offset_min":                  -480,
		"timezone":                             "America/Los_Angeles",
		"conversation_mode":                    map[string]any{"kind": "primary_assistant"},
		"conversation_origin":                  nil,
		"force_paragen":                        false,
		"force_paragen_model_slug":             "",
		"force_rate_limit":                     false,
		"force_use_sse":                        true,
		"paragen_cot_summary_display_override": "allow",
		"paragen_stream_type_override":         nil,
		"reset_rate_limits":                    false,
		"suggestions":                          []any{},
		"supported_encodings":                  []any{},
		"system_hints":                         []any{"picture_v2"},
		"variant_purpose":                      "comparison_implicit",
		"websocket_request_id":                 uuid.New().String(),
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": rand.Intn(450) + 50,
			"page_height":       rand.Intn(500) + 500,
			"page_width":        rand.Intn(1000) + 1000,
			"pixel_ratio":       1.2,
			"screen_height":     rand.Intn(400) + 800,
			"screen_width":      rand.Intn(1000) + 1200,
		},
	}

	resp, err := retry(func() (*fhttp.Response, error) {
		return s.postJSON(baseURL+"/backend-api/conversation", headers, body, 180*time.Second)
	}, 3, 2*time.Second)
	if err != nil {
		return nil, &ImageGenerationError{Message: fmt.Sprintf("conversation request failed: %v", err)}
	}
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		msg := string(respBody)
		if len(msg) > 400 {
			msg = msg[:400]
		}
		if msg == "" {
			msg = fmt.Sprintf("conversation failed: %d", resp.StatusCode)
		}
		return nil, &ImageGenerationError{Message: msg}
	}
	return resp, nil
}

func sendEditConversation(s *session, accessToken, deviceID, chatToken string, proofToken *string, parentMessageID, prompt, model string, images []EditInputImage) (*fhttp.Response, error) {
	headers := map[string]string{
		"Authorization":           fmt.Sprintf("Bearer %s", accessToken),
		"accept":                  "text/event-stream",
		"accept-language":         "zh-CN,zh;q=0.9,en;q=0.8",
		"content-type":            "application/json",
		"oai-device-id":           deviceID,
		"oai-language":            "zh-CN",
		"oai-client-build-number": "5955942",
		"oai-client-version":      "prod-be885abbfcfe7b1f511e88b3003d9ee44757fbad",
		"origin":                  baseURL,
		"referer":                 baseURL + "/",
		"openai-sentinel-chat-requirements-token": chatToken,
	}
	if proofToken != nil {
		headers["openai-sentinel-proof-token"] = *proofToken
	}

	var imageParts []any
	var attachments []any
	for _, img := range images {
		imageParts = append(imageParts, map[string]any{
			"content_type":  "image_asset_pointer",
			"asset_pointer": fmt.Sprintf("sediment://%s", img.FileID),
			"size_bytes":    len(img.Data),
			"width":         img.Width,
			"height":        img.Height,
		})
		attachments = append(attachments, map[string]any{
			"id":           img.FileID,
			"size":         len(img.Data),
			"name":         img.FileName,
			"mime_type":    img.MimeType,
			"width":        img.Width,
			"height":       img.Height,
			"source":       "local",
			"is_big_paste": false,
		})
	}

	parts := append(imageParts, prompt)

	body := map[string]any{
		"action": "next",
		"messages": []any{
			map[string]any{
				"id":     uuid.New().String(),
				"author": map[string]any{"role": "user"},
				"content": map[string]any{
					"content_type": "multimodal_text",
					"parts":        parts,
				},
				"metadata": map[string]any{
					"attachments": attachments,
				},
			},
		},
		"parent_message_id":                    parentMessageID,
		"model":                                model,
		"history_and_training_disabled":        false,
		"timezone_offset_min":                  -480,
		"timezone":                             "America/Los_Angeles",
		"conversation_mode":                    map[string]any{"kind": "primary_assistant"},
		"force_paragen":                        false,
		"force_paragen_model_slug":             "",
		"force_rate_limit":                     false,
		"force_use_sse":                        true,
		"paragen_cot_summary_display_override": "allow",
		"reset_rate_limits":                    false,
		"suggestions":                          []any{},
		"supported_encodings":                  []any{},
		"system_hints":                         []any{"picture_v2"},
		"variant_purpose":                      "comparison_implicit",
		"websocket_request_id":                 uuid.New().String(),
		"client_contextual_info": map[string]any{
			"is_dark_mode":      false,
			"time_since_loaded": rand.Intn(450) + 50,
			"page_height":       rand.Intn(500) + 500,
			"page_width":        rand.Intn(1000) + 1000,
			"pixel_ratio":       1.2,
			"screen_height":     rand.Intn(400) + 800,
			"screen_width":      rand.Intn(1000) + 1200,
		},
	}

	resp, err := retry(func() (*fhttp.Response, error) {
		return s.postJSON(baseURL+"/backend-api/conversation", headers, body, 180*time.Second)
	}, 3, 2*time.Second)
	if err != nil {
		return nil, &ImageGenerationError{Message: fmt.Sprintf("conversation request failed: %v", err)}
	}
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		msg := string(respBody)
		if len(msg) > 400 {
			msg = msg[:400]
		}
		if msg == "" {
			msg = fmt.Sprintf("conversation failed: %d", resp.StatusCode)
		}
		return nil, &ImageGenerationError{Message: msg}
	}
	return resp, nil
}

type sseResult struct {
	ConversationID string
	FileIDs        []string
	Text           string
	Queued         bool
	Rejected       bool
	RejectCode     string
}

func classifyImageText(text string) (bool, bool, string) {
	text = strings.TrimSpace(text)
	rejectCode := detectImageRejectCode(text)
	return isImageQueuedMessage(text), rejectCode != "", rejectCode
}

func messageRole(message map[string]any) string {
	author, _ := message["author"].(map[string]any)
	return strings.ToLower(strings.TrimSpace(fmt.Sprintf("%v", author["role"])))
}

func isImageToolMessage(message map[string]any) bool {
	if message == nil || messageRole(message) != "tool" {
		return false
	}
	metadata, _ := message["metadata"].(map[string]any)
	if metadata == nil || strings.TrimSpace(fmt.Sprintf("%v", metadata["async_task_type"])) != "image_gen" {
		return false
	}
	content, _ := message["content"].(map[string]any)
	return content != nil && strings.TrimSpace(fmt.Sprintf("%v", content["content_type"])) == "multimodal_text"
}

func messageText(message map[string]any) string {
	content, _ := message["content"].(map[string]any)
	if content == nil {
		return ""
	}

	contentType := strings.TrimSpace(fmt.Sprintf("%v", content["content_type"]))
	if contentType != "text" && contentType != "multimodal_text" {
		return ""
	}

	parts, _ := content["parts"].([]any)
	if len(parts) == 0 {
		return ""
	}

	var textParts []string
	for _, part := range parts {
		if s, ok := part.(string); ok && strings.TrimSpace(s) != "" {
			textParts = append(textParts, s)
		}
	}
	return strings.TrimSpace(strings.Join(textParts, ""))
}

func messageTimestamp(message map[string]any) float64 {
	for _, key := range []string{"create_time", "update_time"} {
		switch value := message[key].(type) {
		case float64:
			return value
		case float32:
			return float64(value)
		case int:
			return float64(value)
		case int64:
			return float64(value)
		case json.Number:
			if f, err := value.Float64(); err == nil {
				return f
			}
		case string:
			if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				return f
			}
		}
	}
	return 0
}

func appendMessageFileIDs(message map[string]any, fileIDSet map[string]bool, fileIDs *[]string) {
	content, _ := message["content"].(map[string]any)
	if content == nil {
		return
	}
	parts, _ := content["parts"].([]any)
	for _, part := range parts {
		partMap, ok := part.(map[string]any)
		if !ok {
			continue
		}
		pointer := fmt.Sprintf("%v", partMap["asset_pointer"])
		switch {
		case strings.HasPrefix(pointer, "file-service://"):
			fileID := strings.TrimPrefix(pointer, "file-service://")
			if fileID != "" && !fileIDSet[fileID] {
				fileIDSet[fileID] = true
				*fileIDs = append(*fileIDs, fileID)
			}
		case strings.HasPrefix(pointer, "sediment://"):
			fileID := "sed:" + strings.TrimPrefix(pointer, "sediment://")
			if fileID != "sed:" && !fileIDSet[fileID] {
				fileIDSet[fileID] = true
				*fileIDs = append(*fileIDs, fileID)
			}
		}
	}
}

func isTerminalImageText(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	if IsImageQuotaExceededError(text) {
		return true
	}
	_, rejected, _ := classifyImageText(text)
	return rejected
}

func shouldPreferAssistantText(current string, currentTS float64, candidate string, candidateTS float64) bool {
	current = strings.TrimSpace(current)
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return false
	}
	if current == "" {
		return true
	}

	currentTerminal := isTerminalImageText(current)
	candidateTerminal := isTerminalImageText(candidate)
	if candidateTerminal != currentTerminal {
		return candidateTerminal
	}
	if candidateTS > 0 && currentTS > 0 && candidateTS != currentTS {
		return candidateTS > currentTS
	}
	return len(candidate) >= len(current)
}

func applyImageTextState(result *sseResult, text string) {
	text = strings.TrimSpace(text)
	result.Text = text
	result.Queued, result.Rejected, result.RejectCode = classifyImageText(text)
}

func mergeImageResultState(base, next sseResult) sseResult {
	merged := base
	if next.ConversationID != "" {
		merged.ConversationID = next.ConversationID
	}
	if len(next.FileIDs) > 0 {
		merged.FileIDs = next.FileIDs
	}
	if strings.TrimSpace(next.Text) != "" {
		applyImageTextState(&merged, next.Text)
	}
	return merged
}

func shouldContinuePolling(result sseResult) bool {
	if len(result.FileIDs) > 0 || result.Rejected || IsImageQuotaExceededError(result.Text) {
		return false
	}
	return true
}

func isImageQueuedMessage(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "正在处理图片") ||
		strings.Contains(lower, "目前有很多人在创建图片") ||
		strings.Contains(lower, "图片准备好后") ||
		strings.Contains(lower, "processing your image") ||
		strings.Contains(lower, "lots of people creating images") ||
		strings.Contains(lower, "we'll notify you when") ||
		strings.Contains(lower, "image is taking") ||
		strings.Contains(lower, "high demand")
}

func detectImageRejectCode(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return ""
	}

	contentPolicyPatterns := []string{
		"内容政策",
		"违反了我们的内容政策",
		"违反我们的内容政策",
		"防护限制",
		"如果你认为此判断有误，请重试或修改提示语",
		"裸露、色情或情色内容",
		"content policy",
		"violates our content policy",
		"may violate our content policy",
		"policy violation",
	}
	for _, pattern := range contentPolicyPatterns {
		if strings.Contains(lower, pattern) {
			return "content_policy_violation"
		}
	}

	rejectedPatterns := []string{
		"不能按原要求生成",
		"不能帮助生成",
		"我不能按原要求",
		"我不能生成",
		"无法按原要求生成",
		"sorry",
		"i can't generate",
		"i cannot generate",
		"i can't help with that",
		"i cannot help with that",
		"cannot comply",
		"can't comply",
		"not able to generate",
	}
	for _, pattern := range rejectedPatterns {
		if strings.Contains(lower, pattern) {
			return "image_generation_rejected"
		}
	}

	return ""
}

func parseSSE(resp *fhttp.Response) sseResult {
	defer resp.Body.Close()

	result := sseResult{}
	var fileIDs []string
	var assistantText string
	var assistantTextTS float64
	fileIDSet := make(map[string]bool)

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(line[5:])
		if payload == "" || payload == "[DONE]" {
			break
		}
		if cid := extractConversationIDFromPayload(payload); cid != "" {
			result.ConversationID = cid
		}

		var obj map[string]any
		if err := json.Unmarshal([]byte(payload), &obj); err != nil {
			continue
		}

		if cid, ok := obj["conversation_id"].(string); ok && cid != "" {
			result.ConversationID = cid
		}

		objType, _ := obj["type"].(string)
		if objType == "resume_conversation_token" || objType == "message_marker" || objType == "message_stream_complete" {
			if cid, ok := obj["conversation_id"].(string); ok && cid != "" {
				result.ConversationID = cid
			}
		}

		if data, ok := obj["v"].(map[string]any); ok {
			if cid, ok := data["conversation_id"].(string); ok && cid != "" {
				result.ConversationID = cid
			}
		}

		for _, candidate := range []any{obj["message"], obj["v"]} {
			message, ok := candidate.(map[string]any)
			if !ok {
				continue
			}
			if inner, ok := message["message"].(map[string]any); ok {
				message = inner
			}
			if messageRole(message) != "assistant" {
				continue
			}
			text := messageText(message)
			timestamp := messageTimestamp(message)
			if shouldPreferAssistantText(assistantText, assistantTextTS, text, timestamp) {
				assistantText = text
				assistantTextTS = timestamp
			}
		}

		for _, candidate := range []any{obj["message"], obj["v"]} {
			message, ok := candidate.(map[string]any)
			if !ok {
				continue
			}
			if inner, ok := message["message"].(map[string]any); ok {
				message = inner
			}
			if isImageToolMessage(message) {
				appendMessageFileIDs(message, fileIDSet, &fileIDs)
			}
		}
	}

	result.FileIDs = fileIDs
	applyImageTextState(&result, assistantText)
	return result
}

func extractImageIDs(mapping map[string]any) []string {
	var fileIDs []string
	fileIDSet := make(map[string]bool)

	for _, node := range mapping {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			continue
		}
		message, _ := nodeMap["message"].(map[string]any)
		if message == nil {
			continue
		}
		if !isImageToolMessage(message) {
			continue
		}
		appendMessageFileIDs(message, fileIDSet, &fileIDs)
	}
	return fileIDs
}

func extractConversationState(mapping map[string]any) sseResult {
	result := sseResult{
		FileIDs: extractImageIDs(mapping),
	}
	var assistantText string
	var assistantTextTS float64

	for _, node := range mapping {
		nodeMap, ok := node.(map[string]any)
		if !ok {
			continue
		}
		message, _ := nodeMap["message"].(map[string]any)
		if message == nil || messageRole(message) != "assistant" {
			continue
		}
		text := messageText(message)
		timestamp := messageTimestamp(message)
		if shouldPreferAssistantText(assistantText, assistantTextTS, text, timestamp) {
			assistantText = text
			assistantTextTS = timestamp
		}
	}

	applyImageTextState(&result, assistantText)
	return result
}

func pollImageIDs(s *session, accessToken, deviceID, conversationID string, timeout ...time.Duration) sseResult {
	maxWait := time.Duration(config.GetImagePollTimeoutSecs()) * time.Second
	if len(timeout) > 0 && timeout[0] > 0 {
		maxWait = timeout[0]
	}
	tokenPrefix := accessToken[:min(12, len(accessToken))]
	started := time.Now()
	lastResult := sseResult{ConversationID: conversationID}
	for time.Since(started) < maxWait {
		resp, err := retry(func() (*fhttp.Response, error) {
			return s.get(
				fmt.Sprintf("%s/backend-api/conversation/%s", baseURL, conversationID),
				map[string]string{
					"Authorization": fmt.Sprintf("Bearer %s", accessToken),
					"oai-device-id": deviceID,
					"accept":        "*/*",
				},
				30*time.Second,
			)
		}, 2, 2*time.Second, 429, 502, 503, 504)
		if err != nil || resp.StatusCode != 200 {
			if resp != nil {
				resp.Body.Close()
			}
			time.Sleep(3 * time.Second)
			continue
		}

		var payload map[string]any
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err := json.Unmarshal(body, &payload); err != nil {
			time.Sleep(3 * time.Second)
			continue
		}

		mapping, _ := payload["mapping"].(map[string]any)
		if mapping == nil {
			time.Sleep(3 * time.Second)
			continue
		}

		state := extractConversationState(mapping)
		state.ConversationID = conversationID
		lastResult = mergeImageResultState(lastResult, state)
		if len(state.FileIDs) > 0 {
			elapsed := time.Since(started).Truncate(time.Second)
			fmt.Printf("[image-poll] success token=%s... conversation=%s elapsed=%s\n", tokenPrefix, conversationID, elapsed)
			return lastResult
		}
		if strings.TrimSpace(state.Text) != "" && !shouldContinuePolling(state) {
			elapsed := time.Since(started).Truncate(time.Second)
			fmt.Printf("[image-poll] terminal token=%s... conversation=%s elapsed=%s text=%s\n",
				tokenPrefix, conversationID, elapsed, truncate(state.Text, 200))
			return lastResult
		}
		elapsed := time.Since(started).Truncate(time.Second)
		if elapsed.Seconds() > 0 && int(elapsed.Seconds())%30 == 0 {
			fmt.Printf("[image-poll] waiting token=%s... conversation=%s elapsed=%s/%s\n", tokenPrefix, conversationID, elapsed, maxWait)
		}
		time.Sleep(5 * time.Second)
	}
	fmt.Printf("[image-poll] timeout token=%s... conversation=%s timeout=%s\n", tokenPrefix, conversationID, maxWait)
	return lastResult
}

func canonicalizeFileID(fileID string) string {
	if strings.HasPrefix(fileID, "sed:") {
		return fileID[4:]
	}
	return fileID
}

func filterOutputFileIDs(fileIDs []string, inputFileIDs map[string]bool) []string {
	canonicalInput := make(map[string]bool)
	for id := range inputFileIDs {
		canonicalInput[canonicalizeFileID(id)] = true
	}
	var result []string
	for _, id := range fileIDs {
		if !canonicalInput[canonicalizeFileID(id)] {
			result = append(result, id)
		}
	}
	return result
}

func fetchDownloadURL(s *session, accessToken, deviceID, conversationID, fileID string) string {
	isSediment := strings.HasPrefix(fileID, "sed:")
	rawID := fileID
	if isSediment {
		rawID = fileID[4:]
	}

	var endpoint string
	if isSediment {
		endpoint = fmt.Sprintf("%s/backend-api/conversation/%s/attachment/%s/download", baseURL, conversationID, rawID)
	} else {
		endpoint = fmt.Sprintf("%s/backend-api/files/download/%s?conversation_id=%s", baseURL, rawID, conversationID)
	}

	tokenPrefix := accessToken[:min(12, len(accessToken))]
	fmt.Printf("[image-download] fetchDownloadURL endpoint=%s token=%s...\n", endpoint, tokenPrefix)

	resp, err := s.get(endpoint, map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", accessToken),
		"oai-device-id": deviceID,
	}, 30*time.Second)
	if err != nil || resp.StatusCode != 200 {
		status := 0
		if resp != nil {
			status = resp.StatusCode
			resp.Body.Close()
		}
		fmt.Printf("[image-download] fetchDownloadURL failed status=%d token=%s...\n", status, tokenPrefix)
		return ""
	}
	defer resp.Body.Close()

	var payload map[string]any
	json.NewDecoder(resp.Body).Decode(&payload)
	downloadURL, _ := payload["download_url"].(string)
	fmt.Printf("[image-download] fetchDownloadURL ok download_url=%s\n", truncateStr(downloadURL, 120))
	return downloadURL
}

func downloadAsBase64(s *session, accessToken, downloadURL string) (string, error) {
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", accessToken),
	}
	resp, err := s.get(downloadURL, headers, 60*time.Second)
	if err != nil {
		return "", &ImageGenerationError{Message: fmt.Sprintf("download output image failed: %v url=%s", err, downloadURL)}
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", &ImageGenerationError{Message: fmt.Sprintf("download output image failed: status %d url=%s", resp.StatusCode, downloadURL)}
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return "", &ImageGenerationError{Message: fmt.Sprintf("download output image failed: empty body url=%s", downloadURL)}
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func resolveUpstreamModel(accountService *AccountService, accessToken, requestedModel string) string {
	requestedModel = strings.TrimSpace(requestedModel)
	if requestedModel == "" {
		requestedModel = "gpt-image-1"
	}

	account := accountService.GetAccount(accessToken)
	if account == nil {
		account = map[string]any{}
	}
	isFreeAccount := strings.TrimSpace(fmt.Sprintf("%v", account["type"])) == "Free"

	switch requestedModel {
	case "gpt-image-1":
		return "auto"
	case "gpt-image-2":
		if isFreeAccount {
			return "auto"
		}
		return "gpt-5-3"
	default:
		m := strings.TrimSpace(requestedModel)
		if m == "" {
			return defaultModel
		}
		return m
	}
}

func GenerateImageResult(accountService *AccountService, accessToken, prompt, model string) (map[string]any, error) {
	prompt = strings.TrimSpace(prompt)
	accessToken = strings.TrimSpace(accessToken)
	if prompt == "" {
		return nil, &ImageGenerationError{Message: "prompt is required"}
	}
	if accessToken == "" {
		return nil, &ImageGenerationError{Message: "token is required"}
	}

	s := newSession(accountService, accessToken)
	upstreamModel := resolveUpstreamModel(accountService, accessToken, model)
	tokenPrefix := accessToken[:min(12, len(accessToken))]
	fmt.Printf("[image-upstream] start token=%s... requested_model=%s upstream_model=%s\n",
		tokenPrefix, model, upstreamModel)

	deviceID := bootstrap(s)
	chatToken, powInfo, err := chatRequirements(s, accessToken, deviceID)
	if err != nil {
		fmt.Printf("[image-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
		return nil, err
	}

	var proofToken *string
	if required, ok := powInfo["required"].(bool); ok && required {
		pt := GenerateProofToken(
			fmt.Sprintf("%v", powInfo["seed"]),
			fmt.Sprintf("%v", powInfo["difficulty"]),
			userAgentStr,
			GetPowConfig(userAgentStr),
		)
		proofToken = &pt
	}

	parentMessageID := uuid.New().String()
	resp, err := sendConversation(s, accessToken, deviceID, chatToken, proofToken, parentMessageID, prompt, upstreamModel)
	if err != nil {
		fmt.Printf("[image-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
		return nil, err
	}

	state := parseSSE(resp)
	waitedForResult := false
	waitedWhileQueued := state.Queued
	if state.ConversationID != "" && shouldContinuePolling(state) {
		waitedForResult = true
		pollTimeout := time.Duration(config.GetImagePollTimeoutSecs()) * time.Second
		if state.Queued {
			fmt.Printf("[image-upstream] queued token=%s... conversation=%s text=%s timeout=%s\n",
				tokenPrefix, state.ConversationID, truncate(state.Text, 100), pollTimeout)
		}
		state = mergeImageResultState(state, pollImageIDs(s, accessToken, deviceID, state.ConversationID, pollTimeout))
		waitedWhileQueued = waitedWhileQueued || state.Queued
	}

	fileIDs := state.FileIDs
	responseText := strings.TrimSpace(state.Text)
	if len(fileIDs) == 0 {
		meta := buildImageErrorMeta(state, waitedForResult, waitedWhileQueued)
		if responseText != "" {
			if state.Rejected {
				fmt.Printf("[image-upstream] rejected token=%s... code=%s error=%s\n",
					tokenPrefix, state.RejectCode, truncate(responseText, 200))
				return nil, &ImageGenerationError{
					Message:    responseText,
					StatusCode: 400,
					ErrorType:  "invalid_request_error",
					Code:       state.RejectCode,
					Reason:     state.RejectCode,
					Meta:       meta,
				}
			}
			if IsImageQuotaExceededError(responseText) {
				fmt.Printf("[image-upstream] limited token=%s... error=%s\n", tokenPrefix, truncate(responseText, 200))
				return nil, &ImageGenerationError{Message: responseText, Reason: "quota_exceeded", Meta: meta}
			}
			if waitedForResult {
				if waitedWhileQueued {
					fmt.Printf("[image-upstream] queue-timeout token=%s... error=image generation timed out while queued\n", tokenPrefix)
					return nil, &ImageGenerationError{Message: "image generation timed out while queued: " + responseText, Reason: "timed_out_while_queued", Meta: meta}
				}
				fmt.Printf("[image-upstream] wait-timeout token=%s... error=image generation timed out while waiting\n", tokenPrefix)
				return nil, &ImageGenerationError{Message: "image generation timed out while waiting: " + responseText, Reason: "timed_out_while_waiting", Meta: meta}
			}
			if strings.TrimSpace(state.ConversationID) == "" {
				fmt.Printf("[image-upstream] fail token=%s... error=missing conversation id\n", tokenPrefix)
				return nil, &ImageGenerationError{Message: responseText, Reason: "missing_conversation_id", Meta: meta}
			}
			fmt.Printf("[image-upstream] fail token=%s... error=%s\n", tokenPrefix, responseText)
			return nil, &ImageGenerationError{Message: responseText, Reason: "upstream_text_response", Meta: meta}
		}
		if waitedForResult {
			if waitedWhileQueued {
				fmt.Printf("[image-upstream] queue-timeout token=%s... error=image generation timed out while queued\n", tokenPrefix)
				return nil, &ImageGenerationError{Message: "image generation timed out while queued", Reason: "timed_out_while_queued", Meta: meta}
			}
			fmt.Printf("[image-upstream] wait-timeout token=%s... error=image generation timed out while waiting\n", tokenPrefix)
			return nil, &ImageGenerationError{Message: "image generation timed out while waiting", Reason: "timed_out_while_waiting", Meta: meta}
		}
		if strings.TrimSpace(state.ConversationID) == "" {
			fmt.Printf("[image-upstream] fail token=%s... error=missing conversation id\n", tokenPrefix)
			return nil, &ImageGenerationError{Message: "no image returned from upstream", Reason: "missing_conversation_id", Meta: meta}
		}
		fmt.Printf("[image-upstream] fail token=%s... error=no image returned from upstream\n", tokenPrefix)
		return nil, &ImageGenerationError{Message: "no image returned from upstream", Reason: "no_image_returned", Meta: meta}
	}

	downloadURL := fetchDownloadURL(s, accessToken, deviceID, state.ConversationID, fileIDs[0])
	if downloadURL == "" {
		fmt.Printf("[image-upstream] fail token=%s... error=failed to get download url\n", tokenPrefix)
		return nil, &ImageGenerationError{
			Message: "failed to get download url",
			Reason:  "download_url_missing",
			Meta: map[string]any{
				"upstream_conversation_id": state.ConversationID,
				"upstream_file_count":      len(fileIDs),
			},
		}
	}

	b64, err := downloadAsBase64(s, accessToken, downloadURL)
	if err != nil {
		fmt.Printf("[image-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
		return nil, err
	}

	if state.Queued {
		fmt.Printf("[image-upstream] success token=%s... images=1 (was queued)\n", tokenPrefix)
	} else {
		fmt.Printf("[image-upstream] success token=%s... images=1\n", tokenPrefix)
	}
	return map[string]any{
		"created": time.Now().Unix(),
		"data": []any{
			map[string]any{
				"b64_json":       b64,
				"revised_prompt": prompt,
			},
		},
	}, nil
}

func getImageDimensions(imageData []byte) (int, int) {
	if len(imageData) >= 24 && bytes.Equal(imageData[:8], []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) {
		w := int(binary.BigEndian.Uint32(imageData[16:20]))
		h := int(binary.BigEndian.Uint32(imageData[20:24]))
		return w, h
	}

	if len(imageData) >= 2 && imageData[0] == 0xFF && imageData[1] == 0xD8 {
		r := bytes.NewReader(imageData[2:])
		for {
			var marker [2]byte
			if _, err := io.ReadFull(r, marker[:]); err != nil {
				break
			}
			if marker[0] != 0xFF {
				break
			}
			if marker[1] == 0xC0 || marker[1] == 0xC1 || marker[1] == 0xC2 {
				skip := make([]byte, 3)
				r.Read(skip)
				var h, w uint16
				binary.Read(r, binary.BigEndian, &h)
				binary.Read(r, binary.BigEndian, &w)
				return int(w), int(h)
			}
			var length uint16
			if err := binary.Read(r, binary.BigEndian, &length); err != nil {
				break
			}
			if length < 2 {
				break
			}
			r.Seek(int64(length-2), io.SeekCurrent)
		}
	}

	return 1024, 1024
}

func EditImageResult(accountService *AccountService, accessToken, prompt string, images []RequestImage, model string) (map[string]any, error) {
	prompt = strings.TrimSpace(prompt)
	accessToken = strings.TrimSpace(accessToken)
	if prompt == "" {
		return nil, &ImageGenerationError{Message: "prompt is required"}
	}
	if accessToken == "" {
		return nil, &ImageGenerationError{Message: "token is required"}
	}
	if len(images) == 0 {
		return nil, &ImageGenerationError{Message: "image is required"}
	}

	s := newSession(accountService, accessToken)
	upstreamModel := resolveUpstreamModel(accountService, accessToken, model)
	tokenPrefix := accessToken[:min(12, len(accessToken))]
	fmt.Printf("[image-edit-upstream] start token=%s... requested_model=%s upstream_model=%s images=%d\n",
		tokenPrefix, model, upstreamModel, len(images))

	deviceID := bootstrap(s)

	var uploadedImages []EditInputImage
	for _, img := range images {
		if len(img.Data) == 0 {
			return nil, &ImageGenerationError{Message: "image is required"}
		}
		fileID, err := uploadImage(s, accessToken, deviceID, img.Data, img.FileName, img.MimeType)
		if err != nil {
			fmt.Printf("[image-edit-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
			return nil, err
		}
		fmt.Printf("[image-edit-upstream] uploaded file_id=%s\n", fileID)
		w, h := getImageDimensions(img.Data)
		uploadedImages = append(uploadedImages, EditInputImage{
			FileID:   fileID,
			Data:     img.Data,
			FileName: img.FileName,
			MimeType: img.MimeType,
			Width:    w,
			Height:   h,
		})
	}

	chatToken, powInfo, err := chatRequirements(s, accessToken, deviceID)
	if err != nil {
		fmt.Printf("[image-edit-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
		return nil, err
	}

	var proofToken *string
	if required, ok := powInfo["required"].(bool); ok && required {
		pt := GenerateProofToken(
			fmt.Sprintf("%v", powInfo["seed"]),
			fmt.Sprintf("%v", powInfo["difficulty"]),
			userAgentStr,
			GetPowConfig(userAgentStr),
		)
		proofToken = &pt
	}

	parentMessageID := uuid.New().String()
	resp, err := sendEditConversation(s, accessToken, deviceID, chatToken, proofToken, parentMessageID, prompt, upstreamModel, uploadedImages)
	if err != nil {
		fmt.Printf("[image-edit-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
		return nil, err
	}

	inputFileIDs := make(map[string]bool)
	for _, img := range uploadedImages {
		inputFileIDs[img.FileID] = true
	}

	state := parseSSE(resp)
	waitedForResult := false
	waitedWhileQueued := state.Queued
	if state.ConversationID != "" && shouldContinuePolling(state) {
		waitedForResult = true
		pollTimeout := time.Duration(config.GetImagePollTimeoutSecs()) * time.Second
		if state.Queued {
			fmt.Printf("[image-edit-upstream] queued token=%s... conversation=%s text=%s timeout=%s\n",
				tokenPrefix, state.ConversationID, truncate(state.Text, 100), pollTimeout)
		}
		state = mergeImageResultState(state, pollImageIDs(s, accessToken, deviceID, state.ConversationID, pollTimeout))
		waitedWhileQueued = waitedWhileQueued || state.Queued
	}

	fileIDs := filterOutputFileIDs(state.FileIDs, inputFileIDs)
	responseText := strings.TrimSpace(state.Text)
	if len(fileIDs) == 0 {
		meta := buildImageErrorMeta(state, waitedForResult, waitedWhileQueued)
		if responseText != "" {
			if state.Rejected {
				fmt.Printf("[image-edit-upstream] rejected token=%s... code=%s error=%s\n",
					tokenPrefix, state.RejectCode, truncate(responseText, 200))
				return nil, &ImageGenerationError{
					Message:    responseText,
					StatusCode: 400,
					ErrorType:  "invalid_request_error",
					Code:       state.RejectCode,
					Reason:     state.RejectCode,
					Meta:       meta,
				}
			}
			if IsImageQuotaExceededError(responseText) {
				fmt.Printf("[image-edit-upstream] limited token=%s... error=%s\n", tokenPrefix, truncate(responseText, 200))
				return nil, &ImageGenerationError{Message: responseText, Reason: "quota_exceeded", Meta: meta}
			}
			if waitedForResult {
				if waitedWhileQueued {
					fmt.Printf("[image-edit-upstream] queue-timeout token=%s... error=image generation timed out while queued\n", tokenPrefix)
					return nil, &ImageGenerationError{Message: "image generation timed out while queued: " + responseText, Reason: "timed_out_while_queued", Meta: meta}
				}
				fmt.Printf("[image-edit-upstream] wait-timeout token=%s... error=image generation timed out while waiting\n", tokenPrefix)
				return nil, &ImageGenerationError{Message: "image generation timed out while waiting: " + responseText, Reason: "timed_out_while_waiting", Meta: meta}
			}
			if strings.TrimSpace(state.ConversationID) == "" {
				fmt.Printf("[image-edit-upstream] fail token=%s... error=missing conversation id\n", tokenPrefix)
				return nil, &ImageGenerationError{Message: responseText, Reason: "missing_conversation_id", Meta: meta}
			}
			fmt.Printf("[image-edit-upstream] fail token=%s... error=%s\n", tokenPrefix, responseText)
			return nil, &ImageGenerationError{Message: responseText, Reason: "upstream_text_response", Meta: meta}
		}
		if waitedForResult {
			if waitedWhileQueued {
				fmt.Printf("[image-edit-upstream] queue-timeout token=%s... error=image generation timed out while queued\n", tokenPrefix)
				return nil, &ImageGenerationError{Message: "image generation timed out while queued", Reason: "timed_out_while_queued", Meta: meta}
			}
			fmt.Printf("[image-edit-upstream] wait-timeout token=%s... error=image generation timed out while waiting\n", tokenPrefix)
			return nil, &ImageGenerationError{Message: "image generation timed out while waiting", Reason: "timed_out_while_waiting", Meta: meta}
		}
		if strings.TrimSpace(state.ConversationID) == "" {
			fmt.Printf("[image-edit-upstream] fail token=%s... error=missing conversation id\n", tokenPrefix)
			return nil, &ImageGenerationError{Message: "no image returned from upstream", Reason: "missing_conversation_id", Meta: meta}
		}
		fmt.Printf("[image-edit-upstream] fail token=%s... error=no image returned from upstream\n", tokenPrefix)
		return nil, &ImageGenerationError{Message: "no image returned from upstream", Reason: "no_image_returned", Meta: meta}
	}

	downloadURL := fetchDownloadURL(s, accessToken, deviceID, state.ConversationID, fileIDs[0])
	if downloadURL == "" {
		fmt.Printf("[image-edit-upstream] fail token=%s... error=failed to get download url\n", tokenPrefix)
		return nil, &ImageGenerationError{
			Message: "failed to get download url",
			Reason:  "download_url_missing",
			Meta: map[string]any{
				"upstream_conversation_id": state.ConversationID,
				"upstream_file_count":      len(fileIDs),
			},
		}
	}

	b64, err := downloadAsBase64(s, accessToken, downloadURL)
	if err != nil {
		fmt.Printf("[image-edit-upstream] fail token=%s... error=%v\n", tokenPrefix, err)
		return nil, err
	}

	if state.Queued {
		fmt.Printf("[image-edit-upstream] success token=%s... inputs=%d (was queued)\n", tokenPrefix, len(uploadedImages))
	} else {
		fmt.Printf("[image-edit-upstream] success token=%s... inputs=%d\n", tokenPrefix, len(uploadedImages))
	}
	return map[string]any{
		"created": time.Now().Unix(),
		"data": []any{
			map[string]any{
				"b64_json":       b64,
				"revised_prompt": prompt,
			},
		},
	}, nil
}

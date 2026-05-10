package services

import (
	"io"
	"strings"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

func TestGetImageDimensionsPNG(t *testing.T) {
	// Minimal 1x1 PNG header
	pngHeader := []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
		0x00, 0x00, 0x00, 0x0D, // IHDR chunk length
		0x49, 0x48, 0x44, 0x52, // IHDR
		0x00, 0x00, 0x02, 0x00, // width = 512
		0x00, 0x00, 0x01, 0x00, // height = 256
	}
	w, h := getImageDimensions(pngHeader)
	if w != 512 || h != 256 {
		t.Errorf("PNG dimensions = %dx%d, want 512x256", w, h)
	}
}

func TestGetImageDimensionsUnknown(t *testing.T) {
	w, h := getImageDimensions([]byte{0x00, 0x01, 0x02})
	if w != 1024 || h != 1024 {
		t.Errorf("Unknown dimensions = %dx%d, want 1024x1024 (default)", w, h)
	}
}

func TestIsTokenInvalidError(t *testing.T) {
	tests := []struct {
		message  string
		expected bool
	}{
		{"token_invalidated", true},
		{"token_revoked", true},
		{"authentication token has been invalidated", true},
		{"invalidated oauth token", true},
		{"Token_Invalidated", true},
		{"some other error", false},
		{"rate limit exceeded", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsTokenInvalidError(tt.message)
		if result != tt.expected {
			t.Errorf("IsTokenInvalidError(%q) = %v, want %v", tt.message, result, tt.expected)
		}
	}
}

func TestIsImageQuotaExceededError(t *testing.T) {
	tests := []struct {
		message  string
		expected bool
	}{
		{"You've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25 minutes.", true},
		{"IMAGE GENERATION REQUESTS can create more images when the limit resets in 5 minutes.", true},
		{"rate limit exceeded", false},
		{"", false},
	}

	for _, tt := range tests {
		result := IsImageQuotaExceededError(tt.message)
		if result != tt.expected {
			t.Errorf("IsImageQuotaExceededError(%q) = %v, want %v", tt.message, result, tt.expected)
		}
	}
}

func TestExtractImageQuotaRestoreAt(t *testing.T) {
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	message := "You've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25 minutes."

	restoreAt := ExtractImageQuotaRestoreAt(message, now)
	if restoreAt == nil {
		t.Fatal("ExtractImageQuotaRestoreAt returned nil")
	}

	expected := now.Add(10*time.Hour + 25*time.Minute)
	if !restoreAt.Equal(expected) {
		t.Errorf("restoreAt = %v, want %v", restoreAt, expected)
	}
}

func TestExtractImageQuotaRestoreAtRepeatedText(t *testing.T) {
	now := time.Date(2026, 4, 23, 10, 0, 0, 0, time.UTC)
	message := "You've hit theYou've hit the free plan limit for image generation requests. You can create moreYou've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25You've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25 minutes."

	restoreAt := ExtractImageQuotaRestoreAt(message, now)
	if restoreAt == nil {
		t.Fatal("ExtractImageQuotaRestoreAt returned nil for repeated text")
	}

	expected := now.Add(10*time.Hour + 25*time.Minute)
	if !restoreAt.Equal(expected) {
		t.Errorf("restoreAt = %v, want %v", restoreAt, expected)
	}
}

func TestCanonicalizeFileID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"sed:abc123", "abc123"},
		{"abc123", "abc123"},
		{"sed:", ""},
		{"", ""},
	}
	for _, tt := range tests {
		result := canonicalizeFileID(tt.input)
		if result != tt.expected {
			t.Errorf("canonicalizeFileID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFilterOutputFileIDs(t *testing.T) {
	inputIDs := map[string]bool{
		"file_input_1": true,
		"file_input_2": true,
	}
	allIDs := []string{"sed:file_input_1", "file_output_1", "file_input_2", "file_output_2"}
	result := filterOutputFileIDs(allIDs, inputIDs)
	if len(result) != 2 {
		t.Errorf("len(result) = %d, want 2", len(result))
	}
	if result[0] != "file_output_1" {
		t.Errorf("result[0] = %q, want file_output_1", result[0])
	}
	if result[1] != "file_output_2" {
		t.Errorf("result[1] = %q, want file_output_2", result[1])
	}
}

func TestTruncate(t *testing.T) {
	if truncate("hello world", 5) != "hello" {
		t.Errorf("truncate should truncate to 5 chars")
	}
	if truncate("hi", 10) != "hi" {
		t.Errorf("truncate should not change shorter strings")
	}
}

func TestDetectImageRejectCode(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"非常抱歉，该提示可能违反了我们的内容政策。", "content_policy_violation"},
		{"Sorry, this may violate our content policy.", "content_policy_violation"},
		{"你的描述里涉及了这种元素，所以我不能按原要求生成。", "image_generation_rejected"},
		{"I can't generate that as requested.", "image_generation_rejected"},
		{"正在处理图片，目前有很多人在创建图片。", ""},
	}

	for _, tt := range tests {
		if got := detectImageRejectCode(tt.text); got != tt.want {
			t.Errorf("detectImageRejectCode(%q) = %q, want %q", tt.text, got, tt.want)
		}
	}
}

func TestParseSSEReturnsRejectedFlag(t *testing.T) {
	body := "data: {\"conversation_id\":\"conv_1\",\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"content_type\":\"text\",\"parts\":[\"非常抱歉，该提示可能违反了我们的内容政策。\"]}}}\n\n"
	resp := &fhttp.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	parsed := parseSSE(resp)

	if !parsed.Rejected {
		t.Fatal("expected rejected result")
	}
	if parsed.RejectCode != "content_policy_violation" {
		t.Fatalf("RejectCode = %q, want content_policy_violation", parsed.RejectCode)
	}
	if !strings.Contains(parsed.Text, "内容政策") {
		t.Fatalf("Text = %q, want policy message", parsed.Text)
	}
}

func TestParseSSEPrefersLatestAssistantTerminalText(t *testing.T) {
	body := strings.Join([]string{
		"data: {\"conversation_id\":\"conv_1\",\"message\":{\"author\":{\"role\":\"user\"},\"content\":{\"content_type\":\"text\",\"parts\":[\"{\\\"prompt\\\":{\\\"prompt\\\":\\\"母亲节海报\\\"}}\"]}}}",
		"data: {\"conversation_id\":\"conv_1\",\"message\":{\"author\":{\"role\":\"assistant\"},\"create_time\":1,\"content\":{\"content_type\":\"text\",\"parts\":[\"正在处理图片，目前有很多人在创建图片。\"]}}}",
		"data: {\"conversation_id\":\"conv_1\",\"message\":{\"author\":{\"role\":\"assistant\"},\"create_time\":2,\"content\":{\"content_type\":\"text\",\"parts\":[\"You've hit the free plan limit for image generations requests. You can create more images when the limit resets in 1 hour and 7 minutes.\"]}}}",
		"",
	}, "\n\n")
	resp := &fhttp.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	parsed := parseSSE(resp)
	if strings.Contains(parsed.Text, "\"prompt\"") {
		t.Fatalf("Text should not contain echoed prompt JSON, got %q", parsed.Text)
	}
	if !strings.Contains(parsed.Text, "free plan limit") {
		t.Fatalf("Text = %q, want quota message", parsed.Text)
	}
	if parsed.Queued {
		t.Fatalf("Queued = true, want false for terminal assistant text")
	}
}

func TestParseSSEIgnoresUserAttachmentPointers(t *testing.T) {
	body := strings.Join([]string{
		"data: {\"conversation_id\":\"conv_1\",\"message\":{\"author\":{\"role\":\"user\"},\"content\":{\"content_type\":\"multimodal_text\",\"parts\":[{\"content_type\":\"image_asset_pointer\",\"asset_pointer\":\"sediment://input_file_1\"},\"请基于这张图生成海报\"]}}}",
		"data: {\"conversation_id\":\"conv_1\",\"message\":{\"author\":{\"role\":\"assistant\"},\"content\":{\"content_type\":\"text\",\"parts\":[\"{\\\"prompt\\\":\\\"海报\\\",\\\"size\\\":\\\"1024x1024\\\"}\"]}}}",
		"",
	}, "\n\n")
	resp := &fhttp.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	parsed := parseSSE(resp)
	if len(parsed.FileIDs) != 0 {
		t.Fatalf("FileIDs = %v, want no output file ids from user attachment pointers", parsed.FileIDs)
	}
	if parsed.ConversationID != "conv_1" {
		t.Fatalf("ConversationID = %q, want conv_1", parsed.ConversationID)
	}
}

func TestParseSSEExtractsConversationIDFromRawPayload(t *testing.T) {
	body := "data: {\"conversation_id\":\"conv_raw\",\"message\":invalid-json}\n\n"
	resp := &fhttp.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	parsed := parseSSE(resp)
	if parsed.ConversationID != "conv_raw" {
		t.Fatalf("ConversationID = %q, want conv_raw", parsed.ConversationID)
	}
}

func TestExtractConversationStateStopsOnTerminalAssistantText(t *testing.T) {
	mapping := map[string]any{
		"msg_user": map[string]any{
			"message": map[string]any{
				"author": map[string]any{"role": "user"},
				"content": map[string]any{
					"content_type": "text",
					"parts":        []any{"{\"prompt\":\"母亲节海报\"}"},
				},
				"create_time": 1.0,
			},
		},
		"msg_waiting": map[string]any{
			"message": map[string]any{
				"author": map[string]any{"role": "assistant"},
				"content": map[string]any{
					"content_type": "text",
					"parts":        []any{"正在处理图片，目前有很多人在创建图片。"},
				},
				"create_time": 2.0,
			},
		},
		"msg_final": map[string]any{
			"message": map[string]any{
				"author": map[string]any{"role": "assistant"},
				"content": map[string]any{
					"content_type": "text",
					"parts":        []any{"你的描述里涉及了这种元素，所以我不能按原要求生成。"},
				},
				"create_time": 3.0,
			},
		},
	}

	state := extractConversationState(mapping)
	if !strings.Contains(state.Text, "不能按原要求生成") {
		t.Fatalf("Text = %q, want final rejection text", state.Text)
	}
	if !state.Rejected {
		t.Fatal("expected rejected state")
	}
	if shouldContinuePolling(state) {
		t.Fatal("shouldContinuePolling returned true for terminal assistant text")
	}
}

func TestShouldContinuePollingForPromptEchoText(t *testing.T) {
	state := sseResult{
		ConversationID: "conv_1",
		Text:           `{"prompt":"生成电商主图","size":"1024x1024"}`,
	}

	if !shouldContinuePolling(state) {
		t.Fatal("shouldContinuePolling returned false for prompt echo text")
	}
}

package services

import (
	"io"
	"strings"
	"testing"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
)

// 以下消息结构对照 0622 HAR 真实抓包构造。
func msgPendingImagePlaceholder() map[string]any {
	return map[string]any{
		"author":    map[string]any{"role": "tool"},
		"recipient": "all",
		"content":   map[string]any{"content_type": "text", "parts": []any{""}},
		"end_turn":  true,
		"metadata": map[string]any{
			"image_gen_async":   true,
			"image_gen_task_id": "chatimagegen-us-prod.x:user-y:US",
		},
	}
}

func msgImageResult() map[string]any {
	return map[string]any{
		"author":    map[string]any{"role": "tool"},
		"recipient": "all",
		"content": map[string]any{
			"content_type": "multimodal_text",
			"parts": []any{map[string]any{
				"content_type":  "image_asset_pointer",
				"asset_pointer": "sediment://file_00000000349c720b9a5303cdc938d895",
			}},
		},
		"metadata": map[string]any{"image_gen_title": "神圣龙守护神殿"},
	}
}

func msgDalleToolCall() map[string]any {
	return map[string]any{
		"author":    map[string]any{"role": "assistant"},
		"recipient": "t2uay3k.sj1i4kz",
		"content":   map[string]any{"content_type": "code", "parts": []any{`{"prompt":"a dragon"}`}},
		"end_turn":  false,
	}
}

func msgAssistantTextReply() map[string]any {
	return map[string]any{
		"author":    map[string]any{"role": "assistant"},
		"recipient": "all",
		"content":   map[string]any{"content_type": "text", "parts": []any{"I need an image to use as a reference for edits or generation in this context."}},
		"end_turn":  true,
	}
}

func TestImageMessageClassification(t *testing.T) {
	if !isImageToolMessage(msgImageResult()) {
		t.Error("图片结果消息应被识别为 image tool message")
	}
	if isImageToolMessage(msgPendingImagePlaceholder()) {
		t.Error("占位消息(ct=text)不应被当作图片结果")
	}
	if !isPendingImageTask(msgPendingImagePlaceholder()) {
		t.Error("占位消息应被识别为 pending image task")
	}
	if isPendingImageTask(msgImageResult()) {
		t.Error("图片结果消息不应被识别为 pending image task")
	}
	if !isAssistantTurnComplete(msgAssistantTextReply()) {
		t.Error("recipient=all 且 end_turn 的文本回复应判定为 turn complete")
	}
	if isAssistantTurnComplete(msgDalleToolCall()) {
		t.Error("dalle 工具调用(recipient!=all, end_turn=false)不应判定为 turn complete")
	}
	if isUserFacingAssistant(msgDalleToolCall()) {
		t.Error("dalle 工具调用不应被当作面向用户的文本")
	}
}

func msgSkippedMainline() map[string]any {
	return map[string]any{
		"author":    map[string]any{"role": "assistant"},
		"recipient": "t2uay3k.sj1i4kz",
		"content":   map[string]any{"content_type": "code", "text": `{"skipped_mainline":true}`},
		"end_turn":  false,
	}
}

func TestSkippedMainlineDetection(t *testing.T) {
	if !isSkippedMainlineMessage(msgSkippedMainline()) {
		t.Error("应识别 skipped_mainline 消息")
	}
	// skipped_mainline:false 不应命中
	m := msgSkippedMainline()
	m["content"].(map[string]any)["text"] = `{"skipped_mainline":false}`
	if isSkippedMainlineMessage(m) {
		t.Error("skipped_mainline:false 不应命中")
	}
	// 普通 dalle 工具调用不应命中
	if isSkippedMainlineMessage(msgDalleToolCall()) {
		t.Error("普通工具调用不应命中")
	}
}

func TestExtractConversationStateSkipped(t *testing.T) {
	mapping := map[string]any{
		"n1": map[string]any{"message": msgDalleToolCall()},
		"n2": map[string]any{"message": msgSkippedMainline()},
	}
	st := extractConversationState(mapping)
	if !st.SkippedImage {
		t.Fatal("应检测到 SkippedImage")
	}
	if len(st.FileIDs) != 0 || st.PendingImage {
		t.Fatalf("skip 场景不应有图片/挂起任务, got %+v", st)
	}
	// 旧端点下 skipped_mainline 是终态，应据此提前停止轮询。
	if shouldContinuePolling(st) {
		t.Error("skipped_mainline 且无挂起任务应停止轮询")
	}
}

func TestStalledImageDispatchDetection(t *testing.T) {
	stalledLeaf := map[string]any{
		"children": []any{},
		"message": map[string]any{
			"author":    map[string]any{"role": "assistant"},
			"recipient": "t2uay3k.sj1i4kz",
			"content":   map[string]any{"content_type": "code", "text": `{"size":"1792x1024","prompt":"x"}`},
			"metadata":  map[string]any{"is_complete": true, "finish_details": map[string]any{"type": "stop"}},
		},
	}
	if !isStalledImageDispatch(stalledLeaf) {
		t.Error("应识别 stalled 出图调用叶子")
	}
	withChild := map[string]any{"children": []any{"c"}, "message": stalledLeaf["message"]}
	if isStalledImageDispatch(withChild) {
		t.Error("有子节点不应命中（健康流程后面还有占位/图片）")
	}
	notDone := map[string]any{
		"children": []any{},
		"message": map[string]any{
			"author": map[string]any{"role": "assistant"}, "recipient": "t2uay3k.sj1i4kz",
			"content": map[string]any{"content_type": "code", "text": `{"prompt":"x"}`}, "metadata": map[string]any{},
		},
	}
	if isStalledImageDispatch(notDone) {
		t.Error("未完成的调用不应命中")
	}

	// extractConversationState 应置位 DispatchStalled
	mapping := map[string]any{"tool": stalledLeaf}
	if st := extractConversationState(mapping); !st.DispatchStalled {
		t.Error("应检测到 DispatchStalled")
	}
}

func TestShouldContinuePollingEarlyStop(t *testing.T) {
	// 纯文字答复结束、无挂起图片任务 → 立即停止
	if shouldContinuePolling(sseResult{Text: "need an image", TurnComplete: true, PendingImage: false}) {
		t.Error("turn 已结束且无挂起图片任务，应停止轮询")
	}
	// 图片正在生成（占位存在），即便 end_turn → 继续等
	if !shouldContinuePolling(sseResult{TurnComplete: true, PendingImage: true}) {
		t.Error("有挂起图片任务时应继续轮询")
	}
	// 仅有挂起图片任务 → 继续等
	if !shouldContinuePolling(sseResult{PendingImage: true}) {
		t.Error("挂起图片任务应继续轮询")
	}
	// 拿到图片 → 停止
	if shouldContinuePolling(sseResult{FileIDs: []string{"sed:x"}, PendingImage: true}) {
		t.Error("已拿到图片应停止轮询")
	}
}

func TestExtractConversationStateSignals(t *testing.T) {
	// 模拟一次成功出图的会话：工具调用 + 占位 + 结果图
	mapping := map[string]any{
		"n1": map[string]any{"message": msgDalleToolCall()},
		"n2": map[string]any{"message": msgPendingImagePlaceholder()},
		"n3": map[string]any{"message": msgImageResult()},
	}
	st := extractConversationState(mapping)
	if len(st.FileIDs) != 1 {
		t.Fatalf("应提取到 1 个图片 fileID, got %v", st.FileIDs)
	}
	if !st.PendingImage {
		t.Error("应检测到 pending image task")
	}
	if shouldContinuePolling(st) {
		t.Error("已拿到图片应停止轮询")
	}

	// 模拟纯文字拒绝（无图）
	mapping2 := map[string]any{
		"n1": map[string]any{"message": msgAssistantTextReply()},
	}
	st2 := extractConversationState(mapping2)
	if len(st2.FileIDs) != 0 {
		t.Error("文字拒绝不应有 fileID")
	}
	if !st2.TurnComplete || st2.PendingImage {
		t.Errorf("文字拒绝应 TurnComplete=true PendingImage=false, got %+v", st2)
	}
	if shouldContinuePolling(st2) {
		t.Error("文字拒绝应立即停止轮询(早停)")
	}
	if strings.TrimSpace(st2.Text) == "" {
		t.Error("应捕获到拒绝文本")
	}
}

func TestSummarizeConversationMapping(t *testing.T) {
	mapping := map[string]any{
		"n1": map[string]any{"message": msgDalleToolCall()},
		"n2": map[string]any{"message": msgPendingImagePlaceholder()},
		"n3": map[string]any{"message": msgAssistantTextReply()},
	}
	out := summarizeConversationMapping(mapping)
	for _, want := range []string{"role=assistant", "role=tool", "image_gen_async=true", "recipient=all", "end_turn="} {
		if !strings.Contains(out, want) {
			t.Errorf("summary 缺少 %q:\n%s", want, out)
		}
	}
}

func TestBuildImageErrorMetaTimeoutDiag(t *testing.T) {
	state := sseResult{
		ConversationID: "conv-1",
		DiagMessages:   "  role=tool recipient=all image_gen_async=true",
		DiagRaw:        `{"mapping":{}}`,
	}
	meta := buildImageErrorMeta(state, true, false)
	if meta["timeout_messages"] != state.DiagMessages {
		t.Errorf("meta 缺少 timeout_messages: %v", meta["timeout_messages"])
	}
	if meta["timeout_raw"] != state.DiagRaw {
		t.Errorf("meta 缺少 timeout_raw: %v", meta["timeout_raw"])
	}
	// 非超时（无 diag）时不应带这两个字段
	meta2 := buildImageErrorMeta(sseResult{ConversationID: "c"}, false, false)
	if _, ok := meta2["timeout_messages"]; ok {
		t.Error("无超时诊断时不应出现 timeout_messages")
	}
}

func TestAppendSafetyKeyword(t *testing.T) {
	// 内容政策拦截 → 追加关键词
	if got := appendSafetyKeyword("抱歉，该提示违反了我们的内容政策。", "content_policy_violation"); !strings.Contains(got, contentPolicyKeyword) {
		t.Errorf("内容政策拦截应追加 %q, got %q", contentPolicyKeyword, got)
	}
	// 上游返回文本而非图片 → 追加关键词
	if got := appendSafetyKeyword("Here is a description instead.", "upstream_text_response"); !strings.Contains(got, contentPolicyKeyword) {
		t.Errorf("upstream_text_response 应追加 %q, got %q", contentPolicyKeyword, got)
	}
	// 其它原因 → 原样
	if got := appendSafetyKeyword("I can't generate that", "image_generation_rejected"); strings.Contains(got, contentPolicyKeyword) {
		t.Errorf("非安全类原因不应追加关键词, got %q", got)
	}
	// 幂等：已含关键词不重复追加
	if once := appendSafetyKeyword("内容政策 安全政策", "content_policy_violation"); strings.Count(once, contentPolicyKeyword) != 1 {
		t.Errorf("不应重复追加关键词, got %q", once)
	}
	// 空文本 → 直接是关键词
	if got := appendSafetyKeyword("", "upstream_text_response"); got != contentPolicyKeyword {
		t.Errorf("空文本应返回关键词本身, got %q", got)
	}
}

func TestBuildImageTextResultHasKeyword(t *testing.T) {
	res := buildImageTextResult("a cat", "这是一段文字说明，并非图片。")
	if !strings.Contains(toStr(res["message"]), contentPolicyKeyword) {
		t.Errorf("buildImageTextResult message 应含关键词, got %v", res["message"])
	}
	data, _ := res["data"].([]any)
	if len(data) > 0 {
		if m, ok := data[0].(map[string]any); ok {
			if !strings.Contains(toStr(m["text"]), contentPolicyKeyword) {
				t.Errorf("data[0].text 应含关键词, got %v", m["text"])
			}
		}
	}
}

func toStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestFileUploadThrottleDetection(t *testing.T) {
	msg := `file upload init failed: 429 {"detail":{"code":"throttled","error_code":"throttled","message":"You've reached our limit of file uploads. Please try again in 15 hours.","type":"throttled"}}`
	if !IsFileUploadThrottledError(msg) {
		t.Fatal("应识别文件上传限流")
	}
	if IsFileUploadThrottledError("some other error") {
		t.Error("普通错误不应命中")
	}
	if IsImageQuotaExceededError(msg) {
		t.Error("文件上传限流不应被当作图片额度限流")
	}
	now := time.Date(2026, 7, 7, 12, 0, 0, 0, time.UTC)
	ra := ExtractFileUploadRestoreAt(msg, now)
	if ra == nil {
		t.Fatal("应解析出恢复时间")
	}
	if got := ra.Sub(now); got != 15*time.Hour {
		t.Errorf("恢复时长 = %v, want 15h", got)
	}
}

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
		{"You've hit the free plan limit for image generations requests. You can create more images when the limit resets in 42 minutes.", true},
		{"IMAGE GENERATION REQUESTS can create more images when the limit resets in 5 minutes.", true},
		{"IMAGE GENERATIONS REQUESTS can create more images when the limit resets in 5 minutes.", true},
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
		{"非常抱歉，该提示可能违反了关于裸露、色情或情色内容的防护限制。如果你认为此判断有误，请重试或修改提示语。", "content_policy_violation"},
		{"抱歉，我无法生成带有色情或性暗示内容的图像。请提供一个适合所有观众的图像主题，我可以帮你生成。", "content_policy_violation"},
		{"抱歉，我无法生成包含色情或过度性化内容的图像。如果你希望，我可以帮你生成一张安全的、符合Cosplay主题的东海帝皇角色图像，例如穿着决胜服、帅气或优雅的pose。你希望我按这个方向生成吗？", "content_policy_violation"},
		{"抱歉，我无法生成包含色情或性暗示的内容。你可以提供一个适合的替代主题，比如角色cosplay、决胜服拍照、动作或场景设定，我可以帮你生成。", "content_policy_violation"},
		{"抱歉，我无法生成或编辑涉及色情或性行为的内容，包括性暗示的姿势或裸露。如果你希望，我可以帮助你将图像进行安全、非色情的艺术编辑或姿势调整，例如人物侧面、拥抱姿势（非性暗示）、服装更换等。你希望我按这个方向生成吗？", "content_policy_violation"},
		{"抱歉，我无法生成涉及性内容或色情内容的图像。", "content_policy_violation"},
		{"抱歉，我无法生成涉及性内容或色情场景的图像。请提供其他安全的创意图像请求，我可以帮你生成。", "content_policy_violation"},
		{"抱歉，我无法生成包含性行为或性暗示的内容。", "content_policy_violation"},
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

func TestShouldStopPollingForSandboxFileReference(t *testing.T) {
	tests := []string{
		"你可以在这里下载查看：[下载生成图片](sandbox:/mnt/data/output.png)",
		"已生成图片：sandbox:/tmp/output.png",
	}

	for _, text := range tests {
		state := sseResult{
			ConversationID: "conv_1",
			Text:           text,
		}

		if shouldContinuePolling(state) {
			t.Fatalf("shouldContinuePolling returned true for sandbox file reference %q", text)
		}
	}
}

func TestBuildImageTextResult(t *testing.T) {
	result := buildImageTextResult("draw a cat", "只返回了一段文本")
	// upstream_text_response 现在会追加"安全政策"关键词
	wantText := "只返回了一段文本 " + contentPolicyKeyword
	if result["message"] != wantText {
		t.Fatalf("message = %v, want %q", result["message"], wantText)
	}
	if result["reason"] != "upstream_text_response" {
		t.Fatalf("reason = %v, want upstream_text_response", result["reason"])
	}
	data, ok := result["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("data = %#v, want one item", result["data"])
	}
	item, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("data[0] = %#v, want map", data[0])
	}
	if item["text"] != wantText {
		t.Fatalf("text = %v, want %q", item["text"], wantText)
	}
	if item["revised_prompt"] != "draw a cat" {
		t.Fatalf("revised_prompt = %v, want prompt", item["revised_prompt"])
	}
}

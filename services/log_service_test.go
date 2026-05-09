package services

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"chatgpt2api-go/config"
)

func TestLogServiceAddListDelete(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir)

	c1 := svc.NewCall("/v1/chat/completions", "gpt-4o", "对话调用", "hello world")
	time.Sleep(2 * time.Millisecond)
	c1.Success(nil)

	c2 := svc.NewCall("/v1/images/generations", "gpt-image-1", "文生图", "draw a cat")
	c2.Failure("upstream timeout")

	items := svc.List(LogFilter{Type: LogTypeCall, Limit: 50})
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	// Newest first
	if items[0].Detail["endpoint"] != "/v1/images/generations" {
		t.Errorf("expected newest entry first, got %v", items[0].Detail["endpoint"])
	}
	if items[0].Detail["status"] != "failed" {
		t.Errorf("expected failed status, got %v", items[0].Detail["status"])
	}
	if items[0].Detail["error"] != "upstream timeout" {
		t.Errorf("expected error message preserved, got %v", items[0].Detail["error"])
	}
	if items[1].Detail["request_text"] != "hello world" {
		t.Errorf("expected request_text recorded, got %v", items[1].Detail["request_text"])
	}
	if d, ok := items[1].Detail["duration_ms"].(float64); !ok || d <= 0 {
		t.Errorf("expected positive duration_ms, got %v", items[1].Detail["duration_ms"])
	}

	removed := svc.Delete([]string{items[0].ID})
	if removed != 1 {
		t.Fatalf("expected removed=1, got %d", removed)
	}
	remaining := svc.List(LogFilter{Limit: 50})
	if len(remaining) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(remaining))
	}

	if !strings.HasSuffix(filepath.Clean(svc.path), "logs.jsonl") {
		t.Errorf("unexpected log path: %s", svc.path)
	}
}

func TestLoggedCallStreamFinishOnce(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir)
	c := svc.NewCall("/v1/chat/completions", "gpt-4o", "对话调用", "")
	c.StreamSuccess(nil)
	c.Failure("late error") // should be ignored: already finished

	items := svc.List(LogFilter{Limit: 10})
	if len(items) != 1 {
		t.Fatalf("expected 1 entry (finished once), got %d", len(items))
	}
	if items[0].Detail["status"] != "success" {
		t.Errorf("expected success, got %v", items[0].Detail["status"])
	}
}

func TestLoggedCallRecordsImages(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir)

	c := svc.NewCall("/v1/images/edits", "gpt-image-1", "图生图", "redraw")
	c.AddInputRequestImages([]RequestImage{
		{Data: []byte("\x89PNGfake"), FileName: "in.png", MimeType: "image/png"},
	})
	c.AddOutputsFromImageData(map[string]any{
		"data": []any{
			map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("out1"))},
			map[string]any{"b64_json": ""}, // empty should be skipped
			map[string]any{"b64_json": base64.StdEncoding.EncodeToString([]byte("out2"))},
		},
	})
	c.Success(nil)

	items := svc.List(LogFilter{Limit: 10})
	if len(items) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(items))
	}
	in, ok := items[0].Detail["input_images"].([]any)
	if !ok || len(in) != 1 {
		t.Fatalf("expected 1 input image, got %v", items[0].Detail["input_images"])
	}
	first, _ := in[0].(map[string]any)
	if first["mime"] != "image/png" || first["name"] != "in.png" {
		t.Errorf("unexpected input image meta: %+v", first)
	}
	out, ok := items[0].Detail["output_images"].([]any)
	if !ok || len(out) != 2 {
		t.Fatalf("expected 2 output images (empty filtered), got %v", items[0].Detail["output_images"])
	}
}

func TestLoggedCallChatCompletionOutputExtract(t *testing.T) {
	dir := t.TempDir()
	svc := NewLogService(dir)

	c := svc.NewCall("/v1/chat/completions", "gpt-image-1", "对话调用", "")
	c.AddOutputsFromChatCompletion(map[string]any{
		"choices": []any{
			map[string]any{
				"message": map[string]any{
					"content": "![image_1](data:image/png;base64,AAAA)\n\n![image_2](data:image/jpeg;base64,BBBB)",
				},
			},
		},
	})
	c.Success(nil)

	items := svc.List(LogFilter{Limit: 10})
	out, _ := items[0].Detail["output_images"].([]any)
	if len(out) != 2 {
		t.Fatalf("expected 2 output images extracted from markdown, got %d", len(out))
	}
	second, _ := out[1].(map[string]any)
	if second["mime"] != "image/jpeg" || second["b64"] != "BBBB" {
		t.Errorf("unexpected second output: %+v", second)
	}
}

func TestLogServiceTrimsToMaxEntries(t *testing.T) {
	dir := t.TempDir()
	prevConfig := config.Config
	config.Config = &config.AppSettings{LogMaxEntries: 3}
	defer func() { config.Config = prevConfig }()

	svc := NewLogService(dir)
	for i := 0; i < 7; i++ {
		c := svc.NewCall("/v1/images/generations", "gpt-image-1", "文生图", fmt.Sprintf("p-%d", i))
		c.Success(nil)
	}

	items := svc.List(LogFilter{Limit: 100})
	if len(items) != 3 {
		t.Fatalf("expected file trimmed to 3 entries, got %d", len(items))
	}
	if items[0].Detail["request_text"] != "p-6" || items[2].Detail["request_text"] != "p-4" {
		t.Errorf("expected newest 3 entries (p-6,p-5,p-4), got %v / %v",
			items[0].Detail["request_text"], items[2].Detail["request_text"])
	}
}

func TestRequestExcerptTruncation(t *testing.T) {
	long := strings.Repeat("好", 1500)
	out := requestExcerpt(long, 100)
	r := []rune(out)
	if len(r) != 100 {
		t.Fatalf("expected length 100 runes, got %d", len(r))
	}
	if r[len(r)-1] != '…' {
		t.Errorf("expected ellipsis at end, got %q", r[len(r)-1])
	}
}

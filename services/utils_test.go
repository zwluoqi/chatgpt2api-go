package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsImageChatRequest(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]any
		expected bool
	}{
		{"model gpt-image-1", map[string]any{"model": "gpt-image-1"}, true},
		{"model gpt-image-2", map[string]any{"model": "gpt-image-2"}, true},
		{"model gpt-4o", map[string]any{"model": "gpt-4o"}, false},
		{"modalities with image", map[string]any{"modalities": []any{"image"}}, true},
		{"modalities without image", map[string]any{"modalities": []any{"text"}}, false},
		{"empty body", map[string]any{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsImageChatRequest(tt.body)
			if result != tt.expected {
				t.Errorf("IsImageChatRequest(%v) = %v, want %v", tt.body, result, tt.expected)
			}
		})
	}
}

func TestExtractChatPrompt(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]any
		expected string
	}{
		{
			"direct prompt",
			map[string]any{"prompt": "hello world"},
			"hello world",
		},
		{
			"messages with text content",
			map[string]any{
				"messages": []any{
					map[string]any{
						"role":    "user",
						"content": "draw a cat",
					},
				},
			},
			"draw a cat",
		},
		{
			"messages with content array",
			map[string]any{
				"messages": []any{
					map[string]any{
						"role": "user",
						"content": []any{
							map[string]any{"type": "text", "text": "draw a dog"},
						},
					},
				},
			},
			"draw a dog",
		},
		{
			"skip non-user messages",
			map[string]any{
				"messages": []any{
					map[string]any{"role": "system", "content": "you are helpful"},
					map[string]any{"role": "user", "content": "draw a bird"},
				},
			},
			"draw a bird",
		},
		{
			"multiple user messages",
			map[string]any{
				"messages": []any{
					map[string]any{"role": "user", "content": "line 1"},
					map[string]any{"role": "user", "content": "line 2"},
				},
			},
			"line 1\nline 2",
		},
		{
			"empty body",
			map[string]any{},
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractChatPrompt(tt.body)
			if result != tt.expected {
				t.Errorf("ExtractChatPrompt() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestExtractResponsePrompt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"string input", "hello", "hello"},
		{"nil input", nil, ""},
		{
			"dict with user role",
			map[string]any{"role": "user", "content": "prompt text"},
			"prompt text",
		},
		{
			"dict with system role",
			map[string]any{"role": "system", "content": "sys text"},
			"",
		},
		{
			"list with input_text",
			[]any{
				map[string]any{"type": "input_text", "text": "draw something"},
			},
			"draw something",
		},
		{
			"list with user messages",
			[]any{
				map[string]any{"role": "user", "content": "prompt A"},
				map[string]any{"role": "user", "content": "prompt B"},
			},
			"prompt A\nprompt B",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ExtractResponsePrompt(tt.input)
			if result != tt.expected {
				t.Errorf("ExtractResponsePrompt() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestHasResponseImageGenerationTool(t *testing.T) {
	tests := []struct {
		name     string
		body     map[string]any
		expected bool
	}{
		{
			"tools with image_generation",
			map[string]any{
				"tools": []any{
					map[string]any{"type": "image_generation"},
				},
			},
			true,
		},
		{
			"tool_choice with image_generation",
			map[string]any{
				"tool_choice": map[string]any{"type": "image_generation"},
			},
			true,
		},
		{
			"no image_generation tool",
			map[string]any{
				"tools": []any{
					map[string]any{"type": "code_interpreter"},
				},
			},
			false,
		},
		{
			"empty body",
			map[string]any{},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasResponseImageGenerationTool(tt.body)
			if result != tt.expected {
				t.Errorf("HasResponseImageGenerationTool() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestParseImageCount(t *testing.T) {
	tests := []struct {
		name      string
		input     any
		expected  int
		expectErr bool
	}{
		{"nil defaults to 1", nil, 1, false},
		{"valid float64 2", float64(2), 2, false},
		{"valid float64 4", float64(4), 4, false},
		{"too low", float64(0), 0, true},
		{"too high", float64(5), 0, true},
		{"string type", "abc", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseImageCount(tt.input)
			if tt.expectErr && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("ParseImageCount() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestBuildChatImageCompletion(t *testing.T) {
	imageResult := map[string]any{
		"created": int64(1713800000),
		"data": []any{
			map[string]any{
				"b64_json":       "dGVzdA==",
				"revised_prompt": "a cat",
			},
		},
	}

	result := BuildChatImageCompletion("gpt-image-1", "draw a cat", imageResult)

	if result["object"] != "chat.completion" {
		t.Errorf("object = %v, want chat.completion", result["object"])
	}
	if result["model"] != "gpt-image-1" {
		t.Errorf("model = %v, want gpt-image-1", result["model"])
	}
	choices, ok := result["choices"].([]any)
	if !ok || len(choices) != 1 {
		t.Fatal("expected 1 choice")
	}
	choice, ok := choices[0].(map[string]any)
	if !ok {
		t.Fatal("choice is not map")
	}
	msg, ok := choice["message"].(map[string]any)
	if !ok {
		t.Fatal("message is not map")
	}
	content, ok := msg["content"].(string)
	if !ok {
		t.Fatal("content is not string")
	}
	if content == "" || content == "Image generation completed." {
		t.Error("content should contain image markdown")
	}
}

func TestBuildChatImageCompletionStreamChunks(t *testing.T) {
	completion := map[string]any{
		"id":      "chatcmpl-test",
		"object":  "chat.completion",
		"created": int64(1713800000),
		"model":   "gpt-image-1",
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": "![image_1](data:image/png;base64,dGVzdA==)",
				},
				"finish_reason": "stop",
			},
		},
	}

	chunks := BuildChatImageCompletionStreamChunks(completion)
	if len(chunks) != 3 {
		t.Fatalf("len(chunks) = %d, want 3", len(chunks))
	}
	if chunks[0]["object"] != "chat.completion.chunk" {
		t.Fatalf("object = %v, want chat.completion.chunk", chunks[0]["object"])
	}
	firstChoices, _ := chunks[0]["choices"].([]any)
	firstChoice, _ := firstChoices[0].(map[string]any)
	firstDelta, _ := firstChoice["delta"].(map[string]any)
	if firstDelta["role"] != "assistant" {
		t.Fatalf("first delta role = %v, want assistant", firstDelta["role"])
	}
	secondChoices, _ := chunks[1]["choices"].([]any)
	secondChoice, _ := secondChoices[0].(map[string]any)
	secondDelta, _ := secondChoice["delta"].(map[string]any)
	if !strings.Contains(fmt.Sprintf("%v", secondDelta["content"]), "data:image/png;base64") {
		t.Fatalf("second delta content = %v, want image markdown", secondDelta["content"])
	}
	lastChoices, _ := chunks[2]["choices"].([]any)
	lastChoice, _ := lastChoices[0].(map[string]any)
	if lastChoice["finish_reason"] != "stop" {
		t.Fatalf("finish_reason = %v, want stop", lastChoice["finish_reason"])
	}
}

func TestExtractImageFromMessageContent(t *testing.T) {
	t.Run("multiple images", func(t *testing.T) {
		content := []any{
			map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": "data:image/png;base64,dGVzdDE=",
				},
			},
			map[string]any{
				"type":      "input_image",
				"image_url": "data:image/jpeg;base64,dGVzdDI=",
			},
		}
		images := ExtractImagesFromMessageContent(content)
		if len(images) != 2 {
			t.Fatalf("len(images) = %d, want 2", len(images))
		}
		if string(images[0].Data) != "test1" {
			t.Fatalf("images[0].Data = %q, want test1", string(images[0].Data))
		}
		if images[1].MimeType != "image/jpeg" {
			t.Fatalf("images[1].MimeType = %q, want image/jpeg", images[1].MimeType)
		}
	})

	t.Run("image_url with data URL", func(t *testing.T) {
		content := []any{
			map[string]any{
				"type": "image_url",
				"image_url": map[string]any{
					"url": "data:image/png;base64,dGVzdA==",
				},
			},
		}
		data, mime, found := ExtractImageFromMessageContent(content)
		if !found {
			t.Fatal("expected to find image")
		}
		if mime != "image/png" {
			t.Errorf("mime = %s, want image/png", mime)
		}
		if string(data) != "test" {
			t.Errorf("data = %s, want test", string(data))
		}
	})

	t.Run("no image", func(t *testing.T) {
		content := []any{
			map[string]any{"type": "text", "text": "hello"},
		}
		_, _, found := ExtractImageFromMessageContent(content)
		if found {
			t.Error("expected no image")
		}
	})
}

func TestExtractChatImages(t *testing.T) {
	body := map[string]any{
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "ignore"},
				},
			},
			map[string]any{
				"role": "user",
				"content": []any{
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
	}

	images := ExtractChatImages(body)
	if len(images) != 2 {
		t.Fatalf("len(images) = %d, want 2", len(images))
	}
	if string(images[0].Data) != "test1" {
		t.Fatalf("images[0].Data = %q, want test1", string(images[0].Data))
	}
	if string(images[1].Data) != "test2" {
		t.Fatalf("images[1].Data = %q, want test2", string(images[1].Data))
	}
}

func TestMergePromptWithSize(t *testing.T) {
	tests := []struct {
		name     string
		prompt   string
		size     string
		expected string
	}{
		{
			name:     "appends size to prompt",
			prompt:   "draw a cat",
			size:     "1024x1024",
			expected: "draw a cat\n\nRequested output image size: 1024x1024.",
		},
		{
			name:     "returns prompt when size is empty",
			prompt:   "draw a cat",
			size:     "",
			expected: "draw a cat",
		},
		{
			name:     "size only",
			prompt:   "",
			size:     "1536x1024",
			expected: "Requested output image size: 1536x1024.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MergePromptWithSize(tt.prompt, tt.size); got != tt.expected {
				t.Fatalf("MergePromptWithSize() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestResolveImagesFromMessageContentRemoteURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("test-remote-image"))
	}))
	defer srv.Close()

	content := []any{
		map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": srv.URL + "/image.png",
			},
		},
	}

	images, err := ResolveImagesFromMessageContent(content)
	if err != nil {
		t.Fatalf("ResolveImagesFromMessageContent returned error: %v", err)
	}
	if len(images) != 1 {
		t.Fatalf("len(images) = %d, want 1", len(images))
	}
	if string(images[0].Data) != "test-remote-image" {
		t.Fatalf("images[0].Data = %q, want test-remote-image", string(images[0].Data))
	}
	if images[0].MimeType != "image/png" {
		t.Fatalf("images[0].MimeType = %q, want image/png", images[0].MimeType)
	}
	if images[0].FileName != "image.png" {
		t.Fatalf("images[0].FileName = %q, want image.png", images[0].FileName)
	}
}

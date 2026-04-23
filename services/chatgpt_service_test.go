package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGenerateWithPoolMarksQuotaLimitedAndFallsBack(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a", "token_b"})
	as.UpdateAccount("token_a", map[string]any{"quota": 1, "status": "正常"})
	as.UpdateAccount("token_b", map[string]any{"quota": 2, "status": "正常"})

	previous := generateImageResultFunc
	generateImageResultFunc = func(_ *AccountService, accessToken, prompt, model string) (map[string]any, error) {
		if accessToken == "token_a" {
			return nil, &ImageGenerationError{
				Message: "You've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25 minutes.",
			}
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{
					"b64_json":       "abc",
					"revised_prompt": prompt,
				},
			},
		}, nil
	}
	t.Cleanup(func() {
		generateImageResultFunc = previous
	})

	svc := NewChatGPTService(as)
	result, err := svc.GenerateWithPool("draw a cat", "gpt-image-1", 1)
	if err != nil {
		t.Fatalf("GenerateWithPool returned error: %v", err)
	}

	data, _ := result["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("len(data) = %d, want 1", len(data))
	}

	limited := as.GetAccount("token_a")
	if limited == nil {
		t.Fatal("token_a account not found")
	}
	if limited["status"] != "限流" {
		t.Fatalf("status = %v, want 限流", limited["status"])
	}
	if toInt(limited["quota"]) != 0 {
		t.Fatalf("quota = %v, want 0", limited["quota"])
	}
	if limited["restore_at"] == nil {
		t.Fatal("restore_at should be set")
	}
	if toInt(limited["fail"]) != 1 {
		t.Fatalf("fail = %v, want 1", limited["fail"])
	}

	healthy := as.GetAccount("token_b")
	if healthy == nil {
		t.Fatal("token_b account not found")
	}
	if healthy["status"] != "正常" {
		t.Fatalf("status = %v, want 正常", healthy["status"])
	}
	if toInt(healthy["quota"]) != 1 {
		t.Fatalf("quota = %v, want 1", healthy["quota"])
	}
}

func TestGenerateWithPoolReturnsQuotaExceededMessage(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 1, "status": "正常"})

	message := "You've hit the free plan limit for image generation requests. You can create more images when the limit resets in 10 hours and 25 minutes."
	previous := generateImageResultFunc
	generateImageResultFunc = func(_ *AccountService, accessToken, prompt, model string) (map[string]any, error) {
		return nil, &ImageGenerationError{Message: message}
	}
	t.Cleanup(func() {
		generateImageResultFunc = previous
	})

	svc := NewChatGPTService(as)
	_, err := svc.GenerateWithPool("draw a cat", "gpt-image-1", 1)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "free plan limit for image generation requests") {
		t.Fatalf("error = %q, want original quota exceeded message", err.Error())
	}

	account := as.GetAccount("token_a")
	if account == nil {
		t.Fatal("token_a account not found")
	}
	if account["status"] != "限流" {
		t.Fatalf("status = %v, want 限流", account["status"])
	}
	if toInt(account["quota"]) != 0 {
		t.Fatalf("quota = %v, want 0", account["quota"])
	}
}

func TestCreateImageCompletionUsesAllInputImages(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := editImageResultFunc
	editImageResultFunc = func(_ *AccountService, accessToken, prompt string, images []RequestImage, model string) (map[string]any, error) {
		if len(images) != 2 {
			t.Fatalf("len(images) = %d, want 2", len(images))
		}
		if string(images[0].Data) != "test1" {
			t.Fatalf("images[0].Data = %q, want test1", string(images[0].Data))
		}
		if string(images[1].Data) != "test2" {
			t.Fatalf("images[1].Data = %q, want test2", string(images[1].Data))
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		editImageResultFunc = previous
	})

	svc := NewChatGPTService(as)
	body := map[string]any{
		"model": "gpt-image-1",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "edit this"},
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

	result, httpErr := svc.CreateImageCompletion(body)
	if httpErr != nil {
		t.Fatalf("CreateImageCompletion returned error: %v", httpErr)
	}
	if result["object"] != "chat.completion" {
		t.Fatalf("object = %v, want chat.completion", result["object"])
	}
}

func TestCreateResponseUsesAllInputImages(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := editImageResultFunc
	editImageResultFunc = func(_ *AccountService, accessToken, prompt string, images []RequestImage, model string) (map[string]any, error) {
		if len(images) != 2 {
			t.Fatalf("len(images) = %d, want 2", len(images))
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		editImageResultFunc = previous
	})

	svc := NewChatGPTService(as)
	body := map[string]any{
		"model": "gpt-5",
		"tools": []any{
			map[string]any{"type": "image_generation"},
		},
		"input": []any{
			map[string]any{
				"type": "input_text",
				"text": "edit this",
			},
			map[string]any{
				"type":      "input_image",
				"image_url": "data:image/png;base64,dGVzdDE=",
			},
			map[string]any{
				"type":      "input_image",
				"image_url": "data:image/png;base64,dGVzdDI=",
			},
		},
	}

	result, httpErr := svc.CreateResponse(body)
	if httpErr != nil {
		t.Fatalf("CreateResponse returned error: %v", httpErr)
	}
	output, ok := result["output"].([]any)
	if !ok || len(output) != 1 {
		t.Fatalf("len(output) = %d, want 1", len(output))
	}
}

func TestCreateImageCompletionAppendsSizeToPrompt(t *testing.T) {
	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := generateImageResultFunc
	generateImageResultFunc = func(_ *AccountService, accessToken, prompt, model string) (map[string]any, error) {
		expected := "draw a cat\n\nRequested output image size: 1024x1024."
		if prompt != expected {
			t.Fatalf("prompt = %q, want %q", prompt, expected)
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		generateImageResultFunc = previous
	})

	svc := NewChatGPTService(as)
	body := map[string]any{
		"model": "gpt-image-1",
		"size":  "1024x1024",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "draw a cat"},
				},
			},
		},
	}

	result, httpErr := svc.CreateImageCompletion(body)
	if httpErr != nil {
		t.Fatalf("CreateImageCompletion returned error: %v", httpErr)
	}
	if result["object"] != "chat.completion" {
		t.Fatalf("object = %v, want chat.completion", result["object"])
	}
}

func TestCreateImageCompletionDownloadsRemoteImageURL(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("remote-image-data"))
	}))
	defer imageServer.Close()

	as := tempAccountService(t)
	as.AddAccounts([]string{"token_a"})
	as.UpdateAccount("token_a", map[string]any{"quota": 2, "status": "正常"})

	previous := editImageResultFunc
	editImageResultFunc = func(_ *AccountService, accessToken, prompt string, images []RequestImage, model string) (map[string]any, error) {
		if len(images) != 1 {
			t.Fatalf("len(images) = %d, want 1", len(images))
		}
		if string(images[0].Data) != "remote-image-data" {
			t.Fatalf("images[0].Data = %q, want remote-image-data", string(images[0].Data))
		}
		if images[0].MimeType != "image/png" {
			t.Fatalf("images[0].MimeType = %q, want image/png", images[0].MimeType)
		}
		return map[string]any{
			"created": int64(123),
			"data": []any{
				map[string]any{"b64_json": "abc", "revised_prompt": prompt},
			},
		}, nil
	}
	t.Cleanup(func() {
		editImageResultFunc = previous
	})

	svc := NewChatGPTService(as)
	body := map[string]any{
		"model": "gpt-image-1",
		"messages": []any{
			map[string]any{
				"role": "user",
				"content": []any{
					map[string]any{"type": "text", "text": "edit this"},
					map[string]any{
						"type": "image_url",
						"image_url": map[string]any{
							"url": imageServer.URL + "/image.png",
						},
					},
				},
			},
		},
	}

	result, httpErr := svc.CreateImageCompletion(body)
	if httpErr != nil {
		t.Fatalf("CreateImageCompletion returned error: %v", httpErr)
	}
	if result["object"] != "chat.completion" {
		t.Fatalf("object = %v, want chat.completion", result["object"])
	}
}

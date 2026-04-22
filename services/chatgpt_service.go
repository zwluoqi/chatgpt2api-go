package services

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"
)

type ChatGPTService struct {
	AccountService *AccountService
}

func NewChatGPTService(as *AccountService) *ChatGPTService {
	return &ChatGPTService{AccountService: as}
}

func (svc *ChatGPTService) GenerateWithPool(prompt, model string, n int) (map[string]any, error) {
	var created *int64
	var imageItems []any

	for index := 1; index <= n; index++ {
		for {
			requestToken, err := svc.AccountService.GetAvailableAccessToken()
			if err != nil {
				fmt.Printf("[image-generate] stop index=%d/%d error=%v\n", index, n, err)
				break
			}

			tokenPrefix := requestToken[:min(12, len(requestToken))]
			fmt.Printf("[image-generate] start pooled token=%s... model=%s index=%d/%d\n",
				tokenPrefix, model, index, n)

			result, genErr := GenerateImageResult(svc.AccountService, requestToken, prompt, model)
			if genErr != nil {
				account := svc.AccountService.MarkImageResult(requestToken, false)
				message := genErr.Error()
				quota := "unknown"
				status := "unknown"
				if account != nil {
					quota = fmt.Sprintf("%v", account["quota"])
					status = fmt.Sprintf("%v", account["status"])
				}
				fmt.Printf("[image-generate] fail pooled token=%s... error=%s quota=%s status=%s\n",
					tokenPrefix, message, quota, status)

				if IsTokenInvalidError(message) {
					svc.AccountService.RemoveToken(requestToken)
					fmt.Printf("[image-generate] remove invalid token=%s...\n", tokenPrefix)
					continue
				}
				break
			}

			account := svc.AccountService.MarkImageResult(requestToken, true)
			if created == nil {
				if c, ok := result["created"].(int64); ok {
					created = &c
				} else if c, ok := result["created"].(float64); ok {
					v := int64(c)
					created = &v
				}
			}
			if data, ok := result["data"].([]any); ok {
				for _, item := range data {
					if m, ok := item.(map[string]any); ok {
						imageItems = append(imageItems, m)
					}
				}
			}
			quota := "unknown"
			status := "unknown"
			if account != nil {
				quota = fmt.Sprintf("%v", account["quota"])
				status = fmt.Sprintf("%v", account["status"])
			}
			fmt.Printf("[image-generate] success pooled token=%s... quota=%s status=%s\n",
				tokenPrefix, quota, status)
			break
		}
	}

	if len(imageItems) == 0 {
		return nil, &ImageGenerationError{Message: "image generation failed"}
	}

	result := map[string]any{
		"data": imageItems,
	}
	if created != nil {
		result["created"] = *created
	}
	return result, nil
}

func (svc *ChatGPTService) EditWithPool(prompt string, images []struct {
	Data     []byte
	FileName string
	MimeType string
}, model string, n int) (map[string]any, error) {
	if len(images) == 0 {
		return nil, &ImageGenerationError{Message: "image is required"}
	}

	var created *int64
	var imageItems []any

	for index := 1; index <= n; index++ {
		for {
			requestToken, err := svc.AccountService.GetAvailableAccessToken()
			if err != nil {
				fmt.Printf("[image-edit] stop index=%d/%d error=%v\n", index, n, err)
				break
			}

			tokenPrefix := requestToken[:min(12, len(requestToken))]
			fmt.Printf("[image-edit] start pooled token=%s... model=%s index=%d/%d images=%d\n",
				tokenPrefix, model, index, n, len(images))

			result, editErr := EditImageResult(svc.AccountService, requestToken, prompt, images, model)
			if editErr != nil {
				account := svc.AccountService.MarkImageResult(requestToken, false)
				message := editErr.Error()
				quota := "unknown"
				status := "unknown"
				if account != nil {
					quota = fmt.Sprintf("%v", account["quota"])
					status = fmt.Sprintf("%v", account["status"])
				}
				fmt.Printf("[image-edit] fail pooled token=%s... error=%s quota=%s status=%s\n",
					tokenPrefix, message, quota, status)

				if IsTokenInvalidError(message) {
					svc.AccountService.RemoveToken(requestToken)
					fmt.Printf("[image-edit] remove invalid token=%s...\n", tokenPrefix)
					continue
				}
				break
			}

			account := svc.AccountService.MarkImageResult(requestToken, true)
			if created == nil {
				if c, ok := result["created"].(int64); ok {
					created = &c
				} else if c, ok := result["created"].(float64); ok {
					v := int64(c)
					created = &v
				}
			}
			if data, ok := result["data"].([]any); ok {
				for _, item := range data {
					if m, ok := item.(map[string]any); ok {
						imageItems = append(imageItems, m)
					}
				}
			}
			quota := "unknown"
			status := "unknown"
			if account != nil {
				quota = fmt.Sprintf("%v", account["quota"])
				status = fmt.Sprintf("%v", account["status"])
			}
			fmt.Printf("[image-edit] success pooled token=%s... quota=%s status=%s\n",
				tokenPrefix, quota, status)
			break
		}
	}

	if len(imageItems) == 0 {
		return nil, &ImageGenerationError{Message: "image edit failed"}
	}

	result := map[string]any{
		"data": imageItems,
	}
	if created != nil {
		result["created"] = *created
	}
	return result, nil
}

func extractResponseImage(inputValue any) ([]byte, string, bool) {
	if m, ok := inputValue.(map[string]any); ok {
		return ExtractImageFromMessageContent(m["content"])
	}
	items, ok := inputValue.([]any)
	if !ok {
		return nil, "", false
	}
	for i := len(items) - 1; i >= 0; i-- {
		m, ok := items[i].(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprintf("%v", m["type"])) == "input_image" {
			imageURL := fmt.Sprintf("%v", m["image_url"])
			if strings.HasPrefix(imageURL, "data:") {
				data, mime := parseDataURL(imageURL)
				if data != nil {
					return data, mime, true
				}
			}
		}
		if content, ok := m["content"]; ok {
			data, mime, found := ExtractImageFromMessageContent(content)
			if found {
				return data, mime, true
			}
		}
	}
	return nil, "", false
}

type HTTPError struct {
	StatusCode int
	Detail     map[string]any
}

func (e *HTTPError) Error() string {
	if msg, ok := e.Detail["error"].(string); ok {
		return msg
	}
	return fmt.Sprintf("HTTP %d", e.StatusCode)
}

func (svc *ChatGPTService) CreateImageCompletion(body map[string]any) (map[string]any, *HTTPError) {
	if !IsImageChatRequest(body) {
		return nil, &HTTPError{
			StatusCode: 400,
			Detail:     map[string]any{"error": "only image generation requests are supported on this endpoint"},
		}
	}

	if stream, ok := body["stream"].(bool); ok && stream {
		return nil, &HTTPError{
			StatusCode: 400,
			Detail:     map[string]any{"error": "stream is not supported for image generation"},
		}
	}

	model := strings.TrimSpace(fmt.Sprintf("%v", body["model"]))
	if model == "" || model == "<nil>" {
		model = "gpt-image-1"
	}

	n, err := ParseImageCount(body["n"])
	if err != nil {
		return nil, &HTTPError{StatusCode: 400, Detail: map[string]any{"error": err.Error()}}
	}

	prompt := ExtractChatPrompt(body)
	if prompt == "" {
		return nil, &HTTPError{StatusCode: 400, Detail: map[string]any{"error": "prompt is required"}}
	}

	imageData, mimeType, hasImage := ExtractChatImage(body)

	var imageResult map[string]any
	var genErr error

	if hasImage {
		images := []struct {
			Data     []byte
			FileName string
			MimeType string
		}{{Data: imageData, FileName: "image.png", MimeType: mimeType}}
		imageResult, genErr = svc.EditWithPool(prompt, images, model, n)
	} else {
		imageResult, genErr = svc.GenerateWithPool(prompt, model, n)
	}

	if genErr != nil {
		return nil, &HTTPError{StatusCode: 502, Detail: map[string]any{"error": genErr.Error()}}
	}

	return BuildChatImageCompletion(model, prompt, imageResult), nil
}

func (svc *ChatGPTService) CreateResponse(body map[string]any) (map[string]any, *HTTPError) {
	if stream, ok := body["stream"].(bool); ok && stream {
		return nil, &HTTPError{StatusCode: 400, Detail: map[string]any{"error": "stream is not supported"}}
	}

	if !HasResponseImageGenerationTool(body) {
		return nil, &HTTPError{
			StatusCode: 400,
			Detail:     map[string]any{"error": "only image_generation tool requests are supported on this endpoint"},
		}
	}

	prompt := ExtractResponsePrompt(body["input"])
	if prompt == "" {
		return nil, &HTTPError{StatusCode: 400, Detail: map[string]any{"error": "input text is required"}}
	}

	imageData, mimeType, hasImage := extractResponseImage(body["input"])

	model := strings.TrimSpace(fmt.Sprintf("%v", body["model"]))
	if model == "" || model == "<nil>" {
		model = "gpt-5"
	}

	var imageResult map[string]any
	var genErr error

	if hasImage {
		images := []struct {
			Data     []byte
			FileName string
			MimeType string
		}{{Data: imageData, FileName: "image.png", MimeType: mimeType}}
		imageResult, genErr = svc.EditWithPool(prompt, images, "gpt-image-1", 1)
	} else {
		imageResult, genErr = svc.GenerateWithPool(prompt, "gpt-image-1", 1)
	}

	if genErr != nil {
		return nil, &HTTPError{StatusCode: 502, Detail: map[string]any{"error": genErr.Error()}}
	}

	data, _ := imageResult["data"].([]any)
	var output []any
	for _, item := range data {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		b64JSON := strings.TrimSpace(fmt.Sprintf("%v", m["b64_json"]))
		if b64JSON == "" {
			continue
		}
		_ = base64.StdEncoding
		revisedPrompt := strings.TrimSpace(fmt.Sprintf("%v", m["revised_prompt"]))
		if revisedPrompt == "" || revisedPrompt == "<nil>" {
			revisedPrompt = prompt
		}
		output = append(output, map[string]any{
			"id":             fmt.Sprintf("ig_%d", len(output)+1),
			"type":           "image_generation_call",
			"status":         "completed",
			"result":         b64JSON,
			"revised_prompt": revisedPrompt,
		})
	}

	if len(output) == 0 {
		return nil, &HTTPError{StatusCode: 502, Detail: map[string]any{"error": "image generation failed"}}
	}

	createdAt := time.Now().Unix()
	if c, ok := imageResult["created"].(int64); ok {
		createdAt = c
	}

	return map[string]any{
		"id":                  fmt.Sprintf("resp_%d", createdAt),
		"object":              "response",
		"created_at":          createdAt,
		"status":              "completed",
		"error":               nil,
		"incomplete_details":  nil,
		"model":               model,
		"output":              output,
		"parallel_tool_calls": false,
	}, nil
}

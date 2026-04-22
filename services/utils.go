package services

import (
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ImageModels = map[string]bool{
	"gpt-image-1": true,
	"gpt-image-2": true,
}

func IsImageChatRequest(body map[string]any) bool {
	model := strings.TrimSpace(fmt.Sprintf("%v", body["model"]))
	if ImageModels[model] {
		return true
	}
	if modalities, ok := body["modalities"].([]any); ok {
		for _, item := range modalities {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprintf("%v", item)), "image") {
				return true
			}
		}
	}
	return false
}

func ExtractResponsePrompt(inputValue any) string {
	if s, ok := inputValue.(string); ok {
		return strings.TrimSpace(s)
	}

	if m, ok := inputValue.(map[string]any); ok {
		role := strings.TrimSpace(strings.ToLower(fmt.Sprintf("%v", m["role"])))
		if role != "" && role != "user" {
			return ""
		}
		return extractPromptFromMessageContent(m["content"])
	}

	items, ok := inputValue.([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(fmt.Sprintf("%v", m["type"])) == "input_text" {
			text := strings.TrimSpace(fmt.Sprintf("%v", m["text"]))
			if text != "" {
				parts = append(parts, text)
			}
			continue
		}
		role := strings.TrimSpace(strings.ToLower(fmt.Sprintf("%v", m["role"])))
		if role != "" && role != "user" {
			continue
		}
		prompt := extractPromptFromMessageContent(m["content"])
		if prompt != "" {
			parts = append(parts, prompt)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func HasResponseImageGenerationTool(body map[string]any) bool {
	if tools, ok := body["tools"].([]any); ok {
		for _, tool := range tools {
			if m, ok := tool.(map[string]any); ok {
				if strings.TrimSpace(fmt.Sprintf("%v", m["type"])) == "image_generation" {
					return true
				}
			}
		}
	}
	if tc, ok := body["tool_choice"].(map[string]any); ok {
		if strings.TrimSpace(fmt.Sprintf("%v", tc["type"])) == "image_generation" {
			return true
		}
	}
	return false
}

func extractPromptFromMessageContent(content any) string {
	if s, ok := content.(string); ok {
		return strings.TrimSpace(s)
	}
	items, ok := content.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(fmt.Sprintf("%v", m["type"]))
		if itemType == "text" {
			text := strings.TrimSpace(fmt.Sprintf("%v", m["text"]))
			if text != "" {
				parts = append(parts, text)
			}
		} else if itemType == "input_text" {
			text := strings.TrimSpace(fmt.Sprintf("%v", m["text"]))
			if text == "" {
				text = strings.TrimSpace(fmt.Sprintf("%v", m["input_text"]))
			}
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func ExtractImageFromMessageContent(content any) ([]byte, string, bool) {
	items, ok := content.([]any)
	if !ok {
		return nil, "", false
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(fmt.Sprintf("%v", m["type"]))
		if itemType == "image_url" {
			urlObj := m["image_url"]
			if urlObj == nil {
				urlObj = m
			}
			var url string
			if um, ok := urlObj.(map[string]any); ok {
				url = fmt.Sprintf("%v", um["url"])
			} else {
				url = fmt.Sprintf("%v", urlObj)
			}
			if strings.HasPrefix(url, "data:") {
				data, mime := parseDataURL(url)
				if data != nil {
					return data, mime, true
				}
			}
		}
		if itemType == "input_image" {
			imageURL := fmt.Sprintf("%v", m["image_url"])
			if strings.HasPrefix(imageURL, "data:") {
				data, mime := parseDataURL(imageURL)
				if data != nil {
					return data, mime, true
				}
			}
		}
	}
	return nil, "", false
}

func parseDataURL(url string) ([]byte, string) {
	idx := strings.Index(url, ",")
	if idx < 0 {
		return nil, ""
	}
	header := url[:idx]
	dataStr := url[idx+1:]
	mime := strings.TrimPrefix(strings.Split(header, ";")[0], "data:")
	if mime == "" {
		mime = "image/png"
	}
	decoded, err := base64.StdEncoding.DecodeString(dataStr)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(dataStr)
		if err != nil {
			return nil, ""
		}
	}
	return decoded, mime
}

func ExtractChatImage(body map[string]any) ([]byte, string, bool) {
	messages, ok := body["messages"].([]any)
	if !ok {
		return nil, "", false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		msg, ok := messages[i].(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(fmt.Sprintf("%v", msg["role"])))
		if role != "user" {
			continue
		}
		data, mime, found := ExtractImageFromMessageContent(msg["content"])
		if found {
			return data, mime, true
		}
	}
	return nil, "", false
}

func ExtractChatPrompt(body map[string]any) string {
	directPrompt := strings.TrimSpace(fmt.Sprintf("%v", body["prompt"]))
	if directPrompt != "" && directPrompt != "<nil>" {
		return directPrompt
	}

	messages, ok := body["messages"].([]any)
	if !ok {
		return ""
	}

	var parts []string
	for _, msg := range messages {
		m, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		role := strings.TrimSpace(strings.ToLower(fmt.Sprintf("%v", m["role"])))
		if role != "user" {
			continue
		}
		prompt := extractPromptFromMessageContent(m["content"])
		if prompt != "" {
			parts = append(parts, prompt)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func ParseImageCount(rawValue any) (int, error) {
	if rawValue == nil {
		return 1, nil
	}
	var value int
	switch v := rawValue.(type) {
	case float64:
		value = int(v)
	case int:
		value = v
	case int64:
		value = int(v)
	default:
		return 0, fmt.Errorf("n must be an integer")
	}
	if value < 1 || value > 4 {
		return 0, fmt.Errorf("n must be between 1 and 4")
	}
	return value, nil
}

func BuildChatImageCompletion(model, prompt string, imageResult map[string]any) map[string]any {
	created := time.Now().Unix()
	if c, ok := imageResult["created"].(int64); ok {
		created = c
	} else if c, ok := imageResult["created"].(float64); ok {
		created = int64(c)
	}

	var markdownImages []string
	if data, ok := imageResult["data"].([]any); ok {
		for i, item := range data {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			b64JSON := strings.TrimSpace(fmt.Sprintf("%v", m["b64_json"]))
			if b64JSON == "" {
				continue
			}
			markdownImages = append(markdownImages, fmt.Sprintf("![image_%d](data:image/png;base64,%s)", i+1, b64JSON))
		}
	}

	textContent := "Image generation completed."
	if len(markdownImages) > 0 {
		textContent = strings.Join(markdownImages, "\n\n")
	}

	return map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%s", uuid.New().String()),
		"object":  "chat.completion",
		"created": created,
		"model":   model,
		"choices": []any{
			map[string]any{
				"index": 0,
				"message": map[string]any{
					"role":    "assistant",
					"content": textContent,
				},
				"finish_reason": "stop",
			},
		},
		"usage": map[string]any{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}
}

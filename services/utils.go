package services

import (
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	neturl "net/url"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ImageModels = map[string]bool{
	"gpt-image-1": true,
	"gpt-image-2": true,
}

const MaxEditInputImages = 16

type RequestImage struct {
	Data     []byte
	FileName string
	MimeType string
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

func MergePromptWithSize(prompt, size string) string {
	prompt = strings.TrimSpace(prompt)
	size = strings.TrimSpace(size)
	if size == "" {
		return prompt
	}
	if prompt == "" {
		return fmt.Sprintf("Requested output image size: %s.", size)
	}
	return fmt.Sprintf("%s\n\nRequested output image size: %s.", prompt, size)
}

func buildRequestImage(data []byte, mime string, index int) RequestImage {
	if mime == "" {
		mime = "image/png"
	}
	return RequestImage{
		Data:     data,
		FileName: fmt.Sprintf("image_%d.png", index),
		MimeType: mime,
	}
}

func buildNamedRequestImage(data []byte, mimeType, fileName string, index int) RequestImage {
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	if mimeType == "" || !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		mimeType = "image/png"
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		exts, _ := mime.ExtensionsByType(mimeType)
		ext := ".png"
		if len(exts) > 0 && strings.TrimSpace(exts[0]) != "" {
			ext = exts[0]
		}
		fileName = fmt.Sprintf("image_%d%s", index, ext)
	}
	return RequestImage{
		Data:     data,
		FileName: fileName,
		MimeType: mimeType,
	}
}

func extractImageURLValue(raw any) string {
	if raw == nil {
		return ""
	}
	if m, ok := raw.(map[string]any); ok {
		return strings.TrimSpace(fmt.Sprintf("%v", m["url"]))
	}
	return strings.TrimSpace(fmt.Sprintf("%v", raw))
}

func downloadRemoteImage(urlStr string, index int) (RequestImage, error) {
	parsed, err := neturl.Parse(strings.TrimSpace(urlStr))
	if err != nil {
		return RequestImage{}, fmt.Errorf("invalid image url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return RequestImage{}, fmt.Errorf("unsupported image url scheme")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, parsed.String(), nil)
	if err != nil {
		return RequestImage{}, fmt.Errorf("invalid image url")
	}
	resp, err := client.Do(req)
	if err != nil {
		return RequestImage{}, fmt.Errorf("download image failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RequestImage{}, fmt.Errorf("download image failed: status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 20<<20))
	if err != nil {
		return RequestImage{}, fmt.Errorf("download image failed")
	}
	if len(data) == 0 {
		return RequestImage{}, fmt.Errorf("download image failed")
	}

	mimeType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if idx := strings.Index(mimeType, ";"); idx >= 0 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	if mimeType == "" || !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		mimeType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(mimeType), "image/") {
		return RequestImage{}, fmt.Errorf("image url did not return image content")
	}

	fileName := path.Base(parsed.Path)
	if fileName == "." || fileName == "/" {
		fileName = ""
	}

	return buildNamedRequestImage(data, mimeType, fileName, index), nil
}

func ExtractImagesFromMessageContent(content any) []RequestImage {
	items, ok := content.([]any)
	if !ok {
		return nil
	}

	var images []RequestImage
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
					images = append(images, buildRequestImage(data, mime, len(images)+1))
				}
			}
		}
		if itemType == "input_image" {
			imageURL := fmt.Sprintf("%v", m["image_url"])
			if strings.HasPrefix(imageURL, "data:") {
				data, mime := parseDataURL(imageURL)
				if data != nil {
					images = append(images, buildRequestImage(data, mime, len(images)+1))
				}
			}
		}
	}
	return images
}

func ResolveImagesFromMessageContent(content any) ([]RequestImage, error) {
	items, ok := content.([]any)
	if !ok {
		return nil, nil
	}

	var images []RequestImage
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		itemType := strings.TrimSpace(fmt.Sprintf("%v", m["type"]))
		var imageURL string
		if itemType == "image_url" {
			urlObj := m["image_url"]
			if urlObj == nil {
				urlObj = m
			}
			imageURL = extractImageURLValue(urlObj)
		}
		if itemType == "input_image" {
			imageURL = extractImageURLValue(m["image_url"])
		}
		if imageURL == "" {
			continue
		}
		if strings.HasPrefix(imageURL, "data:") {
			data, mimeType := parseDataURL(imageURL)
			if data != nil {
				images = append(images, buildNamedRequestImage(data, mimeType, "", len(images)+1))
			}
			continue
		}
		image, err := downloadRemoteImage(imageURL, len(images)+1)
		if err != nil {
			return nil, err
		}
		images = append(images, image)
	}
	return images, nil
}

func ExtractImageFromMessageContent(content any) ([]byte, string, bool) {
	images := ExtractImagesFromMessageContent(content)
	if len(images) == 0 {
		return nil, "", false
	}
	return images[0].Data, images[0].MimeType, true
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

func ExtractChatImages(body map[string]any) []RequestImage {
	messages, ok := body["messages"].([]any)
	if !ok {
		return nil
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
		images := ExtractImagesFromMessageContent(msg["content"])
		if len(images) > 0 {
			return images
		}
	}
	return nil
}

func ResolveChatImages(body map[string]any) ([]RequestImage, error) {
	messages, ok := body["messages"].([]any)
	if !ok {
		return nil, nil
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
		images, err := ResolveImagesFromMessageContent(msg["content"])
		if err != nil {
			return nil, err
		}
		if len(images) > 0 {
			return images, nil
		}
	}
	return nil, nil
}

func ExtractChatImage(body map[string]any) ([]byte, string, bool) {
	images := ExtractChatImages(body)
	if len(images) == 0 {
		return nil, "", false
	}
	return images[0].Data, images[0].MimeType, true
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

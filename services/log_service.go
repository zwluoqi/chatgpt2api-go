package services

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"chatgpt2api-go/config"
)

const (
	LogTypeCall = "call"
)

type LogService struct {
	mu   sync.Mutex
	path string
}

func NewLogService(dataDir string) *LogService {
	return &LogService{path: filepath.Join(dataDir, "logs.jsonl")}
}

type LogEntry struct {
	ID      string         `json:"id"`
	Time    string         `json:"time"`
	Type    string         `json:"type"`
	Summary string         `json:"summary"`
	Detail  map[string]any `json:"detail,omitempty"`
}

func newLogID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *LogService) Add(typ, summary string, detail map[string]any) {
	if s == nil {
		return
	}
	entry := LogEntry{
		ID:      newLogID(),
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Type:    typ,
		Summary: summary,
		Detail:  detail,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(s.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
	_, _ = f.Write([]byte("\n"))

	s.trimLocked(config.GetLogMaxEntries())
}

func (s *LogService) trimLocked(maxEntries int) {
	if maxEntries <= 0 {
		return
	}
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	nonEmpty := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			nonEmpty = append(nonEmpty, line)
		}
	}
	if len(nonEmpty) <= maxEntries {
		return
	}
	kept := nonEmpty[len(nonEmpty)-maxEntries:]
	content := strings.Join(kept, "\n") + "\n"
	_ = os.WriteFile(s.path, []byte(content), 0o644)
}

type LogFilter struct {
	Type      string
	StartDate string
	EndDate   string
	Limit     int
}

func (s *LogService) List(filter LogFilter) []LogEntry {
	if s == nil {
		return nil
	}
	if filter.Limit <= 0 {
		filter.Limit = 200
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.Open(s.path)
	if err != nil {
		return []LogEntry{}
	}
	defer f.Close()

	var lines [][]byte
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		raw := append([]byte(nil), scanner.Bytes()...)
		lines = append(lines, raw)
	}

	items := make([]LogEntry, 0, filter.Limit)
	for i := len(lines) - 1; i >= 0; i-- {
		var entry LogEntry
		if err := json.Unmarshal(lines[i], &entry); err != nil {
			continue
		}
		if filter.Type != "" && entry.Type != filter.Type {
			continue
		}
		day := ""
		if len(entry.Time) >= 10 {
			day = entry.Time[:10]
		}
		if filter.StartDate != "" && day < filter.StartDate {
			continue
		}
		if filter.EndDate != "" && day > filter.EndDate {
			continue
		}
		items = append(items, entry)
		if len(items) >= filter.Limit {
			break
		}
	}
	return items
}

func (s *LogService) Delete(ids []string) int {
	if s == nil {
		return 0
	}
	target := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		t := strings.TrimSpace(id)
		if t != "" {
			target[t] = struct{}{}
		}
	}
	if len(target) == 0 {
		return 0
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.path)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(data), "\n")
	kept := make([]string, 0, len(lines))
	removed := 0
	for _, raw := range lines {
		if raw == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			kept = append(kept, raw)
			continue
		}
		if _, ok := target[entry.ID]; ok {
			removed++
			continue
		}
		kept = append(kept, raw)
	}
	if removed == 0 {
		return 0
	}
	content := strings.Join(kept, "\n")
	if content != "" {
		content += "\n"
	}
	if err := os.WriteFile(s.path, []byte(content), 0o644); err != nil {
		return 0
	}
	return removed
}

type LogImage struct {
	Mime string `json:"mime"`
	Name string `json:"name,omitempty"`
	B64  string `json:"b64"`
}

type LoggedCall struct {
	svc         *LogService
	endpoint    string
	model       string
	summary     string
	started     time.Time
	requestText string
	tokenPrefix string
	inputs      []LogImage
	outputs     []LogImage
	finished    bool
}

func (s *LogService) NewCall(endpoint, model, summary, requestText string) *LoggedCall {
	return &LoggedCall{
		svc:         s,
		endpoint:    endpoint,
		model:       model,
		summary:     summary,
		started:     time.Now(),
		requestText: requestText,
	}
}

func (c *LoggedCall) WithTokenPrefix(token string) *LoggedCall {
	if c == nil {
		return nil
	}
	t := strings.TrimSpace(token)
	if len(t) > 12 {
		t = t[:12]
	}
	c.tokenPrefix = t
	return c
}

func (c *LoggedCall) AddInputImage(name, mime string, data []byte) {
	if c == nil || len(data) == 0 {
		return
	}
	if mime == "" {
		mime = "image/png"
	}
	c.inputs = append(c.inputs, LogImage{
		Mime: mime,
		Name: name,
		B64:  base64.StdEncoding.EncodeToString(data),
	})
}

func (c *LoggedCall) AddInputDataURL(url string) {
	if c == nil {
		return
	}
	mime, b64 := splitDataURL(url)
	if b64 == "" {
		return
	}
	c.inputs = append(c.inputs, LogImage{Mime: mime, B64: b64})
}

func (c *LoggedCall) AddOutputB64(mime, b64 string) {
	if c == nil {
		return
	}
	b64 = strings.TrimSpace(b64)
	if b64 == "" || b64 == "<nil>" {
		return
	}
	if mime == "" {
		mime = "image/png"
	}
	c.outputs = append(c.outputs, LogImage{Mime: mime, B64: b64})
}

func (c *LoggedCall) AddInputRequestImages(images []RequestImage) {
	if c == nil {
		return
	}
	for _, img := range images {
		c.AddInputImage(img.FileName, img.MimeType, img.Data)
	}
}

func (c *LoggedCall) AddOutputsFromImageData(result map[string]any) {
	if c == nil || result == nil {
		return
	}
	data, _ := result["data"].([]any)
	for _, item := range data {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c.AddOutputB64("", strings.TrimSpace(fmt.Sprintf("%v", m["b64_json"])))
	}
}

func (c *LoggedCall) AddOutputsFromResponse(result map[string]any) {
	if c == nil || result == nil {
		return
	}
	output, _ := result["output"].([]any)
	for _, item := range output {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		c.AddOutputB64("", strings.TrimSpace(fmt.Sprintf("%v", m["result"])))
	}
}

func (c *LoggedCall) AddOutputsFromChatCompletion(result map[string]any) {
	if c == nil || result == nil {
		return
	}
	choices, _ := result["choices"].([]any)
	for _, ch := range choices {
		m, ok := ch.(map[string]any)
		if !ok {
			continue
		}
		message, _ := m["message"].(map[string]any)
		if message == nil {
			continue
		}
		content, _ := message["content"].(string)
		for _, img := range ExtractDataURLImages(content) {
			c.outputs = append(c.outputs, img)
		}
	}
}

func splitDataURL(raw string) (string, string) {
	if !strings.HasPrefix(raw, "data:") {
		return "", ""
	}
	rest := raw[len("data:"):]
	commaIdx := strings.Index(rest, ",")
	if commaIdx < 0 {
		return "", ""
	}
	header := rest[:commaIdx]
	payload := rest[commaIdx+1:]
	mime := "image/png"
	if header != "" {
		parts := strings.SplitN(header, ";", 2)
		if parts[0] != "" {
			mime = parts[0]
		}
		if len(parts) == 2 && parts[1] == "base64" {
			return mime, payload
		}
	}
	return mime, payload
}

var dataURLRegex = regexp.MustCompile(`data:(image/[a-zA-Z0-9.+-]+);base64,([A-Za-z0-9+/=]+)`)

func ExtractDataURLImages(text string) []LogImage {
	matches := dataURLRegex.FindAllStringSubmatch(text, -1)
	images := make([]LogImage, 0, len(matches))
	for _, m := range matches {
		if len(m) < 3 {
			continue
		}
		images = append(images, LogImage{Mime: m[1], B64: m[2]})
	}
	return images
}

func (c *LoggedCall) Success(extra map[string]any) {
	c.write("success", "调用完成", "", extra)
}

func (c *LoggedCall) StreamSuccess(extra map[string]any) {
	c.write("success", "流式调用结束", "", extra)
}

func (c *LoggedCall) StreamFailure(err string, extra map[string]any) {
	c.write("failed", "流式调用失败", err, extra)
}

func (c *LoggedCall) Failure(err string) {
	c.write("failed", "调用失败", err, nil)
}

func (c *LoggedCall) write(status, suffix, errMsg string, extra map[string]any) {
	if c == nil || c.svc == nil || c.finished {
		return
	}
	c.finished = true

	now := time.Now()
	detail := map[string]any{
		"endpoint":    c.endpoint,
		"model":       c.model,
		"started_at":  c.started.Format("2006-01-02 15:04:05"),
		"ended_at":    now.Format("2006-01-02 15:04:05"),
		"duration_ms": int(now.Sub(c.started).Milliseconds()),
		"status":      status,
	}
	if excerpt := requestExcerpt(c.requestText, 1000); excerpt != "" {
		detail["request_text"] = excerpt
	}
	if c.tokenPrefix != "" {
		detail["account_token_prefix"] = c.tokenPrefix
	}
	if len(c.inputs) > 0 {
		detail["input_images"] = c.inputs
	}
	if len(c.outputs) > 0 {
		detail["output_images"] = c.outputs
	}
	if errMsg != "" {
		detail["error"] = errMsg
	}
	for k, v := range extra {
		detail[k] = v
	}
	c.svc.Add(LogTypeCall, c.summary+suffix, detail)
}

func requestExcerpt(text string, limit int) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}
	t = strings.Join(strings.Fields(t), " ")
	r := []rune(t)
	if len(r) <= limit {
		return t
	}
	return string(r[:limit-1]) + "…"
}

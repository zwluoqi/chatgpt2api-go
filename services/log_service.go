package services

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"chatgpt2api-go/config"
)

const (
	LogTypeCall  = "call"
	logEntryFile = "entry.json"
)

type LogService struct {
	mu         sync.Mutex
	dir        string
	legacyPath string
}

func NewLogService(dataDir string) *LogService {
	return &LogService{
		dir:        filepath.Join(dataDir, "logs"),
		legacyPath: filepath.Join(dataDir, "logs.jsonl"),
	}
}

type LogEntry struct {
	ID      string         `json:"id"`
	Time    string         `json:"time"`
	Type    string         `json:"type"`
	Summary string         `json:"summary"`
	Detail  map[string]any `json:"detail,omitempty"`
}

type storedLogEntry struct {
	Entry   LogEntry
	ModTime time.Time
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

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMigratedLocked()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return
	}
	if err := s.writeEntryLocked(entry); err != nil {
		return
	}

	s.trimLocked(config.GetLogMaxEntries())
}

func (s *LogService) trimLocked(maxEntries int) {
	if maxEntries <= 0 {
		return
	}
	entries := s.loadEntriesLocked()
	if len(entries) <= maxEntries {
		return
	}
	sortStoredLogEntries(entries)
	for _, item := range entries[maxEntries:] {
		_ = os.RemoveAll(s.entryDir(item.Entry.ID))
	}
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

	s.ensureMigratedLocked()

	items := make([]LogEntry, 0, filter.Limit)
	entries := s.loadEntriesLocked()
	sortStoredLogEntries(entries)
	for _, stored := range entries {
		entry := stored.Entry
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

	s.ensureMigratedLocked()

	removed := 0
	for id := range target {
		dir := s.entryDir(id)
		if _, err := os.Stat(dir); err != nil {
			continue
		}
		if err := os.RemoveAll(dir); err == nil {
			removed++
		}
	}
	return removed
}

func (s *LogService) ReadImageAsset(id, fileName string) ([]byte, string, bool) {
	if s == nil {
		return nil, "", false
	}
	cleanFile := filepath.Base(strings.TrimSpace(fileName))
	if cleanFile == "" || cleanFile != strings.TrimSpace(fileName) {
		return nil, "", false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.ensureMigratedLocked()

	entryPath := filepath.Join(s.entryDir(id), logEntryFile)
	data, err := os.ReadFile(entryPath)
	if err != nil {
		return nil, "", false
	}
	var entry LogEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, "", false
	}

	mimeType := ""
	for _, key := range []string{"input_images", "output_images"} {
		for _, image := range normalizeLogImages(entry.Detail[key]) {
			if image.File == cleanFile {
				mimeType = image.Mime
				break
			}
		}
		if mimeType != "" {
			break
		}
	}
	if mimeType == "" {
		mimeType = mime.TypeByExtension(filepath.Ext(cleanFile))
	}

	asset, err := os.ReadFile(filepath.Join(s.entryDir(id), cleanFile))
	if err != nil || len(asset) == 0 {
		return nil, "", false
	}
	return asset, mimeType, true
}

type LogImage struct {
	Mime   string `json:"mime"`
	Name   string `json:"name,omitempty"`
	B64    string `json:"b64,omitempty"`
	File   string `json:"file,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	Bytes  int    `json:"bytes,omitempty"`
}

// imageSizeLabels 汇总一组图片的尺寸标签（如 "1024x1024"），用于在日志详情里
// 以一个易读的顶层字段直接展示尺寸；无尺寸的退化为字节数（如 "12345B"）。
func imageSizeLabels(images []LogImage) []string {
	labels := make([]string, 0, len(images))
	for _, img := range images {
		switch {
		case img.Width > 0 && img.Height > 0:
			labels = append(labels, fmt.Sprintf("%dx%d", img.Width, img.Height))
		case img.Bytes > 0:
			labels = append(labels, fmt.Sprintf("%dB", img.Bytes))
		}
	}
	return labels
}

// fillImageSize 用图片字节填充尺寸与体积（width/height/bytes），便于在日志里记录
// "上游传送的 size"。解析失败则只记录字节数。
func fillImageSize(img *LogImage, data []byte) {
	if img == nil || len(data) == 0 {
		return
	}
	img.Bytes = len(data)
	if w, h := getImageDimensions(data); w > 0 && h > 0 {
		img.Width = w
		img.Height = h
	}
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
	img := LogImage{
		Mime: mime,
		Name: name,
		B64:  base64.StdEncoding.EncodeToString(data),
	}
	fillImageSize(&img, data)
	c.inputs = append(c.inputs, img)
}

func (c *LoggedCall) AddInputDataURL(url string) {
	if c == nil {
		return
	}
	mime, b64 := splitDataURL(url)
	if b64 == "" {
		return
	}
	img := LogImage{Mime: mime, B64: b64}
	if data, err := base64.StdEncoding.DecodeString(b64); err == nil {
		fillImageSize(&img, data)
	}
	c.inputs = append(c.inputs, img)
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
	img := LogImage{Mime: mime, B64: b64}
	if data, err := base64.StdEncoding.DecodeString(b64); err == nil {
		fillImageSize(&img, data)
	}
	c.outputs = append(c.outputs, img)
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

func (c *LoggedCall) FailureWithExtra(err string, extra map[string]any) {
	c.write("failed", "调用失败", err, extra)
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
	if excerpt := requestExcerpt(c.requestText, 0); excerpt != "" {
		detail["request_text"] = excerpt
	}
	if c.tokenPrefix != "" {
		detail["account_token_prefix"] = c.tokenPrefix
	}
	if len(c.inputs) > 0 {
		detail["input_images"] = c.inputs
		if sizes := imageSizeLabels(c.inputs); len(sizes) > 0 {
			detail["input_image_sizes"] = sizes
		}
	}
	if len(c.outputs) > 0 {
		detail["output_images"] = c.outputs
		if sizes := imageSizeLabels(c.outputs); len(sizes) > 0 {
			detail["output_image_sizes"] = sizes
		}
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
	if limit <= 0 {
		return t
	}
	r := []rune(t)
	if len(r) <= limit {
		return t
	}
	return string(r[:limit-1]) + "…"
}

func sortStoredLogEntries(items []storedLogEntry) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Entry.Time != items[j].Entry.Time {
			return items[i].Entry.Time > items[j].Entry.Time
		}
		if !items[i].ModTime.Equal(items[j].ModTime) {
			return items[i].ModTime.After(items[j].ModTime)
		}
		return items[i].Entry.ID > items[j].Entry.ID
	})
}

func (s *LogService) entryDir(id string) string {
	return filepath.Join(s.dir, id)
}

func (s *LogService) writeEntryLocked(entry LogEntry) error {
	if err := os.MkdirAll(s.entryDir(entry.ID), 0o755); err != nil {
		return err
	}
	stored := entry
	stored.Detail = s.prepareDetailForStorageLocked(entry.ID, entry.Detail)
	data, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(s.entryDir(entry.ID), logEntryFile), data, 0o644)
}

func (s *LogService) prepareDetailForStorageLocked(id string, detail map[string]any) map[string]any {
	if len(detail) == 0 {
		return detail
	}
	next := make(map[string]any, len(detail))
	for key, value := range detail {
		next[key] = value
	}
	for key, prefix := range map[string]string{
		"input_images":  "input",
		"output_images": "output",
	} {
		images := normalizeLogImages(next[key])
		if len(images) == 0 {
			continue
		}
		stored := make([]LogImage, 0, len(images))
		for index, image := range images {
			current := image
			if strings.TrimSpace(image.B64) != "" {
				if fileName, ok := s.writeImageAssetLocked(id, prefix, index, image.Mime, image.B64); ok {
					current.File = fileName
					current.B64 = ""
				}
			}
			stored = append(stored, current)
		}
		next[key] = stored
	}
	return next
}

func (s *LogService) writeImageAssetLocked(id, prefix string, index int, mimeType, b64 string) (string, bool) {
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(b64))
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	fileName := fmt.Sprintf("%s-%03d%s", prefix, index+1, imageFileExtension(mimeType))
	if err := os.WriteFile(filepath.Join(s.entryDir(id), fileName), decoded, 0o644); err != nil {
		return "", false
	}
	return fileName, true
}

func (s *LogService) loadEntriesLocked() []storedLogEntry {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	items := make([]storedLogEntry, 0, len(entries))
	for _, item := range entries {
		if !item.IsDir() {
			continue
		}
		entryPath := filepath.Join(s.dir, item.Name(), logEntryFile)
		data, err := os.ReadFile(entryPath)
		if err != nil {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			continue
		}
		info, err := os.Stat(entryPath)
		if err != nil {
			continue
		}
		items = append(items, storedLogEntry{
			Entry:   entry,
			ModTime: info.ModTime(),
		})
	}
	return items
}

func (s *LogService) ensureMigratedLocked() {
	if _, err := os.Stat(s.legacyPath); err != nil {
		return
	}
	f, err := os.Open(s.legacyPath)
	if err != nil {
		return
	}
	defer f.Close()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for scanner.Scan() {
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(raw), &entry); err != nil {
			continue
		}
		_ = s.writeEntryLocked(entry)
	}
	if err := scanner.Err(); err != nil {
		return
	}
	_ = os.Remove(s.legacyPath)
}

func normalizeLogImages(value any) []LogImage {
	switch images := value.(type) {
	case []LogImage:
		return append([]LogImage(nil), images...)
	case []any:
		out := make([]LogImage, 0, len(images))
		for _, item := range images {
			record, ok := item.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, LogImage{
				Mime: strings.TrimSpace(fmt.Sprintf("%v", record["mime"])),
				Name: strings.TrimSpace(fmt.Sprintf("%v", record["name"])),
				B64:  strings.TrimSpace(fmt.Sprintf("%v", record["b64"])),
				File: strings.TrimSpace(fmt.Sprintf("%v", record["file"])),
			})
		}
		return out
	default:
		return nil
	}
}

func imageFileExtension(mimeType string) string {
	if mimeType != "" {
		if exts, err := mime.ExtensionsByType(mimeType); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "image/jpeg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

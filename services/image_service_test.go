package services

import (
	"testing"
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

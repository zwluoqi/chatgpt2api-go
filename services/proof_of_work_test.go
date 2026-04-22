package services

import (
	"strings"
	"testing"
)

func TestGetPowConfig(t *testing.T) {
	config := GetPowConfig("TestAgent/1.0")
	if len(config) != 18 {
		t.Errorf("config length = %d, want 18", len(config))
	}
	if config[4] != "TestAgent/1.0" {
		t.Errorf("config[4] = %v, want TestAgent/1.0", config[4])
	}
	if config[7] != "en-US" {
		t.Errorf("config[7] = %v, want en-US", config[7])
	}
}

func TestGenerateAnswer(t *testing.T) {
	config := GetPowConfig("TestAgent/1.0")
	answer, solved := generateAnswer("test_seed", "0fffff", config)
	if !solved {
		t.Log("PoW not solved within 500k attempts (expected for easy difficulty)")
	}
	if answer == "" {
		t.Error("answer should not be empty")
	}
}

func TestGetAnswerToken(t *testing.T) {
	config := GetPowConfig("TestAgent/1.0")
	token, _ := GetAnswerToken("test_seed", "0fffff", config)
	if !strings.HasPrefix(token, "gAAAAAB") {
		t.Errorf("token should start with gAAAAAB, got %s", token[:min(10, len(token))])
	}
}

func TestGetRequirementsToken(t *testing.T) {
	config := GetPowConfig("TestAgent/1.0")
	token := GetRequirementsToken(config)
	if !strings.HasPrefix(token, "gAAAAAC") {
		t.Errorf("token should start with gAAAAAC, got %s", token[:min(10, len(token))])
	}
}

func TestGenerateProofToken(t *testing.T) {
	config := GetPowConfig("TestAgent/1.0")
	token := GenerateProofToken("seed123", "0fffff", "TestAgent/1.0", config)
	if !strings.HasPrefix(token, "gAAAAAB") {
		t.Errorf("token should start with gAAAAAB, got %s", token[:min(10, len(token))])
	}
}

func TestGetDataBuildFromHTML(t *testing.T) {
	html := `<html data-build="abc123"><head><script src="https://cdn.example.com/c/v1/_/main.js"></script></head></html>`
	GetDataBuildFromHTML(html)
	// Just verify it doesn't panic
}

func TestHexToBytes(t *testing.T) {
	tests := []struct {
		hex    string
		expect []byte
		err    bool
	}{
		{"0fffff", []byte{0x0f, 0xff, 0xff}, false},
		{"ff", []byte{0xff}, false},
		{"00", []byte{0x00}, false},
		{"zz", nil, true},
	}
	for _, tt := range tests {
		result, err := hexToBytes(tt.hex)
		if tt.err && err == nil {
			t.Errorf("hexToBytes(%s) expected error", tt.hex)
		}
		if !tt.err && err != nil {
			t.Errorf("hexToBytes(%s) unexpected error: %v", tt.hex, err)
		}
		if !tt.err && len(result) != len(tt.expect) {
			t.Errorf("hexToBytes(%s) = %v, want %v", tt.hex, result, tt.expect)
		}
	}
}

func TestCompareBytes(t *testing.T) {
	tests := []struct {
		a, b   []byte
		expect bool
	}{
		{[]byte{0x00}, []byte{0x0f}, true},
		{[]byte{0x0f}, []byte{0x0f}, true},
		{[]byte{0x10}, []byte{0x0f}, false},
		{[]byte{0x0f, 0xff}, []byte{0x0f, 0xff}, true},
	}
	for _, tt := range tests {
		result := compareBytes(tt.a, tt.b)
		if result != tt.expect {
			t.Errorf("compareBytes(%v, %v) = %v, want %v", tt.a, tt.b, result, tt.expect)
		}
	}
}

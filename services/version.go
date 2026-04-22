package services

import (
	"os"
	"path/filepath"
	"strings"
)

func GetAppVersion(baseDir string) string {
	data, err := os.ReadFile(filepath.Join(baseDir, "VERSION"))
	if err != nil {
		return "0.0.0"
	}
	v := strings.TrimSpace(string(data))
	if v == "" {
		return "0.0.0"
	}
	return v
}

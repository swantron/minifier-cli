package analyzer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAnalyzerBasic(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "test.log")

	content := "/bin/bash\n/lib/x86_64-linux-gnu/libc.so.6\n/etc/passwd\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log file: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Failed to analyze: %v", err)
	}

	if len(manifest.Files) == 0 {
		t.Fatal("Expected at least some files in manifest")
	}
}

func TestAnalyzerEmptyLog(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "empty.log")

	if err := os.WriteFile(logFile, []byte(""), 0644); err != nil {
		t.Fatalf("Failed to create empty log file: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Failed to analyze: %v", err)
	}

	if len(manifest.Files) == 0 {
		t.Fatal("Expected safelist files even with empty log")
	}
}

func TestAnalyzerNonExistentFile(t *testing.T) {
	a := NewAnalyzer()
	_, err := a.Analyze("/nonexistent/path/to/file.log")
	if err == nil {
		t.Fatal("Expected error when analyzing nonexistent file")
	}
}

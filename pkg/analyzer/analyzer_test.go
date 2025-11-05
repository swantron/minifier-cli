package analyzer

import (
	"debug/elf"
	"os"
	"path/filepath"
	"strings"
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

	// Should have original files plus safelist
	hasOriginalFile := false
	for _, file := range manifest.Files {
		if file == "/bin/bash" || file == "/etc/passwd" {
			hasOriginalFile = true
			break
		}
	}

	if !hasOriginalFile {
		t.Error("Expected to find original files from trace log")
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

func TestAnalyzerDeduplication(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "dedup.log")

	// Same file listed multiple times
	content := "/bin/sh\n/bin/sh\n/bin/sh\n/etc/passwd\n/etc/passwd\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log file: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Failed to analyze: %v", err)
	}

	// Check that files are deduplicated
	fileCount := make(map[string]int)
	for _, file := range manifest.Files {
		fileCount[file]++
	}

	for file, count := range fileCount {
		if count > 1 {
			t.Errorf("File %s appears %d times, should be deduplicated", file, count)
		}
	}
}

func TestIsELFBinary(t *testing.T) {
	a := NewAnalyzer()

	// Test with a real ELF binary if available
	testPaths := []string{
		"/bin/ls",
		"/bin/sh",
		"/usr/bin/env",
		"/bin/bash",
		"/usr/bin/python3",
	}

	foundELF := false
	for _, path := range testPaths {
		if _, err := os.Stat(path); err == nil {
			if a.isELFBinary(path) {
				foundELF = true

				// Verify it's actually an ELF file
				f, err := os.Open(path)
				if err != nil {
					t.Fatalf("Failed to open %s: %v", path, err)
				}
				defer f.Close()

				_, err = elf.NewFile(f)
				if err != nil {
					t.Errorf("isELFBinary returned true for %s, but elf.NewFile failed: %v", path, err)
				}

				break
			}
		}
	}

	// On macOS, binaries are Mach-O, not ELF, so we may not find any
	// This is OK - we're just testing the function doesn't crash
	if !foundELF {
		t.Skip("No ELF binaries found (may be on macOS)")
	}
}

func TestIsELFBinaryNonELF(t *testing.T) {
	a := NewAnalyzer()

	// Create a non-ELF file
	tempDir := t.TempDir()
	textFile := filepath.Join(tempDir, "test.txt")
	if err := os.WriteFile(textFile, []byte("not an ELF file"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	if a.isELFBinary(textFile) {
		t.Error("Text file should not be detected as ELF binary")
	}

	// Test with non-existent file
	if a.isELFBinary("/nonexistent/file") {
		t.Error("Non-existent file should not be detected as ELF binary")
	}
}

func TestResolveSymlinks(t *testing.T) {
	a := NewAnalyzer()
	resolved := make(map[string]struct{})

	tempDir := t.TempDir()

	// Create a real file
	target := filepath.Join(tempDir, "target.txt")
	if err := os.WriteFile(target, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	// Create a symlink
	link := filepath.Join(tempDir, "link.txt")
	if err := os.Symlink("target.txt", link); err != nil {
		t.Fatalf("Failed to create symlink: %v", err)
	}

	// Resolve the symlink
	a.resolveSymlinks(link, resolved)

	// Should have both the link and the target
	if len(resolved) == 0 {
		t.Error("Expected resolved map to contain target file")
	}

	hasTarget := false
	for file := range resolved {
		if strings.Contains(file, "target.txt") {
			hasTarget = true
			break
		}
	}

	if !hasTarget {
		t.Error("Expected to find target file in resolved map")
	}
}

func TestResolveSymlinksNonExistent(t *testing.T) {
	a := NewAnalyzer()
	resolved := make(map[string]struct{})

	// Should not crash on non-existent file
	a.resolveSymlinks("/nonexistent/file", resolved)

	// resolved should remain empty
	if len(resolved) > 0 {
		t.Error("Should not add non-existent files to resolved map")
	}
}

func TestResolveELFDependencies(t *testing.T) {
	a := NewAnalyzer()
	resolved := make(map[string]struct{})

	// Test with a real ELF binary if available
	testBinaries := []string{"/bin/ls", "/bin/sh", "/usr/bin/env"}

	for _, binary := range testBinaries {
		if fileExists(binary) && a.isELFBinary(binary) {
			a.resolveELFDependencies(binary, resolved)

			// Should have found at least the binary itself and some libraries
			if len(resolved) == 0 {
				t.Errorf("Expected to find dependencies for %s", binary)
			}

			// Check for common libraries
			hasLibc := false
			for file := range resolved {
				if strings.Contains(file, "libc") || strings.Contains(file, "ld-") {
					hasLibc = true
					break
				}
			}

			if !hasLibc {
				t.Logf("Warning: Did not find libc or ld in dependencies for %s", binary)
				t.Logf("Resolved files: %v", resolved)
			}

			break
		}
	}
}

func TestGetELFInterpreter(t *testing.T) {
	a := NewAnalyzer()

	// Test with a real ELF binary
	testBinaries := []string{"/bin/ls", "/bin/sh"}

	for _, binary := range testBinaries {
		if fileExists(binary) {
			f, err := os.Open(binary)
			if err != nil {
				continue
			}
			defer f.Close()

			elfFile, err := elf.NewFile(f)
			if err != nil {
				continue
			}
			defer elfFile.Close()

			interp := a.getELFInterpreter(elfFile)

			// Most ELF binaries have an interpreter
			if interp != "" {
				// Should be a valid path
				if !strings.HasPrefix(interp, "/") {
					t.Errorf("Interpreter path should be absolute, got: %s", interp)
				}

				// Common interpreters
				if !strings.Contains(interp, "ld-") && !strings.Contains(interp, "ld.so") {
					t.Logf("Warning: Unusual interpreter path: %s", interp)
				}
			}

			break
		}
	}
}

func TestAddSafelistFiles(t *testing.T) {
	a := NewAnalyzer()
	resolved := make(map[string]struct{})

	a.addSafelistFiles(resolved)

	if len(resolved) == 0 {
		t.Fatal("Expected safelist files to be added")
	}

	// Check for some common safelist files
	expectedFiles := []string{
		"/etc/passwd",
		"/etc/group",
		"/etc/hosts",
	}

	for _, file := range expectedFiles {
		if _, ok := resolved[file]; !ok {
			t.Errorf("Expected safelist to contain %s", file)
		}
	}
}

func TestResolveDependencies(t *testing.T) {
	a := NewAnalyzer()

	fileSet := map[string]struct{}{
		"/etc/passwd": {},
		"/bin/sh":     {},
	}

	files := a.resolveDependencies(fileSet)

	// Should have at least the original files plus safelist
	if len(files) < 2 {
		t.Errorf("Expected at least 2 files, got %d", len(files))
	}

	// Check that original files are included
	hasOriginal := false
	for _, file := range files {
		if file == "/etc/passwd" || file == "/bin/sh" {
			hasOriginal = true
			break
		}
	}

	if !hasOriginal {
		t.Error("Expected original files to be included in dependencies")
	}
}

// Helper functions

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

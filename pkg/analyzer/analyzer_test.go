package analyzer

import (
	"debug/elf"
	"fmt"
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

// Edge case tests

func TestAnalyzerInvalidLogFile(t *testing.T) {
	a := NewAnalyzer()
	_, err := a.Analyze("/nonexistent/path/to/log.txt")
	if err == nil {
		t.Error("Expected error for nonexistent log file")
	}
}

func TestAnalyzerMalformedPaths(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "malformed.log")

	// Test with various malformed paths
	content := `/valid/path/file.txt
/path/with spaces/file.txt
/path/with	tabs/file.txt
/path/with/trailing/slash/
/path//with//double//slashes
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should handle various path formats
	// Note: relative paths and ../ might be included - implementation choice
	if len(manifest.Files) == 0 {
		t.Error("Expected at least safelist files")
	}
}

func TestAnalyzerDuplicatesWithDifferentCasing(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "dupes.log")

	content := `/bin/bash
/bin/bash
/BIN/BASH
/bin/Bash
/lib/libc.so.6
/lib/libc.so.6
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Count occurrences of similar paths (case-sensitive)
	fileCount := make(map[string]int)
	for _, file := range manifest.Files {
		if strings.Contains(file, "/bin/bash") || strings.Contains(file, "/bin/Bash") || strings.Contains(file, "/BIN/BASH") {
			fileCount["bash"]++
		}
	}

	// Should dedupe exact matches
	if fileCount["bash"] > 2 {
		t.Logf("Note: Case-sensitive filesystem detected multiple bash entries")
	}
}

func TestAnalyzerSpecialCharacters(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "special.log")

	// Paths with special characters that might cause issues
	content := `/path/with/unicode/файл.txt
/path/with/emoji/🚀.txt
/path/with/quotes/"file".txt
/path/with/newline
continuation.txt
/path/with|pipe.txt
/path/with;semicolon.txt
`
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should handle these gracefully without crashing
	if len(manifest.Files) == 0 {
		t.Error("Expected at least safelist files")
	}
}

func TestAnalyzerVeryLongPath(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "long.log")

	// Create a very long path (typical PATH_MAX is 4096 on Linux)
	longPath := "/very/long/path/" + strings.Repeat("subdirectory/", 100) + "file.txt"
	content := longPath + "\n/etc/passwd\n"

	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should handle long paths without crashing
	if len(manifest.Files) == 0 {
		t.Error("Expected at least safelist files")
	}
}

func TestAnalyzerCircularSymlink(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a file and a symlink pointing to itself
	realFile := filepath.Join(tempDir, "real-file.txt")
	if err := os.WriteFile(realFile, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create real file: %v", err)
	}
	
	selfLink := filepath.Join(tempDir, "self-link")
	if err := os.Symlink(selfLink, selfLink); err != nil {
		t.Skipf("Cannot create self-referencing symlink: %v", err)
	}

	logFile := filepath.Join(tempDir, "circular.log")
	content := selfLink + "\n" + realFile + "\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create test log: %v", err)
	}

	a := NewAnalyzer()
	// Should not hang or crash on circular symlinks
	// Note: This may expose a bug in symlink resolution
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Logf("Analyze failed on circular symlink (expected): %v", err)
	}

	// Should still have safelist files even if circular link fails
	if len(manifest.Files) == 0 {
		t.Error("Expected at least safelist files")
	}
	
	// Verify the real file was processed
	hasRealFile := false
	for _, f := range manifest.Files {
		if strings.Contains(f, "real-file.txt") {
			hasRealFile = true
			break
		}
	}
	if !hasRealFile {
		t.Error("Expected real file to be in manifest")
	}
}

func TestAnalyzerNonELFBinary(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a shell script (not an ELF binary)
	scriptFile := filepath.Join(tempDir, "script.sh")
	scriptContent := "#!/bin/bash\necho 'Hello World'\n"
	if err := os.WriteFile(scriptFile, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("Failed to create script: %v", err)
	}

	// Check that it's correctly identified as non-ELF
	a := NewAnalyzer()
	isELF := a.isELFBinary(scriptFile)
	if isELF {
		t.Error("Shell script incorrectly identified as ELF binary")
	}
}

func TestAnalyzerPermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("Test requires non-root user")
	}

	tempDir := t.TempDir()
	
	// Create a file with no read permissions
	restrictedFile := filepath.Join(tempDir, "restricted.txt")
	if err := os.WriteFile(restrictedFile, []byte("secret"), 0000); err != nil {
		t.Fatalf("Failed to create restricted file: %v", err)
	}
	defer os.Chmod(restrictedFile, 0644) // Cleanup

	logFile := filepath.Join(tempDir, "restricted.log")
	content := restrictedFile + "\n/etc/passwd\n"
	if err := os.WriteFile(logFile, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to create log: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	// Should handle permission denied gracefully
	if len(manifest.Files) == 0 {
		t.Error("Expected at least safelist files")
	}
}

func TestAnalyzerLargeLogFile(t *testing.T) {
	tempDir := t.TempDir()
	logFile := filepath.Join(tempDir, "large.log")

	// Create a log with many entries
	var content strings.Builder
	for i := 0; i < 1000; i++ {
		content.WriteString(fmt.Sprintf("/tmp/file_%d.txt\n", i))
	}

	if err := os.WriteFile(logFile, []byte(content.String()), 0644); err != nil {
		t.Fatalf("Failed to create large log: %v", err)
	}

	a := NewAnalyzer()
	manifest, err := a.Analyze(logFile)
	if err != nil {
		t.Fatalf("Analyze failed on large log: %v", err)
	}

	// Should handle large files without issues (includes safelist + duplicates resolved)
	if len(manifest.Files) < 10 {
		t.Errorf("Expected many files from large log, got %d", len(manifest.Files))
	}
	
	t.Logf("Processed %d files from large log", len(manifest.Files))
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

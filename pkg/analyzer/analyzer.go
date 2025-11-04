package analyzer

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type FileManifest struct {
	Files []string
}

type Analyzer struct{}

func NewAnalyzer() *Analyzer {
	return &Analyzer{}
}

func (a *Analyzer) Analyze(logFilePath string) (*FileManifest, error) {
	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()

	fileSet := make(map[string]struct{})
	scanner := bufio.NewScanner(file)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			fileSet[line] = struct{}{}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading log file: %w", err)
	}

	files := a.resolveDependencies(fileSet)

	manifest := &FileManifest{
		Files: files,
	}

	return manifest, nil
}

func (a *Analyzer) resolveDependencies(fileSet map[string]struct{}) []string {
	resolved := make(map[string]struct{})
	
	for file := range fileSet {
		resolved[file] = struct{}{}
		
		a.resolveSymlinks(file, resolved)
		
		if a.isELFBinary(file) {
			a.resolveELFDependencies(file, resolved)
		}
	}
	
	a.addSafelistFiles(resolved)
	
	files := make([]string, 0, len(resolved))
	for file := range resolved {
		files = append(files, file)
	}
	
	return files
}

func (a *Analyzer) resolveSymlinks(file string, resolved map[string]struct{}) {
	target, err := filepath.EvalSymlinks(file)
	if err == nil && target != file {
		resolved[target] = struct{}{}
	}
}

func (a *Analyzer) isELFBinary(file string) bool {
	return strings.HasSuffix(file, ".so") || 
		   strings.Contains(file, ".so.") ||
		   (!strings.Contains(file, ".") && strings.HasPrefix(file, "/bin/")) ||
		   (!strings.Contains(file, ".") && strings.HasPrefix(file, "/usr/bin/"))
}

func (a *Analyzer) resolveELFDependencies(file string, resolved map[string]struct{}) {
}

func (a *Analyzer) addSafelistFiles(resolved map[string]struct{}) {
	safelistFiles := []string{
		"/etc/passwd",
		"/etc/group",
		"/etc/hosts",
		"/etc/resolv.conf",
		"/etc/nsswitch.conf",
		"/lib/x86_64-linux-gnu/libc.so.6",
		"/lib/x86_64-linux-gnu/libm.so.6",
		"/lib/x86_64-linux-gnu/libpthread.so.0",
		"/lib64/ld-linux-x86-64.so.2",
	}
	
	for _, file := range safelistFiles {
		resolved[file] = struct{}{}
	}
}

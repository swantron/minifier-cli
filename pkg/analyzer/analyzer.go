package analyzer

import (
	"bufio"
	"debug/elf"
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
	if _, err := os.Lstat(file); err != nil {
		return
	}

	target, err := os.Readlink(file)
	if err != nil {
		return
	}

	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(file), target)
	}

	target = filepath.Clean(target)
	if target != file {
		resolved[target] = struct{}{}
		a.resolveSymlinks(target, resolved)
	}
}

func (a *Analyzer) isELFBinary(file string) bool {
	f, err := os.Open(file)
	if err != nil {
		return false
	}
	defer f.Close()

	_, err = elf.NewFile(f)
	return err == nil
}

func (a *Analyzer) resolveELFDependencies(file string, resolved map[string]struct{}) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()

	elfFile, err := elf.NewFile(f)
	if err != nil {
		return
	}
	defer elfFile.Close()

	libs, err := elfFile.ImportedLibraries()
	if err != nil {
		return
	}

	ldPaths := []string{
		"/lib",
		"/lib64",
		"/usr/lib",
		"/usr/lib64",
		"/lib/x86_64-linux-gnu",
		"/usr/lib/x86_64-linux-gnu",
		"/lib/aarch64-linux-gnu",
		"/usr/lib/aarch64-linux-gnu",
	}

	for _, lib := range libs {
		for _, ldPath := range ldPaths {
			libPath := filepath.Join(ldPath, lib)
			if _, err := os.Stat(libPath); err == nil {
				resolved[libPath] = struct{}{}
				a.resolveSymlinks(libPath, resolved)
				a.resolveELFDependencies(libPath, resolved)
				break
			}
		}
	}

	interpreter := a.getELFInterpreter(elfFile)
	if interpreter != "" {
		resolved[interpreter] = struct{}{}
		a.resolveSymlinks(interpreter, resolved)
	}
}

func (a *Analyzer) getELFInterpreter(elfFile *elf.File) string {
	for _, prog := range elfFile.Progs {
		if prog.Type == elf.PT_INTERP {
			interp := make([]byte, prog.Filesz)
			if _, err := prog.ReadAt(interp, 0); err == nil {
				interpStr := string(interp)
				if idx := strings.IndexByte(interpStr, 0); idx != -1 {
					interpStr = interpStr[:idx]
				}
				return interpStr
			}
		}
	}
	return ""
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

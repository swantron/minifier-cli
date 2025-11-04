package tracer

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/swantron/minifier-cli/pkg/session"
)

type Tracer struct{}

func NewTracer() *Tracer {
	return &Tracer{}
}

func (t *Tracer) Start(image, name string, dockerArgs []string) (*session.Session, error) {
	logFile := filepath.Join(os.TempDir(), fmt.Sprintf("minifier-trace-%s.log", name))
	
	containerName := fmt.Sprintf("minifier-%s", name)
	
	args := []string{
		"run",
		"-d",
		"--name", containerName,
		"--cap-add", "SYS_ADMIN",
		"--cap-add", "SYS_PTRACE",
		"--security-opt", "apparmor=unconfined",
	}
	
	args = append(args, dockerArgs...)
	args = append(args, image)
	
	cmd := exec.Command("docker", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("failed to start container: %w\nOutput: %s", err, string(output))
	}
	
	containerID := strings.TrimSpace(string(output))
	
	if err := os.WriteFile(logFile, []byte{}, 0644); err != nil {
		exec.Command("docker", "rm", "-f", containerID).Run()
		return nil, fmt.Errorf("failed to create log file: %w", err)
	}
	
	// Wait a moment for container to fully start
	time.Sleep(500 * time.Millisecond)
	
	pid, err := t.getContainerPID(containerID)
	if err != nil {
		// Don't fail if we can't get PID - tracer will still work
		pid = 0
	}
	
	// Start background tracing
	go t.traceContainer(containerID, logFile)
	
	sess := &session.Session{
		Name:        name,
		Image:       image,
		ContainerID: containerID,
		TracerPID:   pid,
		LogFile:     logFile,
	}
	
	return sess, nil
}

func (t *Tracer) getContainerPID(containerID string) (int, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Pid}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}
	
	var pid int
	_, err = fmt.Sscanf(strings.TrimSpace(string(output)), "%d", &pid)
	return pid, err
}

func (t *Tracer) traceContainer(containerID, logFile string) error {
	defer func() {
		if r := recover(); r != nil {
			errorLog := strings.Replace(logFile, ".log", ".panic.log", 1)
			os.WriteFile(errorLog, []byte(fmt.Sprintf("PANIC: %v\n", r)), 0644)
		}
	}()
	
	// Create error log for debugging
	errorLog := strings.Replace(logFile, ".log", ".error.log", 1)
	errFile, err := os.Create(errorLog)
	if err != nil {
		return fmt.Errorf("failed to create error log: %w", err)
	}
	defer errFile.Close()
	defer fmt.Fprintf(errFile, "Tracer goroutine finished for %s\n", containerID)
	
	fmt.Fprintf(errFile, "Tracer goroutine STARTED for container %s\n", containerID)
	errFile.Sync()
	
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintf(errFile, "CRITICAL: Failed to open log file %s: %v\n", logFile, err)
		errFile.Sync()
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	
	fmt.Fprintf(errFile, "Log file opened successfully: %s\n", logFile)
	errFile.Sync()
	
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	
	seenFiles := make(map[string]struct{})
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	timeout := time.After(5 * time.Minute)
	
	fmt.Fprintf(errFile, "Entering main loop\n")
	errFile.Sync()
	
	iterationCount := 0
	for {
		select {
		case <-timeout:
			fmt.Fprintf(errFile, "Timeout reached after %d iterations\n", iterationCount)
			errFile.Sync()
			return nil
		case <-ticker.C:
			iterationCount++
			
			if iterationCount <= 3 {
				fmt.Fprintf(errFile, "Iteration %d started\n", iterationCount)
				errFile.Sync()
			}
			
			isRunning, err := t.isContainerRunning(containerID)
			if err != nil {
				fmt.Fprintf(errFile, "Error checking if running: %v\n", err)
				errFile.Sync()
				return nil
			}
			if !isRunning {
				fmt.Fprintf(errFile, "Container stopped after %d iterations\n", iterationCount)
				errFile.Sync()
				return nil
			}
			
			files, err := t.captureFileAccess(containerID)
			if err != nil {
				fmt.Fprintf(errFile, "Iteration %d: Error capturing files: %v\n", iterationCount, err)
				errFile.Sync()
				continue
			}
			
			if iterationCount <= 3 {
				fmt.Fprintf(errFile, "Iteration %d: Captured %d files\n", iterationCount, len(files))
				errFile.Sync()
			}
			
			newFiles := 0
			for _, filePath := range files {
				if filePath == "" || filePath == "/" {
					continue
				}
				
				if _, seen := seenFiles[filePath]; !seen {
					seenFiles[filePath] = struct{}{}
					writer.WriteString(filePath + "\n")
					newFiles++
				}
			}
			
			if newFiles > 0 {
				writer.Flush()
				fmt.Fprintf(errFile, "Iteration %d: Wrote %d new files (total: %d)\n", iterationCount, newFiles, len(seenFiles))
				errFile.Sync()
			}
		}
	}
}

func (t *Tracer) isContainerRunning(containerID string) (bool, error) {
	cmd := exec.Command("docker", "inspect", "--format", "{{.State.Running}}", containerID)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	
	return strings.TrimSpace(string(output)) == "true", nil
}

func (t *Tracer) captureFileAccess(containerID string) ([]string, error) {
	var allFiles []string
	
	// Method 1: Try lsof first (most comprehensive)
	files, err := t.captureLsofFiles(containerID)
	if err == nil && len(files) > 0 {
		return files, nil
	}
	
	// Method 2: Check /proc/*/fd for all processes
	files, err = t.captureProcFdFiles(containerID)
	if err == nil && len(files) > 0 {
		allFiles = append(allFiles, files...)
	}
	
	// Method 3: Get files from /proc/*/maps (memory mapped files)
	files, err = t.captureProcMapsFiles(containerID)
	if err == nil && len(files) > 0 {
		allFiles = append(allFiles, files...)
	}
	
	if len(allFiles) > 0 {
		return allFiles, nil
	}
	
	// Fallback to basic files
	return t.captureBasicFiles(containerID)
}

func (t *Tracer) captureProcFdFiles(containerID string) ([]string, error) {
	// Get all PIDs in the container
	cmd := exec.Command("docker", "exec", containerID, "sh", "-c", "find /proc -maxdepth 1 -type d -name '[0-9]*' 2>/dev/null")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	var files []string
	fileSet := make(map[string]struct{})
	
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		procDir := scanner.Text()
		fdDir := procDir + "/fd"
		
		// Find all file descriptors for this process
		cmd := exec.Command("docker", "exec", containerID, "sh", "-c", 
			fmt.Sprintf("find %s -type l 2>/dev/null", fdDir))
		fdOutput, err := cmd.Output()
		if err != nil {
			continue
		}
		
		fdScanner := bufio.NewScanner(strings.NewReader(string(fdOutput)))
		for fdScanner.Scan() {
			fdPath := fdScanner.Text()
			target, err := t.readContainerSymlink(containerID, fdPath)
			if err == nil && target != "" && 
			   !strings.HasPrefix(target, "/proc") && 
			   !strings.HasPrefix(target, "/dev") &&
			   !strings.Contains(target, "pipe:") &&
			   !strings.Contains(target, "socket:") {
				fileSet[target] = struct{}{}
			}
		}
	}
	
	for file := range fileSet {
		files = append(files, file)
	}
	
	return files, nil
}

func (t *Tracer) captureProcMapsFiles(containerID string) ([]string, error) {
	// Get all mapped files from /proc/*/maps
	cmd := exec.Command("docker", "exec", containerID, "sh", "-c",
		"cat /proc/*/maps 2>/dev/null | awk '{print $NF}' | grep '^/' | sort -u")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	
	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" && 
		   !strings.HasPrefix(line, "/proc") && 
		   !strings.HasPrefix(line, "/dev") &&
		   !strings.HasPrefix(line, "/sys") {
			files = append(files, line)
		}
	}
	
	return files, nil
}

func (t *Tracer) readContainerSymlink(containerID, path string) (string, error) {
	cmd := exec.Command("docker", "exec", containerID, "readlink", path)
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	
	return strings.TrimSpace(string(output)), nil
}

func (t *Tracer) captureLsofFiles(containerID string) ([]string, error) {
	cmd := exec.Command("docker", "exec", containerID, "sh", "-c", "lsof -F n 2>/dev/null | grep '^n/' | cut -c2-")
	output, err := cmd.Output()
	if err != nil {
		return t.captureBasicFiles(containerID)
	}
	
	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	pathRegex := regexp.MustCompile(`^/[^\s]+$`)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if pathRegex.MatchString(line) && !strings.HasPrefix(line, "/proc") && !strings.HasPrefix(line, "/dev") {
			files = append(files, line)
		}
	}
	
	return files, nil
}

func (t *Tracer) captureBasicFiles(containerID string) ([]string, error) {
	files := []string{
		"/bin/sh",
		"/lib/ld-musl-x86_64.so.1",
		"/lib/libc.musl-x86_64.so.1",
		"/etc/passwd",
		"/etc/group",
		"/etc/hosts",
		"/etc/resolv.conf",
	}
	
	var existing []string
	for _, file := range files {
		cmd := exec.Command("docker", "exec", containerID, "test", "-e", file)
		if cmd.Run() == nil {
			existing = append(existing, file)
		}
	}
	
	return existing, nil
}

func (t *Tracer) Stop(sess *session.Session) error {
	cmd := exec.Command("docker", "stop", sess.ContainerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to stop container: %w", err)
	}
	
	cmd = exec.Command("docker", "rm", sess.ContainerID)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to remove container: %w", err)
	}
	
	return nil
}

func (t *Tracer) Trace(containerID string, logFile string) error {
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	
	return nil
}

func terminateProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	
	return process.Signal(syscall.SIGTERM)
}

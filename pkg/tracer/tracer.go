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
	
	pid, err := t.getContainerPID(containerID)
	if err != nil {
		exec.Command("docker", "rm", "-f", containerID).Run()
		return nil, fmt.Errorf("failed to get container PID: %w", err)
	}
	
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
	file, err := os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	
	writer := bufio.NewWriter(file)
	defer writer.Flush()
	
	seenFiles := make(map[string]struct{})
	
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	
	timeout := time.After(5 * time.Minute)
	
	for {
		select {
		case <-timeout:
			return nil
		case <-ticker.C:
			isRunning, err := t.isContainerRunning(containerID)
			if err != nil || !isRunning {
				return nil
			}
			
			files, err := t.captureFileAccess(containerID)
			if err != nil {
				continue
			}
			
			for _, filePath := range files {
				if filePath == "" || filePath == "/" {
					continue
				}
				
				if _, seen := seenFiles[filePath]; !seen {
					seenFiles[filePath] = struct{}{}
					writer.WriteString(filePath + "\n")
					writer.Flush()
				}
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
	cmd := exec.Command("docker", "exec", containerID, "find", "/proc/self/fd", "-type", "l")
	output, err := cmd.Output()
	if err != nil {
		return t.captureLsofFiles(containerID)
	}
	
	var files []string
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		fdPath := scanner.Text()
		target, err := t.readContainerSymlink(containerID, fdPath)
		if err == nil && target != "" && !strings.HasPrefix(target, "/proc") && !strings.HasPrefix(target, "/dev") {
			files = append(files, target)
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

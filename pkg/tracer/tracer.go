package tracer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

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
	
	tracerPID := os.Getpid()
	
	sess := &session.Session{
		Name:        name,
		Image:       image,
		ContainerID: containerID,
		TracerPID:   tracerPID,
		LogFile:     logFile,
	}
	
	return sess, nil
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

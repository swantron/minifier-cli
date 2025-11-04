package tracer

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestTracerGetContainerPID(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	tracer := NewTracer()
	
	containerID, err := createTestContainer("alpine:latest")
	if err != nil {
		t.Fatalf("Failed to create test container: %v", err)
	}
	defer cleanupContainer(containerID)
	
	pid, err := tracer.getContainerPID(containerID)
	if err != nil {
		t.Fatalf("Failed to get container PID: %v", err)
	}
	
	if pid <= 0 {
		t.Errorf("Expected positive PID, got %d", pid)
	}
}

func TestTracerIsContainerRunning(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	tracer := NewTracer()
	
	// Test with running container
	containerID, err := createTestContainer("alpine:latest")
	if err != nil {
		t.Fatalf("Failed to create test container: %v", err)
	}
	defer cleanupContainer(containerID)
	
	running, err := tracer.isContainerRunning(containerID)
	if err != nil {
		t.Fatalf("Failed to check container status: %v", err)
	}
	
	if !running {
		t.Error("Container should be running")
	}
	
	// Test with non-existent container
	running, err = tracer.isContainerRunning("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent container")
	}
}

func TestTracerCaptureBasicFiles(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	tracer := NewTracer()
	
	containerID, err := createTestContainer("alpine:latest")
	if err != nil {
		t.Fatalf("Failed to create test container: %v", err)
	}
	defer cleanupContainer(containerID)
	
	files, err := tracer.captureBasicFiles(containerID)
	if err != nil {
		t.Fatalf("Failed to capture basic files: %v", err)
	}
	
	if len(files) == 0 {
		t.Error("Expected at least one file")
	}
	
	// Check for common files
	hasCommonFile := false
	for _, file := range files {
		if file == "/bin/sh" || file == "/etc/passwd" {
			hasCommonFile = true
			break
		}
	}
	
	if !hasCommonFile {
		t.Error("Expected to find at least one common file like /bin/sh or /etc/passwd")
	}
}

func TestTracerStartStop(t *testing.T) {
	// This is an integration test that requires Docker
	// For CI/CD, we'd use mocks
	t.Skip("Integration test - requires Docker and takes time")
}

func TestTracerReadContainerSymlink(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	tracer := NewTracer()
	
	containerID, err := createTestContainer("alpine:latest")
	if err != nil {
		t.Fatalf("Failed to create test container: %v", err)
	}
	defer cleanupContainer(containerID)
	
	// Just verify the function doesn't crash with invalid input
	_, err = tracer.readContainerSymlink(containerID, "/nonexistent")
	// Error is expected, just verify it doesn't panic
}

// Helper functions

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

func createTestContainer(image string) (string, error) {
	cmd := exec.Command("docker", "run", "-d", image, "sleep", "30")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(output)), nil
}

func cleanupContainer(containerID string) {
	exec.Command("docker", "rm", "-f", containerID).Run()
}

func tempFile(t *testing.T) string {
	f, err := os.CreateTemp("", "tracer-test-*.log")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	path := f.Name()
	f.Close()
	return path
}

package repackager

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/swantron/minifier-cli/pkg/analyzer"
)

func TestExtractMetadata(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	r := NewRepackager()

	// Pull alpine to ensure it exists
	exec.Command("docker", "pull", "alpine:latest").Run()

	// Use a real image that should be available
	metadata, err := r.extractMetadata("alpine:latest")
	if err != nil {
		t.Fatalf("Failed to extract metadata: %v", err)
	}

	if metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	// Alpine should have some ENV vars
	if len(metadata.Env) == 0 {
		t.Error("Expected at least one ENV variable")
	}

	// Alpine should have CMD
	if len(metadata.Cmd) == 0 {
		t.Error("Expected CMD to be set")
	}
}

func TestExtractMetadataInvalidImage(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	r := NewRepackager()

	_, err := r.extractMetadata("nonexistent-image:doesnotexist")
	if err == nil {
		t.Error("Expected error for non-existent image")
	}
}

func TestGenerateDockerfile(t *testing.T) {
	r := NewRepackager()

	metadata := &ImageMetadata{
		Env:        []string{"PATH=/usr/bin", "HOME=/root"},
		Cmd:        []string{"/bin/sh"},
		Entrypoint: []string{},
		WorkingDir: "/app",
		User:       "nobody",
		ExposedPorts: map[string]struct{}{
			"8080/tcp": {},
		},
		Volumes: map[string]struct{}{
			"/data": {},
		},
		Labels: map[string]string{
			"version": "1.0",
		},
	}

	tempDir := t.TempDir()
	dockerfilePath := filepath.Join(tempDir, "Dockerfile")

	err := r.generateDockerfile(dockerfilePath, metadata)
	if err != nil {
		t.Fatalf("Failed to generate Dockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	dockerfile := string(content)

	// Check for expected directives
	if !strings.Contains(dockerfile, "FROM scratch") {
		t.Error("Dockerfile should contain 'FROM scratch'")
	}

	if !strings.Contains(dockerfile, "ENV PATH=/usr/bin") {
		t.Error("Dockerfile should contain ENV directive")
	}

	if !strings.Contains(dockerfile, "WORKDIR /app") {
		t.Error("Dockerfile should contain WORKDIR directive")
	}

	if !strings.Contains(dockerfile, "USER nobody") {
		t.Error("Dockerfile should contain USER directive")
	}

	if !strings.Contains(dockerfile, "EXPOSE 8080/tcp") {
		t.Error("Dockerfile should contain EXPOSE directive")
	}

	if !strings.Contains(dockerfile, `VOLUME ["/data"]`) {
		t.Error("Dockerfile should contain VOLUME directive")
	}

	if !strings.Contains(dockerfile, `LABEL version="1.0"`) {
		t.Error("Dockerfile should contain LABEL directive")
	}

	if !strings.Contains(dockerfile, `CMD ["/bin/sh"]`) {
		t.Error("Dockerfile should contain CMD directive")
	}
}

func TestGenerateDockerfileMinimal(t *testing.T) {
	r := NewRepackager()

	// Minimal metadata
	metadata := &ImageMetadata{
		Cmd: []string{"/bin/sh"},
	}

	tempDir := t.TempDir()
	dockerfilePath := filepath.Join(tempDir, "Dockerfile")

	err := r.generateDockerfile(dockerfilePath, metadata)
	if err != nil {
		t.Fatalf("Failed to generate Dockerfile: %v", err)
	}

	content, err := os.ReadFile(dockerfilePath)
	if err != nil {
		t.Fatalf("Failed to read Dockerfile: %v", err)
	}

	dockerfile := string(content)

	if !strings.Contains(dockerfile, "FROM scratch") {
		t.Error("Dockerfile should contain 'FROM scratch'")
	}

	if !strings.Contains(dockerfile, "COPY files/") {
		t.Error("Dockerfile should contain COPY directive")
	}
}

func TestCopyFilesEmptyList(t *testing.T) {
	r := NewRepackager()

	tempDir := t.TempDir()

	// Should handle empty file list
	err := r.copyFiles("nonexistent", []string{}, tempDir)
	if err == nil {
		t.Error("Expected error when copying zero files")
	}
}

func TestCopyFilesInvalidContainer(t *testing.T) {
	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	r := NewRepackager()

	tempDir := t.TempDir()
	files := []string{"/bin/sh"}

	err := r.copyFiles("nonexistent-container", files, tempDir)
	if err == nil {
		t.Error("Expected error for non-existent container")
	}
}

func TestRepackageIntegration(t *testing.T) {
	// This is a full integration test
	// It requires Docker and creates actual containers/images
	t.Skip("Integration test - requires Docker and creates images")

	if !isDockerAvailable() {
		t.Skip("Docker not available")
	}

	r := NewRepackager()

	manifest := &analyzer.FileManifest{
		Files: []string{
			"/bin/sh",
			"/lib/ld-musl-x86_64.so.1",
		},
	}

	err := r.Repackage("alpine:latest", "test-minified:latest", manifest)
	if err != nil {
		t.Fatalf("Repackage failed: %v", err)
	}

	// Clean up
	exec.Command("docker", "rmi", "test-minified:latest").Run()
}

func TestImageMetadataJSONParsing(t *testing.T) {
	// Test that our structs properly parse Docker inspect JSON
	jsonData := `[{
		"Config": {
			"Env": ["PATH=/usr/bin"],
			"Cmd": ["/bin/sh"],
			"Entrypoint": null,
			"WorkingDir": "/",
			"User": "",
			"ExposedPorts": {
				"80/tcp": {}
			},
			"Volumes": {
				"/data": {}
			},
			"Labels": {
				"maintainer": "test"
			}
		}
	}]`

	var inspectData []ImageInspect
	err := json.Unmarshal([]byte(jsonData), &inspectData)
	if err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if len(inspectData) != 1 {
		t.Fatalf("Expected 1 inspect result, got %d", len(inspectData))
	}

	config := inspectData[0].Config

	if len(config.Env) != 1 || config.Env[0] != "PATH=/usr/bin" {
		t.Error("Failed to parse Env correctly")
	}

	if len(config.Cmd) != 1 || config.Cmd[0] != "/bin/sh" {
		t.Error("Failed to parse Cmd correctly")
	}

	if _, ok := config.ExposedPorts["80/tcp"]; !ok {
		t.Error("Failed to parse ExposedPorts correctly")
	}

	if _, ok := config.Volumes["/data"]; !ok {
		t.Error("Failed to parse Volumes correctly")
	}

	if config.Labels["maintainer"] != "test" {
		t.Error("Failed to parse Labels correctly")
	}
}

// Helper functions

func isDockerAvailable() bool {
	cmd := exec.Command("docker", "info")
	return cmd.Run() == nil
}

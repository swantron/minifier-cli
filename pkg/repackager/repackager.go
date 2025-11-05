package repackager

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swantron/minifier-cli/pkg/analyzer"
)

type Repackager struct{}

type ImageMetadata struct {
	Cmd          []string            `json:"Cmd"`
	Entrypoint   []string            `json:"Entrypoint"`
	Env          []string            `json:"Env"`
	WorkingDir   string              `json:"WorkingDir"`
	User         string              `json:"User"`
	ExposedPorts map[string]struct{} `json:"ExposedPorts"`
	Volumes      map[string]struct{} `json:"Volumes"`
	Labels       map[string]string   `json:"Labels"`
}

type ImageInspect struct {
	Config ImageMetadata `json:"Config"`
}

func NewRepackager() *Repackager {
	return &Repackager{}
}

func (r *Repackager) Repackage(sourceImage, outputImage string, manifest *analyzer.FileManifest) error {
	tempDir, err := os.MkdirTemp("", "minifier-build-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	containerName := fmt.Sprintf("minifier-temp-%d", os.Getpid())

	cmd := exec.Command("docker", "create", "--name", containerName, sourceImage)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create temporary container: %w", err)
	}
	defer exec.Command("docker", "rm", containerName).Run()

	filesDir := filepath.Join(tempDir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("failed to create files directory: %w", err)
	}

	if err := r.copyFiles(containerName, manifest.Files, filesDir); err != nil {
		return fmt.Errorf("failed to copy files: %w", err)
	}

	metadata, err := r.extractMetadata(sourceImage)
	if err != nil {
		return fmt.Errorf("failed to extract metadata: %w", err)
	}

	dockerfilePath := filepath.Join(tempDir, "Dockerfile")
	if err := r.generateDockerfile(dockerfilePath, metadata); err != nil {
		return fmt.Errorf("failed to generate Dockerfile: %w", err)
	}

	cmd = exec.Command("docker", "build", "-t", outputImage, tempDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build image: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *Repackager) copyFiles(containerName string, files []string, destDir string) error {
	successCount := 0
	for _, file := range files {
		if file == "" || file == "/" {
			continue
		}

		destPath := filepath.Join(destDir, file)
		destFileDir := filepath.Dir(destPath)

		if err := os.MkdirAll(destFileDir, 0755); err != nil {
			continue
		}

		srcPath := fmt.Sprintf("%s:%s", containerName, file)
		cmd := exec.Command("docker", "cp", srcPath, destPath)
		if err := cmd.Run(); err == nil {
			successCount++
		}
	}

	if successCount == 0 {
		return fmt.Errorf("failed to copy any files from container")
	}

	return nil
}

func (r *Repackager) extractMetadata(image string) (*ImageMetadata, error) {
	cmd := exec.Command("docker", "inspect", image)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to inspect image: %w", err)
	}

	var inspectData []ImageInspect
	if err := json.Unmarshal(output, &inspectData); err != nil {
		return nil, fmt.Errorf("failed to parse inspect output: %w", err)
	}

	if len(inspectData) == 0 {
		return nil, fmt.Errorf("no inspect data returned for image")
	}

	return &inspectData[0].Config, nil
}

func (r *Repackager) generateDockerfile(path string, metadata *ImageMetadata) error {
	var content strings.Builder

	content.WriteString("FROM scratch\n\n")

	content.WriteString("COPY files/ /\n\n")

	if len(metadata.Env) > 0 {
		for _, env := range metadata.Env {
			content.WriteString(fmt.Sprintf("ENV %s\n", env))
		}
		content.WriteString("\n")
	}

	if len(metadata.Labels) > 0 {
		for key, value := range metadata.Labels {
			content.WriteString(fmt.Sprintf("LABEL %s=\"%s\"\n", key, value))
		}
		content.WriteString("\n")
	}

	if len(metadata.ExposedPorts) > 0 {
		ports := make([]string, 0, len(metadata.ExposedPorts))
		for port := range metadata.ExposedPorts {
			ports = append(ports, port)
		}
		content.WriteString(fmt.Sprintf("EXPOSE %s\n\n", strings.Join(ports, " ")))
	}

	if len(metadata.Volumes) > 0 {
		for volume := range metadata.Volumes {
			content.WriteString(fmt.Sprintf("VOLUME [\"%s\"]\n", volume))
		}
		content.WriteString("\n")
	}

	if metadata.WorkingDir != "" {
		content.WriteString(fmt.Sprintf("WORKDIR %s\n\n", metadata.WorkingDir))
	}

	if metadata.User != "" {
		content.WriteString(fmt.Sprintf("USER %s\n\n", metadata.User))
	}

	if len(metadata.Entrypoint) > 0 {
		entrypointJSON, _ := json.Marshal(metadata.Entrypoint)
		content.WriteString(fmt.Sprintf("ENTRYPOINT %s\n", string(entrypointJSON)))
	}

	if len(metadata.Cmd) > 0 {
		cmdJSON, _ := json.Marshal(metadata.Cmd)
		content.WriteString(fmt.Sprintf("CMD %s\n", string(cmdJSON)))
	}

	return os.WriteFile(path, []byte(content.String()), 0644)
}

package repackager

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/swantron/minifier-cli/pkg/analyzer"
)

type Repackager struct{}

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
	for _, file := range files {
		destPath := filepath.Join(destDir, file)
		destFileDir := filepath.Dir(destPath)
		
		if err := os.MkdirAll(destFileDir, 0755); err != nil {
			continue
		}

		srcPath := fmt.Sprintf("%s:%s", containerName, file)
		cmd := exec.Command("docker", "cp", srcPath, destPath)
		cmd.Run()
	}
	
	return nil
}

func (r *Repackager) extractMetadata(image string) (map[string]string, error) {
	cmd := exec.Command("docker", "inspect", image, "--format", "{{.Config.Cmd}}|{{.Config.Entrypoint}}|{{.Config.WorkingDir}}|{{.Config.User}}")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	parts := strings.Split(strings.TrimSpace(string(output)), "|")
	metadata := map[string]string{
		"cmd":        parts[0],
		"entrypoint": parts[1],
		"workdir":    parts[2],
		"user":       parts[3],
	}

	return metadata, nil
}

func (r *Repackager) generateDockerfile(path string, metadata map[string]string) error {
	content := "FROM scratch\n\n"
	content += "COPY files/ /\n\n"

	if metadata["workdir"] != "" {
		content += fmt.Sprintf("WORKDIR %s\n", metadata["workdir"])
	}

	if metadata["user"] != "" {
		content += fmt.Sprintf("USER %s\n", metadata["user"])
	}

	if metadata["entrypoint"] != "" && metadata["entrypoint"] != "[]" {
		content += fmt.Sprintf("ENTRYPOINT %s\n", metadata["entrypoint"])
	}

	if metadata["cmd"] != "" && metadata["cmd"] != "[]" {
		content += fmt.Sprintf("CMD %s\n", metadata["cmd"])
	}

	return os.WriteFile(path, []byte(content), 0644)
}

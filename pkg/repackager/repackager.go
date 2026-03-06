package repackager

import (
	"archive/tar"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
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

func randomSuffix() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (r *Repackager) Repackage(sourceImage, outputImage string, manifest *analyzer.FileManifest) error {
	tempDir, err := os.MkdirTemp("", "minifier-build-")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	containerName := fmt.Sprintf("minifier-temp-%s", randomSuffix())

	cmd := exec.Command("docker", "create", "--name", containerName, sourceImage)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create temporary container: %w", err)
	}
	defer func() { _ = exec.Command("docker", "rm", containerName).Run() }()

	filesDir := filepath.Join(tempDir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		return fmt.Errorf("failed to create files directory: %w", err)
	}

	copied, missing, err := r.copyFiles(containerName, manifest.Files, filesDir)
	if err != nil {
		return fmt.Errorf("failed to copy files: %w", err)
	}
	if len(missing) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: %d files from trace log not found in container:\n", len(missing))
		for _, f := range missing {
			fmt.Fprintf(os.Stderr, "  %s\n", f)
		}
	}
	if copied == 0 {
		return fmt.Errorf("failed to copy any files from container")
	}

	// Second pass: resolve ELF dependencies against the extracted filesystem.
	// This correctly resolves shared library paths for the container's architecture
	// rather than the host's.
	a := analyzer.NewAnalyzerWithRoot(filesDir)
	fileSet := make(map[string]struct{})
	for _, f := range manifest.Files {
		fileSet[f] = struct{}{}
	}
	additionalFiles := a.ResolveDependencies(fileSet)

	existingSet := make(map[string]struct{})
	for _, f := range manifest.Files {
		existingSet[f] = struct{}{}
	}
	var newFiles []string
	for _, f := range additionalFiles {
		if _, ok := existingSet[f]; !ok {
			newFiles = append(newFiles, f)
		}
	}
	if len(newFiles) > 0 {
		fmt.Printf("Resolved %d additional dependencies via ELF analysis\n", len(newFiles))
		if _, _, err := r.copyFiles(containerName, newFiles, filesDir); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: ELF dependency copy incomplete: %v\n", err)
		}
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

// copyFiles uses docker export to stream the entire container filesystem as a tar
// and extract only the requested files in a single pass, avoiding one subprocess
// per file.
func (r *Repackager) copyFiles(containerName string, files []string, destDir string) (copied int, missing []string, err error) {
	if len(files) == 0 {
		return 0, nil, fmt.Errorf("no files requested")
	}

	// Build a lookup set with both slash-prefixed and unprefixed forms.
	wanted := make(map[string]bool) // container path -> found?
	for _, f := range files {
		if f == "" || f == "/" {
			continue
		}
		wanted[f] = false
	}

	cmd := exec.Command("docker", "export", containerName)
	stdout, pipeErr := cmd.StdoutPipe()
	if pipeErr != nil {
		return 0, nil, fmt.Errorf("failed to create pipe: %w", pipeErr)
	}
	cmd.Stderr = os.Stderr

	if startErr := cmd.Start(); startErr != nil {
		return 0, nil, fmt.Errorf("failed to start docker export: %w", startErr)
	}

	tr := tar.NewReader(stdout)
	for {
		header, tarErr := tr.Next()
		if tarErr == io.EOF {
			break
		}
		if tarErr != nil {
			_ = cmd.Wait()
			return copied, nil, fmt.Errorf("error reading tar stream: %w", tarErr)
		}

		// Normalize: tar entries are "./path/to/file", map to "/path/to/file"
		name := header.Name
		name = strings.TrimPrefix(name, "./")
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		name = filepath.Clean(name)

		if _, ok := wanted[name]; !ok {
			continue
		}
		wanted[name] = true

		destPath := filepath.Join(destDir, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if mkErr := os.MkdirAll(destPath, os.FileMode(header.Mode)|0111); mkErr == nil {
				copied++
			}
		case tar.TypeReg:
			if mkErr := os.MkdirAll(filepath.Dir(destPath), 0755); mkErr != nil {
				continue
			}
			f, openErr := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
			if openErr != nil {
				continue
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				continue
			}
			f.Close()
			copied++
		case tar.TypeSymlink:
			if mkErr := os.MkdirAll(filepath.Dir(destPath), 0755); mkErr != nil {
				continue
			}
			os.Remove(destPath) // remove stale link if present
			if linkErr := os.Symlink(header.Linkname, destPath); linkErr == nil {
				copied++
			}
		case tar.TypeLink:
			// Hard link — copy the target file instead
			srcPath := filepath.Join(destDir, filepath.Clean("/"+header.Linkname))
			if mkErr := os.MkdirAll(filepath.Dir(destPath), 0755); mkErr != nil {
				continue
			}
			if linkErr := os.Link(srcPath, destPath); linkErr != nil {
				// Fall back to copy if hard link fails (e.g. cross-device)
				srcFile, openErr := os.Open(srcPath)
				if openErr != nil {
					continue
				}
				dstFile, openErr := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(header.Mode))
				if openErr != nil {
					srcFile.Close()
					continue
				}
				_, _ = io.Copy(dstFile, srcFile)
				srcFile.Close()
				dstFile.Close()
			}
			copied++
		}
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		return copied, nil, fmt.Errorf("docker export failed: %w", waitErr)
	}

	for path, found := range wanted {
		if !found {
			missing = append(missing, path)
		}
	}

	return copied, missing, nil
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
			parts := strings.SplitN(env, "=", 2)
			if len(parts) == 2 {
				// Always quote the value to handle spaces and special characters.
				escapedVal := strings.ReplaceAll(parts[1], `\`, `\\`)
				escapedVal = strings.ReplaceAll(escapedVal, `"`, `\"`)
				content.WriteString(fmt.Sprintf("ENV %s=\"%s\"\n", parts[0], escapedVal))
			} else {
				content.WriteString(fmt.Sprintf("ENV %s\n", env))
			}
		}
		content.WriteString("\n")
	}

	if len(metadata.Labels) > 0 {
		for key, value := range metadata.Labels {
			escapedVal := strings.ReplaceAll(value, `\`, `\\`)
			escapedVal = strings.ReplaceAll(escapedVal, `"`, `\"`)
			content.WriteString(fmt.Sprintf("LABEL %s=\"%s\"\n", key, escapedVal))
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

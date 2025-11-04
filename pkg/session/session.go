package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Session struct {
	Name        string `json:"name"`
	Image       string `json:"image"`
	ContainerID string `json:"container_id"`
	TracerPID   int    `json:"tracer_pid"`
	LogFile     string `json:"log_file"`
}

func getSessionPath(name string) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("minifier-session-%s.json", name))
}

func Save(sess *Session) error {
	data, err := json.Marshal(sess)
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	path := getSessionPath(sess.Name)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	return nil
}

func Load(name string) (*Session, error) {
	path := getSessionPath(name)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session '%s' not found", name)
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var sess Session
	if err := json.Unmarshal(data, &sess); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &sess, nil
}

func Delete(name string) error {
	path := getSessionPath(name)
	return os.Remove(path)
}

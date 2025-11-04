package session

import (
	"testing"
)

func TestSessionSaveAndLoad(t *testing.T) {
	sess := &Session{
		Name:        "test-session",
		Image:       "test-image:latest",
		ContainerID: "abc123",
		TracerPID:   12345,
		LogFile:     "/tmp/test.log",
	}

	err := Save(sess)
	if err != nil {
		t.Fatalf("Failed to save session: %v", err)
	}
	defer Delete("test-session")

	loaded, err := Load("test-session")
	if err != nil {
		t.Fatalf("Failed to load session: %v", err)
	}

	if loaded.Name != sess.Name {
		t.Errorf("Expected name %s, got %s", sess.Name, loaded.Name)
	}
	if loaded.Image != sess.Image {
		t.Errorf("Expected image %s, got %s", sess.Image, loaded.Image)
	}
	if loaded.ContainerID != sess.ContainerID {
		t.Errorf("Expected container ID %s, got %s", sess.ContainerID, loaded.ContainerID)
	}
}

func TestSessionLoadNonExistent(t *testing.T) {
	_, err := Load("nonexistent-session")
	if err == nil {
		t.Fatal("Expected error when loading nonexistent session")
	}
}

func TestSessionDelete(t *testing.T) {
	sess := &Session{
		Name:        "delete-test",
		Image:       "test:latest",
		ContainerID: "xyz789",
		TracerPID:   99999,
		LogFile:     "/tmp/delete-test.log",
	}

	Save(sess)
	
	err := Delete("delete-test")
	if err != nil {
		t.Fatalf("Failed to delete session: %v", err)
	}

	_, err = Load("delete-test")
	if err == nil {
		t.Fatal("Session should not exist after deletion")
	}
}

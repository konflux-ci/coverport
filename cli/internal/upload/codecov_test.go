package upload

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNewCodecovUploader_EmptyToken(t *testing.T) {
	_, err := NewCodecovUploader("")
	if err == nil {
		t.Fatal("expected error for empty token")
	}
}

func TestNewCodecovUploader_CodecovNotInPath(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	uploader, err := NewCodecovUploader("test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uploader.codecovPath != "" {
		t.Errorf("expected empty codecovPath, got %q", uploader.codecovPath)
	}
	if uploader.downloadedCLI != false {
		t.Error("expected downloadedCLI=false")
	}
}

func TestNewCodecovUploader_CodecovInPath(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "codecov")

	if err := os.WriteFile(fakeBin, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake codecov: %v", err)
	}

	t.Setenv("PATH", tmpDir)

	// Verify our fake binary is discoverable
	path, err := exec.LookPath("codecov")
	if err != nil {
		t.Skipf("LookPath can't find fake binary (platform issue): %v", err)
	}

	uploader, err := NewCodecovUploader("test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if uploader.codecovPath != path {
		t.Errorf("expected codecovPath=%q, got %q", path, uploader.codecovPath)
	}
}

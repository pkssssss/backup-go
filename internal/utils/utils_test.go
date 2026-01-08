package utils

import (
	"os"
	"strings"
	"testing"
)

func TestGetCurrentWorkingDir(t *testing.T) {
	wd := GetCurrentWorkingDir()
	if wd == "unknown" {
		t.Error("Expected valid working directory, got 'unknown'")
	}
	
	realWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get real wd: %v", err)
	}

	if wd != realWd {
		t.Errorf("Expected %s, got %s", realWd, wd)
	}
}

func TestGetCurrentExecutablePath(t *testing.T) {
	exe := GetCurrentExecutablePath()
	if exe == "unknown" {
		t.Error("Expected valid executable path, got 'unknown'")
	}
	// Note: Running 'go test' creates a temporary binary, so we just check it's not empty/unknown
	if !strings.Contains(exe, os.TempDir()) && !strings.Contains(exe, "/var/folders") && !strings.Contains(exe, "backup-go") {
		// This check is loose because location depends on how go test runs
		t.Logf("Executable path found: %s", exe)
	}
}

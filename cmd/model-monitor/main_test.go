package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunRejectsInvalidCommands(t *testing.T) {
	tests := [][]string{{}, {"unknown"}, {"mock"}, {"mock", "start"}}
	for _, args := range tests {
		if err := run(args); err == nil {
			t.Fatalf("run(%v) unexpectedly succeeded", args)
		}
	}
}

func TestRunPrivacyCheck(t *testing.T) {
	if err := privacyCheck(); err != nil {
		t.Fatal(err)
	}
}

func TestRunStatusOperationalError(t *testing.T) {
	t.Setenv("MODEL_MONITOR_DATABASE", filepath.Join(t.TempDir(), "status.db"))
	if err := status(); err != nil {
		t.Fatal(err)
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv("MODEL_MONITOR_DATABASE", "")
	if err := status(); err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("error = %v", err)
	}
	_ = os.Unsetenv("MODEL_MONITOR_DATABASE")
}

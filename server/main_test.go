package main

import (
	"strings"
	"testing"
)

func TestExecuteSafeCommand_Allowlist(t *testing.T) {
	// Should fail: not in allowlist
	_, err := ExecuteSafeCommand("test-req", "rm", []string{"-rf", "/"}, nil)
	if err == nil {
		t.Fatal("Expected error for 'rm', got nil")
	}
	if !strings.Contains(err.Error(), "PERMISSION DENIED") {
		t.Errorf("Expected PERMISSION DENIED, got: %v", err)
	}

	// Should pass permit check (but might fail execution depending on environment, which is fine, we just want to ensure it doesn't return the permission denied error)
	_, err = ExecuteSafeCommand("test-req", "echo", []string{"hello"}, nil)
	if err != nil && strings.Contains(err.Error(), "PERMISSION DENIED") {
		t.Errorf("Did not expect PERMISSION DENIED for 'echo', got: %v", err)
	}
}

func TestExecuteSafeCommand_Sanitization(t *testing.T) {
	// 1. Shell characters
	_, err := ExecuteSafeCommand("test-req", "echo", []string{"hello", "&&", "ls"}, nil)
	if err == nil || !strings.Contains(err.Error(), "illegal characters") {
		t.Errorf("Expected illegal characters error for '&&', got: %v", err)
	}

	_, err = ExecuteSafeCommand("test-req", "echo", []string{"hi", ">", "file.txt"}, nil)
	if err == nil || !strings.Contains(err.Error(), "illegal characters") {
		t.Errorf("Expected illegal characters error for '>', got: %v", err)
	}

	// 2. Path Traversal
	_, err = ExecuteSafeCommand("test-req", "cat", []string{"../../etc/passwd"}, nil)
	if err == nil || !strings.Contains(err.Error(), "Path traversal (..)") {
		t.Errorf("Expected path traversal error, got: %v", err)
	}

	// 3. Absolute Paths
	_, err = ExecuteSafeCommand("test-req", "ls", []string{"/var/log"}, nil)
	if err == nil || !strings.Contains(err.Error(), "Absolute paths are not allowed") {
		t.Errorf("Expected absolute path error, got: %v", err)
	}
}

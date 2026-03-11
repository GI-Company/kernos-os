package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStripThinkTags(t *testing.T) {
	// ==========================================
	// Test Case 1: Valid <think> block
	// ==========================================
	t.Run("ValidThinkBlock", func(t *testing.T) {
		input := "<think>\nThinking about the rules...\n</think>\n1. Do not use rm -rf\n2. Be safe"
		expected := "1. Do not use rm -rf\n2. Be safe"
		result := stripThinkTags(input)

		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})

	// ==========================================
	// Test Case 2: No <think> block
	// ==========================================
	t.Run("NoThinkBlock", func(t *testing.T) {
		input := "1. Do not use rm -rf\n2. Be safe"
		expected := "1. Do not use rm -rf\n2. Be safe"
		result := stripThinkTags(input)

		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	})
}

func TestLoadSynapticWeights(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatalf("Could not get user home dir: %v", err)
	}

	kernosDir := filepath.Join(home, ".kernos")
	os.MkdirAll(kernosDir, 0755)
	
	weightsPath := filepath.Join(kernosDir, SynapticWeightsFilename)
	
	// Create a backup of existing weights if they exist
	backupContents := ""
	if data, err := os.ReadFile(weightsPath); err == nil {
		backupContents = string(data)
	}

	defer func() {
		if backupContents != "" {
			os.WriteFile(weightsPath, []byte(backupContents), 0644)
		} else {
			os.Remove(weightsPath)
		}
	}()

	// ==========================================
	// Test Case 1: Empty or nonexistent file
	// ==========================================
	t.Run("EmptyFile", func(t *testing.T) {
		os.Remove(weightsPath)
		result := LoadSynapticWeights()
		if result != "" {
			t.Errorf("Expected empty string, got %q", result)
		}
	})

	// ==========================================
	// Test Case 2: File with weights
	// ==========================================
	t.Run("ExistingWeights", func(t *testing.T) {
		os.WriteFile(weightsPath, []byte("1. Be cautious."), 0644)
		result := LoadSynapticWeights()
		if !strings.Contains(result, "=== SYNAPTIC OVERRIDE") {
			t.Errorf("Expected override header, got %q", result)
		}
		if !strings.Contains(result, "1. Be cautious.") {
			t.Errorf("Expected weights text, got %q", result)
		}
	})
}

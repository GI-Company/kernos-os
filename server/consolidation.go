package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SynapticConsolidator handles the Nightly RLHF processing.
// It retrieves failed/aborted DAGs from the SQLite database and uses the
// Architect Agent (Qwen-Thinking) to synthesize a set of "Lessons Learned".
// These lessons are saved to a weights file and injected into the Agent roster on boot.

const SynapticWeightsFilename = "synaptic_weights.txt"

func RunSynapticConsolidation(lmURL string) {
	log.Println("[RLHF] 🧠 Starting Synaptic Consolidation (REM Sleep)...")

	if GlobalTelemetry == nil {
		log.Println("[RLHF] ❌ Telemetry not initialized. Aborting.")
		return
	}

	// Phase 14: Trigger Hippocampus Vector Decay
	if GlobalVectorDB != nil {
		GlobalVectorDB.DecayWeights()
	}

	// Fetch failures from the last 24 hours
	since := time.Now().Add(-24 * time.Hour)
	failures, err := GlobalTelemetry.FetchFailedExecutions(since)
	if err != nil {
		log.Printf("[RLHF] ❌ Error fetching telemetry: %v", err)
		return
	}

	if len(failures) == 0 {
		log.Println("[RLHF] 💤 No failed tasks to consolidate. Restful sleep.")
		return
	}

	log.Printf("[RLHF] Found %d failed task sequences. Synthesizing lessons...", len(failures))

	// Construct the prompt for the Architect
	promptBuilder := strings.Builder{}
	promptBuilder.WriteString("Analyze the following failed or aborted OS task executions from the past 24 hours.\n")
	promptBuilder.WriteString("Synthesize a concise list of 3-5 'Lessons Learned' or 'Developer Preferences' to add to the system prompt so we avoid these mistakes in the future.\n")
	promptBuilder.WriteString("Output ONLY the actionable rules (e.g. 'Never use rm -rf', 'Always use verbose logging'). Do not include conversational filler.\n\n")

	for i, f := range failures {
		promptBuilder.WriteString(fmt.Sprintf("--- Failure %d ---\n", i+1))
		promptBuilder.WriteString(fmt.Sprintf("Original Prompt/Context:\n%s\n", f["prompt"]))
		promptBuilder.WriteString(fmt.Sprintf("Outcome: %s\n", f["outcome"]))
		promptBuilder.WriteString(fmt.Sprintf("Error Log:\n%s\n\n", f["error_log"]))
	}

	// Find Architect Config
	var architect EmbeddedAgent
	for _, a := range DefaultAgents(lmURL) {
		if a.ID == "agent-architect" {
			architect = a
			break
		}
	}

	// 1. Fetch current weights to see if they need pruning
	home, _ := os.UserHomeDir()
	weightsPath := filepath.Join(home, ".kernos", SynapticWeightsFilename)
	existingWeights := ""
	if data, err := os.ReadFile(weightsPath); err == nil {
		existingWeights = string(data)
	}

	// 2. Query Qwen-Thinking to synthesize new lessons
	response, err := queryLM(architect.LMStudioURL, architect.Model, architect.SystemPrompt, promptBuilder.String(), nil)
	if err != nil {
		log.Printf("[RLHF] ❌ Architect synthesis failed: %v", err)
		return
	}

	cleanResponse := stripThinkTags(response)

	// 4. Combine and check if Compression is needed
	combinedWeights := existingWeights + "\n" + cleanResponse
	finalWeights := combinedWeights

	// If the file is getting too long (rough token estimation via byte length)
	if len(combinedWeights) > 4000 {
		log.Println("[RLHF] ⚠️ Synaptic weights exceeding context bounds. Initiating Pruning/Compression...")

		compressionPrompt := "The following rules list has grown too large. Please synthesize, compress, and deduplicate these rules into a core set of maximum 10 fundamental operating principles. Maintain all critical safety limits. Output ONLY the compressed rules.\n\n" + combinedWeights
		compressedRes, err := queryLM(architect.LMStudioURL, architect.Model, architect.SystemPrompt, compressionPrompt, nil)
		if err == nil {
			finalWeights = stripThinkTags(compressedRes)
			log.Println("[RLHF] ✂️ Compression successful. Context window restored.")
		}
	}

	err = os.WriteFile(weightsPath, []byte(finalWeights), 0644)
	if err != nil {
		log.Printf("[RLHF] ❌ Failed to write synaptic weights: %v", err)
		return
	}

	log.Printf("[RLHF] ✅ Synaptic Plasticity Matrix updated (Length: %d bytes).", len(finalWeights))
}

// LoadSynapticWeights reads the long-term memory file.
// If it exists, these weights are appended to all agents' system prompts.
func LoadSynapticWeights() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}

	weightsPath := filepath.Join(home, ".kernos", SynapticWeightsFilename)
	data, err := os.ReadFile(weightsPath)
	if err != nil {
		return "" // File doesn't exist yet, that's fine
	}

	weights := string(data)
	if weights == "" {
		return ""
	}

	return "\n\n=== SYNAPTIC OVERRIDE (LEARNED PREFERENCES) ===\n" + weights + "\n============================================\n"
}

// Helper to remove <think>...</think> blocks from Qwen/DeepSeek outputs
func stripThinkTags(input string) string {
	startIdx := strings.Index(input, "<think>")
	endIdx := strings.Index(input, "</think>")

	if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
		// Cut out the think block
		return strings.TrimSpace(input[:startIdx] + input[endIdx+8:])
	}
	return strings.TrimSpace(input)
}

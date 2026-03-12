package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════════
// Neuroplasticity Engine — Real-Time Concurrent Learning
// ════════════════════════════════════════════════════════════════════════════════
//
// The Neuroplasticity Engine is the "awake state" counterpart to the existing
// Synaptic Consolidation (which is the "sleep state"). While consolidation runs
// nightly in batch mode, this engine processes learning signals in REAL-TIME
// using Go's concurrency primitives: channels, goroutines, and worker pools.
//
// Three concurrent pipelines operate simultaneously:
//
// Pipeline 1: REWARD SIGNALS
//   Every agent response and task execution emits a reward signal into a channel.
//   A worker pool of goroutines consumes these signals, scores them, and updates
//   the synaptic weights file in real-time — not nightly.
//
// Pipeline 2: ERROR PATTERN RECOGNITION
//   Terminal errors are analyzed for recurring patterns. When the engine detects
//   the same class of error 3+ times, it synthesizes a micro-rule and injects it
//   into the agent weights immediately.
//
// Pipeline 3: EMBEDDING INGESTION
//   Agent conversations and terminal outputs are continuously indexed into the
//   Vector Memory in the background, keeping the semantic graph fresh.
//
// Biological Analogy:
//   Active state (awake) = real-time synaptic firing + short-term potentiation
//   Sleep state (consolidation) = long-term memory compression + pruning
//
// This engine supplements consolidation.go. It does NOT replace it.
// ════════════════════════════════════════════════════════════════════════════════

// --- Signal Types ---

type RewardSignal struct {
	AgentID   string
	UserID    string
	Prompt    string
	Response  string
	Reward    float64 // +1.0 success, -1.0 failure, 0.5 neutral
	Timestamp time.Time
}

type ErrorPattern struct {
	Command   string
	ErrorText string
	Count     int
	FirstSeen time.Time
	LastSeen  time.Time
}

type EmbeddingJob struct {
	ID   string // e.g. "chat:agent-dispatcher:1234"
	Text string
}

// --- The Engine ---

type NeuroplasticityEngine struct {
	bus    *Bus
	lmURL string

	// Pipeline channels
	rewardChan    chan RewardSignal
	errorChan     chan ErrorPattern
	embeddingChan chan EmbeddingJob

	// Error pattern tracker (fast in-memory frequency map)
	errorPatterns map[string]*ErrorPattern
	errorMutex    sync.Mutex

	// Micro-rules learned in real-time (accumulated between consolidation cycles)
	microRules []string
	rulesMutex sync.RWMutex

	// Metrics
	totalRewardsProcessed int64
	totalPatternsDetected int64
	totalEmbeddingsQueued int64
	metricsMutex          sync.Mutex
}

var GlobalNeuroplasticity *NeuroplasticityEngine

func InitNeuroplasticityEngine(bus *Bus, lmURL string) *NeuroplasticityEngine {
	ne := &NeuroplasticityEngine{
		bus:           bus,
		lmURL:         lmURL,
		rewardChan:    make(chan RewardSignal, 100),   // Buffered: absorb burst traffic
		errorChan:     make(chan ErrorPattern, 50),
		embeddingChan: make(chan EmbeddingJob, 200),
		errorPatterns: make(map[string]*ErrorPattern),
	}

	GlobalNeuroplasticity = ne

	// Launch worker pools
	ne.startRewardWorkers(4)
	ne.startErrorPatternWorkers(2)
	ne.startEmbeddingWorkers(3)

	// Launch the micro-rule flush daemon (writes accumulated rules to disk periodically)
	go ne.microRuleFlusher()

	// Launch metrics reporter
	go ne.metricsReporter()

	log.Println("[Neuroplasticity] ⚡ Engine online. 3 concurrent learning pipelines active.")
	log.Println("[Neuroplasticity]   → Pipeline 1: Reward Signals (4 workers)")
	log.Println("[Neuroplasticity]   → Pipeline 2: Error Pattern Recognition (2 workers)")
	log.Println("[Neuroplasticity]   → Pipeline 3: Embedding Ingestion (3 workers)")

	return ne
}

// ════════════════════════════════════════════════════════════════════════════════
// Pipeline 1: REWARD SIGNALS — Real-Time Weight Updates
// ════════════════════════════════════════════════════════════════════════════════

func (ne *NeuroplasticityEngine) startRewardWorkers(count int) {
	for i := 0; i < count; i++ {
		go func(workerID int) {
			for signal := range ne.rewardChan {
				ne.processRewardSignal(workerID, signal)
			}
		}(i)
	}
}

func (ne *NeuroplasticityEngine) processRewardSignal(workerID int, signal RewardSignal) {
	ne.metricsMutex.Lock()
	ne.totalRewardsProcessed++
	ne.metricsMutex.Unlock()

	// Only learn from strong signals (avoid noise from neutral interactions)
	if signal.Reward > -0.5 && signal.Reward < 0.5 {
		return
	}

	// Log to telemetry DB (existing infrastructure)
	if GlobalTelemetry != nil {
		outcome := "SUCCESS"
		if signal.Reward < 0 {
			outcome = "ERROR"
		}
		go GlobalTelemetry.LogDagExecution(
			fmt.Sprintf("neuro-%s-%d", signal.AgentID, signal.Timestamp.UnixMilli()),
			signal.Prompt,
			outcome,
			signal.Response,
			signal.Reward,
		)
	}

	// For strongly negative signals, synthesize an immediate micro-rule
	if signal.Reward <= -0.8 {
		truncatedPrompt := signal.Prompt
		if len(truncatedPrompt) > 100 {
			truncatedPrompt = truncatedPrompt[:100]
		}
		truncatedResponse := signal.Response
		if len(truncatedResponse) > 100 {
			truncatedResponse = truncatedResponse[:100]
		}

		rule := fmt.Sprintf("AVOID: When asked '%s...', do NOT respond with '%s...' (negative reward: %.1f)",
			truncatedPrompt, truncatedResponse, signal.Reward)

		ne.rulesMutex.Lock()
		ne.microRules = append(ne.microRules, rule)
		ne.rulesMutex.Unlock()

		log.Printf("[Neuroplasticity:W%d] 🔴 Negative reinforcement captured → micro-rule generated", workerID)
	}

	// For strongly positive signals, reinforce the pattern
	if signal.Reward >= 0.8 {
		truncatedPrompt := signal.Prompt
		if len(truncatedPrompt) > 100 {
			truncatedPrompt = truncatedPrompt[:100]
		}

		rule := fmt.Sprintf("PREFER: When asked '%s...', the approach that worked had reward %.1f — replicate this pattern",
			truncatedPrompt, signal.Reward)

		ne.rulesMutex.Lock()
		ne.microRules = append(ne.microRules, rule)
		ne.rulesMutex.Unlock()

		log.Printf("[Neuroplasticity:W%d] 🟢 Positive reinforcement captured → micro-rule generated", workerID)
	}
}

// EmitReward sends a reward signal into the pipeline (non-blocking)
func (ne *NeuroplasticityEngine) EmitReward(agentID, userID, prompt, response string, reward float64) {
	select {
	case ne.rewardChan <- RewardSignal{
		AgentID:   agentID,
		UserID:    userID,
		Prompt:    prompt,
		Response:  response,
		Reward:    reward,
		Timestamp: time.Now(),
	}:
	default:
		// Channel full — drop signal to avoid blocking the main loop
		log.Printf("[Neuroplasticity] ⚠️ Reward channel full, signal dropped")
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// Pipeline 2: ERROR PATTERN RECOGNITION — Recurrence Detection
// ════════════════════════════════════════════════════════════════════════════════

func (ne *NeuroplasticityEngine) startErrorPatternWorkers(count int) {
	for i := 0; i < count; i++ {
		go func(workerID int) {
			for pattern := range ne.errorChan {
				ne.processErrorPattern(workerID, pattern)
			}
		}(i)
	}
}

func (ne *NeuroplasticityEngine) processErrorPattern(workerID int, incoming ErrorPattern) {
	ne.metricsMutex.Lock()
	ne.totalPatternsDetected++
	ne.metricsMutex.Unlock()

	// Classify the error into a category key (normalize)
	key := classifyError(incoming.ErrorText)

	ne.errorMutex.Lock()
	existing, found := ne.errorPatterns[key]
	if found {
		existing.Count++
		existing.LastSeen = time.Now()
	} else {
		ne.errorPatterns[key] = &ErrorPattern{
			Command:   incoming.Command,
			ErrorText: incoming.ErrorText,
			Count:     1,
			FirstSeen: time.Now(),
			LastSeen:  time.Now(),
		}
	}
	count := 0
	if ne.errorPatterns[key] != nil {
		count = ne.errorPatterns[key].Count
	}
	ne.errorMutex.Unlock()

	// Threshold: if the same error class occurs 3+ times, synthesize a micro-rule
	if count == 3 {
		log.Printf("[Neuroplasticity:W%d] 🔄 Recurring error pattern detected (%dx): %s", workerID, count, key)

		rule := fmt.Sprintf("RECURRING ERROR: The error class '%s' has occurred %d times. When the user runs commands that trigger this type of error, proactively suggest the fix before they ask.",
			key, count)

		ne.rulesMutex.Lock()
		ne.microRules = append(ne.microRules, rule)
		ne.rulesMutex.Unlock()

		// Publish a bus event so the UI can show a notification
		ne.bus.Publish(Envelope{
			Topic: "neuro.pattern:detected",
			From:  "kernel",
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"pattern": key,
				"count":   count,
				"msg":     fmt.Sprintf("Recurring error detected: %s (%dx). Learning to prevent it.", key, count),
			},
		})
	}
}

// EmitError sends an error observation into the pattern recognition pipeline
func (ne *NeuroplasticityEngine) EmitError(command, errorText string) {
	select {
	case ne.errorChan <- ErrorPattern{Command: command, ErrorText: errorText}:
	default:
		// Channel full
	}
}

// classifyError normalizes error text into a category key for pattern matching
func classifyError(errorText string) string {
	lower := strings.ToLower(errorText)

	// Go errors
	if strings.Contains(lower, "undefined:") {
		return "go:undefined-reference"
	}
	if strings.Contains(lower, "cannot find package") || strings.Contains(lower, "module not found") {
		return "go:missing-module"
	}
	if strings.Contains(lower, "syntax error") {
		return "syntax-error"
	}

	// Node/NPM errors
	if strings.Contains(lower, "npm err!") || strings.Contains(lower, "enoent") {
		return "npm:package-error"
	}
	if strings.Contains(lower, "module not found") || strings.Contains(lower, "cannot find module") {
		return "node:missing-module"
	}

	// System errors
	if strings.Contains(lower, "permission denied") {
		return "sys:permission-denied"
	}
	if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") {
		return "sys:file-not-found"
	}
	if strings.Contains(lower, "command not found") {
		return "sys:command-not-found"
	}
	if strings.Contains(lower, "panic:") {
		return "runtime:panic"
	}

	// Generic fallback: first 50 chars as key
	if len(lower) > 50 {
		return "generic:" + lower[:50]
	}
	return "generic:" + lower
}

// ════════════════════════════════════════════════════════════════════════════════
// Pipeline 3: EMBEDDING INGESTION — Continuous Vector Memory Updates
// ════════════════════════════════════════════════════════════════════════════════

func (ne *NeuroplasticityEngine) startEmbeddingWorkers(count int) {
	for i := 0; i < count; i++ {
		go func(workerID int) {
			for job := range ne.embeddingChan {
				ne.processEmbeddingJob(workerID, job)
			}
		}(i)
	}
}

func (ne *NeuroplasticityEngine) processEmbeddingJob(workerID int, job EmbeddingJob) {
	if GlobalVectorDB == nil || len(job.Text) < 20 {
		return
	}

	ne.metricsMutex.Lock()
	ne.totalEmbeddingsQueued++
	ne.metricsMutex.Unlock()

	// Generate embedding and insert into vector memory
	prefix := "search_document: "
	vectors, err := GlobalVectorDB.GenerateEmbeddings([]string{prefix + job.Text})
	if err != nil {
		log.Printf("[Neuroplasticity:W%d] Embedding generation failed: %v", workerID, err)
		return
	}

	if len(vectors) > 0 {
		chunk := DocumentChunk{
			ID:             job.ID,
			FilePath:       "neuroplasticity",
			Text:           job.Text,
			Vector:         vectors[0],
			Weight:         1.0,
			LastAccessTime: time.Now(),
		}
		GlobalVectorDB.Chunks = append(GlobalVectorDB.Chunks, chunk)
		GlobalVectorDB.SaveChunkToDB(chunk)
		log.Printf("[Neuroplasticity:W%d] 📥 Ingested embedding: %s (%.30s...)", workerID, job.ID, job.Text)
	}
}

// EmitEmbedding queues a text chunk for background vector indexing
func (ne *NeuroplasticityEngine) EmitEmbedding(id, text string) {
	select {
	case ne.embeddingChan <- EmbeddingJob{ID: id, Text: text}:
	default:
		// Channel full — non-critical, skip
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// Micro-Rule Flusher — Periodically writes accumulated rules to disk
// ════════════════════════════════════════════════════════════════════════════════

func (ne *NeuroplasticityEngine) microRuleFlusher() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ne.FlushMicroRules()
	}
}

func (ne *NeuroplasticityEngine) FlushMicroRules() {
	ne.rulesMutex.Lock()
	if len(ne.microRules) == 0 {
		ne.rulesMutex.Unlock()
		return
	}
	rules := make([]string, len(ne.microRules))
	copy(rules, ne.microRules)
	ne.microRules = nil
	ne.rulesMutex.Unlock()

	home, err := os.UserHomeDir()
	if err != nil {
		return
	}

	weightsPath := filepath.Join(home, ".kernos", SynapticWeightsFilename)

	// Read existing weights
	existing := ""
	if data, readErr := os.ReadFile(weightsPath); readErr == nil {
		existing = string(data)
	}

	// Append new micro-rules with timestamp
	newRules := fmt.Sprintf("\n\n=== NEUROPLASTICITY (Real-Time) [%s] ===\n",
		time.Now().Format("2006-01-02 15:04:05"))
	for _, rule := range rules {
		newRules += "• " + rule + "\n"
	}
	newRules += "=== END NEUROPLASTICITY ===\n"

	combined := existing + newRules

	// Safety check: if weights file is getting too large, the nightly consolidation
	// will handle compression. We just append here for speed.
	if len(combined) > 8000 {
		log.Printf("[Neuroplasticity] ⚠️ Weights file approaching limit (%d bytes). Nightly consolidation will compress.", len(combined))
	}

	if err := os.MkdirAll(filepath.Dir(weightsPath), 0755); err != nil {
		return
	}

	if err := os.WriteFile(weightsPath, []byte(combined), 0644); err != nil {
		log.Printf("[Neuroplasticity] ❌ Failed to flush micro-rules: %v", err)
		return
	}

	log.Printf("[Neuroplasticity] ✅ Flushed %d micro-rules to synaptic weights", len(rules))

	// Publish bus event so UI can notify the user
	ne.bus.Publish(Envelope{
		Topic: "neuro.learn",
		From:  "kernel",
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{
			"rulesLearned": len(rules),
			"totalWeights": len(combined),
		},
	})
}

// ════════════════════════════════════════════════════════════════════════════════
// Metrics Reporter — Periodic stats logging
// ════════════════════════════════════════════════════════════════════════════════

func (ne *NeuroplasticityEngine) metricsReporter() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ne.metricsMutex.Lock()
		rewards := ne.totalRewardsProcessed
		patterns := ne.totalPatternsDetected
		embeddings := ne.totalEmbeddingsQueued
		ne.metricsMutex.Unlock()

		ne.rulesMutex.RLock()
		pendingRules := len(ne.microRules)
		ne.rulesMutex.RUnlock()

		ne.errorMutex.Lock()
		uniqueErrors := len(ne.errorPatterns)
		ne.errorMutex.Unlock()

		log.Printf("[Neuroplasticity] 📊 Status: rewards=%d patterns=%d embeddings=%d pendingRules=%d uniqueErrors=%d",
			rewards, patterns, embeddings, pendingRules, uniqueErrors)

		// Publish metrics to the bus for the System Metrics UI
		ne.bus.Publish(Envelope{
			Topic: "neuro.metrics",
			From:  "kernel",
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"rewardsProcessed": rewards,
				"patternsDetected": patterns,
				"embeddingsQueued": embeddings,
				"pendingRules":     pendingRules,
				"uniqueErrors":     uniqueErrors,
			},
		})
	}
}

// ════════════════════════════════════════════════════════════════════════════════
// Bus Integration — Hooks called from the kernel's message handler
// ════════════════════════════════════════════════════════════════════════════════

// ObserveAgentReply should be called when any agent produces a chat reply.
// It emits both a reward signal and an embedding job concurrently.
func (ne *NeuroplasticityEngine) ObserveAgentReply(agentID, userID, prompt, reply string, confidence float64) {
	// Convert confidence to reward: high confidence = positive, low = negative
	reward := (confidence - 0.5) * 2.0 // Maps [0,1] → [-1, +1]

	ne.EmitReward(agentID, userID, prompt, reply, reward)

	// Also index the conversation into vector memory
	conversationText := fmt.Sprintf("Q: %s\nA: %s", prompt, reply)
	embeddingID := fmt.Sprintf("chat:%s:%d", agentID, time.Now().UnixMilli())
	ne.EmitEmbedding(embeddingID, conversationText)
}

// ObserveTerminalOutput should be called when terminal output is produced.
// It feeds error patterns and indexes useful outputs.
func (ne *NeuroplasticityEngine) ObserveTerminalOutput(command, output string) {
	if containsErrorSignal(output) {
		ne.EmitError(command, output)
	}

	// Index substantial terminal outputs (skip trivial ones)
	if len(output) > 50 && !containsErrorSignal(output) {
		id := fmt.Sprintf("term:%d", time.Now().UnixMilli())
		ne.EmitEmbedding(id, fmt.Sprintf("Command: %s\nOutput: %s", command, output))
	}
}

// ObserveTaskExecution should be called when a DAG task completes.
func (ne *NeuroplasticityEngine) ObserveTaskExecution(graphID string, success bool) {
	reward := 1.0
	if !success {
		reward = -1.0
	}
	ne.EmitReward("task-engine", "system", graphID, "", reward)
}

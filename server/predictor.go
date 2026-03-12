package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// PredictionEngine intercepts editor telemetry and asks the local AI model
// to predict the next terminal command the user is likely to run.

type PredictionEngine struct {
	bus        *Bus
	lmURL      string
	dispatcher EmbeddedAgent

	// Cache predictions to avoid spamming the model for every keystroke
	predictionCache map[string]struct {
		Cmd       string
		Timestamp time.Time
	}
	cacheMutex      sync.RWMutex
}

func NewPredictionEngine(bus *Bus, lmURL string) *PredictionEngine {
	// Find the dispatcher agent config
	var dispatcher EmbeddedAgent
	agents := DefaultAgents(lmURL)
	for _, a := range agents {
		if a.ID == "agent-dispatcher" {
			dispatcher = a
			break
		}
	}

	pe := &PredictionEngine{
		bus:             bus,
		lmURL:           lmURL,
		dispatcher:      dispatcher,
		predictionCache: make(map[string]struct {
			Cmd       string
			Timestamp time.Time
		}),
	}
	go pe.sweepCache()
	return pe
}

func (pe *PredictionEngine) sweepCache() {
	for {
		time.Sleep(10 * time.Minute)
		pe.cacheMutex.Lock()
		for k, v := range pe.predictionCache {
			if time.Since(v.Timestamp) >= 5*time.Minute {
				delete(pe.predictionCache, k)
			}
		}
		pe.cacheMutex.Unlock()
	}
}

// Predict NextCommand takes a code snippet and returns a predicted terminal command.
// For safety, it only returns a command if it's in the allowlist OR a safe test command.
func (pe *PredictionEngine) PredictNextCommand(filename, codeSnippet string) string {
	if len(codeSnippet) < 10 {
		return ""
	}

	// Simple heuristic caching to save GPU cycles
	cacheKey := filename + ":" + codeSnippet
	pe.cacheMutex.RLock()
	entry, exists := pe.predictionCache[cacheKey]
	pe.cacheMutex.RUnlock()
	if exists {
		if time.Since(entry.Timestamp) < 5*time.Minute {
			return entry.Cmd
		}
		pe.cacheMutex.Lock()
		delete(pe.predictionCache, cacheKey)
		pe.cacheMutex.Unlock()
	}

	// ── Feature 3: Memory-Informed Predictions ──
	// Query the Vector DB for past commands and context related to this file
	memoryContext := ""
	if GlobalVectorDB != nil {
		query := "search_query: terminal commands for " + filename
		vectors, err := GlobalVectorDB.GenerateEmbeddings([]string{query})
		if err == nil && len(vectors) > 0 {
			results := GlobalVectorDB.Search(vectors[0], 2)
			if len(results) > 0 {
				memoryContext = "\n\nHistorical Context (from semantic memory):\n"
				for _, res := range results {
					memoryContext += fmt.Sprintf("- %s (similarity: %.2f)\n", res.Chunk.Text, res.Similarity)
				}
				log.Printf("[Predictor] Injected %d memory vectors into prediction context", len(results))
			}
		}
	}

	systemPrompt := `You are the Kernos Prediction Engine. The user is actively typing code.
Based on the file name, the code snippet, and any historical context from semantic memory,
predict exactly ONE terminal command they are likely to run next to test or verify this code.
Only predict safe, read-only, or test commands like: 'go test', 'go build', 'npm test', 'npm run lint'.
Output ONLY the raw command string, nothing else. No markdown, no explanations.
If you cannot confidently predict a logical command, output exactly "NONE".`

	userMsg := "Filename: " + filename + "\n\nCode Snippet:\n" + codeSnippet + memoryContext

	// Query the local model (reuse queryLM from embedded_agents.go)
	response, err := queryLM(pe.lmURL, pe.dispatcher.Model, systemPrompt, userMsg, nil)
	if err != nil {
		log.Printf("[Predictor] Error querying model: %v", err)
		return ""
	}

	predictedCmd := strings.TrimSpace(response)

	// Strip any stray markdown backticks the model might have added
	predictedCmd = strings.TrimPrefix(predictedCmd, "`")
	predictedCmd = strings.TrimSuffix(predictedCmd, "`")

	if predictedCmd == "NONE" || predictedCmd == "" {
		return ""
	}

	// Safety check: only allow test/build commands for spec-exec
	isSafe := strings.HasPrefix(predictedCmd, "go test") ||
		strings.HasPrefix(predictedCmd, "go build") ||
		strings.HasPrefix(predictedCmd, "npm test") ||
		strings.HasPrefix(predictedCmd, "cargo test") ||
		strings.HasPrefix(predictedCmd, "pytest")

	if !isSafe {
		log.Printf("[Predictor] Rejected unsafe prediction: %s", predictedCmd)
		return ""
	}

	log.Printf("[Predictor] Synthesized semantic branch -> %s", predictedCmd)

	pe.cacheMutex.Lock()
	pe.predictionCache[cacheKey] = struct {
		Cmd       string
		Timestamp time.Time
	}{
		Cmd:       predictedCmd,
		Timestamp: time.Now(),
	}
	pe.cacheMutex.Unlock()

	return predictedCmd
}

// handleEditorTyping intercepts keystroke telemetry from the UI
func (pe *PredictionEngine) HandleEditorTyping(env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	filename, _ := payload["filename"].(string)
	snippet, _ := payload["snippet"].(string)

	if filename == "" || snippet == "" {
		return
	}

	go func() {
		// 1. Existing Shadow Task Prediction
		cmd := pe.PredictNextCommand(filename, snippet)
		if cmd != "" {
			// Trigger a shadow task for the predicted command
			GlobalShadowEngine.SpawnShadowTask(cmd)
		}

		// 2. Phase 14: Sensory Cortex Hallucinatory RAG
		pe.HallucinateFutureContext(filename, snippet)
	}()
}

// HandleTerminalTyping intercepts the user typing in the shell to provide Ghost Commands
func (pe *PredictionEngine) HandleTerminalTyping(env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	input, _ := payload["input"].(string)
	if len(input) < 3 {
		return
	}

	// Cache key
	pe.cacheMutex.RLock()
	entry, exists := pe.predictionCache["term:"+input]
	pe.cacheMutex.RUnlock()
	
	if exists {
		if time.Since(entry.Timestamp) < 5*time.Minute {
			GlobalShadowEngine.SpawnShadowTask(entry.Cmd)
			pe.bus.Publish(Envelope{
				Topic:   "terminal.predict",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]string{"prediction": entry.Cmd},
				Time:    time.Now().Format(time.RFC3339),
			})
			return
		}
	}

	go func() {
		systemPrompt := `You are the Kernos Terminal Prediction Engine. The user is actively typing a command in the shell.
Based on the partial input, predict the complete intended command. 
If the input ends with a space, predict the next arguments.
Output ONLY the raw command string, nothing else. No markdown, no prefixes.
If you cannot confidently predict, output exactly "NONE".`
		
		response, err := queryLM(pe.lmURL, pe.dispatcher.Model, systemPrompt, "Partial Input: "+input, nil)
		if err != nil {
			return
		}

		prediction := strings.TrimSpace(response)
		prediction = strings.TrimPrefix(prediction, "`")
		prediction = strings.TrimSuffix(prediction, "`")

		if prediction == "NONE" || prediction == "" {
			return
		}

		pe.cacheMutex.Lock()
		pe.predictionCache["term:"+input] = struct {
			Cmd       string
			Timestamp time.Time
		}{
			Cmd:       prediction,
			Timestamp: time.Now(),
		}
		pe.cacheMutex.Unlock()

		GlobalShadowEngine.SpawnShadowTask(prediction)

		pe.bus.Publish(Envelope{
			Topic:   "terminal.predict",
			From:    "kernel",
			To:      env.From,
			Payload: map[string]string{"prediction": prediction},
			Time:    time.Now().Format(time.RFC3339),
		})
	}()
}

// HandleTerminalIntent translates plain English to shell commands
func (pe *PredictionEngine) HandleTerminalIntent(env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	intent, _ := payload["intent"].(string)
	if intent == "" {
		return
	}

	go func() {
		systemPrompt := `You are the Kernos Natural Language Shell. The user is describing an objective in plain English.
Translate their intent into a single, valid, side-effect-free (if possible) bash CLI command.
Output ONLY the raw bash command string, nothing else. No markdown.`

		response, err := queryLM(pe.lmURL, pe.dispatcher.Model, systemPrompt, "Intent: "+intent, nil)
		if err != nil {
			return
		}

		command := strings.TrimSpace(response)
		command = strings.TrimPrefix(command, "`")
		command = strings.TrimSuffix(command, "`")
		command = strings.TrimPrefix(command, "bash")
		command = strings.TrimSpace(command)

		if command != "" {
			pe.bus.Publish(Envelope{
				Topic:   "sys.terminal.intent:ack",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]string{"command": command},
				Time:    time.Now().Format(time.RFC3339),
			})
		}
	}()
}

// HallucinateFutureContext triggers speculative RAG (Sensory Cortex)
func (pe *PredictionEngine) HallucinateFutureContext(filename, snippet string) {
	if GlobalVectorDB == nil || len(snippet) < 20 {
		return
	}

	systemPrompt := `You are the Kernos Sensory Cortex. The user is actively typing code.
Based on this snippet, hallucinate the likely NEXT concept, question, or codebase area the user will need across the semantic graph.
Your output must be ONE short hallucinated search query (max 10 words) that captures this future intent. No markdown, no prefixes, no explanations.`

	userMsg := "Filename: " + filename + "\n\nSnippet:\n" + snippet

	response, err := queryLM(pe.lmURL, pe.dispatcher.Model, systemPrompt, userMsg, nil)
	if err != nil || response == "" {
		return
	}

	hallucination := strings.TrimSpace(response)
	if len(hallucination) > 100 {
		hallucination = hallucination[:100] // safety crop
	}

	log.Printf("[SensoryCortex] Speculative Hallucination: '%s'", hallucination)

	// Pre-warm the semantic cache with this hallucinated future
	vectors, err := GlobalVectorDB.GenerateEmbeddings([]string{"search_query: " + hallucination})
	if err == nil && len(vectors) > 0 {
		results := GlobalVectorDB.Search(vectors[0], 3)

		GlobalVectorDB.HotCacheMutex.Lock()
		GlobalVectorDB.HotCache = results
		GlobalVectorDB.HotCacheMutex.Unlock()

		if len(results) > 0 {
			log.Printf("[SensoryCortex] Successfully pre-warmed context hot-cache with %d nodes.", len(results))
		}
	}
}

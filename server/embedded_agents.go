package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

// ───────────────────────────────────────────────────────────────────────────────
// Feature 4: Conversation History — maintains a sliding context window per user
// ───────────────────────────────────────────────────────────────────────────────

type chatTurn struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

var (
	conversationHistories = make(map[string][]chatTurn) // key: agentID + ":" + userID
	convHistMutex         sync.RWMutex
	maxConvTurns          = 10 // sliding window: last N exchanges
)

func appendConvTurn(agentID, userID, role, content string) {
	key := agentID + ":" + userID
	convHistMutex.Lock()
	defer convHistMutex.Unlock()
	conversationHistories[key] = append(conversationHistories[key], chatTurn{Role: role, Content: content})
	// Trim to sliding window
	if len(conversationHistories[key]) > maxConvTurns*2 {
		conversationHistories[key] = conversationHistories[key][len(conversationHistories[key])-maxConvTurns*2:]
	}
}

func getConvHistory(agentID, userID string) []chatTurn {
	key := agentID + ":" + userID
	convHistMutex.RLock()
	defer convHistMutex.RUnlock()
	history := make([]chatTurn, len(conversationHistories[key]))
	copy(history, conversationHistories[key])
	return history
}

// EmbeddedAgent is an agent proxy that runs as a goroutine inside the kernel.
// It connects to the kernel's own WebSocket, registers, and handles messages.
// No separate process needed.
type EmbeddedAgent struct {
	ID           string `yaml:"id"`
	DisplayName  string `yaml:"display_name"`
	Model        string `yaml:"model"`
	SystemPrompt string `yaml:"system_prompt"`
	LMStudioURL  string `yaml:"-"`
	KernelWSAddr string `yaml:"-"`
}

type AgentConfig struct {
	Agents []EmbeddedAgent `yaml:"agents"`
}

// DefaultAgents returns the pre-configured agent roster.
// It loads them from agents.yaml and injects Synaptic Weights.
func DefaultAgents(lmStudioURL string) []EmbeddedAgent {
	synapticWeights := LoadSynapticWeights()

	data, err := os.ReadFile("agents.yaml")
	if err != nil {
		log.Printf("[Agents] Could not read agents.yaml, falling back to empty roster: %v", err)
		return []EmbeddedAgent{}
	}

	var config AgentConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		log.Printf("[Agents] Could not parse agents.yaml: %v", err)
		return []EmbeddedAgent{}
	}

	for i := range config.Agents {
		config.Agents[i].SystemPrompt += synapticWeights
		config.Agents[i].LMStudioURL = lmStudioURL
		config.Agents[i].KernelWSAddr = "127.0.0.1:8080"
	}

	return config.Agents
}

// StartEmbeddedAgents launches all agents as goroutines. They dial into the
// kernel's WebSocket endpoint from within the same process.
// It also watches agents.yaml for changes and hot-reloads the agents.
func StartEmbeddedAgents(lmStudioURL string, authToken string) {
	// Small delay to let the HTTP server start accepting connections
	time.Sleep(500 * time.Millisecond)

	var cancelFuncs []context.CancelFunc

	startAgents := func() {
		agents := DefaultAgents(lmStudioURL)
		for _, agent := range agents {
			ctx, cancel := context.WithCancel(context.Background())
			cancelFuncs = append(cancelFuncs, cancel)
			go runAgent(ctx, agent, authToken)
		}
	}

	stopAgents := func() {
		for _, cancel := range cancelFuncs {
			cancel()
		}
		cancelFuncs = nil
	}

	startAgents()

	// Setup Watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[Agents] Watcher failed: %v", err)
		return
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Write == fsnotify.Write {
					log.Printf("[Agents] 🔄 agents.yaml modified! Hot-reloading AI Agent cluster...")
					stopAgents()
					time.Sleep(1 * time.Second) // Let old connections die
					startAgents()
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[Agents] Watcher error: %v", err)
			}
		}
	}()

	err = watcher.Add("agents.yaml")
	if err != nil {
		log.Printf("[Agents] Could not watch agents.yaml: %v", err)
	}
}

func runAgent(ctx context.Context, agent EmbeddedAgent, authToken string) {
	retryCount := 0
	maxRetries := 30

	for retryCount < maxRetries {
		select {
		case <-ctx.Done():
			log.Printf("[%s] Agent shutdown signal received.", agent.ID)
			return
		default:
		}

		if retryCount > 0 {
			backoff := time.Duration(retryCount) * 2 * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			log.Printf("[%s] Reconnecting (attempt %d/%d, backoff %s)...", agent.ID, retryCount+1, maxRetries, backoff)
			time.Sleep(backoff)
		}

		err := runAgentSession(ctx, agent, authToken)
		if err == nil {
			return // Clean shutdown via context cancellation
		}

		retryCount++
		log.Printf("[%s] Session ended: %v", agent.ID, err)
	}

	log.Printf("[%s] ❌ Max retries (%d) reached. Agent permanently stopped.", agent.ID, maxRetries)
}

func runAgentSession(ctx context.Context, agent EmbeddedAgent, authToken string) error {
	u := url.URL{Scheme: "ws", Host: agent.KernelWSAddr, Path: "/ws"}

	// Retry connection with backoff
	var conn *websocket.Conn
	var err error
	for i := 0; i < 10; i++ {
		conn, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
		if err == nil {
			break
		}
		log.Printf("[%s] Connection attempt %d failed, retrying...", agent.ID, i+1)
		time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
	}
	if err != nil {
		log.Printf("[%s] Failed to connect after retries: %v", agent.ID, err)
		return err
	}
	defer conn.Close()

	// 1. Authenticate with the kernel first
	authMsg := agentEnvelope{
		Topic: "sys.auth",
		From:  agent.ID,
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]string{
			"token": authToken,
		},
	}
	_ = conn.WriteJSON(authMsg)

	// Register with the kernel
	regMsg := agentEnvelope{
		Topic: "sys.register",
		From:  agent.ID,
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]string{
			"id":    agent.ID,
			"role":  "agent",
			"name":  agent.DisplayName,
			"model": agent.Model,
		},
	}
	_ = conn.WriteJSON(regMsg)
	log.Printf("[%s] ✅ Registered as %s (model: %s)", agent.ID, agent.DisplayName, agent.Model)

	// Listen for messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("[%s] Disconnected: %v", agent.ID, err)
			return err
		}

		var env agentEnvelope
		if err := json.Unmarshal(message, &env); err != nil {
			continue
		}

		// Handle chat requests targeted to this agent
		if env.Topic == "agent.chat" && env.To == agent.ID {
			payloadMap, ok := env.Payload.(map[string]interface{})
			if !ok {
				continue
			}
			prompt, _ := payloadMap["msg"].(string)
			if prompt == "" {
				continue
			}
			imageB64, _ := payloadMap["image"].(string)

			// Check for context cancellation
			select {
			case <-ctx.Done():
				return nil
			default:
			}

			log.Printf("[%s] Chat from %s: %.60s...", agent.ID, env.From, prompt)

			go func(from string, msg string, imgB64 string) {
				// ── Feature 4: Record user message in conversation history ──
				appendConvTurn(agent.ID, from, "user", msg)

				// ++ SOTA RAG INJECTION (PHASE 14) ++
				ragContext := ""
				if agent.ID == "agent-dispatcher" && GlobalVectorDB != nil {
					var results []SearchResult

					GlobalVectorDB.HotCacheMutex.RLock()
					hotCacheLen := len(GlobalVectorDB.HotCache)
					if hotCacheLen > 0 {
						results = make([]SearchResult, hotCacheLen)
						copy(results, GlobalVectorDB.HotCache)
					}
					GlobalVectorDB.HotCacheMutex.RUnlock()

					if hotCacheLen > 0 {
						log.Printf("[RAG] ⚡ CACHE HIT! Instant speculative context loaded via Sensory Cortex.")
						GlobalVectorDB.HotCacheMutex.Lock()
						GlobalVectorDB.HotCache = nil
						GlobalVectorDB.HotCacheMutex.Unlock()
					} else {
						log.Printf("[RAG] Dispatcher querying semantic latent space for: %.30s", msg)
						vectors, err := GlobalVectorDB.GenerateEmbeddings([]string{"search_query: " + msg})
						if err == nil && len(vectors) > 0 {
							results = GlobalVectorDB.Search(vectors[0], 3)
						}
					}

					if len(results) > 0 {
						ragContext = "\n\n--- SEMANTIC WORKSPACE CONTEXT ---\n"
						for _, res := range results {
							ragContext += fmt.Sprintf("File: %s (Similarity: %.2f)\n```\n%s\n```\n\n", res.Chunk.ID, res.Similarity, res.Chunk.Text)
						}
						log.Printf("[RAG] Injected %d semantic chunks into System Prompt", len(results))
					}
				}

				// Inject RAG into System Prompt dynamically
				finalSystemPrompt := agent.SystemPrompt + ragContext

				// ── Feature 4: Build messages with conversation history ──
				var messages []map[string]interface{}
				messages = append(messages, map[string]interface{}{"role": "system", "content": finalSystemPrompt})

				// Inject prior conversation turns (sliding window)
				history := getConvHistory(agent.ID, from)
				// Skip the last entry since it's the current message we just appended
				if len(history) > 1 {
					for _, turn := range history[:len(history)-1] {
						messages = append(messages, map[string]interface{}{"role": turn.Role, "content": turn.Content})
					}
				}

				// Build the user message — either plain text or multimodal (text + image)
				var userContent interface{}
				if imgB64 != "" {
					log.Printf("[%s] 🖼️ VL Image Analysis activated (%.1fKB)", agent.ID, float64(len(imgB64))/1024.0)
					userContent = []map[string]interface{}{
						{"type": "text", "text": msg},
						{"type": "image_url", "image_url": map[string]string{"url": "data:image/png;base64," + imgB64}},
					}
				} else {
					userContent = msg
				}
				messages = append(messages, map[string]interface{}{"role": "user", "content": userContent})

				// Build the request with full conversation context
				reqBody := map[string]interface{}{
					"model":    agent.Model,
					"stream":   true,
					"messages": messages,
				}

				jsonData, err := json.Marshal(reqBody)
				if err != nil {
					log.Printf("[%s] JSON marshal error: %v", agent.ID, err)
					return
				}

				resp, err := http.Post(agent.LMStudioURL, "application/json", bytes.NewBuffer(jsonData))
				if err != nil {
					log.Printf("[%s] LM Studio unreachable: %v", agent.ID, err)
					return
				}
				defer resp.Body.Close()

				reader := bufio.NewReader(resp.Body)
				var fullContent strings.Builder
				for {
					line, err := reader.ReadBytes('\n')
					if err != nil {
						break
					}
					lineStr := string(line)
					if strings.HasPrefix(lineStr, "data: ") {
						dataStr := strings.TrimSpace(strings.TrimPrefix(lineStr, "data: "))
						if dataStr == "[DONE]" {
							break
						}
						var chunk struct {
							Choices []struct {
								Delta struct {
									Content string `json:"content"`
								} `json:"delta"`
							} `json:"choices"`
						}
						if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
							if len(chunk.Choices) > 0 {
								token := chunk.Choices[0].Delta.Content
								if token != "" {
									fullContent.WriteString(token)
									streamEnv := agentEnvelope{
										Topic: "agent.chat:stream",
										From:  agent.ID,
										To:    from,
										Time:  time.Now().Format(time.RFC3339),
										Payload: map[string]string{
											"chunk": token,
										},
									}
									_ = conn.WriteJSON(streamEnv)
								}
							}
						}
					}
				}

				response := fullContent.String()
				if response == "" {
					response = "No response from model"
				}

				// ── Feature 4: Record assistant reply in history ──
				cleanReply := stripThinkTags(response)
				appendConvTurn(agent.ID, from, "assistant", cleanReply)

				// ── Feature 5: Confidence Scoring ──
				confidence := assessConfidence(cleanReply)
				log.Printf("[%s] Confidence: %.2f for response to %s", agent.ID, confidence, from)

				reply := agentEnvelope{
					Topic: "agent.chat:reply",
					From:  agent.ID,
					To:    from,
					Time:  time.Now().Format(time.RFC3339),
					Payload: map[string]interface{}{
						"reply":      response,
						"confidence": fmt.Sprintf("%.2f", confidence),
					},
				}
				_ = conn.WriteJSON(reply)

				// ── Feature 5: Low confidence → auto-delegate to Architect for second opinion ──
				if confidence < 0.5 && agent.ID != "agent-architect" {
					log.Printf("[%s] ⚠️ Low confidence (%.2f). Delegating to Architect for second opinion...", agent.ID, confidence)
					internalReq := agentEnvelope{
						Topic: "agent.internal",
						From:  agent.ID,
						To:    "agent-architect",
						Time:  time.Now().Format(time.RFC3339),
						Payload: map[string]string{
							"type":         "second-opinion",
							"originalUser": from,
							"question":     msg,
							"firstAnswer":  cleanReply,
						},
					}
					_ = conn.WriteJSON(internalReq)
				}
			}(env.From, prompt, imageB64)
		}

		// ── Feature 1: Agent-to-Agent Communication ──
		// Handle internal agent delegation requests
		if env.Topic == "agent.internal" && env.To == agent.ID {
			payloadMap, ok := env.Payload.(map[string]interface{})
			if !ok {
				continue
			}

			reqType, _ := payloadMap["type"].(string)
			log.Printf("[%s] 🔗 Agent-to-Agent request from %s: type=%s", agent.ID, env.From, reqType)

			go func(fromAgent string, payload map[string]interface{}) {
				switch reqType {
				case "second-opinion":
					// Feature 5: Architect provides a second opinion on low-confidence responses
					originalUser, _ := payload["originalUser"].(string)
					question, _ := payload["question"].(string)
					firstAnswer, _ := payload["firstAnswer"].(string)

					prompt := fmt.Sprintf(`Another agent responded to a user's question but has low confidence. Review and provide a refined answer.

User Question: %s

First Answer (low confidence): %s

Provide a more thorough, accurate response. If the first answer is correct, confirm it with additional detail.`, question, firstAnswer)

					resp, err := queryLM(agent.LMStudioURL, agent.Model, agent.SystemPrompt, prompt, nil)
					if err != nil {
						log.Printf("[%s] Second opinion failed: %v", agent.ID, err)
						return
					}

					clean := stripThinkTags(resp)
					_ = conn.WriteJSON(agentEnvelope{
						Topic: "agent.chat:reply",
						From:  agent.ID,
						To:    originalUser,
						Time:  time.Now().Format(time.RFC3339),
						Payload: map[string]interface{}{
							"reply":      "🧠 **Architect Second Opinion:**\n\n" + clean,
							"confidence": "0.90",
							"source":     "second-opinion",
						},
					})

				case "delegate":
					// Generic delegation: one agent asks another to handle a task
					msg, _ := payload["msg"].(string)
					origUser, _ := payload["originalUser"].(string)

					resp, err := queryLM(agent.LMStudioURL, agent.Model, agent.SystemPrompt, msg, nil)
					if err != nil {
						log.Printf("[%s] Delegation failed: %v", agent.ID, err)
						return
					}

					clean := stripThinkTags(resp)
					_ = conn.WriteJSON(agentEnvelope{
						Topic: "agent.chat:reply",
						From:  agent.ID,
						To:    origUser,
						Time:  time.Now().Format(time.RFC3339),
						Payload: map[string]interface{}{
							"reply":  "🔗 **" + agent.DisplayName + " (delegated):**\n\n" + clean,
							"source": "delegation",
						},
					})
				}
			}(env.From, payloadMap)
		}

		// ── Feature 2: Proactive Agent Initiative ──
		// Dispatcher watches terminal output for errors and proactively suggests fixes
		if env.Topic == "vm.output" && agent.ID == "agent-dispatcher" {
			payloadMap, ok := env.Payload.(map[string]interface{})
			if !ok {
				continue
			}
			output, _ := payloadMap["output"].(string)
			command, _ := payloadMap["command"].(string)

			// Only trigger on error-like outputs
			if containsErrorSignal(output) {
				go func(cmd, out string) {
					log.Printf("[Proactive] 🔍 Error detected in terminal output. Generating proactive suggestion...")

					proactivePrompt := fmt.Sprintf(`The user just ran a command in the terminal and it produced an error. 
Analyze the error and provide a SHORT, actionable suggestion to fix it. 
Be concise — max 2-3 sentences. If you can provide the exact fix command, do so.

Command: %s
Output:
%s`, cmd, truncateString(out, 500))

					resp, err := queryLM(agent.LMStudioURL, agent.Model, agent.SystemPrompt, proactivePrompt, nil)
					if err != nil {
						return
					}

					clean := stripThinkTags(resp)
					if len(clean) > 0 {
						_ = conn.WriteJSON(agentEnvelope{
							Topic: "agent.proactive",
							From:  agent.ID,
							Time:  time.Now().Format(time.RFC3339),
							Payload: map[string]string{
								"suggestion": clean,
								"command":    cmd,
								"type":       "error-fix",
							},
						})
						log.Printf("[Proactive] 💡 Suggestion sent: %.60s...", clean)
					}
				}(command, output)
			}
		}

		// Handle ping
		if env.Topic == "agent.ping" && env.To == agent.ID {
			pong := agentEnvelope{
				Topic:   "agent.pong",
				From:    agent.ID,
				To:      env.From,
				Time:    time.Now().Format(time.RFC3339),
				Payload: map[string]string{"msg": "Pong!"},
			}
			_ = conn.WriteJSON(pong)
		}
	}
}

// agentEnvelope mirrors the Envelope struct but is self-contained here
// to avoid import cycles with the main package's Envelope.
type agentEnvelope struct {
	Topic   string      `json:"topic"`
	From    string      `json:"from"`
	To      string      `json:"to,omitempty"`
	Payload interface{} `json:"payload"`
	Time    string      `json:"time"`
}

// LM Studio types
type lmChatRequest struct {
	Model    string      `json:"model"`
	Stream   bool        `json:"stream,omitempty"`
	Messages []lmMessage `json:"messages"`
}

type lmMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type lmChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func queryLM(apiURL, model, systemPrompt, userMsg string, onToken func(string)) (string, error) {
	reqBody := lmChatRequest{
		Model:  model,
		Stream: onToken != nil,
		Messages: []lmMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userMsg},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(apiURL, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("LM Studio unreachable at %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	if reqBody.Stream {
		reader := bufio.NewReader(resp.Body)
		var fullContent strings.Builder
		for {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				if err == io.EOF {
					break
				}
				return fullContent.String(), err
			}
			lineStr := string(line)
			if strings.HasPrefix(lineStr, "data: ") {
				dataStr := strings.TrimSpace(strings.TrimPrefix(lineStr, "data: "))
				if dataStr == "[DONE]" {
					break
				}
				var chunk struct {
					Choices []struct {
						Delta struct {
							Content string `json:"content"`
						} `json:"delta"`
					} `json:"choices"`
				}
				if err := json.Unmarshal([]byte(dataStr), &chunk); err == nil {
					if len(chunk.Choices) > 0 {
						token := chunk.Choices[0].Delta.Content
						if token != "" {
							fullContent.WriteString(token)
							onToken(token)
						}
					}
				}
			}
		}
		return fullContent.String(), nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatResp lmChatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}

	return "No response from model", nil
}

// ───────────────────────────────────────────────────────────────────────────────
// Feature 5: Confidence Scoring — heuristic self-assessment of response quality
// ───────────────────────────────────────────────────────────────────────────────

func assessConfidence(response string) float64 {
	if response == "" || response == "No response from model" {
		return 0.0
	}

	confidence := 0.8 // Base confidence

	// Hedging language lowers confidence
	hedges := []string{"I'm not sure", "I think", "maybe", "possibly", "it might", "I believe",
		"perhaps", "not certain", "could be", "I don't know", "unsure", "unclear"}
	for _, h := range hedges {
		if strings.Contains(strings.ToLower(response), h) {
			confidence -= 0.1
		}
	}

	// Very short responses are suspect
	if len(response) < 20 {
		confidence -= 0.2
	}

	// Contradictions within the response
	if strings.Contains(response, "however") && strings.Contains(response, "but") {
		confidence -= 0.05
	}

	// Long, detailed responses boost confidence
	if len(response) > 300 {
		confidence += 0.05
	}

	// Code blocks suggest concrete, actionable answers
	if strings.Contains(response, "```") {
		confidence += 0.05
	}

	// Clamp to [0, 1]
	return math.Max(0, math.Min(1.0, confidence))
}

// ───────────────────────────────────────────────────────────────────────────────
// Feature 2: Proactive Initiative — error signal detection in terminal output
// ───────────────────────────────────────────────────────────────────────────────

func containsErrorSignal(output string) bool {
	lower := strings.ToLower(output)
	errorSignals := []string{
		"error:", "fatal:", "panic:", "exception:", "traceback",
		"command not found", "no such file", "permission denied",
		"failed to", "cannot find", "segmentation fault",
		"build failed", "compilation error", "syntax error",
		"npm err!", "enoent", "module not found",
	}
	for _, sig := range errorSignals {
		if strings.Contains(lower, sig) {
			return true
		}
	}
	return false
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "\n[...truncated...]"
}


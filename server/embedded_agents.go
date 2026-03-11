package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"gopkg.in/yaml.v3"
)

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
		return
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
			return
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
				return
			default:
			}

			log.Printf("[%s] Chat from %s: %.60s...", agent.ID, env.From, prompt)

			go func(from string, msg string, imgB64 string) {
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

				// Build the request manually to support multimodal content
				reqBody := map[string]interface{}{
					"model":  agent.Model,
					"stream": true,
					"messages": []map[string]interface{}{
						{"role": "system", "content": finalSystemPrompt},
						{"role": "user", "content": userContent},
					},
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

				reply := agentEnvelope{
					Topic: "agent.chat:reply",
					From:  agent.ID,
					To:    from,
					Time:  time.Now().Format(time.RFC3339),
					Payload: map[string]string{
						"reply": response,
					},
				}
				_ = conn.WriteJSON(reply)
			}(env.From, prompt, imageB64)
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

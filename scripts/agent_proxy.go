//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
)

type Envelope struct {
	Topic   string      `json:"topic"`
	From    string      `json:"from"`
	To      string      `json:"to,omitempty"`
	Payload interface{} `json:"payload"`
	Time    string      `json:"time"`
}

func main() {
	agentID := flag.String("id", "agent-dispatcher", "The unique identity of this agent")
	displayName := flag.String("name", "Dispatcher", "Human-readable display name")
	role := flag.String("role", "agent", "The role of this client")
	model := flag.String("model", "mistralai/codestral-22b-v0.1", "The LM Studio model to use")
	systemPrompt := flag.String("system", "You are an autonomous AI agent integrated into Kernos OS.", "System prompt for the LLM")
	lmStudioQueryURL := flag.String("lmurl", "http://127.0.0.1:1234/v1/chat/completions", "LM Studio API URL")
	flag.Parse()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)

	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8080", Path: "/ws"}
	log.Printf("Connecting to %s as %s (%s) using model %s", u.String(), *agentID, *displayName, *model)

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	done := make(chan struct{})

	// Listen for messages
	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("read:", err)
				return
			}
			var env Envelope
			if err := json.Unmarshal(message, &env); err != nil {
				log.Printf("Raw form ignoring parsing error: %s", message)
				continue
			}

			log.Printf("[RECV] Topic: %s | From: %s | To: %s", env.Topic, env.From, env.To)

			// Simple AI chat via LM Studio
			if env.Topic == "agent.chat" && env.To == *agentID {
				payloadMap, ok := env.Payload.(map[string]interface{})
				if !ok {
					continue
				}
				prompt, _ := payloadMap["msg"].(string)
				log.Printf("Received chat request: %s", prompt)

				go func(requestFrom string, requestPrompt string) {
					responseStr, err := queryLMStudio(*lmStudioQueryURL, *model, *systemPrompt, requestPrompt)
					if err != nil {
						log.Printf("LM Studio error: %v", err)
						responseStr = fmt.Sprintf("Error calling LM Studio: %v", err)
					}

					reply := Envelope{
						Topic: "agent.chat:reply",
						From:  *agentID,
						To:    requestFrom,
						Time:  time.Now().Format(time.RFC3339),
						Payload: map[string]string{
							"reply": responseStr,
						},
					}
					_ = c.WriteJSON(reply)
				}(env.From, prompt)
			}

			// If we get a direct ping, pong back
			if env.Topic == "agent.ping" && env.To == *agentID {
				log.Printf("Received ping, replying to %s", env.From)
				pong := Envelope{
					Topic:   "agent.pong",
					From:    *agentID,
					To:      env.From,
					Time:    time.Now().Format(time.RFC3339),
					Payload: map[string]string{"msg": "Pong!"},
				}
				_ = c.WriteJSON(pong)
			}
		}
	}()

	// Register with Kernel
	regMsg := Envelope{
		Topic: "sys.register",
		From:  *agentID,
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]string{
			"id":    *agentID,
			"role":  *role,
			"name":  *displayName,
			"model": *model,
		},
	}
	err = c.WriteJSON(regMsg)
	if err != nil {
		log.Println("register write:", err)
		return
	}

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			log.Println("interrupt")
			err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			if err != nil {
				log.Println("write close:", err)
				return
			}
			select {
			case <-done:
			case <-time.After(time.Second):
			}
			return
		}
	}
}

type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func queryLMStudio(url string, model string, systemPrompt string, prompt string) (string, error) {
	reqBody := ChatRequest{
		Model: model,
		Messages: []Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: prompt},
		},
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var chatResp ChatResponse
	err = json.Unmarshal(body, &chatResp)
	if err != nil {
		return "", err
	}

	if len(chatResp.Choices) > 0 {
		return chatResp.Choices[0].Message.Content, nil
	}

	return "No response from model", nil
}

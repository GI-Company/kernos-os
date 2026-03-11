//go:build ignore

package main

import (
	"encoding/json"
	"log"
	"net/url"
	"strings"
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
	u := url.URL{Scheme: "ws", Host: "127.0.0.1:8080", Path: "/ws"}
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatal("dial:", err)
	}
	defer c.Close()

	// Register as UI
	regMsg := Envelope{
		Topic:   "sys.register",
		From:    "ui-script",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"id": "ui-script", "role": "ui"},
	}
	_ = c.WriteJSON(regMsg)
	time.Sleep(500 * time.Millisecond)

	// Dispatch a request to Gemma to draft a DAG
	draftReq := Envelope{
		Topic: "agent.chat",
		From:  "ui-script",
		To:    "agent-gemma", // Gemma is the fast dispatcher
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]string{
			"msg": `The user wants to find all instances of 'EventBus' in the code and count them.
Please draft a 2-node JSON DAG to do this.
Node 1 should use 'grep'. Node 2 should use 'wc'.
Do not execute it, just return the JSON DAG. 
The output MUST be a strict JSON array of objects, where each object has: "id" (string), "command" (string), and "dependencies" (array of strings). Do not use markdown blocks. Return only raw JSON.`,
		},
	}
	log.Println("Asking Gemma to draft a DAG...")
	_ = c.WriteJSON(draftReq)

	var draftJSON string

	// Wait for Gemma's Draft
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Fatal("read:", err)
		}
		var env Envelope
		json.Unmarshal(message, &env)

		if env.Topic == "agent.chat:reply" && env.From == "agent-gemma" {
			payloadMap, _ := env.Payload.(map[string]interface{})
			draftJSON, _ = payloadMap["reply"].(string)
			log.Printf("Gemma drafted the DAG:\n%s", draftJSON)
			break
		}
	}

	time.Sleep(1 * time.Second)

	// Send the draft to Qwen (Architect) for review
	reviewReq := Envelope{
		Topic: "agent.chat",
		From:  "ui-script",
		To:    "agent-qwen",
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]string{
			"msg": "Please review this drafted DAG for the Kernos TaskEngine. Return 'APPROVED' if it is safe and correct, otherwise explain the flaws:\n\n" + draftJSON,
		},
	}

	log.Println("Asking Qwen to review Gemma's draft...")
	_ = c.WriteJSON(reviewReq)

	// Wait for Qwen's Review
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Fatal("read:", err)
		}
		var env Envelope
		json.Unmarshal(message, &env)

		if env.Topic == "agent.chat:reply" && env.From == "agent-qwen" {
			payloadMap, _ := env.Payload.(map[string]interface{})
			review, _ := payloadMap["reply"].(string)
			log.Printf("Qwen's Review:\n%s", review)
			break
		}
	}

	time.Sleep(1 * time.Second)

	// Clean up JSON (in case LLM used markdown block)
	cleanJSON := strings.TrimSpace(draftJSON)
	if strings.HasPrefix(cleanJSON, "```json") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```json")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	} else if strings.HasPrefix(cleanJSON, "```") {
		cleanJSON = strings.TrimPrefix(cleanJSON, "```")
		cleanJSON = strings.TrimSuffix(cleanJSON, "```")
	}
	cleanJSON = strings.TrimSpace(cleanJSON)

	// Parse JSON into []interface{}
	var nodes []interface{}
	err = json.Unmarshal([]byte(cleanJSON), &nodes)
	if err != nil {
		log.Printf("Failed to parse DAG JSON: %v", err)
		log.Printf("Raw JSON was: %s", cleanJSON)
		return
	}

	// Send to TaskEngine
	log.Println("Submitting approved DAG to the TaskEngine...")
	runReq := Envelope{
		Topic: "task.run",
		From:  "ui-script",
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{
			"graphId": "ai-generated-dag",
			"nodes":   nodes,
		},
	}
	_ = c.WriteJSON(runReq)

	// Wait for task completion
	for {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Fatal("read task reply:", err)
		}
		var env Envelope
		json.Unmarshal(message, &env)

		if env.Topic == "task.event" {
			payloadMap, _ := env.Payload.(map[string]interface{})
			step, _ := payloadMap["step"].(string)
			status, _ := payloadMap["status"].(string)
			output, _ := payloadMap["output"].(string)
			log.Printf("[TaskEngine] Step '%s' changed to %s. Output:\n%s", step, status, output)
		}

		if env.Topic == "task.done" {
			log.Println("SUCCESS! The DAG fully executed.")
			break
		}
	}
}

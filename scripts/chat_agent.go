//go:build ignore

package main

import (
	"encoding/json"
	"log"
	"net/url"
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

	// Register
	regMsg := Envelope{
		Topic:   "sys.register",
		From:    "ui-script",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"id": "ui-script", "role": "tester"},
	}
	_ = c.WriteJSON(regMsg)
	time.Sleep(500 * time.Millisecond)

	// Send Chat
	chatReq := Envelope{
		Topic:   "agent.chat",
		From:    "ui-script",
		To:      "agent-gemma",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"msg": "What is the purpose of Kernos OS?"},
	}
	log.Println("Sent direct chat to agent-gemma")
	_ = c.WriteJSON(chatReq)

	// Wait for Reply
	for i := 0; i < 5; i++ {
		_, message, err := c.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			return
		}
		var env Envelope
		json.Unmarshal(message, &env)
		log.Printf("[RECV] Topic: %s", env.Topic)
		if env.Topic == "agent.chat:reply" {
			payloadMap, _ := env.Payload.(map[string]interface{})
			replyStr, _ := payloadMap["reply"].(string)
			log.Printf("SUCCESS! Received reply from agent: %s", replyStr)
			break
		}
	}
}

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// Chat History — Persistent Conversation Storage
// ═══════════════════════════════════════════════════════════════
//
// Stores chat conversations per user in ~/.kernos/sys.db.
// Each conversation has an ID, title, agent, and messages.
// ═══════════════════════════════════════════════════════════════

type ChatConversation struct {
	ID        string `json:"id"`
	UserID    string `json:"user_id"`
	Title     string `json:"title"`
	AgentID   string `json:"agent_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Messages  string `json:"messages"` // JSON array of messages
}

// InitChatHistoryDB creates the chat_history table if it doesn't exist
func InitChatHistoryDB(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS chat_history (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL DEFAULT 'guest',
			title TEXT NOT NULL DEFAULT 'New Chat',
			agent_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			messages TEXT NOT NULL DEFAULT '[]'
		);
		CREATE INDEX IF NOT EXISTS idx_chat_user ON chat_history(user_id, updated_at);
	`)
	if err != nil {
		log.Printf("[ChatHistory] ⚠️ Failed to init table: %v", err)
	} else {
		log.Printf("[ChatHistory] ✅ Chat history table ready")
	}
	return err
}

// handleChatHistory dispatches chat history WebSocket topics
func handleChatHistory(bus *Bus, env Envelope) {
	payload, _ := env.Payload.(map[string]interface{})

	switch env.Topic {

	// ── List conversations for a user ──
	case "chat.list":
		userID, _ := payload["user_id"].(string)
		if userID == "" {
			userID = "guest"
		}
		limit := 50

		if GlobalSysDB == nil {
			return
		}

		rows, err := GlobalSysDB.DB.Query(`
			SELECT id, user_id, title, agent_id, created_at, updated_at
			FROM chat_history
			WHERE user_id = ?
			ORDER BY updated_at DESC
			LIMIT ?
		`, userID, limit)
		if err != nil {
			bus.Publish(Envelope{
				Topic:   "chat.list:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": err.Error()},
			})
			return
		}
		defer rows.Close()

		conversations := []map[string]string{}
		for rows.Next() {
			var id, uid, title, agent, created, updated string
			rows.Scan(&id, &uid, &title, &agent, &created, &updated)
			conversations = append(conversations, map[string]string{
				"id":         id,
				"user_id":    uid,
				"title":      title,
				"agent_id":   agent,
				"created_at": created,
				"updated_at": updated,
			})
		}

		bus.Publish(Envelope{
			Topic: "chat.list:resp",
			From:  "kernel",
			To:    env.From,
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"conversations": conversations,
			},
		})

	// ── Load a specific conversation ──
	case "chat.load":
		chatID, _ := payload["id"].(string)
		if chatID == "" || GlobalSysDB == nil {
			return
		}

		var conv ChatConversation
		err := GlobalSysDB.DB.QueryRow(`
			SELECT id, user_id, title, agent_id, created_at, updated_at, messages
			FROM chat_history WHERE id = ?
		`, chatID).Scan(&conv.ID, &conv.UserID, &conv.Title, &conv.AgentID, &conv.CreatedAt, &conv.UpdatedAt, &conv.Messages)

		if err != nil {
			bus.Publish(Envelope{
				Topic:   "chat.load:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": "Conversation not found"},
			})
			return
		}

		// Parse messages JSON
		var messages []interface{}
		json.Unmarshal([]byte(conv.Messages), &messages)

		bus.Publish(Envelope{
			Topic: "chat.load:resp",
			From:  "kernel",
			To:    env.From,
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"id":         conv.ID,
				"user_id":    conv.UserID,
				"title":      conv.Title,
				"agent_id":   conv.AgentID,
				"created_at": conv.CreatedAt,
				"updated_at": conv.UpdatedAt,
				"messages":   messages,
			},
		})

	// ── Save / update a conversation ──
	case "chat.save":
		chatID, _ := payload["id"].(string)
		userID, _ := payload["user_id"].(string)
		title, _ := payload["title"].(string)
		agentID, _ := payload["agent_id"].(string)
		messagesRaw := payload["messages"]

		if chatID == "" || GlobalSysDB == nil {
			return
		}
		if userID == "" {
			userID = "guest"
		}
		if title == "" {
			title = "New Chat"
		}

		// Serialize messages
		messagesJSON, _ := json.Marshal(messagesRaw)
		now := time.Now().Format(time.RFC3339)

		_, err := GlobalSysDB.DB.Exec(`
			INSERT INTO chat_history (id, user_id, title, agent_id, created_at, updated_at, messages)
			VALUES (?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				title = excluded.title,
				agent_id = excluded.agent_id,
				updated_at = excluded.updated_at,
				messages = excluded.messages
		`, chatID, userID, title, agentID, now, now, string(messagesJSON))

		if err != nil {
			log.Printf("[ChatHistory] ⚠️ Save failed: %v", err)
			bus.Publish(Envelope{
				Topic:   "chat.save:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": err.Error()},
			})
			return
		}

		bus.Publish(Envelope{
			Topic: "chat.save:resp",
			From:  "kernel",
			To:    env.From,
			Time:  now,
			Payload: map[string]interface{}{
				"success": true,
				"id":      chatID,
			},
		})

	// ── Delete a conversation ──
	case "chat.delete":
		chatID, _ := payload["id"].(string)
		if chatID == "" || GlobalSysDB == nil {
			return
		}

		GlobalSysDB.DB.Exec("DELETE FROM chat_history WHERE id = ?", chatID)
		log.Printf("[ChatHistory] 🗑️ Deleted conversation: %s", chatID)

		bus.Publish(Envelope{
			Topic:   "chat.delete:resp",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{"success": true, "id": chatID},
		})
	}

	_ = fmt.Sprintf("") // avoid unused import
}

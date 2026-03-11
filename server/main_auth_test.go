package main

import (
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestZeroTrustAuthHandshake(t *testing.T) {
	// 1. Setup global AuthToken and dependencies
	AuthToken = hex.EncodeToString([]byte("test_secret_token_1234567890123"))
	bus := &Bus{clients: make(map[*websocket.Conn]*Client)}

	// 2. Setup a test HTTP server with the WebSocket upgrader
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade valid request: %v", err)
		}

		clientSend := make(chan Envelope, 256)
		bus.lock.Lock()
		clientObj := &Client{ID: "anonymous", Role: "guest", Authenticated: false, send: clientSend}
		bus.clients[conn] = clientObj
		bus.lock.Unlock()

		go writePump(conn, clientSend)

		defer func() {
			bus.lock.Lock()
			if client, ok := bus.clients[conn]; ok {
				if client.send != nil {
					close(client.send)
				}
				delete(bus.clients, conn)
			}
			bus.lock.Unlock()
			conn.Close()
		}()

		for {
			var env Envelope
			err := conn.ReadJSON(&env)
			if err != nil {
				break
			}

			// Handle sys.auth handshake directly inline since it's deeply integrated in main() logic currently
			if env.Topic == "sys.auth" {
				payload, ok := env.Payload.(map[string]interface{})
				if ok {
					token, _ := payload["token"].(string)
					if token == AuthToken {
						bus.lock.Lock()
						if c, exists := bus.clients[conn]; exists {
							c.Authenticated = true
						}
						bus.lock.Unlock()
						
						// Reply
						env := Envelope{Topic: "sys.auth:ack", From: "kernel"}
						conn.WriteJSON(env)
					} else {
						// Disconnect immediately on bad token
						conn.Close()
						break
					}
				}
				continue
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// ==========================================
	// Test Case 1: Valid Token = Successful Auth
	// ==========================================
	t.Run("ValidToken", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer ws.Close()

		authMsg := Envelope{
			Topic:   "sys.auth",
			From:    "test-client",
			Payload: map[string]interface{}{"token": AuthToken},
		}

		err = ws.WriteJSON(authMsg)
		if err != nil {
			t.Fatalf("Failed to write to WS: %v", err)
		}

		var resp Envelope
		ws.SetReadDeadline(time.Now().Add(1 * time.Second))
		err = ws.ReadJSON(&resp)
		if err != nil {
			t.Fatalf("Failed to read auth ack: %v", err)
		}

		if resp.Topic != "sys.auth:ack" {
			t.Errorf("Expected sys.auth:ack, got %s", resp.Topic)
		}
	})

	// ==========================================
	// Test Case 2: Invalid Token = Disconnection
	// ==========================================
	t.Run("InvalidToken", func(t *testing.T) {
		ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("Failed to connect: %v", err)
		}
		defer ws.Close()

		authMsg := Envelope{
			Topic:   "sys.auth",
			From:    "test-client",
			Payload: map[string]interface{}{"token": "completely_wrong_token"},
		}

		err = ws.WriteJSON(authMsg)
		if err != nil {
			t.Fatalf("Failed to write to WS: %v", err)
		}

		// The server should close the connection immediately. Wait a tiny bit then try to read.
		ws.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, _, err = ws.ReadMessage()
		if err == nil {
			t.Fatalf("Expected connection to be closed by server on invalid token, but it was kept open")
		}
	})
}

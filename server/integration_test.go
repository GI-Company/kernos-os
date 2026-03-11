package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestKernelIntegration_E2E(t *testing.T) {
	// 1. Initialize Core OS Subsystems
	t.Setenv("HOME", t.TempDir())
	InitSysDB()
	bus := &Bus{clients: make(map[*websocket.Conn]*Client)}
	taskEngine := NewTaskEngine(bus)
	predEngine := &PredictionEngine{} // Mocked for simplicity
	appEngine := NewAppletEngine(bus)
	p2pGateway := NewP2PGateway(bus)

	// 2. Setup the HTTP/WS Server resembling main()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("Failed to upgrade valid request: %v", err)
		}

		clientSend := make(chan Envelope, 256)
		bus.lock.Lock()
		clientObj := &Client{ID: "anonymous", Role: "guest", Authenticated: false, Subscriptions: make(map[string]bool), send: clientSend}
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
						conn.WriteJSON(Envelope{Topic: "sys.auth:ack", From: "kernel"})
					} else {
						conn.Close()
						break
					}
				}
				continue
			}

			// Require Auth for anything else
			bus.lock.Lock()
			client := bus.clients[conn]
			authenticated := client != nil && client.Authenticated
			bus.lock.Unlock()

			if !authenticated {
				continue
			}

			// Route standard kernel messages
			go handleEnvelope(bus, env, taskEngine, predEngine, appEngine, p2pGateway)
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// 3. Connect as Client
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()

	// 4. Authenticate
	ws.WriteJSON(Envelope{
		Topic:   "sys.auth",
		From:    "integration-client",
		Payload: map[string]interface{}{"token": AuthToken},
	})

	var authAck Envelope
	ws.SetReadDeadline(time.Now().Add(1 * time.Second))
	ws.ReadJSON(&authAck)
	if authAck.Topic != "sys.auth:ack" {
		t.Fatalf("Expected sys.auth:ack, got %v", authAck.Topic)
	}

	// 5. Send vm.spawn command (echo "integration passed")
	ws.WriteJSON(Envelope{
		Topic: "vm.spawn",
		From:  "integration-client",
		Payload: map[string]interface{}{
			"_request_id": "test-req-123",
			"cmd":         "echo",
			"args":        []interface{}{"integration", "test", "passed"},
			"cwd":         ".",
		},
	})

	// 6. Wait for vm.stdout
	passed := false
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	for {
		var resp Envelope
		err := ws.ReadJSON(&resp)
		if err != nil {
			t.Fatalf("WebSocket closed or timeout before receiving vm.stdout: %v", err)
		}

		if resp.Topic == "vm.stdout" {
			payloadMap, ok := resp.Payload.(map[string]interface{})
			if !ok {
				continue
			}
			text, _ := payloadMap["text"].(string)
			if strings.Contains(text, "integration test passed") {
				passed = true
				break
			}
		}
	}

	if !passed {
		t.Errorf("Failed to receive expected stdout output from vm.spawn execution")
	}
}

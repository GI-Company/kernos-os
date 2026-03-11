package main

import (
	"testing"

	"github.com/gorilla/websocket"
)

func TestP2PGateway(t *testing.T) {
	bus := &Bus{clients: make(map[*websocket.Conn]*Client)}
	gateway := NewP2PGateway(bus)

	// ==========================================
	// Test Case 1: Generate PIN
	// ==========================================
	t.Run("GeneratePIN", func(t *testing.T) {
		pin := gateway.GeneratePIN()
		if len(pin) != 4 {
			t.Errorf("Expected 4 digit PIN, got %v", pin)
		}
		if gateway.connected {
			t.Errorf("Gateway should not be connected upon generating fresh PIN")
		}
	})

	// ==========================================
	// Test Case 2: Wrong PIN
	// ==========================================
	t.Run("WrongPIN", func(t *testing.T) {
		gateway.GeneratePIN() // Assigns a new pin
		
		authEnv := Envelope{
			Topic: "p2p.auth",
			From:  "guest",
			Payload: map[string]interface{}{
				"pin": "0000", // statistically very unlikely to match the true random pin
			},
		}

		// Try to auth
		gateway.HandleP2P(authEnv)

		if gateway.connected {
			t.Errorf("Gateway should strictly reject a wrong PIN but it set connected=true")
		}
	})

	// ==========================================
	// Test Case 3: Correct PIN
	// ==========================================
	t.Run("CorrectPIN", func(t *testing.T) {
		pin := gateway.GeneratePIN()
		
		authEnv := Envelope{
			Topic: "p2p.auth",
			From:  "guest",
			Payload: map[string]interface{}{
				"pin": pin,
			},
		}

		gateway.HandleP2P(authEnv)

		if !gateway.connected {
			t.Errorf("Gateway should accept the correct dynamically generated PIN")
		}
	})

	// ==========================================
	// Test Case 4: Route Allowed Topic
	// ==========================================
	t.Run("RouteAllowedTopic", func(t *testing.T) {
		// Create a mock channel to capture what the bus tried to publish locally
		captureChan := make(chan Envelope, 10)
		
		// Setup a mock client on the bus to receive broadcasts
		mockClient := &Client{ID: "local-node", send: captureChan}
		var mockConn *websocket.Conn // use nil to represent a dummy client connection key
		
		bus.lock.Lock()
		bus.clients[mockConn] = mockClient
		bus.lock.Unlock()

		gateway.GeneratePIN()
		gateway.connected = true // Force connected

		routeEnv := Envelope{
			Topic: "p2p.route",
			From:  "guest",
			Payload: map[string]interface{}{
				"topic": "vm.spawn", // allowed
				"from":  "guest-peer",
				"to":    "local-node",
				"payload": map[string]interface{}{"cmd": "echo"},
			},
		}

		gateway.HandleP2P(routeEnv)

		// Check if it was forwarded
		select {
		case pubEnv := <-captureChan:
			if pubEnv.Topic != "vm.spawn" {
				t.Errorf("Expected inner topic 'vm.spawn', got %v", pubEnv.Topic)
			}
			if pubEnv.From != "guest-peer" {
				t.Errorf("Expected from 'guest-peer', got %v", pubEnv.From)
			}
		default:
			t.Errorf("Expected bus.Publish to be called, but nothing was sent")
		}
		
		// Clean up bus mock
		bus.lock.Lock()
		delete(bus.clients, mockConn)
		bus.lock.Unlock()
	})

	// ==========================================
	// Test Case 5: Route Blocked Topic
	// ==========================================
	t.Run("RouteBlockedTopic", func(t *testing.T) {
		captureChan := make(chan Envelope, 10)
		mockClient := &Client{ID: "local-node", send: captureChan}
		var mockConn *websocket.Conn
		
		bus.lock.Lock()
		bus.clients[mockConn] = mockClient
		bus.lock.Unlock()

		gateway.GeneratePIN()
		gateway.connected = true 

		routeEnv := Envelope{
			Topic: "p2p.route",
			From:  "guest",
			Payload: map[string]interface{}{
				"topic": "sys.register", // explicitly absolutely NOT allowed
				"from":  "guest-peer",
				"to":    "local-node",
			},
		}

		gateway.HandleP2P(routeEnv)

		// Should NOT be forwarded
		select {
		case <-captureChan:
			t.Errorf("Expected blocked topic to be rejected, but it was published locally")
		default:
			// Success, channel is empty
		}
		
		bus.lock.Lock()
		delete(bus.clients, mockConn)
		bus.lock.Unlock()
	})
}

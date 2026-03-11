package main

import (
	"log"
	"math/rand"
	"time"
)

// P2PGateway manages P2P session pins and routing rules
type P2PGateway struct {
	bus       *Bus
	pin       string
	connected bool
	allowed   map[string]bool
}

func NewP2PGateway(bus *Bus) *P2PGateway {
	return &P2PGateway{
		bus: bus,
		// Example allowed public topics
		allowed: map[string]bool{
			"vfs:read": true,
			"vm.spawn": true,
			"ai.chat":  true,
		},
	}
}

// GeneratePIN creates a new 4-digit PIN for pairing
func (g *P2PGateway) GeneratePIN() string {
	const digits = "0123456789"
	b := make([]byte, 4)
	for i := range b {
		b[i] = digits[rand.Intn(len(digits))]
	}
	g.pin = string(b)
	g.connected = false
	log.Printf("[P2P] Genererated new Session PIN: %s", g.pin)
	return g.pin
}

// HandleP2P routes incoming p2p envelopes from the UI Proxy
func (g *P2PGateway) HandleP2P(env Envelope) {
	if env.Topic == "p2p.host:start" {
		pin := g.GeneratePIN()
		g.bus.Publish(Envelope{
			Topic: "p2p.host:ready",
			From:  "kernel",
			To:    env.From,
			Payload: map[string]string{
				"pin": pin,
			},
			Time: time.Now().Format(time.RFC3339),
		})
		return
	}

	if env.Topic == "p2p.auth" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			pin, _ := payload["pin"].(string)
			if pin == g.pin && g.pin != "" {
				g.connected = true
				log.Println("✅ P2P Guest Authenticated")
				
				g.bus.Publish(Envelope{
					Topic: "p2p.auth:ack",
					From:  "kernel",
					To:    env.From,
					Time:  time.Now().Format(time.RFC3339),
				})
				
				// Share topology
				g.bus.Publish(Envelope{
					Topic: "p2p.topology",
					From:  "kernel",
					To:    env.From,
					Payload: map[string]interface{}{
						"allowed_topics": g.allowed,
					},
					Time: time.Now().Format(time.RFC3339),
				})
			} else {
				log.Printf("⛔ P2P Auth Failed. Given: %s, Expected: %s", pin, g.pin)
			}
		}
		return
	}

	if env.Topic == "p2p.route" {
		if !g.connected {
			log.Println("⛔ P2P unauthorized route attempt rejected.")
			return
		}

		payloadMap, ok := env.Payload.(map[string]interface{})
		if !ok {
			return
		}
		
		topic, _ := payloadMap["topic"].(string)
		
		// Auth Gate
		if !g.allowed[topic] {
			log.Printf("⛔ P2P unauthorized topic attempt: %s", topic)
			return
		}

		from, _ := payloadMap["from"].(string)
		to, _ := payloadMap["to"].(string)
		innerPayload := payloadMap["payload"]
		
		// Reconstruct inner envelope
		innerEnv := Envelope{
			Topic:   topic,
			From:    from, // e.g. "node-b/terminal"
			To:      to,   // e.g. "node-a/vm"
			Payload: innerPayload,
			Time:    time.Now().Format(time.RFC3339),
		}
		
		log.Printf("[P2P] Injecting routed envelope: %s (From: %s)", topic, from)
		
		// Forward it to the local bus - but ensure responses get routed back.
		// Local handlers will reply to innerEnv.From.
		// Our bus needs to know to proxy anything addressed to "node-*/..." out to WebRTC.
		g.bus.Publish(innerEnv)
	}
}

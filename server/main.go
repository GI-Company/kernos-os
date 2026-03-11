package main

import (
	"context"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/time/rate"
)

//go:embed dist/*
var embeddedUI embed.FS

// Global AuthToken for Zero-Trust Handshake
var AuthToken string

var upgrader = websocket.Upgrader{
	// In production, strictly check origin to prevent CSRF
	CheckOrigin: func(r *http.Request) bool { return true },
}

// SECURITY: Only allow these specific commands to be executed.
var ALLOWED_COMMANDS = map[string]bool{
	"ls":      true,
	"echo":    true,
	"cat":     true,
	"mkdir":   true,
	"touch":   true,
	"date":    true,
	"whoami":  true,
	"grep":    true,
	"wc":      true,
	"git":     true, // Caution: git can still be powerful
	"node":    true, // Caution: node allows code execution
	"python3": true,
}

type Envelope struct {
	Topic   string      `json:"topic"`
	From    string      `json:"from"`
	To      string      `json:"to,omitempty"`
	Payload interface{} `json:"payload"`
	Time    string      `json:"time"`
}

type Client struct {
	ID            string `json:"id"`
	Role          string `json:"role"`
	Name          string `json:"name,omitempty"`
	Model         string `json:"model,omitempty"`
	Authenticated bool   `json:"-"` // Internal flag, not exposed
	Subscriptions map[string]bool `json:"-"`
	send          chan Envelope
}

type Bus struct {
	clients map[*websocket.Conn]*Client
	lock    sync.Mutex
}

func (b *Bus) Publish(env Envelope) {
	b.lock.Lock()
	defer b.lock.Unlock()
	for conn, client := range b.clients {
		// If To is specified and it's not this client, check for P2P cross-routing
		if env.To != "" && (client == nil || client.ID != env.To) {
			// If the destination has a '/' (like 'node-b/vm'), forward it to the UI proxy
			// so the WebRTC layer can tunnel it to the correct peer.
			if strings.Contains(env.To, "/") && client != nil && client.Role == "ui" {
				// Allow this envelope to drop through to the UI client
			} else {
				continue
			}
		}

		// Event Subscription Filter
		if client != nil && len(client.Subscriptions) > 0 {
			matched := false
			for sub := range client.Subscriptions {
				if strings.HasPrefix(env.Topic, sub) {
					matched = true
					break
				}
			}
			if !matched && (env.To == "" || env.To != client.ID) { // Still route direct messages
				continue
			}
		}

		if client != nil && client.send != nil {
			select {
			case client.send <- env:
			default:
				// Channel full, slow client. Disconnect them to prevent blocking others.
				// Closing the connection will cause ReadJSON to fail, triggering cleanup defer.
				conn.Close()
			}
		}
	}
}

func writePump(conn *websocket.Conn, send chan Envelope) {
	defer conn.Close()
	for env := range send {
		conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if err := conn.WriteJSON(env); err != nil {
			return
		}
	}
}

func main() {
	// CLI flags
	lmURL = flag.String("lm", "http://192.168.1.82:1234/v1/chat/completions", "LM Studio API URL")
	noBrowser := flag.Bool("no-browser", false, "Don't auto-open the browser")
	noAgents := flag.Bool("no-agents", false, "Don't start embedded AI agents")
	workspaceDir := flag.String("workspace", ".", "Directory to auto-index for Semantic VFS")
	flag.Parse()

	// Initialize Structured Logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	// Optionally bridge standard log to slog:
	log.SetFlags(0)
	log.SetOutput(os.Stdout)
	slog.Info("Initializing Kernos OS Cognitive Microkernel...")

	// Intercept CLI tool commands
	if flag.NArg() > 0 && flag.Arg(0) == "create-applet" {
		name := "UntitledApplet"
		if flag.NArg() > 1 {
			name = flag.Arg(1)
		}
		appletPath := name + ".tsx"
		template := `import React, { useState } from 'react';
import * as Lucide from 'lucide-react';

export default function ` + name + `() {
  const [count, setCount] = useState(0);

  return (
    <div className="p-4 text-white font-sans flex flex-col items-center justify-center h-full">
      <Lucide.Sparkles className="text-pink-400 mb-4" size={32} />
      <h2 className="text-xl font-bold mb-2">Hello from ` + name + `!</h2>
      <p className="text-slate-400 mb-4">This is a dynamically compiled React Applet.</p>
      
      <button 
        onClick={() => setCount(c => c + 1)}
        className="px-4 py-2 bg-pink-500/20 hover:bg-pink-500/40 text-pink-300 rounded transition-colors"
      >
        Clicked {count} times
      </button>
    </div>
  );
}
`
		err := os.WriteFile(appletPath, []byte(template), 0644)
		if err != nil {
			fmt.Printf("❌ Failed to create applet %s: %v\n", appletPath, err)
			os.Exit(1)
		}
		fmt.Printf("✅ Created dynamic React Applet: %s\n", appletPath)
		fmt.Printf("👉 Type 'kernos' to boot the OS, then open this file in the Editor and click 'Launch Applet'!\n")
		os.Exit(0)
	}

	// ---------------------------------------------------------------------------
	// ZERO-TRUST PERSISTENT SYSTEM DB
	// Initializes ~/.kernos/sys.db to load the root token, JWT secrets, and config.
	// ---------------------------------------------------------------------------
	InitSysDB()
	log.Printf("🔐 Persistent Root Auth Token (sys.db): %s", AuthToken)

	bus := &Bus{clients: make(map[*websocket.Conn]*Client)}
	taskEngine := NewTaskEngine(bus)

	// Global Context and WaitGroup for precise OS lifecycle management
	ctx, cancelOS := context.WithCancel(context.Background())
	var wg sync.WaitGroup

	// Initialize System Daemons
	StartCron(ctx, &wg, bus)
	StartVFSWatcher(bus, ".")

	// Initialize Speculative Execution Engines
	InitShadowEngine()
	predEngine := NewPredictionEngine(bus, *lmURL)

	// Initialize Applet Engine (React Compilation)
	appEngine := NewAppletEngine(bus)

	// Initialize P2P Gateway
	p2pGateway := NewP2PGateway(bus)

	// Initialize RLHF Telemetry DB
	fmt.Println("📊 Initializing Local RLHF Telemetry...")
	InitTelemetry()

	// Initialize Vector Engine and auto-index the workspace
	fmt.Println("🧠 Initializing Cognitive Vector Engine...")
	vdb := InitVectorDB(*lmURL)
	go vdb.AutoIndexWorkspace(*workspaceDir)

	// HTTP endpoint for JWT Session Login
	http.HandleFunc("/auth/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Token string `json:"token"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad Request", http.StatusBadRequest)
			return
		}

		if req.Token != AuthToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		jwtToken, err := GlobalSysDB.CreateSessionToken()
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		http.SetCookie(w, &http.Cookie{
			Name:     "kernos_session",
			Value:    jwtToken,
			Path:     "/",
			HttpOnly: true,
			Expires:  time.Now().Add(30 * 24 * time.Hour),
		})

		GlobalSysDB.LogAudit("sys.login", "root", "User logged in with Root Token via HTTP POST")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// Phase 11: GitHub OAuth + RBAC Routes
	InitOAuthRoutes(http.DefaultServeMux)

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println("Upgrade error:", err)
			return
		}

		// Check for valid JWT Cookie seamlessly
		isAuthenticated := false
		if cookie, err := r.Cookie("kernos_session"); err == nil {
			if GlobalSysDB.ValidateSession(cookie.Value) {
				isAuthenticated = true
			}
		}

		clientSend := make(chan Envelope, 256)
		bus.lock.Lock()
		clientObj := &Client{
			ID: "anonymous", 
			Role: "guest", 
			Authenticated: isAuthenticated, 
			Subscriptions: make(map[string]bool),
			send: clientSend,
		}
		bus.clients[conn] = clientObj
		bus.lock.Unlock()

		if isAuthenticated {
			slog.Info("Client securely re-authenticated via JWT session cookie.")
		}

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
			log.Println("Client connection closed and cleaned up")
		}()

		log.Println("Client TCP connection established, awaiting sys.auth...")

		// Authentication Timeout
		go func(c *websocket.Conn) {
			time.Sleep(3 * time.Second)
			bus.lock.Lock()
			if client, ok := bus.clients[c]; ok && !client.Authenticated {
				log.Println("⛔ Client failed to authenticate within 3s. Disconnecting.")
				c.Close()
				delete(bus.clients, c)
			}
			bus.lock.Unlock()
		}(conn)

		for {
			var env Envelope
			err := conn.ReadJSON(&env)
			if err != nil {
				// Defer will handle cleanup
				break
			}

			// 1. Enforce Authentication Handshake first
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
						log.Println("✅ Client successfully authenticated via Zero-Trust handhshake.")
						bus.Publish(Envelope{
							Topic: "sys.auth:ack",
							From:  "kernel",
							Time:  time.Now().Format(time.RFC3339),
						})
					} else {
						log.Printf("⛔ Invalid Auth Token received: %s", token)
						conn.Close()
					}
				}
				continue
			}

			// Block all other messages if unauthenticated
			bus.lock.Lock()
			isAuthenticated := bus.clients[conn] != nil && bus.clients[conn].Authenticated
			bus.lock.Unlock()

			if !isAuthenticated {
				log.Printf("⛔ Ignoring unauthorized message topic: %s", env.Topic)
				continue
			}

			// 2. Handle sys.register
			if env.Topic == "sys.register" {
				payload, ok := env.Payload.(map[string]interface{})
				if ok {
					id, _ := payload["id"].(string)
					role, _ := payload["role"].(string)
					name, _ := payload["name"].(string)
					model, _ := payload["model"].(string)
					bus.lock.Lock()
					if client, exists := bus.clients[conn]; exists {
						client.ID = id
						client.Role = role
						client.Name = name
						client.Model = model
					}
					bus.lock.Unlock()
					log.Printf("Client registered: %s (%s) [%s]", id, role, name)

					// Send ACK
					bus.Publish(Envelope{
						Topic: "sys.register:ack",
						From:  "kernel",
						To:    id,
						Time:  time.Now().Format(time.RFC3339),
					})
				}
				continue
			}

			// Route message
			go bus.Publish(env)
			// Route standard kernel messages
			go handleEnvelope(bus, env, taskEngine, predEngine, appEngine, p2pGateway)
		}
	})

	// SECURITY: Bind to localhost only to prevent external network access
	addr := "127.0.0.1:8080"

	// Heartbeat: broadcast client list every 5s
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			bus.lock.Lock()
			var clientList []Client
			for _, c := range bus.clients {
				if c != nil && c.ID != "anonymous" {
					clientList = append(clientList, *c)
				}
			}
			bus.lock.Unlock()
			bus.Publish(Envelope{
				Topic:   "sys.client_list",
				From:    "kernel",
				Time:    time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{"clients": clientList},
			})
		}
	}()
	// --- Serve embedded frontend as SPA ---
	ui, err := fs.Sub(embeddedUI, "dist")
	if err != nil {
		log.Fatal("Failed to load embedded UI:", err)
	}
	fileServer := http.FileServer(http.FS(ui))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// SPA fallback: if the file doesn't exist, serve index.html
		path := r.URL.Path
		if path == "/" {
			fileServer.ServeHTTP(w, r)
			return
		}
		// Try to open the file from the embedded FS
		_, err := fs.Stat(ui, strings.TrimPrefix(path, "/"))
		if err != nil {
			// File not found — serve index.html for SPA routing
			r.URL.Path = "/"
		}
		fileServer.ServeHTTP(w, r)
	})

	fmt.Println("╔══════════════════════════════════════╗")
	fmt.Println("║         KERNOS OS v1.0.0             ║")
	fmt.Println("║    Browser-Native Cognitive OS       ║")
	fmt.Println("╠══════════════════════════════════════╣")
	fmt.Printf("║  UI:     http://%s          ║\n", addr)
	fmt.Printf("║  WS:     ws://%s/ws        ║\n", addr)
	fmt.Printf("║  Mode:   SECURE (Allowlist)           ║\n")
	fmt.Printf("╚══════════════════════════════════════╝\n\n")
	// Background reaper for execution jails
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				slog.Info("Halting sandbox execution reaper...")
				return
			case <-ticker.C:
				entries, err := os.ReadDir(os.TempDir())
				if err == nil {
					for _, entry := range entries {
						if strings.HasPrefix(entry.Name(), "kernos_jail_") {
							info, err := entry.Info()
							if err == nil && time.Since(info.ModTime()) > 24*time.Hour {
								os.RemoveAll(filepath.Join(os.TempDir(), entry.Name()))
							}
						}
					}
				}
			}
		}
	}()

	// Auto-start embedded agents
	if !*noAgents {
		fmt.Println("🤖 Starting AI agents...")
		go StartEmbeddedAgents(*lmURL, AuthToken)
	}

	// Auto-open browser with the auth token in the hash
	if !*noBrowser {
		go func() {
			time.Sleep(800 * time.Millisecond)
			openBrowser(fmt.Sprintf("http://%s/#auth=%s", addr, AuthToken))
		}()
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: nil, // uses http.DefaultServeMux
	}

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP Server failed to start", "error", err)
			os.Exit(1)
		}
	}()

	// Graceful Shutdown Handler
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	<-stop
	slog.Info("Received shutdown signal. Commencing graceful teardown...")

	// Cancel the master context to stop Cron, Jails, Vectors, etc.
	cancelOS()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Kernel forced to shutdown due to timeout or error", "error", err)
	}

	// Wait for any critical WaitGroups here...
	slog.Info("Waiting for background subsystems to sync states...")
	c := make(chan struct{})
	go func() {
		defer close(c)
		wg.Wait()
	}()
	select {
	case <-c:
		slog.Info("Subsystems halted safely.")
	case <-shutdownCtx.Done():
		slog.Warn("Forcing closure of running background jobs after 10s wait period.")
	}

	// Flush caches, close vector DB DB handles, etc.
	if GlobalVectorDB != nil && GlobalVectorDB.DB != nil {
		slog.Info("Flushing vector graph database to disk...")
		GlobalVectorDB.DB.Close()
	}

	slog.Info("Kernos OS offline. Goodbye.")
}

func openBrowser(url string) {
	var err error
	switch runtime.GOOS {
	case "darwin":
		err = exec.Command("open", url).Start()
	case "linux":
		err = exec.Command("xdg-open", url).Start()
	case "windows":
		err = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
	if err != nil {
		log.Printf("Could not open browser: %v", err)
	}
}

var lmURL *string

func handleEnvelope(bus *Bus, env Envelope, te *TaskEngine, pe *PredictionEngine, ae *AppletEngine, p2p *P2PGateway) {
	// ---------------------------------------------------------------------------
	// KERNEL PANIC RECOVERY
	// If a subsystem crashes while processing a message, we catch the panic here,
	// print the stack trace, and notify the UI, rather than letting the Go binary die.
	// ---------------------------------------------------------------------------
	defer func() {
		if r := recover(); r != nil {
			errStr := fmt.Sprintf("CRITICAL SUBSYSTEM PANIC: %v", r)
			log.Println("\n=======================================================")
			log.Println("=======================================================")
			bus.Publish(Envelope{
				Topic: "sys.kernel_panic",
				From:  "kernel",
				Payload: map[string]string{
					"subsystem": env.Topic,
					"error":     errStr,
				},
				Time: time.Now().Format(time.RFC3339),
			})
		}
	}()

	if strings.HasPrefix(env.Topic, "p2p.") {
		p2p.HandleP2P(env)
	}

	if strings.HasPrefix(env.Topic, "vm.spawn") {
		handleVMSpawn(bus, env)
	}

	if strings.HasPrefix(env.Topic, "task.run") {
		handleTaskRun(env, te)
	}
	if env.Topic == "plugin.run" {
		handlePluginRun(bus, env)
	}
	if strings.HasPrefix(env.Topic, "vfs:semantic") {
		handleVFSSemantic(bus, env)
	}
	if env.Topic == "vfs:read" {
		handleVFSRead(bus, env)
	}
	if env.Topic == "editor.typing" {
		pe.HandleEditorTyping(env)
	}
	if env.Topic == "terminal.typing" {
		pe.HandleTerminalTyping(env)
	}
	if env.Topic == "sys.terminal.intent" {
		pe.HandleTerminalIntent(env)
	}
	if env.Topic == "terminal.check_shadow" {
		handleTerminalCheckShadow(bus, env)
	}
	if env.Topic == "sys.consolidate" {
		go RunSynapticConsolidation(*lmURL)
	}
	if env.Topic == "applet.compile" {
		go ae.CompileApplet(env)
	}
	if env.Topic == "pkg.install" {
		handlePkgInstall(bus, env)
	}
	if env.Topic == "sys.config:get" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			if key, kOk := payload["key"].(string); kOk {
				val := GlobalSysDB.GetConfig(key)
				bus.Publish(Envelope{
					Topic: "sys.config:ack",
					From:  "kernel",
					To:    env.From,
					Time:  time.Now().Format(time.RFC3339),
					Payload: map[string]string{"key": key, "value": val},
				})
			}
		}
	}
	if env.Topic == "sys.config:set" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			key, kOk := payload["key"].(string)
			val, vOk := payload["value"].(string)
			if kOk && vOk {
				oldVal := GlobalSysDB.GetConfig(key)
				GlobalSysDB.LogUndoSnapshot("sys.config", key, oldVal)
				
				GlobalSysDB.SetConfig(key, val)
				GlobalSysDB.LogAudit("sys.config:set", env.From, "Updated OS configuration: "+key)
			}
		}
	}
	if env.Topic == "sys.undo:trigger" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			category, _ := payload["category"].(string)
			target, _ := payload["target"].(string)
			evt, err := GlobalSysDB.RevertLastAction(category, target)
			if err == nil && evt != nil {
				if category == "sys.config" {
					GlobalSysDB.SetConfig(target, evt.PrevData)
				}
				bus.Publish(Envelope{
					Topic:   "sys.notify:toast",
					From:    "kernel",
					Payload: map[string]string{"message": "Reverted " + target + " to previous state."},
					Time:    time.Now().Format(time.RFC3339),
				})
			}
		}
	}
	if env.Topic == "sys.subscribe" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			topic, _ := payload["topic"].(string)
			bus.lock.Lock()
			for _, c := range bus.clients {
				if c != nil && c.ID == env.From {
					c.Subscriptions[topic] = true
				}
			}
			bus.lock.Unlock()
		}
	}
	if env.Topic == "sys.unsubscribe" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			topic, _ := payload["topic"].(string)
			bus.lock.Lock()
			for _, c := range bus.clients {
				if c != nil && c.ID == env.From {
					delete(c.Subscriptions, topic)
				}
			}
			bus.lock.Unlock()
		}
	}
	if env.Topic == "sys.ps" {
		procLock.Lock()
		var procs []map[string]interface{}
		for id, cmd := range activeProcesses {
			pid := -1
			if cmd.Process != nil {
				pid = cmd.Process.Pid
			}
			procs = append(procs, map[string]interface{}{
				"id":   id,
				"pid":  pid,
				"args": cmd.Args,
			})
		}
		procLock.Unlock()
		bus.Publish(Envelope{
			Topic:   "sys.ps:reply",
			From:    "kernel",
			To:      env.From,
			Payload: map[string]interface{}{"processes": procs},
			Time:    time.Now().Format(time.RFC3339),
		})
	}
	if env.Topic == "sys.kill" {
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			id, _ := payload["id"].(string)
			procLock.Lock()
			if cmd, exists := activeProcesses[id]; exists && cmd.Process != nil {
				cmd.Process.Kill()
				delete(activeProcesses, id)
			}
			procLock.Unlock()
		}
	}
	if env.Topic == "sys.clipboard:copy" {
		// Broadcast clipboard update to all UI nodes and peers
		payload, ok := env.Payload.(map[string]interface{})
		if ok {
			text, _ := payload["text"].(string)
			GlobalSysDB.LogAudit("sys.clipboard", env.From, "Clipboard synced across OS")
			bus.Publish(Envelope{
				Topic:   "sys.clipboard:update",
				From:    env.From, // So the sender knows it's their own clip
				Payload: map[string]string{"text": text},
				Time:    time.Now().Format(time.RFC3339),
			})
		}
	}
	if env.Topic == "sys.notify" {
		// Allows any agent/client to trigger a UI toast
		bus.Publish(Envelope{
			Topic:   "sys.notify:toast",
			From:    env.From,
			Payload: env.Payload,
			Time:    time.Now().Format(time.RFC3339),
		})
	}
}

func handleVFSRead(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	id, _ := payload["id"].(string)
	reqId, _ := payload["_request_id"].(string)

	if id == "" {
		return
	}

	content, err := os.ReadFile(id)
	if err != nil {
		log.Printf("[VFS] Read Error (%s): %v", id, err)
	}

	bus.Publish(Envelope{
		Topic: "vfs:read:resp",
		From:  "kernel",
		To:    env.From,
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{
			"_request_id": reqId,
			"id":          id,
			"content":     string(content),
		},
	})
}

// handleVFSSemantic intercepts virtual file system queries for the embedding space
func handleVFSSemantic(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}
	query, _ := payload["query"].(string)
	if query == "" {
		return
	}

	go func() {
		// Embed the query
		vectors, err := GlobalVectorDB.GenerateEmbeddings([]string{"search_query: " + query})
		if err != nil || len(vectors) == 0 {
			log.Printf("[VFS] Error embedding query: %v", err)
			return
		}

		// Search the vector space
		results := GlobalVectorDB.Search(vectors[0], 5)

		// Format as synthetic VFS nodes
		var nodes []map[string]interface{}
		for _, r := range results {
			nodes = append(nodes, map[string]interface{}{
				"id":         r.Chunk.ID,
				"name":       fmt.Sprintf("%s (Lines %d-%d)", r.Chunk.FilePath, r.Chunk.StartLine, r.Chunk.EndLine),
				"type":       "semantic_node",
				"similarity": r.Similarity,
				"content":    r.Chunk.Text,
			})
		}

		bus.Publish(Envelope{
			Topic: "vfs:semantic:result",
			From:  "kernel",
			To:    env.From,
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"query":   query,
				"results": nodes,
			},
		})
	}()
}

// Global Rate Limiter Map for VM executions
var vmLimiters = make(map[string]*rate.Limiter)
var vmLimitersLock sync.Mutex

func getClientLimiter(clientID string) *rate.Limiter {
	vmLimitersLock.Lock()
	defer vmLimitersLock.Unlock()
	
	limiter, exists := vmLimiters[clientID]
	if !exists {
		// 2 commands per second, burst of 5
		limiter = rate.NewLimiter(rate.Limit(2), 5)
		vmLimiters[clientID] = limiter
	}
	return limiter
}

func handleVMSpawn(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	reqID, _ := payload["_request_id"].(string)

	// Rate Limiting Check
	limiter := getClientLimiter(env.From)
	if !limiter.Allow() {
		bus.Publish(Envelope{
			Topic:   "vm.stderr",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]string{"_request_id": reqID, "text": "RATE LIMIT EXCEEDED: Too many commands executed recently. Please slow down.\n"},
		})
		bus.Publish(Envelope{
			Topic:   "vm.exit",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{"_request_id": reqID, "code": 429},
		})
		return
	}
	cmdStr, _ := payload["cmd"].(string)
	argsInterface, _ := payload["args"].([]interface{})

	var args []string
	for _, v := range argsInterface {
		arg, ok := v.(string)
		if !ok {
			continue
		}
		args = append(args, arg)
	}
	
	GlobalSysDB.LogAudit("vm.spawn", env.From, "Execution request: "+cmdStr+" "+strings.Join(args, " "))

	// 1. ACK
	bus.Publish(Envelope{
		Topic:   "vm.spawn:ack",
		From:    "kernel",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"_request_id": reqID, "instanceId": "live-inst-1"},
	})

	output, err := ExecuteSafeCommand(reqID, cmdStr, args)

	if err != nil {
		// Build error message. If allowed/validation error, output is empty but err has text.
		// If exec error, output might have stderr.
		errMsg := err.Error()
		if output != "" {
			errMsg += "\n" + output
		}

		bus.Publish(Envelope{
			Topic:   "vm.stderr",
			From:    "kernel",
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]string{"_request_id": reqID, "text": errMsg},
		})
		bus.Publish(Envelope{
			Topic:   "vm.exit",
			From:    "kernel",
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{"_request_id": reqID, "code": 1},
		})
		return
	}

	bus.Publish(Envelope{
		Topic:   "vm.stdout",
		From:    "kernel",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"_request_id": reqID, "text": output},
	})

	bus.Publish(Envelope{
		Topic:   "vm.exit",
		From:    "kernel",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{"_request_id": reqID, "code": 0},
	})
}

func sendError(bus *Bus, reqID string, msg string) {
	bus.Publish(Envelope{
		Topic:   "vm.stderr",
		From:    "kernel",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"_request_id": reqID, "text": msg + "\n"},
	})
	bus.Publish(Envelope{
		Topic:   "vm.exit",
		From:    "kernel",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{"_request_id": reqID, "code": 126},
	})
}


func handleTaskRun(env Envelope, te *TaskEngine) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	graphID, _ := payload["graphId"].(string)
	if graphID == "" {
		graphID = "dynamic-graph-" + time.Now().Format("150405")
	}

	// 1. Phase 9: Autonomous Agent Loop (ReAct)
	if goal, exists := payload["goal"].(string); exists && goal != "" {
		go te.ExecuteGoal(graphID, goal)
		return
	}

	// 2. Try to parse dynamic nodes if provided
	if nodesRaw, exists := payload["nodes"]; exists {
		// Use JSON marshaling roundtrip to convert map[string]interface{} to []TaskNode
		nodesBytes, err := json.Marshal(nodesRaw)
		if err == nil {
			var parsedNodes []TaskNode
			if err := json.Unmarshal(nodesBytes, &parsedNodes); err == nil {
				log.Printf("Successfully parsed dynamic graph with %d nodes", len(parsedNodes))
				go te.ExecuteGraph(graphID, parsedNodes)
				return
			} else {
				log.Printf("Failed to unmarshal dynamic nodes: %v\nJSON was: %s", err, string(nodesBytes))
			}
		} else {
			log.Printf("Failed to marshal nodesRaw: %v", err)
		}
	} else {
		log.Printf("No 'nodes' field found in payload")
	}

	// Fallback to hardcoded MVP pipeline
	if graphID == "build-pipeline" {
		nodes := []TaskNode{
			{ID: "lint", Command: "echo 'linting: all good'", Dependencies: []string{}},
			{ID: "test", Command: "whoami", Dependencies: []string{"lint"}},
			{ID: "build", Command: "ls -la", Dependencies: []string{"test"}},
			{ID: "deploy", Command: "date", Dependencies: []string{"build"}},
		}
		go te.ExecuteGraph(graphID, nodes)
	}
}

var (
	activeProcesses = make(map[string]*exec.Cmd)
	procLock        sync.Mutex
)

func ExecuteSafeCommand(reqID string, cmdStr string, args []string) (string, error) {
	// SECURITY CHECK 1: Is the command allowed?
	if !ALLOWED_COMMANDS[cmdStr] {
		return "", fmt.Errorf("PERMISSION DENIED: Command '%s' is not in the kernel allowlist.", cmdStr)
	}

	var safeArgs []string
	for _, arg := range args {
		// SECURITY CHECK 2: Input Sanitization
		if strings.ContainsAny(arg, "&|;`$()<>") {
			return "", fmt.Errorf("PERMISSION DENIED: Argument '%s' contains illegal characters.", arg)
		}

		// SECURITY CHECK 3: Path Traversal
		if strings.Contains(arg, "..") {
			return "", fmt.Errorf("PERMISSION DENIED: Path traversal (..) is not allowed.")
		}

		// SECURITY CHECK 4: Absolute Paths
		if strings.HasPrefix(arg, "/") || strings.HasPrefix(arg, "\\") {
			return "", fmt.Errorf("PERMISSION DENIED: Absolute paths are not allowed.")
		}

		safeArgs = append(safeArgs, arg)
	}

	// ---------------------------------------------------------------------------
	// DETERMINISTIC TASK JAIL (Phase 11 Hardened):
	// Even if an AI Agent approves a command, we do NOT run it in the host workspace.
	// We create a temporary sandbox directory to act as a pseudo-chroot.
	// ---------------------------------------------------------------------------
	jailDir, err := os.MkdirTemp("", "kernos_jail_*")
	if err != nil {
		return "", fmt.Errorf("JAIL ERROR: Could not create execution sandbox: %v", err)
	}
	defer os.RemoveAll(jailDir) // Phase 11: Auto-cleanup jail after execution

	// Phase 11: 30-second hard timeout to prevent infinite loops
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, cmdStr, safeArgs...)
	cmd.Dir = jailDir // Force execution inside the jail

	// Phase 11: Strip all host environment variables to prevent host discovery
	cmd.Env = []string{
		"PATH=/usr/bin:/bin",
		"HOME=" + jailDir,
		"TMPDIR=" + jailDir,
		"LANG=en_US.UTF-8",
	}

	procLock.Lock()
	activeProcesses[reqID] = cmd
	procLock.Unlock()

	defer func() {
		procLock.Lock()
		delete(activeProcesses, reqID)
		procLock.Unlock()
	}()

	output, err := cmd.CombinedOutput()

	// Phase 11: Output size cap (1MB) to prevent memory bombs
	const maxOutputBytes = 1024 * 1024
	if len(output) > maxOutputBytes {
		output = append(output[:maxOutputBytes], []byte("\n... [OUTPUT TRUNCATED AT 1MB]")...)
	}

	// Detect timeout specifically
	if ctx.Err() == context.DeadlineExceeded {
		return string(output), fmt.Errorf("TIMEOUT: Process exceeded 30-second execution limit and was killed.")
	}

	return string(output), err
}

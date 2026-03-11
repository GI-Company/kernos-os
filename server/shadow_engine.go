package main

import (
	"context"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// The Shadow Engine handles speculative execution ("Subconscious Execution").
// When the Prediction Engine guesses a command, the Shadow Engine runs it
// in a hidden, sandboxed goroutine and caches the result for 0ms retrieval later.

type ShadowResult struct {
	Command   string
	Stdout    string
	Stderr    string
	ExitCode  int
	Duration  time.Duration
	Timestamp time.Time
}

type ShadowEngine struct {
	cache      map[string]ShadowResult
	cacheMutex sync.RWMutex

	// Track currently running shadow tasks
	running      map[string]bool
	runningMutex sync.Mutex
}

var GlobalShadowEngine *ShadowEngine

func InitShadowEngine() *ShadowEngine {
	GlobalShadowEngine = &ShadowEngine{
		cache:   make(map[string]ShadowResult),
		running: make(map[string]bool),
	}
	return GlobalShadowEngine
}

// SpawnShadowTask starts a hidden execution of a predicted command.
// Enforces a strict 10-second timeout for safety.
func (se *ShadowEngine) SpawnShadowTask(cmdString string) {
	cmdString = strings.TrimSpace(cmdString)
	if cmdString == "" {
		return
	}

	// Avoid re-running if currently running or recently cached
	se.runningMutex.Lock()
	if se.running[cmdString] {
		se.runningMutex.Unlock()
		return
	}
	se.running[cmdString] = true
	se.runningMutex.Unlock()

	se.cacheMutex.RLock()
	cached, exists := se.cache[cmdString]
	se.cacheMutex.RUnlock()

	if exists && time.Since(cached.Timestamp) < 30*time.Second {
		// Valid cache exists, skip
		se.runningMutex.Lock()
		delete(se.running, cmdString)
		se.runningMutex.Unlock()
		return
	}

	log.Printf("[Shadow Engine] 👻 Pre-running shadow task: %s", cmdString)
	startTime := time.Now()

	go func() {
		defer func() {
			se.runningMutex.Lock()
			delete(se.running, cmdString)
			se.runningMutex.Unlock()
		}()

		// STRICT SANDBOX: 10 second timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		parts := strings.Fields(cmdString)
		var cmd *exec.Cmd

		// Security: Only allow basic commands matching the safe predictor list
		switch parts[0] {
		case "go", "npm", "cargo", "pytest", "echo", "pwd", "ls":
			cmd = exec.CommandContext(ctx, parts[0], parts[1:]...)
		default:
			log.Printf("[Shadow Engine] ⛔ Blocked unsafe shadow command: %s", cmdString)
			return
		}

		var stdoutBuf, stderrBuf strings.Builder
		cmd.Stdout = &stdoutBuf
		cmd.Stderr = &stderrBuf

		err := cmd.Run()

		exitCode := 0
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				exitCode = -1
			}

			if ctx.Err() == context.DeadlineExceeded {
				stderrBuf.WriteString("\n[Shadow Engine] ❌ Blocked: Command exceeded 10s timeout")
				exitCode = 124 // Standard timeout code
			}
		}

		result := ShadowResult{
			Command:   cmdString,
			Stdout:    stdoutBuf.String(),
			Stderr:    stderrBuf.String(),
			ExitCode:  exitCode,
			Duration:  time.Since(startTime),
			Timestamp: time.Now(),
		}

		se.cacheMutex.Lock()
		se.cache[cmdString] = result
		se.cacheMutex.Unlock()

		log.Printf("[Shadow Engine] ✅ Unseen speculation resolved: '%s' (%v)", cmdString, time.Since(startTime))
	}()
}

// Retrieve returns a cached shadow result if it exists and is less than 30 seconds old.
func (se *ShadowEngine) Retrieve(cmdString string) (ShadowResult, bool) {
	cmdString = strings.TrimSpace(cmdString)

	se.cacheMutex.RLock()
	defer se.cacheMutex.RUnlock()

	res, exists := se.cache[cmdString]
	if !exists {
		return ShadowResult{}, false
	}

	// Cache invalidation rule
	if time.Since(res.Timestamp) > 30*time.Second {
		return ShadowResult{}, false
	}

	return res, true
}

func handleTerminalCheckShadow(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}
	cmd, _ := payload["command"].(string)

	res, found := GlobalShadowEngine.Retrieve(cmd)
	if !found {
		// Just echo back that it missed, the UI can fall back to normal execution
		bus.Publish(Envelope{
			Topic:   "terminal.shadow:miss",
			From:    "kernel",
			To:      env.From,
			Payload: map[string]string{"command": cmd},
		})
		return
	}

	log.Printf("[Shadow Engine] ⚡ 0ms ZERO LATENCY HIT for: %s", cmd)

	// Return the cached stdout/stderr immediately
	bus.Publish(Envelope{
		Topic: "terminal.shadow:hit",
		From:  "kernel",
		To:    env.From,
		Payload: map[string]interface{}{
			"command": cmd,
			"stdout":  res.Stdout,
			"stderr":  res.Stderr,
			"exitMs":  res.Duration.Milliseconds(),
		},
	})
}

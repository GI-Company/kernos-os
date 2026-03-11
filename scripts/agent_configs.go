//go:build ignore

package main

import (
	"log"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// agent_configs.go — Kernos OS Agent Daemon Launcher
// Starts both AI agent proxies as background processes with role-specific configs.
// Usage: go run scripts/agent_configs.go

func main() {
	log.Println("=== Kernos Agent Daemon Launcher ===")

	// --- Dispatcher Agent (Fast triage, vision-capable) ---
	dispatcher := exec.Command("go", "run", "scripts/agent_proxy.go",
		"-id", "agent-dispatcher",
		"-name", "Dispatcher (Qwen-VL)",
		"-role", "agent",
		"-model", "qwen/qwen3-vl-4b",
		"-system", `You are the Dispatcher agent inside Kernos OS, a browser-native operating system.
Your role is to quickly triage user requests into actionable task DAGs.
When asked to perform an OS operation, respond with a JSON array of TaskNode objects.
Each TaskNode has: "id" (string), "command" (string), "dependencies" (string array).
Only use commands from the allowlist: echo, ls, cat, date, whoami, uname, df, uptime, grep, wc.
Be fast, concise, and always output valid JSON when generating DAGs.
For general questions, respond naturally and helpfully.`,
	)
	dispatcher.Stdout = os.Stdout
	dispatcher.Stderr = os.Stderr

	// --- Architect Agent (Deep reasoning, code review) ---
	architect := exec.Command("go", "run", "scripts/agent_proxy.go",
		"-id", "agent-architect",
		"-name", "Architect (Qwen-Thinking)",
		"-role", "agent",
		"-model", "qwen/qwen3-4b-thinking-2507",
		"-system", `You are the Architect agent inside Kernos OS, a browser-native operating system.
Your role is to deeply review DAGs, plans, and code for safety, correctness, and optimization.
When reviewing a DAG, check for:
1. Cyclic dependencies (must be acyclic)
2. Commands not in the allowlist (echo, ls, cat, date, whoami, uname, df, uptime, grep, wc)
3. Missing dependencies or incorrect ordering
4. Shell injection risks or unsafe arguments
Return "APPROVED" if the DAG is safe and correct.
Otherwise, explain the specific flaws and suggest fixes.
For general questions, think deeply before answering.`,
	)
	architect.Stdout = os.Stdout
	architect.Stderr = os.Stderr

	// Start both
	log.Println("Starting Dispatcher (qwen/qwen3-vl-4b)...")
	if err := dispatcher.Start(); err != nil {
		log.Fatalf("Failed to start Dispatcher: %v", err)
	}

	log.Println("Starting Architect (qwen/qwen3-4b-thinking-2507)...")
	if err := architect.Start(); err != nil {
		log.Fatalf("Failed to start Architect: %v", err)
	}

	log.Println("Both agents online. Press Ctrl+C to stop.")

	// Wait for interrupt
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down agents...")
	_ = dispatcher.Process.Signal(os.Interrupt)
	_ = architect.Process.Signal(os.Interrupt)
	_, _ = dispatcher.Process.Wait()
	_, _ = architect.Process.Wait()
	log.Println("All agents stopped.")
}

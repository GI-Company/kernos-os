package main

import (
	"fmt"
	"log"
	"strings"
	"time"
)

// TaskStatus represents the state of a task node
type TaskStatus string

const (
	StatusPending   TaskStatus = "pending"
	StatusRunning   TaskStatus = "running"
	StatusCompleted TaskStatus = "completed"
	StatusFailed    TaskStatus = "failed"
)

// TaskNode represents a single unit of work in the DAG
type TaskNode struct {
	ID           string     `json:"id"`
	Command      string     `json:"command"` // e.g., "echo 'building'"
	Dependencies []string   `json:"dependencies"`
	Status       TaskStatus `json:"status"`
}

// TaskGraph represents the entire workflow
type TaskGraph struct {
	ID    string               `json:"id"`
	Nodes map[string]*TaskNode `json:"nodes"`
}

// EventBus abstracts the WebSocket bus for testing
type EventBus interface {
	Publish(Envelope)
}

// TaskEngine orchestrates the execution of TaskGraphs
type TaskEngine struct {
	bus EventBus
}

// NewTaskEngine creates a new engine instance
func NewTaskEngine(bus EventBus) *TaskEngine {
	return &TaskEngine{bus: bus}
}

// ExecuteGraph validates and runs the task graph
func (te *TaskEngine) ExecuteGraph(graphID string, nodes []TaskNode) {
	// 1. Build the graph map
	nodeMap := make(map[string]*TaskNode)
	for i := range nodes {
		// Initialize status
		nodes[i].Status = StatusPending
		nodeMap[nodes[i].ID] = &nodes[i]
	}

	// 2. Validate using the SOTA Architect Agent (Qwen-Thinking)
	go te.ValidateAndExecute(graphID, nodeMap, nodes)
}

func (te *TaskEngine) ValidateAndExecute(graphID string, nodeMap map[string]*TaskNode, originalList []TaskNode) {
	te.bus.Publish(Envelope{
		Topic: "task.status",
		Payload: map[string]interface{}{
			"id":     graphID,
			"node":   "sys",
			"status": "validating (Architect Thinking...)",
		},
	})

	// Format Graph for the Architect
	var graphDesc string
	for _, n := range originalList {
		graphDesc += fmt.Sprintf("- ID: %s | Cmd: '%s' | Deps: %v\n", n.ID, n.Command, n.Dependencies)
	}

	// 1. Find the Architect config
	var architect EmbeddedAgent
	for _, a := range DefaultAgents("http://127.0.0.1:1234/v1/chat/completions") {
		if a.ID == "agent-architect" {
			architect = a
			break
		}
	}

	prompt := "Please review the following Task DAG for execution safety and topological cycles:\n\n" + graphDesc

	resp, err := queryLM(architect.LMStudioURL, architect.Model, architect.SystemPrompt, prompt, nil)
	if err != nil || !strings.Contains(resp, "APPROVED") {
		reason := "Architect rejected building the DAG. Unsafe or ill-formed."
		if err != nil {
			reason = err.Error()
		} else {
			// Extract the failure reason (ignoring the <think> blocks if present)
			reason = resp
		}

		te.bus.Publish(Envelope{
			Topic: "task.status",
			Payload: map[string]interface{}{
				"id":     graphID,
				"node":   "sys",
				"status": "validation failed",
				"reason": reason,
			},
		})

		// Log Telemetry (RLHF prep)
		if GlobalTelemetry != nil {
			go GlobalTelemetry.LogDagExecution(graphID, prompt, "ERROR", reason, -2.0)
		}
		return
	}

	// 4. Execution Loop
	// A real production engine would use a topological sort or a work queue.
	// We'll use a simple polling loop for the MVP.
	go func() {
		// Notify start
		te.bus.Publish(Envelope{
			Topic:   "task.run:ack",
			From:    "kernel",
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]string{"runId": graphID},
		})

		completedCount := 0
		totalCount := len(originalList)

		for completedCount < totalCount {
			progressMade := false

			for _, node := range nodeMap {
				if node.Status != StatusPending {
					continue
				}

				// Check dependencies
				canRun := true
				for _, depID := range node.Dependencies {
					if dep, ok := nodeMap[depID]; ok {
						if dep.Status != StatusCompleted {
							canRun = false
							break
						}
					}
				}

				if canRun {
					// Execute Node
					progressMade = true
					te.runNode(graphID, node)

					// Phase 14: Mutating DAGs (Self-Repairing Logic)
					if node.Status == StatusFailed {
						log.Printf("[TaskEngine] Node %s failed. Triggering Architect Emergency Graft...", node.ID)
						te.emitEvent(graphID, "sys", StatusRunning, 50, "Node failed. Architect Initiating Probabilistic DAG Mutation...")

						mutatedGraph := te.AttemptDagMutation(graphID, node, nodeMap)
						if mutatedGraph != nil {
							nodeMap = mutatedGraph
							totalCount = len(nodeMap)
						} else {
							// If mutation fails, we halt execution here for dependent nodes
							log.Printf("[TaskEngine] Architect failed to graft recovery node.")
						}
					}

					completedCount++
				}
			}

			if !progressMade && completedCount < totalCount {
				// Deadlock or cycle detected (or just waiting for async ops if we did real async)
				// For this synchronous implementation, if we loop without progress, it's a deadlock.
				fmt.Println("Cycle or missing dependency detected, aborting graph execution.")
				break
			}
			// Small sleep to prevent CPU spinning if we had async nodes
			time.Sleep(10 * time.Millisecond)
		}

		// Notify done
		te.bus.Publish(Envelope{
			Topic:   "task.done",
			From:    "kernel",
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]string{"runId": graphID},
		})

		// Emit positive RLHF scalar reward
		if GlobalTelemetry != nil {
			go GlobalTelemetry.LogDagExecution(graphID, "Task execution graph succeeded", "SUCCESS", "", 1.0)
		}
	}()
}

func (te *TaskEngine) runNode(runID string, node *TaskNode) {
	// 1. Mark Running
	node.Status = StatusRunning
	te.emitEvent(runID, node.ID, StatusRunning, 0, "")

	// 2. Execute
	parts := strings.Fields(node.Command)
	if len(parts) == 0 {
		node.Status = StatusCompleted
		te.emitEvent(runID, node.ID, StatusCompleted, 100, "")
		return
	}

	// reqID tracking: Use graphID + taskNode ID for precise kill control
	reqID := fmt.Sprintf("%s-%s", runID, node.ID)
	output, err := ExecuteSafeCommand(reqID, node.Command, []string{})

	if err != nil {
		node.Status = StatusFailed
		// Append error to output
		fullOutput := output
		if fullOutput != "" {
			fullOutput += "\n"
		}
		fullOutput += err.Error()
		te.emitEvent(runID, node.ID, StatusFailed, 0, fullOutput)
		return
	}

	// 3. Mark Completed
	node.Status = StatusCompleted
	te.emitEvent(runID, node.ID, StatusCompleted, 100, output)
}

func (te *TaskEngine) emitEvent(runID, stepID string, status TaskStatus, progress int, output string) {
	te.bus.Publish(Envelope{
		Topic: "task.event",
		From:  "kernel",
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{
			"runId":    runID,
			"step":     stepID,
			"status":   status,
			"progress": progress,
			"output":   output,
		},
	})
}

// AttemptDagMutation dynamically repairs a failing DAG by having the Architect synthesize
// and graft a new recovery node, re-routing dependent nodes topologically.
func (te *TaskEngine) AttemptDagMutation(graphID string, failedNode *TaskNode, nodeMap map[string]*TaskNode) map[string]*TaskNode {
	// Find Architect
	var architect EmbeddedAgent
	for _, a := range DefaultAgents("http://127.0.0.1:1234/v1/chat/completions") {
		if a.ID == "agent-architect" {
			architect = a
			break
		}
	}

	// Phase 14: Query Semantic Graph for past similar failures
	semanticContext := ""
	if GlobalVectorDB != nil {
		vectors, err := GlobalVectorDB.GenerateEmbeddings([]string{"search_query: error running command " + failedNode.Command})
		if err == nil && len(vectors) > 0 {
			results := GlobalVectorDB.Search(vectors[0], 2)
			if len(results) > 0 {
				semanticContext = "\n\nPast Historical Context (Semantic Graph):\n"
				for _, res := range results {
					semanticContext += fmt.Sprintf("File Context:\n```\n%s\n```\n\n", res.Chunk.Text)
				}
			}
		}
	}

	prompt := fmt.Sprintf(`The Task Pipeline failed at node '%s'.
Command: %s
This node failed to execute.
%s
You must GRAFT recovery paths into the DAG. 
Generate exactly TWO distinct, alternative bash commands to recover (e.g. branch 1: 'npm install --legacy-peer-deps', branch 2: 'rm -rf node_modules && npm install').
Output ONLY the two raw command strings separated by a newline. Do not include branch labels or markup. If you cannot recover, output "FATAL".`, failedNode.ID, failedNode.Command, semanticContext)

	resp, err := queryLM(architect.LMStudioURL, architect.Model, architect.SystemPrompt, prompt, nil)
	if err != nil {
		return nil
	}

	recoveryCmds := strings.Split(strings.TrimSpace(resp), "\n")
	if len(recoveryCmds) == 0 || strings.Contains(strings.ToLower(recoveryCmds[0]), "fatal") {
		return nil
	}

	// Clean up parsed commands
	var safeCmds []string
	for _, c := range recoveryCmds {
		clean := strings.TrimSpace(c)
		clean = strings.TrimPrefix(clean, "`")
		clean = strings.TrimSuffix(clean, "`")
		if clean != "" && clean != "FATAL" {
			safeCmds = append(safeCmds, clean)
		}
	}

	if len(safeCmds) == 0 {
		return nil
	}

	log.Printf("[TaskEngine] 🧬 Architect synthesized %d divergent reality branches. Initiating quantum race...", len(safeCmds))
	
	// Phase 14: Quantum Parallel Race-Condition
	// We execute all divergent branches simultaneously. The first to exit 0 collapses the wave function.
	winnerChan := make(chan string, len(safeCmds))

	for i, cmdStr := range safeCmds {
		go func(branchID int, cmd string) {
			log.Printf("   -> [Branch %d]: %s", branchID, cmd)
			reqID := fmt.Sprintf("%s-branch%d", failedNode.ID, branchID)
			_, err := ExecuteSafeCommand(reqID, cmd, []string{})
			if err == nil {
				winnerChan <- cmd
			}
		}(i+1, cmdStr)
	}

	// Wait up to 30 seconds for a winner
	var winningCmd string
	select {
	case winningCmd = <-winnerChan:
		log.Printf("[TaskEngine] 🥇 Branch won the race and collapsed the wave function: %s", winningCmd)
	case <-time.After(30 * time.Second):
		log.Printf("[TaskEngine] 💀 All divergent branches failed or timed out.")
		if GlobalTelemetry != nil {
			go GlobalTelemetry.LogDagExecution(failedNode.ID, failedNode.Command, "ERROR", "Quantum branch collapse failed (timeout)", -1.0)
		}
		return nil
	}

	// Graft the winning node into the DAG
	newNodeID := failedNode.ID + "-recovery"
	newNode := &TaskNode{
		ID:           newNodeID,
		Command:      winningCmd,
		Dependencies: []string{}, // Recover immediately
		Status:       StatusCompleted, // It already ran successfully during the race
	}

	nodeMap[newNode.ID] = newNode

	// Re-route dependent nodes topologically
	for _, n := range nodeMap {
		for i, dep := range n.Dependencies {
			if dep == failedNode.ID {
				n.Dependencies[i] = newNodeID // Swap broken dependency to the grafted fix
			}
		}
	}

	return nodeMap
}

// ExecuteGoal runs a recursive ReAct (Reason and Act) loop to autonomously fulfill an abstract goal
func (te *TaskEngine) ExecuteGoal(graphID, goal string) {
	te.emitEvent(graphID, "sys", StatusRunning, 0, "Initializing Autonomous Agent Loop for Goal: "+goal)

	// Find the Architect
	var architect EmbeddedAgent
	for _, a := range DefaultAgents("http://127.0.0.1:1234/v1/chat/completions") {
		if a.ID == "agent-architect" {
			architect = a
			break
		}
	}

	systemPrompt := `You are an Autonomous ReAct Agent operating inside Kernos OS.
You are given a high-level GOAL and the conversation history of previous commands and their outputs.
Based on the current state, determine the SINGLE NEXT logical bash command to execute to progress towards the goal.
If the goal has been fully achieved, output exactly "DONE".
Otherwise, output ONLY the raw bash command string, nothing else. No markdown.`

	conversation := "GOAL: " + goal + "\n\n"

	for step := 1; step <= 10; step++ { // Hard cap at 10 steps to prevent infinite loops
		te.emitEvent(graphID, fmt.Sprintf("step-%d", step), StatusRunning, step*10, "Planning next step...")

		resp, err := queryLM(architect.LMStudioURL, architect.Model, systemPrompt, conversation, nil)
		if err != nil {
			te.emitEvent(graphID, fmt.Sprintf("step-%d", step), StatusFailed, step*10, "Architect Offline: "+err.Error())
			return
		}

		cmdStr := strings.TrimSpace(resp)
		cmdStr = strings.TrimPrefix(cmdStr, "`")
		cmdStr = strings.TrimSuffix(cmdStr, "`")
		cmdStr = strings.TrimPrefix(cmdStr, "bash")
		cmdStr = strings.TrimSpace(cmdStr)

		if cmdStr == "DONE" {
			te.emitEvent(graphID, "sys", StatusCompleted, 100, "Goal achieved autonomously.")
			te.bus.Publish(Envelope{
				Topic:   "task.done",
				From:    "kernel",
				Time:    time.Now().Format(time.RFC3339),
				Payload: map[string]string{"runId": graphID},
			})
			return
		}

		if cmdStr == "" {
			te.emitEvent(graphID, fmt.Sprintf("step-%d", step), StatusFailed, step*10, "Agent generated empty command.")
			return
		}

		te.emitEvent(graphID, fmt.Sprintf("step-%d", step), StatusRunning, step*10, "Executing: "+cmdStr)

		// Execute the command via Sandbox
		reqID := fmt.Sprintf("%s-step%d", graphID, step)
		output, err := ExecuteSafeCommand(reqID, cmdStr, []string{})

		logStr := output
		status := StatusCompleted
		if err != nil {
			logStr += "\n" + err.Error()
			status = StatusFailed
			te.emitEvent(graphID, fmt.Sprintf("step-%d", step), status, step*10, "Error: "+logStr)
		} else {
			te.emitEvent(graphID, fmt.Sprintf("step-%d", step), status, step*10, output)
		}

		// Append to context window
		conversation += "COMMAND: " + cmdStr + "\nOUTPUT:\n" + logStr + "\n\n"
	}

	te.emitEvent(graphID, "sys", StatusFailed, 100, "Autonomous loop hit max steps (10) without achieving DONE.")
}

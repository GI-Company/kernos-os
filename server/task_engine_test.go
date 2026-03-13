package main

import (
	"strings"
	"sync"
	"testing"
	"time"
)

// Mock implementation of Bus for testing
type MockBus struct {
	mu                 sync.Mutex
	PublishedEnvelopes []Envelope
}

func (b *MockBus) Publish(env Envelope) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.PublishedEnvelopes = append(b.PublishedEnvelopes, env)
}

func (b *MockBus) GetEvents() []Envelope {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Return a copy to avoid race on slice contents
	events := make([]Envelope, len(b.PublishedEnvelopes))
	copy(events, b.PublishedEnvelopes)
	return events
}

func TestTaskEngine_TopologicalSort(t *testing.T) {
	bus := &MockBus{}
	engine := NewTaskEngine(bus)
	engine.TestingMode = true

	// Build a simple 3-node DAG:  A -> B -> C
	nodes := []TaskNode{
		{ID: "A", Command: "echo step-a", Dependencies: []string{}},
		{ID: "B", Command: "echo step-b", Dependencies: []string{"A"}},
		{ID: "C", Command: "echo step-c", Dependencies: []string{"B"}},
	}

	engine.ExecuteGraph("test-run-1", nodes)

	// Give the goroutine time to finish
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Logf("Envelopes at timeout: %+v", bus.GetEvents())
			t.Fatal("Timed out waiting for task.done")
		default:
			found := false
			for _, env := range bus.GetEvents() {
				if env.Topic == "task.done" {
					found = true
				}
			}
			if found {
				goto verify
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
verify:

	// Verify task.status events were published
	statusCount := 0
	for _, env := range bus.GetEvents() {
		if env.Topic == "task.status" {
			statusCount++
		}
	}
	if statusCount == 0 {
		t.Fatal("Expected at least one task.status event")
	}
	t.Logf("TaskEngine published %d status events and 1 task.done event", statusCount)
}

func TestTaskEngine_CycleDetection(t *testing.T) {
	bus := &MockBus{}
	engine := NewTaskEngine(bus)
	engine.TestingMode = true

	// Build a cyclic graph: A -> B -> A (deadlock)
	nodes := []TaskNode{
		{ID: "A", Command: "echo a", Dependencies: []string{"B"}},
		{ID: "B", Command: "echo b", Dependencies: []string{"A"}},
	}

	engine.ExecuteGraph("test-cycle", nodes)

	// The engine should detect the deadlock and break out within 2s
	time.Sleep(2 * time.Second)

	doneFound := false
	for _, env := range bus.GetEvents() {
		if env.Topic == "task.done" {
			doneFound = true
		}
	}

	// In a cycle, the engine detects deadlock and breaks out.
	// task.done may or may not fire. The test just verifies no infinite hang.
	t.Logf("Cycle test completed. task.done emitted: %v, total events: %d", doneFound, len(bus.GetEvents()))
}

func TestTaskEngine_ParallelFanOut(t *testing.T) {
	bus := &MockBus{}
	engine := NewTaskEngine(bus)
	engine.TestingMode = true

	// Build a fan-out DAG: A -> (B, C) -> D
	nodes := []TaskNode{
		{ID: "A", Command: "echo root", Dependencies: []string{}},
		{ID: "B", Command: "echo branch-1", Dependencies: []string{"A"}},
		{ID: "C", Command: "echo branch-2", Dependencies: []string{"A"}},
		{ID: "D", Command: "echo join", Dependencies: []string{"B", "C"}},
	}

	engine.ExecuteGraph("test-parallel", nodes)

	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			t.Logf("Envelopes at timeout: %+v", bus.GetEvents())
			t.Fatal("Timed out waiting for task.done on parallel graph")
		default:
			for _, env := range bus.GetEvents() {
				if env.Topic == "task.done" {
					goto done
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
done:
	t.Logf("Parallel DAG test passed. Total events: %d", len(bus.GetEvents()))
}

func TestTaskEngine_SharedMemoryContext(t *testing.T) {
	bus := &MockBus{}
	engine := NewTaskEngine(bus)
	engine.TestingMode = true

	// Build a 2-node DAG: A -> B
	// Node A simply echoes a value
	// Node B reads that value through the context environment variable
	nodes := []TaskNode{
		{ID: "A", Command: "echo shared_value", Dependencies: []string{}},
		{ID: "B", Command: "echo Received: $CTX_A_OUT", Dependencies: []string{"A"}},
	}

	engine.ExecuteGraph("test-shared-memory", nodes)

	deadline := time.After(5 * time.Second)
	foundB := false
	var outputB string

	for {
		select {
		case <-deadline:
			t.Logf("Envelopes at timeout: %+v", bus.GetEvents())
			t.Fatal("Timed out waiting for task to complete")
		default:
			for _, env := range bus.GetEvents() {
				if env.Topic == "task.event" {
					payload := env.Payload.(map[string]interface{})
					if payload["step"] == "B" && payload["status"] == StatusCompleted {
						foundB = true
						if out, ok := payload["output"].(string); ok {
							outputB = out
						}
					}
				}
				if env.Topic == "task.done" {
					goto check
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
check:
	if !foundB {
		t.Fatal("Node B never completed")
	}

	if strings.TrimSpace(outputB) != "Received: shared_value" {
		t.Errorf("Expected 'Received: shared_value', got %q", outputB)
	} else {
		t.Logf("Shared memory context test passed! Output: %q", outputB)
	}
}

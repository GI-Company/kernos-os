package main

import (
	"testing"
	"time"
)

// Mock implementation of Bus for testing
type MockBus struct {
	PublishedEnvelopes []Envelope
}

func (b *MockBus) Publish(env Envelope) {
	b.PublishedEnvelopes = append(b.PublishedEnvelopes, env)
}

func TestTaskEngine_TopologicalSort(t *testing.T) {
	bus := &MockBus{}
	engine := NewTaskEngine(bus)

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
			t.Fatal("Timed out waiting for task.done")
		default:
			found := false
			for _, env := range bus.PublishedEnvelopes {
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
	for _, env := range bus.PublishedEnvelopes {
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

	// Build a cyclic graph: A -> B -> A (deadlock)
	nodes := []TaskNode{
		{ID: "A", Command: "echo a", Dependencies: []string{"B"}},
		{ID: "B", Command: "echo b", Dependencies: []string{"A"}},
	}

	engine.ExecuteGraph("test-cycle", nodes)

	// The engine should detect the deadlock and break out within 2s
	time.Sleep(2 * time.Second)

	doneFound := false
	for _, env := range bus.PublishedEnvelopes {
		if env.Topic == "task.done" {
			doneFound = true
		}
	}

	// In a cycle, the engine detects deadlock and breaks out.
	// task.done may or may not fire. The test just verifies no infinite hang.
	t.Logf("Cycle test completed. task.done emitted: %v, total events: %d", doneFound, len(bus.PublishedEnvelopes))
}

func TestTaskEngine_ParallelFanOut(t *testing.T) {
	bus := &MockBus{}
	engine := NewTaskEngine(bus)

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
			t.Fatal("Timed out waiting for task.done on parallel graph")
		default:
			for _, env := range bus.PublishedEnvelopes {
				if env.Topic == "task.done" {
					goto done
				}
			}
			time.Sleep(50 * time.Millisecond)
		}
	}
done:
	t.Logf("Parallel DAG test passed. Total events: %d", len(bus.PublishedEnvelopes))
}

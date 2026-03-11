package main

import (
	"context"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// StartCron initializes the OS-level scheduled task daemon.
// It runs background maintenance jobs like Hippocampus decay and cache clearing.
func StartCron(ctx context.Context, wg *sync.WaitGroup, bus *Bus) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		slog.Info("[Cron] Scheduled Task Daemon started")

		// Ticker for Synaptic Decay (every 1 hour)
		decayTicker := time.NewTicker(1 * time.Hour)
		defer decayTicker.Stop()

		// Ticker for Telemetry Flush (every 5 minutes)
		flushTicker := time.NewTicker(5 * time.Minute)
		defer flushTicker.Stop()

		// Ticker for Live System Metrics
		metricsTicker := time.NewTicker(2 * time.Second)
		defer metricsTicker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("[Cron] Shutting down scheduled tasks...")
				return
			case <-decayTicker.C:
				if GlobalVectorDB != nil {
					slog.Info("[Cron] Triggering Hippocampus Synaptic Decay")
					GlobalVectorDB.DecayWeights()
				}
			case <-flushTicker.C:
				slog.Debug("[Cron] Triggering Telemetry Flush")
				// Flush RLHF items or other metrics here safely
			case <-metricsTicker.C:
				var m runtime.MemStats
				runtime.ReadMemStats(&m)
				
				bus.lock.Lock()
				numClients := len(bus.clients)
				bus.lock.Unlock()
				
				procLock.Lock()
				numProcs := len(activeProcesses)
				procLock.Unlock()
				
				metrics := map[string]interface{}{
					"heapAlloc_mb": m.HeapAlloc / 1024 / 1024,
					"sysAlloc_mb":  m.Sys / 1024 / 1024,
					"numGoroutine": runtime.NumGoroutine(),
					"numClients":   numClients,
					"activeProcs":  numProcs,
				}
				
				bus.Publish(Envelope{
					Topic:   "sys.metrics",
					From:    "kernel",
					Payload: metrics,
					Time:    time.Now().Format(time.RFC3339),
				})
			}
		}
	}()
}

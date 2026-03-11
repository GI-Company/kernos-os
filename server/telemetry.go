package main

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// TelemetryLogger handles writing Task Engine DAG outcomes to a local SQLite database
// for Nightly RLHF Consolidation (Synaptic Plasticity).
type TelemetryLogger struct {
	db *sql.DB
}

var GlobalTelemetry *TelemetryLogger

func InitTelemetry() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Printf("[Telemetry] Error getting home dir: %v", err)
		return
	}

	kernosDir := filepath.Join(home, ".kernos")
	if err := os.MkdirAll(kernosDir, 0755); err != nil {
		log.Printf("[Telemetry] Error creating .kernos dir: %v", err)
		return
	}

	dbPath := filepath.Join(kernosDir, "memory.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("[Telemetry] Failed to open SQLite memory database: %v", err)
		return
	}

	// Create table if not exists
	schema := `
	CREATE TABLE IF NOT EXISTS dag_executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		graph_id TEXT,
		prompt TEXT,
		outcome TEXT,
		error_log TEXT
	);
	`
	if _, err := db.Exec(schema); err != nil {
		log.Printf("[Telemetry] Error creating schema: %v", err)
		return
	}

	GlobalTelemetry = &TelemetryLogger{db: db}
	log.Printf("[Telemetry] Started local RLHF memory database at %s", dbPath)

	// Trigger a background sweep to delete logs older than 7 days
	go GlobalTelemetry.sweepOldLogs()
}

func (t *TelemetryLogger) LogDagExecution(graphID, prompt, outcome, errorLog string) {
	if t == nil || t.db == nil {
		return
	}

	query := `INSERT INTO dag_executions (graph_id, prompt, outcome, error_log) VALUES (?, ?, ?, ?)`
	_, err := t.db.Exec(query, graphID, prompt, outcome, errorLog)
	if err != nil {
		log.Printf("[Telemetry] Failed to log DAG execution: %v", err)
	} else {
		log.Printf("[Telemetry] Logged execution %s -> %s", graphID, outcome)
	}
}

// FetchFailedExecutions retrieves error/aborted logs for the RLHF consolidation script
func (t *TelemetryLogger) FetchFailedExecutions(since time.Time) ([]map[string]string, error) {
	if t == nil || t.db == nil {
		return nil, nil
	}

	query := `SELECT prompt, outcome, error_log FROM dag_executions WHERE outcome != 'SUCCESS' AND timestamp >= ? ORDER BY timestamp DESC`
	rows, err := t.db.Query(query, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]string
	for rows.Next() {
		var prompt, outcome, errorLog string
		if err := rows.Scan(&prompt, &outcome, &errorLog); err != nil {
			continue
		}

		results = append(results, map[string]string{
			"prompt":    prompt,
			"outcome":   outcome,
			"error_log": errorLog,
		})
	}

	return results, nil
}

func (t *TelemetryLogger) sweepOldLogs() {
	for {
		time.Sleep(24 * time.Hour)
		if t == nil || t.db == nil {
			return
		}
		t.db.Exec(`DELETE FROM dag_executions WHERE timestamp < datetime('now', '-7 days')`)
	}
}

package main

import (
	"log"
	"time"

	"github.com/google/uuid"
)

// UndoEvent represents a snapshot of state before a destructive action occurs
type UndoEvent struct {
	ID        string
	Category  string // e.g. "sys.config", "vfs.write", "vm.spawn"
	Target    string // e.g. "theme", "/path/to/file.txt"
	PrevData  string // The old config value or the old file contents
	Timestamp time.Time
}

// LogUndoSnapshot serializes the precise state before an operation, enabling time-travel
func (db *SysDB) LogUndoSnapshot(category, target, prevData string) {
	evtInit := time.Now()
	id := uuid.New().String()
	
	query := `INSERT INTO sys_history (id, category, target, prev_data, timestamp) VALUES (?, ?, ?, ?, ?)`
	_, err := db.DB.Exec(query, id, category, target, prevData, evtInit.Format(time.RFC3339))
	if err != nil {
		log.Printf("[SysDB] Failed to log undo snapshot: %v", err)
	}
}

// RevertLastAction fetches the most recent UndoEvent matching the category and target
func (db *SysDB) RevertLastAction(category, target string) (*UndoEvent, error) {
	query := `SELECT id, category, target, prev_data, timestamp FROM sys_history 
	          WHERE category = ? AND target = ? 
	          ORDER BY timestamp DESC LIMIT 1`
	          
	row := db.DB.QueryRow(query, category, target)
	var evt UndoEvent
	var ts string
	
	err := row.Scan(&evt.ID, &evt.Category, &evt.Target, &evt.PrevData, &ts)
	if err != nil {
		return nil, err
	}
	evt.Timestamp, _ = time.Parse(time.RFC3339, ts)
	
	// Delete from history so it can't be reverted twice
	db.DB.Exec(`DELETE FROM sys_history WHERE id = ?`, evt.ID)
	
	return &evt, nil
}

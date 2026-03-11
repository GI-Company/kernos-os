package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

// StartVFSWatcher initializes a recursive directory watcher on the kernel's execution context
// and broadcasts "vfs.changed" envelopes over the bus whenever files are modified.
func StartVFSWatcher(bus *Bus, rootDir string) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("[VFS] Failed to initialize fsnotify: %v", err)
		return
	}

	go func() {
		defer watcher.Close()
		
		// debounce map to prevent event spam
		lastEvent := make(map[string]time.Time)

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				// Only notify on writes, creates, or removes
				if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create || event.Op&fsnotify.Remove == fsnotify.Remove {
					// Debounce within 500ms
					if lastTime, exists := lastEvent[event.Name]; exists {
						if time.Since(lastTime) < 500*time.Millisecond {
							continue
						}
					}
					lastEvent[event.Name] = time.Now()

					log.Printf("[VFS] File changed: %s", event.Name)
					
					// Broadcast to the OS bus
					bus.Publish(Envelope{
						Topic:   "vfs.changed",
						From:    "kernel",
						Payload: map[string]string{"path": event.Name, "operation": event.Op.String()},
						Time:    time.Now().Format(time.RFC3339),
					})
				}

			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[VFS] Watcher error: %v", err)
			}
		}
	}()

	// Recursively add directories to the watcher (ignoring node_modules, .git, .kernos)
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == ".kernos" || name == "dist" {
				return filepath.SkipDir
			}
			watcher.Add(path)
		}
		return nil
	})

	if err != nil {
		log.Printf("[VFS] Error walking directory tree: %v", err)
	} else {
		log.Printf("[VFS] System Watcher initialized on %s", rootDir)
	}
}

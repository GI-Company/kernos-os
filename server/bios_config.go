package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"
)

// ═══════════════════════════════════════════════════════════════
// Kernos BIOS Configuration System
// ═══════════════════════════════════════════════════════════════
//
// Provides backend handlers for the BIOS setup screen:
//   • Read/write agents.yaml
//   • Read/write command allowlist
//   • Read/write system config (LM endpoint, root path, etc.)
//   • Firmware update mechanism (self-update)
// ═══════════════════════════════════════════════════════════════

// handleBIOSConfig dispatches BIOS-related WebSocket topics
func handleBIOSConfig(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	switch env.Topic {

	// ── Read agents.yaml ──
	case "bios.agents:read":
		data, err := os.ReadFile("agents.yaml")
		if err != nil {
			bus.Publish(Envelope{
				Topic:   "bios.agents:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": err.Error()},
			})
			return
		}
		bus.Publish(Envelope{
			Topic: "bios.agents:resp",
			From:  "kernel",
			To:    env.From,
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"content": string(data),
			},
		})

	// ── Write agents.yaml (triggers hot-reload) ──
	case "bios.agents:write":
		content, _ := payload["content"].(string)
		if content == "" {
			return
		}
		err := os.WriteFile("agents.yaml", []byte(content), 0644)
		if err != nil {
			bus.Publish(Envelope{
				Topic:   "bios.agents:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": err.Error()},
			})
			return
		}
		log.Printf("[BIOS] ✏️ agents.yaml updated via BIOS setup")
		if GlobalSysDB != nil {
			GlobalSysDB.LogAudit("bios.agents:write", env.From, "Updated agents.yaml via BIOS")
		}
		bus.Publish(Envelope{
			Topic:   "bios.agents:resp",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{"success": true, "message": "agents.yaml saved. Hot-reload will trigger automatically."},
		})

	// ── Read command allowlist ──
	case "bios.allowlist:read":
		cmds := make([]string, 0, len(ALLOWED_COMMANDS))
		for cmd := range ALLOWED_COMMANDS {
			cmds = append(cmds, cmd)
		}
		sort.Strings(cmds)
		bus.Publish(Envelope{
			Topic: "bios.allowlist:resp",
			From:  "kernel",
			To:    env.From,
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"commands": cmds,
				"count":    len(cmds),
			},
		})

	// ── Modify command allowlist ──
	case "bios.allowlist:add":
		cmd, _ := payload["command"].(string)
		if cmd != "" && !strings.ContainsAny(cmd, " &|;`$()<>") {
			ALLOWED_COMMANDS[cmd] = true
			log.Printf("[BIOS] ➕ Added command to allowlist: %s", cmd)
			if GlobalSysDB != nil {
				GlobalSysDB.LogAudit("bios.allowlist:add", env.From, "Added command: "+cmd)
			}
			bus.Publish(Envelope{
				Topic:   "bios.allowlist:resp",
				From:    "kernel",
				To:      env.From,
				Time:    time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{"success": true, "action": "added", "command": cmd},
			})
		}

	case "bios.allowlist:remove":
		cmd, _ := payload["command"].(string)
		if cmd != "" {
			delete(ALLOWED_COMMANDS, cmd)
			log.Printf("[BIOS] ➖ Removed command from allowlist: %s", cmd)
			if GlobalSysDB != nil {
				GlobalSysDB.LogAudit("bios.allowlist:remove", env.From, "Removed command: "+cmd)
			}
			bus.Publish(Envelope{
				Topic:   "bios.allowlist:resp",
				From:    "kernel",
				To:      env.From,
				Time:    time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{"success": true, "action": "removed", "command": cmd},
			})
		}

	// ── Read all system config ──
	case "bios.sysconfig:read":
		if GlobalSysDB == nil {
			return
		}
		configKeys := []string{
			"root_path", "lm_endpoint", "ai_model", "theme",
			"font_size", "jwt_secret", "root_auth_token",
			"github_client_id", "github_client_secret",
		}
		configs := make(map[string]string)
		for _, key := range configKeys {
			val := GlobalSysDB.GetConfig(key)
			configs[key] = val
		}
		bus.Publish(Envelope{
			Topic:   "bios.sysconfig:resp",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: configs,
		})

	// ── Write system config key ──
	case "bios.sysconfig:set":
		key, _ := payload["key"].(string)
		value, _ := payload["value"].(string)
		if key != "" && GlobalSysDB != nil {
			GlobalSysDB.SetConfig(key, value)
			log.Printf("[BIOS] ⚙️ System config updated: %s = %s", key, value)
			GlobalSysDB.LogAudit("bios.sysconfig:set", env.From, fmt.Sprintf("BIOS set %s = %s", key, value))
			bus.Publish(Envelope{
				Topic:   "bios.sysconfig:resp",
				From:    "kernel",
				To:      env.From,
				Time:    time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{"success": true, "key": key},
			})
		}

	// ── System info (for BIOS info panel) ──
	case "bios.sysinfo":
		users := 0
		if GlobalUserDB != nil {
			userList, _ := GlobalUserDB.ListUsers()
			users = len(userList)
		}
		chunks := 0
		if GlobalVectorDB != nil {
			chunks = len(GlobalVectorDB.Chunks)
		}

		info := map[string]interface{}{
			"version":        VERSION,
			"codename":       CODENAME,
			"go_version":     "1.22",
			"agents_loaded":  len(DefaultAgents("")),
			"allowlist_size": len(ALLOWED_COMMANDS),
			"user_count":     users,
			"vector_chunks":  chunks,
			"data_dir":       getKernosDir(),
		}
		bus.Publish(Envelope{
			Topic:   "bios.sysinfo:resp",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: info,
		})

	// ── Firmware: Check for updates (real implementation) ──
	case "bios.firmware:check":
		go func() {
			status, err := CheckForUpdates()
			if err != nil {
				log.Printf("[Firmware] ⚠️ Update check failed: %v", err)
				// Fallback: report current version without remote check
				bus.Publish(Envelope{
					Topic: "bios.firmware:resp",
					From:  "kernel",
					To:    env.From,
					Time:  time.Now().Format(time.RFC3339),
					Payload: map[string]interface{}{
						"current_version": VERSION,
						"current_codename": CODENAME,
						"latest_version":  VERSION,
						"up_to_date":      true,
						"changelog":       []string{"Unable to check for updates: " + err.Error()},
						"platform":        fmt.Sprintf("%s_%s", "darwin", "arm64"),
					},
				})
				return
			}
			bus.Publish(Envelope{
				Topic:   "bios.firmware:resp",
				From:    "kernel",
				To:      env.From,
				Time:    time.Now().Format(time.RFC3339),
				Payload: status,
			})
		}()

	// ── Firmware: Apply update (download + replace binary) ──
	case "bios.firmware:apply":
		downloadURL, _ := payload["download_url"].(string)
		if downloadURL == "" {
			bus.Publish(Envelope{
				Topic:   "bios.firmware:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": "No download URL provided"},
			})
			return
		}
		go func() {
			bus.Publish(Envelope{
				Topic:   "bios.firmware:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"status": "downloading", "message": "Downloading update..."},
			})
			updateDir, err := DownloadUpdate(downloadURL)
			if err != nil {
				bus.Publish(Envelope{
					Topic:   "bios.firmware:resp",
					From:    "kernel",
					To:      env.From,
					Payload: map[string]interface{}{"error": "Download failed: " + err.Error()},
				})
				return
			}
			bus.Publish(Envelope{
				Topic:   "bios.firmware:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"status": "applying", "message": "Applying update..."},
			})
			if err := ApplyUpdate(updateDir); err != nil {
				bus.Publish(Envelope{
					Topic:   "bios.firmware:resp",
					From:    "kernel",
					To:      env.From,
					Payload: map[string]interface{}{"error": "Apply failed: " + err.Error()},
				})
				return
			}
			bus.Publish(Envelope{
				Topic: "bios.firmware:resp",
				From:  "kernel",
				To:    env.From,
				Time:  time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{
					"status":  "complete",
					"message": "Update applied successfully. Restart the server to use the new version.",
				},
			})
			if GlobalSysDB != nil {
				GlobalSysDB.LogAudit("bios.firmware:apply", env.From, "Firmware update applied")
			}
		}()

	// ── Firmware: Export system snapshot ──
	case "bios.firmware:export":
		snapshot, err := ExportSystemSnapshot()
		if err != nil {
			bus.Publish(Envelope{
				Topic:   "bios.firmware:resp",
				From:    "kernel",
				To:      env.From,
				Payload: map[string]interface{}{"error": err.Error()},
			})
			return
		}
		bus.Publish(Envelope{
			Topic: "bios.firmware:resp",
			From:  "kernel",
			To:    env.From,
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"type":     "export",
				"snapshot": snapshot,
			},
		})
	}
}

func getKernosDir() string {
	home, _ := os.UserHomeDir()
	return home + "/.kernos"
}

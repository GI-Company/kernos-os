package main

import (
	"bufio"
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type PluginManifest struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Cmd         string   `yaml:"cmd"`
	Args        []string `yaml:"args"`
}

type PluginConfig struct {
	Plugins []PluginManifest `yaml:"plugins"`
}

// LoadYAMLPlugins reads the plugins.yaml definition array
func LoadYAMLPlugins() map[string]PluginManifest {
	registry := make(map[string]PluginManifest)
	
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".kernos", "plugins.yaml")
	
	data, err := os.ReadFile(path)
	if err != nil {
		return registry
	}
	
	var config PluginConfig
	if err := yaml.Unmarshal(data, &config); err == nil {
		for _, p := range config.Plugins {
			registry[p.ID] = p
		}
	}
	
	return registry
}

// PluginEngine handles sandboxed compilation and execution of dynamically
// generated Go plugins. This is the "Autopoiesis" layer of Kernos OS — the
// kernel can write and install its own tools at runtime.
//
// SECURITY MODEL:
// 1. Import allowlist — only safe stdlib packages permitted
// 2. Temp directory jail — compiles in isolated /tmp/ dirs
// 3. Hard timeout — 30s max execution time
// 4. No filesystem access — plugins communicate via stdout only

// SafeImports defines the set of Go packages that generated plugins may import.
// Anything not in this list (os/exec, net, syscall, etc.) is blocked.
var SafeImports = map[string]bool{
	"fmt":             true,
	"strings":         true,
	"strconv":         true,
	"math":            true,
	"math/rand":       true,
	"sort":            true,
	"encoding/json":   true,
	"encoding/csv":    true,
	"encoding/hex":    true,
	"encoding/base64": true,
	"time":            true,
	"unicode":         true,
	"unicode/utf8":    true,
	"bytes":           true,
	"errors":          true,
	"regexp":          true,
	"crypto/sha256":   true,
	"crypto/md5":      true,
	"hash":            true,
	"io":              true,
	"log":             true,
	"text/template":   true,
	"path":            true,
}

// BlockedImports are explicitly dangerous and should be flagged clearly.
var BlockedImports = []string{
	"os/exec",
	"net",
	"net/http",
	"syscall",
	"unsafe",
	"plugin",
	"runtime/debug",
	"os",        // blocks os.Remove, os.Create, os.Open etc.
	"io/ioutil", // blocks file read/write
	"reflect",   // prevents reflection-based bypasses
}

// ValidateSource parses Go source code and checks that all imports are in the
// SafeImports allowlist. Returns an error describing any violations.
func ValidateSource(source string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "plugin.go", source, parser.ImportsOnly)
	if err != nil {
		return fmt.Errorf("failed to parse source: %w", err)
	}

	var violations []string
	for _, imp := range f.Imports {
		// Strip quotes from import path
		path := strings.Trim(imp.Path.Value, `"`)

		if !SafeImports[path] {
			violations = append(violations, path)
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("BLOCKED imports detected: [%s]. Only these packages are allowed: %s",
			strings.Join(violations, ", "),
			safeImportList())
	}

	return nil
}

// CompileAndRun takes Go source code, validates it, compiles it in a temp
// sandbox, and executes it with a hard timeout. Returns stdout and stderr.
func CompileAndRun(source string, timeout time.Duration) (stdout string, stderr string, err error) {
	// Phase 1: Validate imports
	if err := ValidateSource(source); err != nil {
		return "", "", fmt.Errorf("security validation failed: %w", err)
	}

	// Phase 2: Create sandbox directory
	sandboxDir, err := os.MkdirTemp("", "kernos-sandbox-")
	if err != nil {
		return "", "", fmt.Errorf("failed to create sandbox: %w", err)
	}
	defer os.RemoveAll(sandboxDir)

	// Write source file
	srcPath := filepath.Join(sandboxDir, "main.go")
	if err := os.WriteFile(srcPath, []byte(source), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write source: %w", err)
	}

	// Initialize a Go module in the sandbox
	modContent := "module kernos-plugin\n\ngo 1.21\n"
	modPath := filepath.Join(sandboxDir, "go.mod")
	if err := os.WriteFile(modPath, []byte(modContent), 0644); err != nil {
		return "", "", fmt.Errorf("failed to write go.mod: %w", err)
	}

	// Phase 3: Compile
	binPath := filepath.Join(sandboxDir, "plugin_bin")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, srcPath)
	buildCmd.Dir = sandboxDir
	buildOutput, buildErr := buildCmd.CombinedOutput()
	if buildErr != nil {
		return "", string(buildOutput), fmt.Errorf("compilation failed: %s", string(buildOutput))
	}

	// Phase 4: Execute with timeout
	runCtx, runCancel := context.WithTimeout(context.Background(), timeout)
	defer runCancel()

	runCmd := exec.CommandContext(runCtx, binPath)
	runCmd.Dir = sandboxDir

	var stdoutBuf, stderrBuf strings.Builder

	runCmd.Stdout = &stdoutBuf
	runCmd.Stderr = &stderrBuf

	runErr := runCmd.Run()

	stdoutStr := stdoutBuf.String()
	stderrStr := stderrBuf.String()

	if runErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			return stdoutStr, stderrStr, fmt.Errorf("execution timed out after %s", timeout)
		}
		return stdoutStr, stderrStr, fmt.Errorf("execution failed: %w", runErr)
	}

	return stdoutStr, stderrStr, nil
}

// handlePluginRun processes a plugin.run message from the bus.
// It expects payload: { "source": "<go source code>" }
func handlePluginRun(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	// Route 1: Pre-defined YAML Command Plugins via plugin ID
	pluginID, _ := payload["id"].(string)
	if pluginID != "" {
		registry := LoadYAMLPlugins()
		plugin, exists := registry[pluginID]
		if !exists {
			bus.Publish(Envelope{
				Topic: "plugin.run:error",
				From:  "kernel",
				To:    env.From,
				Time:  time.Now().Format(time.RFC3339),
				Payload: map[string]string{"error": "Plugin ID not found"},
			})
			return
		}
		
		go func() {
			var runtimeArgs []string
			if providedArgs, ok := payload["args"].([]interface{}); ok {
				for _, arg := range providedArgs {
					if strArg, isStr := arg.(string); isStr {
						runtimeArgs = append(runtimeArgs, strArg)
					}
				}
			}
			finalArgs := append(plugin.Args, runtimeArgs...)
			log.Printf("[PluginEngine] Executing YAML plugin '%s' (%s)", plugin.Name, plugin.Cmd)
			
			// Plugins invoke processes outside the usual whitelist bounds using exec.Command natively
			// to avoid ExecuteSafeCommand limitations since plugins are statically defined in YAML by host
			cmd := exec.Command(plugin.Cmd, finalArgs...)
			outBytes, err := cmd.CombinedOutput()
			
			result := map[string]interface{}{
				"id": pluginID,
				"stdout": string(outBytes),
			}
			topic := "plugin.run:done"
			if err != nil {
				result["error"] = err.Error()
				topic = "plugin.run:error"
			}
			
			bus.Publish(Envelope{
				Topic:   topic,
				From:    "kernel",
				To:      env.From,
				Time:    time.Now().Format(time.RFC3339),
				Payload: result,
			})
		}()
		return
	}

	// Route 2: Dynamic Go Source Code Sandboxed Compilation
	source, _ := payload["source"].(string)
	if source == "" {
		bus.Publish(Envelope{
			Topic:   "plugin.run:error",
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: map[string]string{"error": "No source code provided"},
		})
		return
	}

	log.Printf("[PluginEngine] Compiling plugin from %s (%d bytes)", env.From, len(source))

	go func() {
		stdout, stderr, err := CompileAndRun(source, 30*time.Second)

		result := map[string]string{
			"stdout": stdout,
			"stderr": stderr,
		}
		topic := "plugin.run:done"

		if err != nil {
			result["error"] = err.Error()
			topic = "plugin.run:error"
			log.Printf("[PluginEngine] FAILED: %v", err)
		} else {
			log.Printf("[PluginEngine] SUCCESS: %d bytes of output", len(stdout))
		}

		bus.Publish(Envelope{
			Topic:   topic,
			From:    "kernel",
			To:      env.From,
			Time:    time.Now().Format(time.RFC3339),
			Payload: result,
		})
	}()
}

func safeImportList() string {
	imports := make([]string, 0, len(SafeImports))
	for k := range SafeImports {
		imports = append(imports, k)
	}
	return strings.Join(imports, ", ")
}

// Suppress unused import warnings for bufio — it's used in advanced plugin I/O
var _ = bufio.Scanner{}

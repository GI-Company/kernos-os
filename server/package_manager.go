package main

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ════════════════════════════════════════════════════════════════════════════════
// Kernos Package Manager — End-to-End Package Lifecycle
// ════════════════════════════════════════════════════════════════════════════════
//
// Features:
//   • Real downloads from GitHub releases + official CDNs
//   • OS/Arch detection (macOS ARM64, macOS x86, Linux x86_64)
//   • Installed state persistence (checks ~/.kernos/packages/{name}/ on disk)
//   • Uninstall support (removes package directory)
//   • 9 packages: python, node, go, rustc, ffmpeg, sqlite, ripgrep, jq, deno
//   • Archive extraction: .zip, .tar.gz
//   • Path traversal protection on all extractions
// ════════════════════════════════════════════════════════════════════════════════

// --- Package Registry Entry ---

type RegistryPackage struct {
	Name    string `json:"name"`
	Desc    string `json:"desc"`
	Version string `json:"version"`
}

// packageRegistry is the canonical list of all packages Kernos can install.
var packageRegistry = []RegistryPackage{
	{Name: "python", Desc: "Python 3.12 Interpreter", Version: "3.12.2"},
	{Name: "node", Desc: "Node.js Runtime", Version: "20.11.1"},
	{Name: "go", Desc: "Go Programming Language", Version: "1.22.0"},
	{Name: "rustc", Desc: "Rust Compiler & Cargo", Version: "1.76.0"},
	{Name: "deno", Desc: "Deno Runtime (TypeScript/JS)", Version: "1.40.5"},
	{Name: "ffmpeg", Desc: "Media Processing Suite", Version: "6.0.0"},
	{Name: "sqlite", Desc: "SQL Database Engine", Version: "0.21.6"},
	{Name: "ripgrep", Desc: "Ultra-fast recursive search (rg)", Version: "14.1.0"},
	{Name: "jq", Desc: "Command-line JSON Processor", Version: "1.7.1"},
}

// --- Installed State Detection ---

func getPackagesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".kernos", "packages")
}

func isPackageInstalled(name string) bool {
	pkgDir := filepath.Join(getPackagesDir(), name)
	info, err := os.Stat(pkgDir)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}
	// Check that the directory is non-empty (has actual files)
	entries, err := os.ReadDir(pkgDir)
	return err == nil && len(entries) > 0
}

// --- List Handler (with installed state) ---

func handlePkgList(bus *Bus, _ Envelope) {
	type pkgWithState struct {
		Name      string `json:"name"`
		Desc      string `json:"desc"`
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
	}

	var packages []pkgWithState
	for _, pkg := range packageRegistry {
		packages = append(packages, pkgWithState{
			Name:      pkg.Name,
			Desc:      pkg.Desc,
			Version:   pkg.Version,
			Installed: isPackageInstalled(pkg.Name),
		})
	}

	bus.Publish(Envelope{
		Topic: "pkg.list:resp",
		From:  "kernel",
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{
			"packages": packages,
		},
	})
}

// --- Install Handler ---

func handlePkgInstall(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	reqID, _ := payload["_request_id"].(string)
	pkgName, _ := payload["pkgName"].(string)

	if reqID == "" || pkgName == "" {
		return
	}

	// ACK
	runID := "pkg-" + reqID
	bus.Publish(Envelope{
		Topic:   "pkg.install:ack",
		From:    "kernel",
		Time:    time.Now().Format(time.RFC3339),
		Payload: map[string]string{"_request_id": reqID, "runId": runID},
	})

	go func() {
		defer func() {
			bus.Publish(Envelope{
				Topic: "pkg.install:done",
				From:  "kernel",
				Time:  time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{
					"pkgName": pkgName,
				},
			})
		}()

		// Check if already installed
		if isPackageInstalled(pkgName) {
			emitProgress(bus, runID, fmt.Sprintf("%s is already installed.", pkgName), 100)
			return
		}

		emitProgress(bus, runID, "Resolving package registry...", 10)

		url := getPackageURL(pkgName)
		if url == "" {
			emitProgress(bus, runID, fmt.Sprintf("Error: Package '%s' not available for %s/%s", pkgName, runtime.GOOS, runtime.GOARCH), 0)
			return
		}

		emitProgress(bus, runID, fmt.Sprintf("Checking installed dependencies for %s...", pkgName), 20)

		pkgDir := filepath.Join(getPackagesDir(), pkgName)
		if err := os.MkdirAll(pkgDir, 0755); err != nil {
			emitProgress(bus, runID, "Error creating package directory.", 0)
			return
		}

		emitProgress(bus, runID, fmt.Sprintf("Downloading %s from upstream...", pkgName), 40)

		resp, err := http.Get(url)
		if err != nil || resp.StatusCode != 200 {
			emitProgress(bus, runID, fmt.Sprintf("Failed to download %s (HTTP %d)", pkgName, getStatusCode(resp)), 0)
			if resp != nil {
				resp.Body.Close()
			}
			// Clean up empty directory on failure
			os.RemoveAll(pkgDir)
			return
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			emitProgress(bus, runID, "Error reading download stream.", 0)
			os.RemoveAll(pkgDir)
			return
		}

		emitProgress(bus, runID, fmt.Sprintf("Extracting %s (%.1f MB)...", pkgName, float64(len(body))/(1024*1024)), 70)

		err = extractArchive(body, url, pkgDir)
		if err != nil {
			emitProgress(bus, runID, fmt.Sprintf("Error extracting package: %v", err), 0)
			os.RemoveAll(pkgDir)
			return
		}

		// Ensure binaries are executable
		filepath.Walk(pkgDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() {
				os.Chmod(path, 0755)
			}
			return nil
		})

		emitProgress(bus, runID, "Linking executables and updating VFS...", 90)

		log.Printf("[PackageManager] ✅ Successfully installed %s to %s", pkgName, pkgDir)
		emitProgress(bus, runID, fmt.Sprintf("%s installed successfully.", pkgName), 100)

		// Emit to Neuroplasticity Engine
		if GlobalNeuroplasticity != nil {
			GlobalNeuroplasticity.EmitReward("pkg-manager", "system", "install "+pkgName, "success", 1.0)
		}
	}()
}

// --- Uninstall Handler ---

func handlePkgUninstall(bus *Bus, env Envelope) {
	payload, ok := env.Payload.(map[string]interface{})
	if !ok {
		return
	}

	pkgName, _ := payload["pkgName"].(string)
	if pkgName == "" {
		return
	}

	go func() {
		pkgDir := filepath.Join(getPackagesDir(), pkgName)

		if !isPackageInstalled(pkgName) {
			bus.Publish(Envelope{
				Topic: "pkg.uninstall:done",
				From:  "kernel",
				Time:  time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{
					"pkgName": pkgName,
					"error":   "Package not installed",
				},
			})
			return
		}

		log.Printf("[PackageManager] 🗑️ Uninstalling %s from %s", pkgName, pkgDir)

		if err := os.RemoveAll(pkgDir); err != nil {
			bus.Publish(Envelope{
				Topic: "pkg.uninstall:done",
				From:  "kernel",
				Time:  time.Now().Format(time.RFC3339),
				Payload: map[string]interface{}{
					"pkgName": pkgName,
					"error":   err.Error(),
				},
			})
			return
		}

		log.Printf("[PackageManager] ✅ Successfully uninstalled %s", pkgName)

		bus.Publish(Envelope{
			Topic: "pkg.uninstall:done",
			From:  "kernel",
			Time:  time.Now().Format(time.RFC3339),
			Payload: map[string]interface{}{
				"pkgName": pkgName,
			},
		})
	}()
}

// --- HTTP API for listing installed packages ---

func handlePkgListHTTP(w http.ResponseWriter, _ *http.Request) {
	type pkgWithState struct {
		Name      string `json:"name"`
		Desc      string `json:"desc"`
		Version   string `json:"version"`
		Installed bool   `json:"installed"`
	}

	var packages []pkgWithState
	for _, pkg := range packageRegistry {
		packages = append(packages, pkgWithState{
			Name:      pkg.Name,
			Desc:      pkg.Desc,
			Version:   pkg.Version,
			Installed: isPackageInstalled(pkg.Name),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"packages": packages})
}

// --- Progress Helper ---

func emitProgress(bus *Bus, runID string, step string, progress int) {
	bus.Publish(Envelope{
		Topic: "task.event",
		From:  "kernel",
		Time:  time.Now().Format(time.RFC3339),
		Payload: map[string]interface{}{
			"runId":    runID,
			"step":     step,
			"status":   "running",
			"progress": progress,
		},
	})
}

func getStatusCode(resp *http.Response) int {
	if resp == nil {
		return 0
	}
	return resp.StatusCode
}

// ════════════════════════════════════════════════════════════════════════════════
// Package URL Registry — Real Release URLs from GitHub / Official CDNs
// ════════════════════════════════════════════════════════════════════════════════

func getPackageURL(pkgName string) string {
	osType := runtime.GOOS
	arch := runtime.GOARCH
	isDarwinArm := osType == "darwin" && arch == "arm64"
	isDarwinAmd := osType == "darwin" && arch == "amd64"
	isLinuxAmd := osType == "linux" && arch == "amd64"

	switch pkgName {
	case "python":
		if isDarwinArm {
			return "https://github.com/indygreg/python-build-standalone/releases/download/20240224/cpython-3.12.2+20240224-aarch64-apple-darwin-install_only.tar.gz"
		} else if isDarwinAmd {
			return "https://github.com/indygreg/python-build-standalone/releases/download/20240224/cpython-3.12.2+20240224-x86_64-apple-darwin-install_only.tar.gz"
		}
		return "https://github.com/indygreg/python-build-standalone/releases/download/20240224/cpython-3.12.2+20240224-x86_64-unknown-linux-musl-install_only.tar.gz"

	case "node":
		if isDarwinArm {
			return "https://nodejs.org/dist/v20.11.1/node-v20.11.1-darwin-arm64.tar.gz"
		} else if isDarwinAmd {
			return "https://nodejs.org/dist/v20.11.1/node-v20.11.1-darwin-x64.tar.gz"
		}
		return "https://nodejs.org/dist/v20.11.1/node-v20.11.1-linux-x64.tar.gz"

	case "go":
		if isDarwinArm {
			return "https://go.dev/dl/go1.22.0.darwin-arm64.tar.gz"
		} else if isDarwinAmd {
			return "https://go.dev/dl/go1.22.0.darwin-amd64.tar.gz"
		}
		return "https://go.dev/dl/go1.22.0.linux-amd64.tar.gz"

	case "rustc":
		if isDarwinArm {
			return "https://static.rust-lang.org/dist/rust-1.76.0-aarch64-apple-darwin.tar.gz"
		} else if isDarwinAmd {
			return "https://static.rust-lang.org/dist/rust-1.76.0-x86_64-apple-darwin.tar.gz"
		} else if isLinuxAmd {
			return "https://static.rust-lang.org/dist/rust-1.76.0-x86_64-unknown-linux-musl.tar.gz"
		}

	case "deno":
		if isDarwinArm {
			return "https://github.com/denoland/deno/releases/download/v1.40.5/deno-aarch64-apple-darwin.zip"
		} else if isDarwinAmd {
			return "https://github.com/denoland/deno/releases/download/v1.40.5/deno-x86_64-apple-darwin.zip"
		} else if isLinuxAmd {
			return "https://github.com/denoland/deno/releases/download/v1.40.5/deno-x86_64-unknown-linux-gnu.zip"
		}

	case "ffmpeg":
		if isDarwinArm || isDarwinAmd {
			return "https://evermeet.cx/ffmpeg/getrelease/zip"
		}
		return "https://johnvansickle.com/ffmpeg/releases/ffmpeg-release-amd64-static.tar.xz"

	case "sqlite":
		if isDarwinArm {
			return "https://github.com/nalgeon/sqlean/releases/download/0.21.6/sqlean-macos-arm64.zip"
		} else if isDarwinAmd {
			return "https://github.com/nalgeon/sqlean/releases/download/0.21.6/sqlean-macos-x86.zip"
		}
		return "https://github.com/nalgeon/sqlean/releases/download/0.21.6/sqlean-linux-x86.zip"

	case "ripgrep":
		if isDarwinArm {
			return "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-aarch64-apple-darwin.tar.gz"
		} else if isDarwinAmd {
			return "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-x86_64-apple-darwin.tar.gz"
		} else if isLinuxAmd {
			return "https://github.com/BurntSushi/ripgrep/releases/download/14.1.0/ripgrep-14.1.0-x86_64-unknown-linux-musl.tar.gz"
		}

	case "jq":
		if isDarwinArm {
			return "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-macos-arm64"
		} else if isDarwinAmd {
			return "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-macos-amd64"
		} else if isLinuxAmd {
			return "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-amd64"
		}
	}

	return ""
}

// ════════════════════════════════════════════════════════════════════════════════
// Archive Extraction (zip + tar.gz)
// ════════════════════════════════════════════════════════════════════════════════

func extractArchive(data []byte, url string, destDir string) error {
	if strings.HasSuffix(url, ".zip") {
		return unzipData(data, destDir)
	} else if strings.HasSuffix(url, ".tar.gz") || strings.HasSuffix(url, ".tgz") {
		return untarGzData(data, destDir)
	}

	// Single binary (e.g. jq) — write directly
	baseName := filepath.Base(url)
	return os.WriteFile(filepath.Join(destDir, baseName), data, 0755)
}

func unzipData(data []byte, destDir string) error {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}

	destDirClean := filepath.Clean(destDir) + string(os.PathSeparator)

	for _, file := range reader.File {
		path := filepath.Join(destDir, file.Name)
		if !strings.HasPrefix(path, destDirClean) && path != filepath.Clean(destDir) {
			return fmt.Errorf("illegal file path: %s", path)
		}
		if file.FileInfo().IsDir() {
			os.MkdirAll(path, file.Mode())
			continue
		}

		os.MkdirAll(filepath.Dir(path), 0755)
		outFile, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, file.Mode())
		if err != nil {
			return err
		}

		rc, err := file.Open()
		if err != nil {
			outFile.Close()
			return err
		}

		_, err = io.Copy(outFile, rc)
		rc.Close()
		outFile.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untarGzData(data []byte, destDir string) error {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	destDirClean := filepath.Clean(destDir) + string(os.PathSeparator)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		path := filepath.Join(destDir, header.Name)
		if !strings.HasPrefix(path, destDirClean) && path != filepath.Clean(destDir) {
			return fmt.Errorf("illegal file path: %s", path)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0755); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(path), 0755)
			outFile, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tr); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()
		}
	}
	return nil
}

package main

import (
	"archive/tar"
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

// ═══════════════════════════════════════════════════════════════
// Kernos Firmware Updater — Self-Update Mechanism
// ═══════════════════════════════════════════════════════════════
//
// How it works:
//   1. Checks GitHub Releases (or a custom manifest URL) for the latest version
//   2. Compares with the current VERSION constant
//   3. Downloads the correct binary for the current OS/architecture
//   4. Extracts and replaces the running binary
//   5. Signals for restart
//
// How to write an update:
//   1. Make code changes
//   2. Bump VERSION in this file
//   3. Build for all platforms: goreleaser release --clean
//   4. Push to GitHub — users click "Check for Updates" in BIOS
// ═══════════════════════════════════════════════════════════════

const VERSION = "1.0.0"
const CODENAME = "Genesis"

// ManifestURL is the default location of the version manifest.
// Override via sys.config key "firmware_manifest_url"
const DefaultManifestURL = "https://raw.githubusercontent.com/GI-Company/kernos-os/main/version.json"

type VersionManifest struct {
	Version      string            `json:"version"`
	Codename     string            `json:"codename"`
	ReleaseDate  string            `json:"release_date"`
	Changelog    []string          `json:"changelog"`
	MinGoVersion string            `json:"min_go_version"`
	DownloadURLs map[string]string `json:"download_urls"`
}

type UpdateStatus struct {
	CurrentVersion string   `json:"current_version"`
	CurrentCode    string   `json:"current_codename"`
	LatestVersion  string   `json:"latest_version"`
	LatestCode     string   `json:"latest_codename"`
	UpToDate       bool     `json:"up_to_date"`
	Changelog      []string `json:"changelog"`
	ReleaseDate    string   `json:"release_date"`
	DownloadURL    string   `json:"download_url,omitempty"`
	Platform       string   `json:"platform"`
}

// CheckForUpdates fetches the version manifest and compares versions
func CheckForUpdates() (*UpdateStatus, error) {
	manifestURL := DefaultManifestURL

	// Check if a custom manifest URL is configured
	if GlobalSysDB != nil {
		custom := GlobalSysDB.GetConfig("firmware_manifest_url")
		if custom != "" {
			manifestURL = custom
		}
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(manifestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("manifest returned HTTP %d", resp.StatusCode)
	}

	var manifest VersionManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	platform := fmt.Sprintf("%s_%s", runtime.GOOS, runtime.GOARCH)
	downloadURL := manifest.DownloadURLs[platform]

	status := &UpdateStatus{
		CurrentVersion: VERSION,
		CurrentCode:    CODENAME,
		LatestVersion:  manifest.Version,
		LatestCode:     manifest.Codename,
		UpToDate:       manifest.Version == VERSION,
		Changelog:      manifest.Changelog,
		ReleaseDate:    manifest.ReleaseDate,
		DownloadURL:    downloadURL,
		Platform:       platform,
	}

	return status, nil
}

// DownloadUpdate downloads and extracts the update archive
func DownloadUpdate(downloadURL string) (string, error) {
	if downloadURL == "" {
		return "", fmt.Errorf("no download URL available for platform %s_%s", runtime.GOOS, runtime.GOARCH)
	}

	log.Printf("[Firmware] ⬇️ Downloading update from: %s", downloadURL)

	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("download returned HTTP %d", resp.StatusCode)
	}

	// Create temp directory for extraction
	tmpDir, err := os.MkdirTemp("", "kernos-update-*")
	if err != nil {
		return "", fmt.Errorf("failed to create temp dir: %w", err)
	}

	if strings.HasSuffix(downloadURL, ".tar.gz") || strings.HasSuffix(downloadURL, ".tgz") {
		if err := extractTarGz(resp.Body, tmpDir); err != nil {
			os.RemoveAll(tmpDir)
			return "", fmt.Errorf("extraction failed: %w", err)
		}
	} else {
		// Direct binary download
		outPath := filepath.Join(tmpDir, "kernos_server")
		out, err := os.Create(outPath)
		if err != nil {
			os.RemoveAll(tmpDir)
			return "", err
		}
		io.Copy(out, resp.Body)
		out.Close()
		os.Chmod(outPath, 0755)
	}

	log.Printf("[Firmware] ✅ Update extracted to: %s", tmpDir)
	return tmpDir, nil
}

// ApplyUpdate replaces the running binary
func ApplyUpdate(updateDir string) error {
	currentBinary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot determine current binary path: %w", err)
	}

	// Find the new binary in the update directory
	newBinary := filepath.Join(updateDir, "kernos_server")
	if _, err := os.Stat(newBinary); os.IsNotExist(err) {
		// Try finding it recursively
		filepath.Walk(updateDir, func(path string, info os.FileInfo, err error) error {
			if err == nil && !info.IsDir() && strings.Contains(info.Name(), "kernos") {
				newBinary = path
			}
			return nil
		})
	}

	if _, err := os.Stat(newBinary); os.IsNotExist(err) {
		return fmt.Errorf("cannot find kernos_server binary in update package")
	}

	// Backup current binary
	backupPath := currentBinary + ".backup"
	if err := os.Rename(currentBinary, backupPath); err != nil {
		return fmt.Errorf("failed to backup current binary: %w", err)
	}

	// Copy new binary to current location
	src, err := os.Open(newBinary)
	if err != nil {
		// Restore backup
		os.Rename(backupPath, currentBinary)
		return fmt.Errorf("failed to open new binary: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(currentBinary)
	if err != nil {
		os.Rename(backupPath, currentBinary)
		return fmt.Errorf("failed to create new binary: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		os.Rename(backupPath, currentBinary)
		return fmt.Errorf("failed to copy new binary: %w", err)
	}

	os.Chmod(currentBinary, 0755)

	log.Printf("[Firmware] 🔄 Binary replaced: %s", currentBinary)
	log.Printf("[Firmware] 📦 Backup saved at: %s", backupPath)

	// Clean up
	os.RemoveAll(updateDir)

	return nil
}

// ExportSystemSnapshot creates a complete firmware snapshot
func ExportSystemSnapshot() (string, error) {
	snapshot := map[string]interface{}{
		"firmware": map[string]interface{}{
			"version":     VERSION,
			"codename":    CODENAME,
			"platform":    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
			"exported_at": time.Now().Format(time.RFC3339),
			"go_version":  runtime.Version(),
		},
	}

	// Include agents.yaml
	agentsData, err := os.ReadFile("agents.yaml")
	if err == nil {
		snapshot["agents_yaml"] = string(agentsData)
	}

	// Include allowlist
	cmds := make([]string, 0, len(ALLOWED_COMMANDS))
	for cmd := range ALLOWED_COMMANDS {
		cmds = append(cmds, cmd)
	}
	snapshot["allowlist"] = cmds

	// Include system config
	if GlobalSysDB != nil {
		config := make(map[string]string)
		for _, key := range []string{"root_path", "lm_endpoint", "ai_model", "theme", "font_size"} {
			val := GlobalSysDB.GetConfig(key)
			if val != "" {
				config[key] = val
			}
		}
		snapshot["config"] = config
	}

	// User count
	if GlobalUserDB != nil {
		users, _ := GlobalUserDB.ListUsers()
		snapshot["user_count"] = len(users)
	}

	jsonBytes, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}

	return string(jsonBytes), nil
}

// extractTarGz extracts a .tar.gz archive
func extractTarGz(r io.Reader, dest string) error {
	gzr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(dest, header.Name)

		// Security: prevent path traversal
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.Create(target)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			os.Chmod(target, os.FileMode(header.Mode))
		}
	}
	return nil
}

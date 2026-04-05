package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	githubRepo    = "anish749/cc-tool-reviewer"
	updateCheckTTL = 24 * time.Hour
)

// releaseInfo is the subset of the GitHub releases API response we need.
type releaseInfo struct {
	TagName string         `json:"tag_name"`
	Assets  []releaseAsset `json:"assets"`
}

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// fetchLatestRelease queries GitHub for the latest release.
func fetchLatestRelease() (*releaseInfo, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", githubRepo)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var info releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return &info, nil
}

// runSelfUpdate fetches the latest release and replaces the current binary.
func runSelfUpdate() error {
	fmt.Println("Checking for updates...")

	release, err := fetchLatestRelease()
	if err != nil {
		return fmt.Errorf("fetching latest release: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	if currentVersion == latestVersion {
		fmt.Printf("Already up to date (v%s).\n", currentVersion)
		return nil
	}

	fmt.Printf("Updating v%s -> v%s\n", currentVersion, latestVersion)

	// Build the expected archive name to match goreleaser output:
	// cc-tool-reviewer_{version}_{os}_{arch}.tar.gz
	archiveName := fmt.Sprintf("cc-tool-reviewer_%s_%s_%s.tar.gz", latestVersion, runtime.GOOS, runtime.GOARCH)

	var archiveURL string
	var checksumURL string
	for _, asset := range release.Assets {
		if asset.Name == archiveName {
			archiveURL = asset.BrowserDownloadURL
		}
		if asset.Name == "checksums.txt" {
			checksumURL = asset.BrowserDownloadURL
		}
	}

	if archiveURL == "" {
		return fmt.Errorf("no release asset found for %s", archiveName)
	}
	if checksumURL == "" {
		return fmt.Errorf("no checksums.txt found in release")
	}

	// Download checksums
	expectedChecksum, err := fetchExpectedChecksum(checksumURL, archiveName)
	if err != nil {
		return fmt.Errorf("fetching checksums: %w", err)
	}

	// Download the archive
	fmt.Printf("Downloading %s...\n", archiveName)
	archiveData, err := downloadFile(archiveURL)
	if err != nil {
		return fmt.Errorf("downloading archive: %w", err)
	}

	// Validate checksum
	actualChecksum := sha256sum(archiveData)
	if actualChecksum != expectedChecksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
	}
	fmt.Println("Checksum verified.")

	// Extract binary from tar.gz
	binaryData, err := extractFromTarGz(archiveData, "cc-tool-reviewer")
	if err != nil {
		return fmt.Errorf("extracting binary: %w", err)
	}

	// Replace the current binary atomically
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("finding current executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("resolving symlinks: %w", err)
	}

	tmpFile := execPath + ".tmp"
	if err := os.WriteFile(tmpFile, binaryData, 0755); err != nil {
		return fmt.Errorf("writing temp file: %w", err)
	}

	if err := os.Rename(tmpFile, execPath); err != nil {
		os.Remove(tmpFile) // best-effort cleanup
		return fmt.Errorf("replacing binary: %w", err)
	}

	fmt.Printf("Successfully updated cc-tool-reviewer: v%s -> v%s\n", currentVersion, latestVersion)
	return nil
}

// checkForUpdateSilently runs a background update check that never blocks
// the main program and silently handles all errors.
func checkForUpdateSilently() {
	defer func() {
		// Catch any panics so we never crash the main program.
		recover() //nolint:errcheck
	}()

	if version == "dev" {
		return
	}

	stateDir := updateCheckDir()
	timestampFile := filepath.Join(stateDir, "last_update_check")

	// Check if we should run (TTL not expired yet)
	if info, err := os.Stat(timestampFile); err == nil {
		if time.Since(info.ModTime()) < updateCheckTTL {
			return
		}
	}

	release, err := fetchLatestRelease()
	if err != nil {
		return
	}

	// Update the timestamp file regardless of version comparison.
	_ = os.MkdirAll(stateDir, 0755)
	_ = os.WriteFile(timestampFile, []byte(time.Now().Format(time.RFC3339)), 0644)

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	if latestVersion != currentVersion {
		fmt.Fprintf(os.Stderr, "A new version of cc-tool-reviewer is available: v%s (current: v%s). Run 'cc-tool-reviewer update' to upgrade.\n", latestVersion, currentVersion)
	}
}

// updateCheckDir returns the directory for storing update check state.
func updateCheckDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "cc-tool-reviewer")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "cc-tool-reviewer")
}

// fetchExpectedChecksum downloads checksums.txt and extracts the checksum
// for the given file name.
func fetchExpectedChecksum(checksumURL, fileName string) (string, error) {
	data, err := downloadFile(checksumURL)
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(data), "\n") {
		// Format: "<checksum>  <filename>"
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == fileName {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %s", fileName)
}

// downloadFile fetches a URL and returns the body bytes.
func downloadFile(url string) ([]byte, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d for %s", resp.StatusCode, url)
	}
	return io.ReadAll(resp.Body)
}

// sha256sum returns the hex-encoded SHA-256 digest of data.
func sha256sum(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

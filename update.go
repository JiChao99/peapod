package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	appVersion         = "0.12.0"
	latestReleaseURL   = "https://api.github.com/repos/JiChao99/peapod/releases/latest"
	maxReleaseBytes    = 1 << 20
	releaseHTTPTimeout = 12 * time.Second
)

type releaseAsset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

type releaseInfo struct {
	Version     string         `json:"version"`
	Tag         string         `json:"tag"`
	Name        string         `json:"name"`
	URL         string         `json:"url"`
	PublishedAt time.Time      `json:"publishedAt"`
	Assets      []releaseAsset `json:"assets"`
}

type updateState struct {
	Checking  bool
	Status    string
	Latest    *releaseInfo
	Err       string
	CheckedAt time.Time

	// Phase is empty between operations, or downloading, verifying, installing.
	Phase     string
	Progress  float64 // 0-100
	AssetName string
}

func fetchLatestRelease(ctx context.Context, endpoint string) (*releaseInfo, error) {
	return fetchLatestReleaseWithClient(ctx, endpoint, newSecureUpdateHTTPClient(releaseHTTPTimeout, nil))
}

// fetchLatestReleaseWithClient keeps the network boundary replaceable in tests.
// Production callers use fetchLatestRelease and its HTTPS/host policy.
func fetchLatestReleaseWithClient(ctx context.Context, endpoint string, client *http.Client) (*releaseInfo, error) {
	if err := validateUpdateURL(endpoint); err != nil {
		return nil, err
	}
	if client == nil {
		client = newSecureUpdateHTTPClient(releaseHTTPTimeout, nil)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Peapod/"+appVersion)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub releases returned %s", resp.Status)
	}
	data, err := readBoundedReader(resp.Body, maxReleaseBytes)
	if err != nil {
		return nil, fmt.Errorf("reading release metadata: %w", err)
	}
	var payload struct {
		TagName     string         `json:"tag_name"`
		Name        string         `json:"name"`
		HTMLURL     string         `json:"html_url"`
		PublishedAt time.Time      `json:"published_at"`
		Prerelease  bool           `json:"prerelease"`
		Draft       bool           `json:"draft"`
		Assets      []releaseAsset `json:"assets"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if payload.Draft || payload.Prerelease {
		return nil, fmt.Errorf("latest stable release is unavailable")
	}
	tag := strings.TrimSpace(payload.TagName)
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	if !semver.IsValid(tag) {
		return nil, fmt.Errorf("invalid release version %q", payload.TagName)
	}
	releaseURL := strings.TrimSpace(payload.HTMLURL)
	if err := validateUpdateURL(releaseURL); err != nil || releaseHost(releaseURL) != "github.com" {
		return nil, fmt.Errorf("invalid release URL")
	}
	return &releaseInfo{
		Version:     strings.TrimPrefix(tag, "v"),
		Tag:         tag,
		Name:        payload.Name,
		URL:         releaseURL,
		PublishedAt: payload.PublishedAt,
		Assets:      append([]releaseAsset(nil), payload.Assets...),
	}, nil
}

func newerRelease(current, latest string) bool {
	currentTag := "v" + strings.TrimPrefix(current, "v")
	latestTag := "v" + strings.TrimPrefix(latest, "v")
	if !semver.IsValid(currentTag) || !semver.IsValid(latestTag) {
		return false
	}
	return semver.Compare(latestTag, currentTag) > 0
}

func cloneReleaseInfo(info *releaseInfo) *releaseInfo {
	if info == nil {
		return nil
	}
	clone := *info
	clone.Assets = append([]releaseAsset(nil), info.Assets...)
	return &clone
}

func (a *app) checkForUpdates() {
	a.mu.Lock()
	if a.update.Checking || a.update.Phase != "" {
		a.mu.Unlock()
		return
	}
	a.update.Checking = true
	a.update.Status = "checking"
	a.update.Err = ""
	a.update.Phase = ""
	a.update.Progress = 0
	a.update.AssetName = ""
	a.mu.Unlock()
	a.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), releaseHTTPTimeout)
		defer cancel()
		info, err := fetchLatestRelease(ctx, latestReleaseURL)
		a.mu.Lock()
		a.update.Checking = false
		a.update.CheckedAt = time.Now()
		a.update.Err = ""
		a.update.Latest = nil
		switch {
		case err != nil:
			a.update.Status = "error"
			a.update.Err = err.Error()
		case newerRelease(appVersion, info.Version):
			a.update.Status = "available"
			a.update.Latest = info
		default:
			a.update.Status = "current"
			a.update.Latest = info
		}
		a.mu.Unlock()
		a.invalidate()
	}()
}

func (a *app) autoCheckForUpdates() {
	if a.defaults.AutoUpdate {
		a.checkForUpdates()
	}
}

// downloadAndInstallUpdate is the asynchronous UI entry point for the full
// download, SHA256 verification, installation and restart sequence.
func (a *app) downloadAndInstallUpdate() {
	a.installUpdate()
}

// installUpdate starts the same full operation and can be called directly.
// It uses the release most recently stored by checkForUpdates and returns
// immediately. Failures leave Latest intact so the UI can offer its release
// page as a fallback.
func (a *app) installUpdate() {
	a.mu.Lock()
	if a.update.Checking || a.update.Phase != "" {
		a.mu.Unlock()
		return
	}
	info := cloneReleaseInfo(a.update.Latest)
	if info == nil {
		a.update.Status = "error"
		a.update.Err = "no update is available to install"
		a.mu.Unlock()
		a.invalidate()
		return
	}
	if !newerRelease(appVersion, info.Version) {
		a.update.Status = "current"
		a.update.Err = ""
		a.update.Phase = ""
		a.update.Progress = 0
		a.mu.Unlock()
		a.invalidate()
		return
	}
	if selected, err := selectUpdateAssets(info, runtime.GOOS, runtime.GOARCH); err == nil {
		a.update.AssetName = selected.Asset.Name
	} else {
		a.update.AssetName = ""
	}
	a.update.Phase = "downloading"
	a.update.Progress = 0
	a.update.Err = ""
	a.update.Status = "available"
	a.mu.Unlock()
	a.invalidate()
	go a.runUpdateInstallation(info)
}

func (a *app) runUpdateInstallation(info *releaseInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTimeout)
	defer cancel()
	progress := func(phase string, value float64) {
		value = clampUpdateProgress(value)
		a.mu.Lock()
		changed := a.update.Phase != phase || a.update.Progress == 0 || value-a.update.Progress >= 1 || value == 100
		if changed {
			a.update.Phase = phase
			a.update.Progress = value
		}
		a.mu.Unlock()
		if changed {
			a.invalidate()
		}
	}
	verified, err := downloadAndVerifyUpdate(ctx, info, runtime.GOOS, runtime.GOARCH, newSecureUpdateHTTPClient(updateDownloadTimeout, nil), progress)
	if err == nil {
		err = installVerifiedUpdate(ctx, verified, progress)
	}

	a.mu.Lock()
	a.update.Phase = ""
	a.update.Err = ""
	if err != nil {
		a.update.Progress = 0
		// Keep the release available for retry and for the release-page fallback.
		a.update.Status = "available"
		a.update.Err = err.Error()
	} else {
		a.update.Progress = 100
		a.update.Status = "installed"
	}
	a.mu.Unlock()
	a.invalidate()
	if err == nil {
		restartUpdatedApplication(runtime.GOOS)
	}
}

func clampUpdateProgress(value float64) float64 {
	switch {
	case value < 0:
		return 0
	case value > 100:
		return 100
	default:
		return value
	}
}

func (a *app) updateSnapshot() updateState {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.update
	state.Latest = cloneReleaseInfo(state.Latest)
	return state
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

const (
	appVersion         = "0.10.1"
	latestReleaseURL   = "https://api.github.com/repos/JiChao99/peapod/releases/latest"
	maxReleaseBytes    = 1 << 20
	releaseHTTPTimeout = 12 * time.Second
)

type releaseInfo struct {
	Version     string    `json:"version"`
	Tag         string    `json:"tag"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	PublishedAt time.Time `json:"publishedAt"`
}

type updateState struct {
	Checking  bool
	Status    string
	Latest    *releaseInfo
	Err       string
	CheckedAt time.Time
}

func fetchLatestRelease(ctx context.Context, endpoint string) (*releaseInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Peapod/"+appVersion)
	client := &http.Client{Timeout: releaseHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("GitHub releases returned %s", resp.Status)
	}
	var payload struct {
		TagName     string    `json:"tag_name"`
		Name        string    `json:"name"`
		HTMLURL     string    `json:"html_url"`
		PublishedAt time.Time `json:"published_at"`
		Prerelease  bool      `json:"prerelease"`
		Draft       bool      `json:"draft"`
	}
	dec := json.NewDecoder(io.LimitReader(resp.Body, maxReleaseBytes))
	if err := dec.Decode(&payload); err != nil {
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
	parsed, err := url.Parse(releaseURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" {
		return nil, fmt.Errorf("invalid release URL")
	}
	return &releaseInfo{
		Version:     strings.TrimPrefix(tag, "v"),
		Tag:         tag,
		Name:        payload.Name,
		URL:         releaseURL,
		PublishedAt: payload.PublishedAt,
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

func (a *app) checkForUpdates() {
	a.mu.Lock()
	if a.update.Checking {
		a.mu.Unlock()
		return
	}
	a.update.Checking = true
	a.update.Status = "checking"
	a.update.Err = ""
	a.mu.Unlock()
	a.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), releaseHTTPTimeout)
		defer cancel()
		info, err := fetchLatestRelease(ctx, latestReleaseURL)
		a.mu.Lock()
		defer a.mu.Unlock()
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
		a.invalidate()
	}()
}

func (a *app) autoCheckForUpdates() {
	if a.defaults.AutoUpdate {
		a.checkForUpdates()
	}
}

func (a *app) updateSnapshot() updateState {
	a.mu.Lock()
	defer a.mu.Unlock()
	state := a.update
	if state.Latest != nil {
		info := *state.Latest
		state.Latest = &info
	}
	return state
}

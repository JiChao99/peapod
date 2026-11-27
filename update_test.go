package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchLatestRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("Accept = %q", r.Header.Get("Accept"))
		}
		if !strings.HasPrefix(r.Header.Get("User-Agent"), "Peapod/") {
			t.Errorf("User-Agent = %q", r.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v0.11.0",
			"name":"Peapod 0.11.0",
			"html_url":"https://github.com/JiChao99/peapod/releases/tag/v0.11.0",
			"published_at":"2026-11-26T00:00:00Z",
			"prerelease":false,
			"draft":false
		}`))
	}))
	defer srv.Close()
	got, err := fetchLatestRelease(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "0.11.0" || got.URL != "https://github.com/JiChao99/peapod/releases/tag/v0.11.0" {
		t.Fatalf("release = %+v", got)
	}
}

func TestFetchLatestReleaseRejectsInvalidMetadata(t *testing.T) {
	cases := []string{
		`{"tag_name":"latest","html_url":"https://github.com/JiChao99/peapod/releases/tag/latest"}`,
		`{"tag_name":"v0.11.0","html_url":"https://example.com/release"}`,
		`{"tag_name":"v0.11.0","html_url":"https://github.com/JiChao99/peapod/releases/tag/v0.11.0","prerelease":true}`,
	}
	for _, body := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		_, err := fetchLatestRelease(context.Background(), srv.URL)
		srv.Close()
		if err == nil {
			t.Errorf("invalid release accepted: %s", body)
		}
	}
}

func TestFetchLatestReleaseHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	if _, err := fetchLatestRelease(context.Background(), srv.URL); err == nil {
		t.Fatal("HTTP error was accepted")
	}
}

func TestNewerRelease(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"0.9.0", "0.10.0", true},
		{"0.10.0", "0.9.0", false},
		{"0.10.0", "0.10.0", false},
		{"0.10.0", "0.10.1", true},
		{"dev", "0.10.0", false},
	}
	for _, c := range cases {
		if got := newerRelease(c.current, c.latest); got != c.want {
			t.Errorf("newerRelease(%q, %q) = %v, want %v", c.current, c.latest, got, c.want)
		}
	}
}

func TestUpdateSnapshotCopiesRelease(t *testing.T) {
	a := newApp()
	a.update.Latest = &releaseInfo{Version: "0.11.0"}
	got := a.updateSnapshot()
	got.Latest.Version = "changed"
	if a.update.Latest.Version != "0.11.0" {
		t.Fatal("update snapshot aliases app state")
	}
}

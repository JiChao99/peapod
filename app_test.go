package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateURL(t *testing.T) {
	a := newApp()
	cases := []struct {
		raw     string
		wantErr bool
	}{
		{"https://example.com", false},
		{"http://localhost:8080/path?q=1", false},
		{"", true},
		{"not a url", true},
		{"ftp://example.com", true},
		{"file:///tmp/x", true},
		{"https://", true},
		{"https://user:pass@example.com", true},
	}
	for _, c := range cases {
		if err := a.validateURL(c.raw); (err != nil) != c.wantErr {
			t.Errorf("validateURL(%q) error = %v, wantErr %v", c.raw, err, c.wantErr)
		}
	}
}

func TestClampRes(t *testing.T) {
	cases := []struct{ v, fallback, want float64 }{
		{0, 1280, 1280},
		{-5, 800, 800},
		{100, 1280, 200},
		{5000, 1280, 3840},
		{1920, 1280, 1920},
	}
	for _, c := range cases {
		if got := clampRes(c.v, c.fallback); got != c.want {
			t.Errorf("clampRes(%v, %v) = %v, want %v", c.v, c.fallback, got, c.want)
		}
	}
}

func TestSanitizeFileName(t *testing.T) {
	cases := []struct{ in, want string }{
		{`a/b\c:d*e?f"g<h>i|j`, "a_b_c_d_e_f_g_h_i_j"},
		{"  name  ", "name"},
		{"页面 1", "页面 1"},
	}
	for _, c := range cases {
		if got := sanitizeFileName(c.in); got != c.want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestOriginOf(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://example.com/path?q=1", "https://example.com"},
		{"http://localhost:8080/x", "http://localhost:8080"},
		{"://bad", "://bad"},
	}
	for _, c := range cases {
		if got := originOf(c.in); got != c.want {
			t.Errorf("originOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildRequestInheritsPrimaryURL(t *testing.T) {
	a := newApp()
	a.configs[0].url = "https://example.com"
	a.configs[1].name = "Staging"
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if len(req.pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(req.pages))
	}
	wantNames := []string{"主页面", "Staging", "页面 3"}
	for i, p := range req.pages {
		if p.url != "https://example.com" {
			t.Errorf("page %d url = %q, want inherited primary", i, p.url)
		}
		if p.name != wantNames[i] {
			t.Errorf("page %d name = %q, want %q", i, p.name, wantNames[i])
		}
		if p.resW != 1280 || p.resH != 800 {
			t.Errorf("page %d resolution = %vx%v, want 1280x800", i, p.resW, p.resH)
		}
	}
}

func TestBuildRequestRejectsBadForm(t *testing.T) {
	a := newApp()
	a.configs = a.configs[:1]
	if _, err := a.buildRequest(); err == nil {
		t.Error("want error for fewer than 2 pages")
	}

	a = newApp()
	if _, err := a.buildRequest(); err == nil {
		t.Error("want error for empty primary URL")
	}

	a = newApp()
	a.configs[0].url = "ftp://example.com"
	if _, err := a.buildRequest(); err == nil {
		t.Error("want error for non-http primary URL")
	}
}

func TestUiZoom(t *testing.T) {
	cases := []struct {
		pct  string
		want float64
	}{
		{"100", 1},
		{"125", 1.25},
		{"150", 1.5},
		{"abc", 1},
		{"0", 1},
		{"", 1},
	}
	for _, c := range cases {
		a := &app{scalePct: c.pct}
		if got := a.uiZoom(); got != c.want {
			t.Errorf("uiZoom() with scalePct %q = %v, want %v", c.pct, got, c.want)
		}
	}
}

func TestBuildRequestUserAgent(t *testing.T) {
	a := newApp()
	a.configs[0].url = "https://example.com"

	// Default preset follows the view mode.
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("desktop default: %v", err)
	}
	if req.userAgent != "" {
		t.Errorf("desktop default userAgent = %q, want engine default", req.userAgent)
	}
	a.viewMode = "mobile"
	req, err = a.buildRequest()
	if err != nil {
		t.Fatalf("mobile default: %v", err)
	}
	if req.userAgent != mobileUA {
		t.Errorf("mobile default userAgent = %q, want mobileUA", req.userAgent)
	}

	// An explicit preset wins over the mode default. The expectation is the
	// literal UA, not resolveUA's own output, so a bad table entry fails here.
	a.uaPreset = "android"
	req, err = a.buildRequest()
	if err != nil {
		t.Fatalf("android preset: %v", err)
	}
	const androidUA = "Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36"
	if req.userAgent != androidUA {
		t.Errorf("android preset userAgent = %q, want %q", req.userAgent, androidUA)
	}

	// Custom string is trimmed; empty custom falls back to the mode.
	a.uaPreset = uaCustom
	a.uaCustom = "  MyUA/1.0  "
	req, err = a.buildRequest()
	if err != nil {
		t.Fatalf("custom: %v", err)
	}
	if req.userAgent != "MyUA/1.0" {
		t.Errorf("custom userAgent = %q, want trimmed MyUA/1.0", req.userAgent)
	}
	a.uaCustom = ""
	req, err = a.buildRequest()
	if err != nil {
		t.Fatalf("empty custom: %v", err)
	}
	if req.userAgent != mobileUA {
		t.Errorf("empty custom in mobile mode = %q, want mobileUA fallback", req.userAgent)
	}
}

func TestScreenshotDirUserWritable(t *testing.T) {
	home := t.TempDir()
	// os.UserHomeDir reads USERPROFILE on Windows and HOME elsewhere; pin both
	// so the expectation holds on every platform.
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	dir, err := screenshotDir("batch:1")
	if err != nil {
		t.Fatalf("screenshotDir: %v", err)
	}
	wantRoot := filepath.Join(home, "Pictures", "Peapod", "screenshots")
	if !strings.HasPrefix(dir, wantRoot) {
		t.Fatalf("dir = %q, want it under %q (never next to the executable)", dir, wantRoot)
	}
	if !filepath.IsAbs(dir) {
		t.Fatalf("dir = %q, want absolute", dir)
	}
	if !strings.Contains(filepath.Base(dir), "batch_1_") {
		t.Fatalf("dir base = %q, want sanitized prefix + timestamp", filepath.Base(dir))
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("batch dir not created: %v", err)
	}
}

func TestUserArtifactRootFallback(t *testing.T) {
	// With neither a home nor a config dir available, artifacts fall back to a
	// relative folder — still never the executable's (possibly read-only) dir.
	for _, k := range []string{"HOME", "USERPROFILE", "HOMEDRIVE", "HOMEPATH", "APPDATA", "XDG_CONFIG_HOME"} {
		t.Setenv(k, "")
	}
	if got := userArtifactRoot(); got != "Peapod" {
		t.Fatalf("userArtifactRoot with no home/config = %q, want relative Peapod", got)
	}
}

func TestBatchDirSanitizesAndCreates(t *testing.T) {
	root := t.TempDir()
	dir, err := batchDir(root, `a/b:c`)
	if err != nil {
		t.Fatalf("batchDir: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(dir), "a_b_c_") {
		t.Fatalf("dir base = %q, want sanitized prefix", filepath.Base(dir))
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Fatalf("dir not created: %v", err)
	}
}

// TestViewModeResolutionMemory checks that each view mode remembers its
// last-used resolution: switching away and back restores the custom values
// instead of the factory preset, and every row is reseeded as before.
func TestViewModeResolutionMemory(t *testing.T) {
	a := newApp()
	// desktop → mobile seeds the mobile default.
	a.viewMode = "mobile"
	a.reseedViewMode()
	if a.resW != 390 || a.resH != 844 {
		t.Fatalf("mobile seed = %v×%v, want 390×844", a.resW, a.resH)
	}
	// The user customizes the primary row while in mobile.
	a.configs[0].resW, a.configs[0].resH = 414, 896
	// mobile → desktop restores the desktop preset.
	a.viewMode = "desktop"
	a.reseedViewMode()
	if a.resW != 1280 || a.resH != 800 {
		t.Fatalf("desktop restore = %v×%v, want 1280×800", a.resW, a.resH)
	}
	// desktop → mobile restores the custom mobile values, not 390×844.
	a.viewMode = "mobile"
	a.reseedViewMode()
	if a.resW != 414 || a.resH != 896 {
		t.Fatalf("mobile memory = %v×%v, want the custom 414×896", a.resW, a.resH)
	}
	for i, cfg := range a.configs {
		if cfg.resW != 414 || cfg.resH != 896 {
			t.Fatalf("row %d res = %v×%v, want reseeded 414×896", i, cfg.resW, cfg.resH)
		}
	}
	// A no-op call must not clobber anything.
	a.reseedViewMode()
	if a.resW != 414 || a.resH != 896 || a.lastViewMode != "mobile" {
		t.Fatalf("no-op reseed changed state: %v×%v mode=%q", a.resW, a.resH, a.lastViewMode)
	}
}

// TestCountdownProgress checks the panel's countdown derivation from the
// mu-guarded start timestamp: 3→2→1 whole seconds and a 0→100 ring percent.
func TestCountdownProgress(t *testing.T) {
	a := newApp()
	if sec, pct := a.countdownProgress(); sec != 0 || pct != 0 {
		t.Fatalf("idle countdown = %d/%v, want 0/0", sec, pct)
	}
	a.mu.Lock()
	a.status = "starting"
	a.startAt = time.Now()
	a.mu.Unlock()
	if sec, _ := a.countdownProgress(); sec != 3 {
		t.Fatalf("fresh countdown sec = %d, want 3", sec)
	}
	a.mu.Lock()
	t0 := time.Now()
	a.startAt = t0.Add(-1500 * time.Millisecond)
	a.mu.Unlock()
	sec, pct := a.countdownProgress()
	// Derive expectations from measured elapsed so a slow CI tick can't flake.
	elapsed := 1500*time.Millisecond + time.Since(t0)
	wantSec := 3 - int(elapsed/time.Second)
	if wantSec < 1 {
		wantSec = 1
	}
	wantPct := float32(min(float64(elapsed)/float64(3*time.Second)*100, 100))
	if sec != wantSec {
		t.Fatalf("mid countdown sec = %d, want %d (elapsed %v)", sec, wantSec, elapsed)
	}
	if pct < wantPct-5 || pct > wantPct+5 {
		t.Fatalf("mid countdown pct = %v, want ~%v (elapsed %v)", pct, wantPct, elapsed)
	}
	a.mu.Lock()
	a.startAt = time.Now().Add(-5 * time.Second)
	a.mu.Unlock()
	sec, pct = a.countdownProgress()
	if sec != 1 || pct != 100 {
		t.Fatalf("expired countdown = %d/%v, want clamped 1/100", sec, pct)
	}
}

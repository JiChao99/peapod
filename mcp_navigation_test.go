package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNavigationWarningClassification(t *testing.T) {
	policy := mcpNavigationPolicy{WarnPrivate: true, Allowlist: map[string]bool{}}
	cases := []struct {
		url    string
		reason string
	}{
		{"https://example.com/", ""},
		{"http://localhost:8080/", "localhost"},
		{"https://qa.localhost/", "localhost"},
		{"http://127.0.0.1:8099/", "loopback"},
		{"http://10.1.2.3/", "private"},
		{"http://172.16.0.1/", "private"},
		{"http://192.168.1.2/", "private"},
		{"http://169.254.169.254/", "link-local"},
		{"http://100.64.0.1/", "shared-address"},
		{"http://255.255.255.255/", "broadcast"},
		{"http://0.0.0.0/", "unspecified"},
		{"http://[::1]:8080/", "loopback"},
		{"http://[fd00::1]/", "private"},
		{"http://[fe80::1]/", "link-local"},
		{"http://peapod.local/", "mdns"},
		{"ftp://localhost/file", ""},
	}
	for _, tc := range cases {
		warning, ok := navigationWarningForURL(tc.url, policy)
		if tc.reason == "" {
			if ok {
				t.Errorf("%s unexpectedly warned: %+v", tc.url, warning)
			}
			continue
		}
		if !ok || warning.Reason != tc.reason || warning.URL != tc.url || warning.Host == "" {
			t.Errorf("%s warning = %+v, ok = %v, want reason %q", tc.url, warning, ok, tc.reason)
		}
	}
}

func TestNavigationWarningPolicy(t *testing.T) {
	allowlist := normalizeMCPPrivateAllowlist(" LOCALHOST.\n127.0.0.1, [fd00::1]\n*.example.com\n10.0.0.0/8")
	policy := mcpNavigationPolicy{WarnPrivate: true, Allowlist: allowlist}
	for _, raw := range []string{"http://localhost/", "http://127.0.0.1/", "http://[fd00::1]/"} {
		if warning, ok := navigationWarningForURL(raw, policy); ok {
			t.Errorf("allowlisted host warned: %s %+v", raw, warning)
		}
	}
	for _, raw := range []string{"http://qa.localhost/", "http://10.0.0.1/", "http://example.com/"} {
		warning, ok := navigationWarningForURL(raw, policy)
		if raw == "http://example.com/" {
			if ok {
				t.Errorf("public host warned: %+v", warning)
			}
			continue
		}
		if !ok {
			t.Errorf("non-allowlisted local host did not warn: %s", raw)
		}
	}
	policy.WarnPrivate = false
	if warning, ok := navigationWarningForURL("http://127.0.0.1/", policy); ok {
		t.Errorf("disabled policy warned: %+v", warning)
	}
	wildcard := mcpNavigationPolicy{WarnPrivate: true, Allowlist: normalizeMCPPrivateAllowlist("*.localhost\n10.0.0.0/8\nhttp://127.0.0.1")}
	for _, raw := range []string{"http://localhost/", "http://127.0.0.1/", "http://10.0.0.1/"} {
		if _, ok := navigationWarningForURL(raw, wildcard); !ok {
			t.Errorf("invalid allowlist entry suppressed warning: %s", raw)
		}
	}
}

func TestMCPNavigationResponses(t *testing.T) {
	a := newApp()
	start := a.mcpStartResultFor(startRequest{pages: []resolvedPage{
		{url: "http://127.0.0.1:8099/"},
		{url: "http://127.0.0.1:8099/"},
		{url: "http://10.0.0.2/"},
		{url: "https://example.com/"},
	}})
	if start.Status != "starting" || len(start.Warnings) != 2 {
		t.Fatalf("start result = %+v", start)
	}
	navigate := a.mcpNavigateResultFor("http://192.168.1.2/admin")
	if navigate.OK != "true" || len(navigate.Warnings) != 1 || navigate.Warnings[0].Host != "192.168.1.2" {
		t.Fatalf("navigate result = %+v", navigate)
	}
}

func TestMCPSettingsPrivateWarningMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"language":"en"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFrom(path)
	if err != nil || !got.MCPWarnPrivate {
		t.Fatalf("legacy settings = %+v, err = %v", got, err)
	}
	if err := os.WriteFile(path, []byte(`{"mcpWarnPrivate":false,"mcpPrivateAllowlist":"localhost\n10.0.0.1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err = readSettingsFrom(path)
	if err != nil || got.MCPWarnPrivate || got.MCPPrivateAllowlist != "localhost\n10.0.0.1" {
		t.Fatalf("explicit settings = %+v, err = %v", got, err)
	}
	if err := writeSettingsTo(path, got); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := readSettingsFrom(path)
	if err != nil || roundTrip != got {
		t.Fatalf("round trip = %+v, want %+v, err = %v", roundTrip, got, err)
	}
}

func TestMCPNavigationPolicyUpdatesFromSettings(t *testing.T) {
	a := newApp()
	settings := defaultUserSettings()
	settings.MCPWarnPrivate = false
	settings.MCPPrivateAllowlist = "localhost"
	a.applyUserSettings(settings)
	if warnings := a.navigationWarningsForURLs([]string{"http://localhost/", "http://10.0.0.1/"}); len(warnings) != 0 {
		t.Fatalf("disabled policy warnings = %+v", warnings)
	}
	a.settingsDraft = defaultUserSettings()
	a.settingsDraft.MCPPrivateAllowlist = "10.0.0.1"
	a.saveSettingsTo(filepath.Join(t.TempDir(), "settings.json"))
	warnings := a.navigationWarningsForURLs([]string{"http://localhost/", "http://10.0.0.1/"})
	if len(warnings) != 1 || warnings[0].Host != "localhost" {
		t.Fatalf("saved policy warnings = %+v", warnings)
	}
}

func TestMCPNavigationPolicyRefreshesExistingPages(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", url: "http://127.0.0.1:8099/"}
	a.pages["p1"] = p
	a.setMCPNavigationPolicy(userSettings{MCPWarnPrivate: true})
	if len(p.navWarnings) != 1 {
		t.Fatalf("enabled policy warnings = %+v", p.navWarnings)
	}
	a.setMCPNavigationPolicy(userSettings{MCPWarnPrivate: false})
	if len(p.navWarnings) != 0 {
		t.Fatalf("disabled policy kept warnings = %+v", p.navWarnings)
	}
	a.setMCPNavigationPolicy(userSettings{MCPWarnPrivate: true, MCPPrivateAllowlist: "127.0.0.1"})
	if len(p.navWarnings) != 0 {
		t.Fatalf("allowlisted page kept warnings = %+v", p.navWarnings)
	}
	a.setMCPNavigationPolicy(userSettings{MCPWarnPrivate: true, MCPPrivateAllowlist: "localhost"})
	if len(p.navWarnings) != 1 || p.navWarnings[0].Host != "127.0.0.1" {
		t.Fatalf("updated policy warnings = %+v", p.navWarnings)
	}
}

func TestMCPNavigationPolicyRefreshUsesRequestedURL(t *testing.T) {
	a := newApp()
	publicInFlight := &pageRecord{id: "p1", url: "http://127.0.0.1:8099/old", navWarnURL: "https://example.com/next"}
	privateInFlight := &pageRecord{id: "p2", url: "https://example.com/old", navWarnURL: "http://10.0.0.2/next"}
	a.pages = map[string]*pageRecord{"p1": publicInFlight, "p2": privateInFlight}
	a.setMCPNavigationPolicy(userSettings{MCPWarnPrivate: true})
	if len(publicInFlight.navWarnings) != 0 {
		t.Fatalf("public in-flight navigation restored stale warning: %+v", publicInFlight.navWarnings)
	}
	if len(privateInFlight.navWarnings) != 1 || privateInFlight.navWarnings[0].URL != privateInFlight.navWarnURL {
		t.Fatalf("private in-flight navigation lost warning: %+v", privateInFlight.navWarnings)
	}
}

func TestMCPNavigationNoticeUsesHostArgument(t *testing.T) {
	a := newApp()
	a.notifyMCPNavigationWarning(navigationWarning{Host: "127.0.0.1"})
	if a.noticeKey != "notice.mcpPrivateNavigation" || len(a.noticeArgs) != 1 || a.noticeArgs[0] != "127.0.0.1" {
		t.Fatalf("notice key/args = %q %+v", a.noticeKey, a.noticeArgs)
	}
	if !strings.Contains(a.notice, "127.0.0.1") || strings.Contains(a.notice, "%!(EXTRA") {
		t.Fatalf("notice = %q", a.notice)
	}
}

func TestPageNavigationWarningsTrackFinalURL(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", name: "Primary"}
	a.mu.Lock()
	warning, changed := a.updatePageNavigationWarningsLocked(p, "https://example.com/")
	a.mu.Unlock()
	if changed || warning.Host != "" || len(p.navWarnings) != 0 {
		t.Fatalf("public navigation warning = %+v, changed = %v", warning, changed)
	}
	a.mu.Lock()
	warning, changed = a.updatePageNavigationWarningsLocked(p, "http://127.0.0.1:8099/final")
	_, duplicate := a.updatePageNavigationWarningsLocked(p, "http://127.0.0.1:8099/final")
	a.mu.Unlock()
	if !changed || duplicate || warning.Host != "127.0.0.1" || len(p.navWarnings) != 1 {
		t.Fatalf("private redirect warning = %+v, changed = %v, duplicate = %v", warning, changed, duplicate)
	}
	a.mu.Lock()
	_, changed = a.updatePageNavigationWarningsLocked(p, "https://example.com/next")
	a.mu.Unlock()
	if !changed || len(p.navWarnings) != 0 {
		t.Fatalf("public navigation did not clear warnings: %+v", p.navWarnings)
	}
}

func TestSnapshotIncludesNavigationWarnings(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", name: "Primary", url: "http://localhost/", navWarnings: []navigationWarning{{URL: "http://localhost/", Host: "localhost", Reason: "localhost"}}}
	a.pages["p1"] = p
	a.order = []string{"p1"}
	state := a.snapshot()
	if len(state.Pages) != 1 || len(state.Pages[0].Warnings) != 1 || state.Pages[0].Warnings[0].Host != "localhost" {
		t.Fatalf("snapshot = %+v", state)
	}
	state.Pages[0].Warnings[0].Host = "changed"
	if p.navWarnings[0].Host != "localhost" {
		t.Fatal("snapshot aliases page warnings")
	}
}

func TestNavigatePageRecordsRequestedWarnings(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", url: "https://example.com/", status: "ready"}
	a.pages["p1"] = p
	_ = a.navigatePage("p1", "http://127.0.0.1:8099/next")
	if len(p.navWarnings) != 1 || p.navWarnings[0].Host != "127.0.0.1" {
		t.Fatalf("navigate warnings = %+v", p.navWarnings)
	}
	if a.noticeKey != "notice.mcpPrivateNavigation" || len(a.noticeArgs) != 1 || a.noticeArgs[0] != "127.0.0.1" {
		t.Fatalf("explicit navigation notice = %q %+v", a.noticeKey, a.noticeArgs)
	}
}

func TestFollowTargetsCarriesMappedNavigationWarnings(t *testing.T) {
	a, primary := followFixture()
	a.pages["p2"].base = "http://127.0.0.1:8099"
	a.pages["p3"].syncOn = false
	targets := a.followTargets(primary, "https://a.com/step2")
	if len(targets) != 1 || targets[0].url != "http://127.0.0.1:8099/step2" {
		t.Fatalf("targets = %+v", targets)
	}
	if targets[0].warning.Host != "127.0.0.1" {
		t.Fatalf("follower notice warning = %+v", targets[0].warning)
	}
	if warnings := a.pages["p2"].navWarnings; len(warnings) != 1 || warnings[0].Host != "127.0.0.1" {
		t.Fatalf("follower warnings = %+v", warnings)
	}
	if len(a.pages["p3"].navWarnings) != 0 {
		t.Fatalf("unsynced follower warned: %+v", a.pages["p3"].navWarnings)
	}
}

func TestPendingFollowWarnsWhenReconciled(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p2", status: "ready", syncOn: true, pendingNav: "http://10.0.0.2/step2", queue: newOpQueue(0)}
	a.mu.Lock()
	nav := a.reconcileNavLocked(p, false)
	a.mu.Unlock()
	if nav == "" {
		t.Fatal("pending navigation was skipped")
	}
	if len(p.navWarnings) != 1 || p.navWarnings[0].Host != "10.0.0.2" {
		t.Fatalf("pending warnings = %+v", p.navWarnings)
	}
}

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeSettings(t *testing.T) {
	got := normalizeSettings(userSettings{
		Lang:                "zh-TW",
		Theme:               "invalid",
		Scale:               "125",
		AutoUpdate:          true,
		DefaultViewMode:     "free",
		DefaultShareStorage: true,
		DefaultPrimaryOnly:  true,
		DefaultRecord:       true,
		DefaultSyncDelay:    "250",
	})
	if got.Lang != "zh-Hant" || got.Theme != "system" || got.Scale != "125" {
		t.Fatalf("normalized appearance = %+v", got)
	}
	if got.DefaultViewMode != "free" || got.DefaultSyncDelay != "250" {
		t.Fatalf("normalized task defaults = %+v", got)
	}
	if !got.AutoUpdate || !got.DefaultShareStorage || !got.DefaultPrimaryOnly || !got.DefaultRecord {
		t.Fatalf("normalized booleans = %+v", got)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Peapod", "settings.json")
	want := userSettings{
		Lang:                "ja",
		Theme:               "dark",
		Scale:               "150",
		AutoUpdate:          true,
		DefaultViewMode:     "mobile",
		DefaultShareStorage: true,
		DefaultPrimaryOnly:  true,
		DefaultRecord:       true,
		DefaultSyncDelay:    "500",
	}
	if err := writeSettingsTo(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("settings round trip = %+v, want %+v", got, want)
	}
	if err := writeSettingsTo(path, defaultUserSettings()); err != nil {
		t.Fatalf("replace settings: %v", err)
	}
	got, err = readSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != defaultUserSettings() {
		t.Fatalf("replacement settings = %+v", got)
	}
}

func TestReadSettingsRejectsOversizeFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, make([]byte, settingsMaxBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readSettingsFrom(path); err == nil {
		t.Fatal("oversize settings file was accepted")
	}
}

func TestLoadSettingsBacksUpInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	invalid := []byte(`{"language":`)
	if err := os.WriteFile(path, invalid, 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	if err := a.loadSettingsFrom(path); err == nil {
		t.Fatal("invalid settings file was accepted")
	}
	if a.defaults != defaultUserSettings() {
		t.Fatalf("defaults after invalid settings = %+v", a.defaults)
	}
	backups, err := filepath.Glob(path + ".invalid-*")
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, err=%v", backups, err)
	}
	got, err := os.ReadFile(backups[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(invalid) {
		t.Fatalf("backup contents = %q, want %q", got, invalid)
	}
}

func TestAppearanceChangesAutosaveWithoutLosingTaskDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := newApp()
	a.defaults = userSettings{
		Lang:                "zh",
		Theme:               "system",
		Scale:               "100",
		AutoUpdate:          true,
		DefaultViewMode:     "mobile",
		DefaultShareStorage: true,
		DefaultPrimaryOnly:  true,
		DefaultRecord:       true,
		DefaultSyncDelay:    "250",
	}
	a.lang, a.themeMode, a.scalePct = "en", "dark", "125"
	if err := a.saveAppearanceChanges(path); err != nil {
		t.Fatal(err)
	}
	got, err := readSettingsFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Lang != "en" || got.Theme != "dark" || got.Scale != "125" {
		t.Fatalf("saved appearance = %+v", got)
	}
	if !got.AutoUpdate || got.DefaultViewMode != "mobile" || !got.DefaultShareStorage || !got.DefaultPrimaryOnly || !got.DefaultRecord || got.DefaultSyncDelay != "250" {
		t.Fatalf("task defaults were lost: %+v", got)
	}
}

func TestOpenSettingsKeepsReadErrorVisible(t *testing.T) {
	a := newApp()
	a.settingsErr = "settings read failed"
	a.openSettings()
	if !a.settingsOpen || a.settingsErr != "settings read failed" {
		t.Fatalf("settings warning = %q, open=%v", a.settingsErr, a.settingsOpen)
	}
}

func TestSaveSettingsAppearancePreservesPageResolutions(t *testing.T) {
	a := newApp()
	a.status = "idle"
	a.defaults = defaultUserSettings()
	for _, cfg := range a.configs {
		cfg.resW, cfg.resH = 1111, 777
	}
	a.settingsDraft = a.defaults
	a.settingsDraft.Lang = "en"
	a.saveSettingsTo(filepath.Join(t.TempDir(), "settings.json"))
	for _, cfg := range a.configs {
		if cfg.resW != 1111 || cfg.resH != 777 {
			t.Fatalf("appearance save reset page resolution to %gx%g", cfg.resW, cfg.resH)
		}
	}
}

func TestSaveSettingsTaskDefaultsApplyWhenIdle(t *testing.T) {
	a := newApp()
	a.status = "idle"
	a.defaults = defaultUserSettings()
	a.settingsDraft = a.defaults
	a.settingsDraft.DefaultViewMode = "mobile"
	a.saveSettingsTo(filepath.Join(t.TempDir(), "settings.json"))
	for _, cfg := range a.configs {
		if cfg.resW != 390 || cfg.resH != 844 {
			t.Fatalf("task default resolution = %gx%g, want 390x844", cfg.resW, cfg.resH)
		}
	}
}

func TestSaveSettingsOptionDefaultsPreservePageResolutions(t *testing.T) {
	a := newApp()
	a.status = "idle"
	a.defaults = defaultUserSettings()
	for _, cfg := range a.configs {
		cfg.resW, cfg.resH = 1111, 777
	}
	a.settingsDraft = a.defaults
	a.settingsDraft.DefaultShareStorage = true
	a.settingsDraft.DefaultPrimaryOnly = true
	a.settingsDraft.DefaultRecord = true
	a.settingsDraft.DefaultSyncDelay = "250"
	a.saveSettingsTo(filepath.Join(t.TempDir(), "settings.json"))
	for _, cfg := range a.configs {
		if cfg.resW != 1111 || cfg.resH != 777 {
			t.Fatalf("option defaults reset page resolution to %gx%g", cfg.resW, cfg.resH)
		}
	}
	if !a.shareStorage || !a.primaryOnly || !a.record || a.syncDelay != "250" {
		t.Fatalf("option defaults were not applied: share=%v primary=%v record=%v delay=%q", a.shareStorage, a.primaryOnly, a.record, a.syncDelay)
	}
}

func TestSaveSettingsOptionDefaultsWaitForIdle(t *testing.T) {
	a := newApp()
	a.status = "running"
	a.defaults = defaultUserSettings()
	for _, cfg := range a.configs {
		cfg.resW, cfg.resH = 1111, 777
	}
	a.settingsDraft = a.defaults
	a.settingsDraft.DefaultShareStorage = true
	a.saveSettingsTo(filepath.Join(t.TempDir(), "settings.json"))
	if !a.pendingTaskDefaults || a.pendingTaskDefaultsMode {
		t.Fatalf("pending option defaults = %v, mode=%v", a.pendingTaskDefaults, a.pendingTaskDefaultsMode)
	}
	a.status = "idle"
	a.applyPendingTaskDefaults()
	if !a.shareStorage {
		t.Fatal("deferred option defaults were not applied")
	}
	for _, cfg := range a.configs {
		if cfg.resW != 1111 || cfg.resH != 777 {
			t.Fatalf("deferred option defaults reset page resolution to %gx%g", cfg.resW, cfg.resH)
		}
	}
}

func TestSaveSettingsTaskDefaultsWaitForIdle(t *testing.T) {
	a := newApp()
	a.status = "running"
	a.defaults = defaultUserSettings()
	for _, cfg := range a.configs {
		cfg.resW, cfg.resH = 1111, 777
	}
	a.settingsDraft = a.defaults
	a.settingsDraft.DefaultViewMode = "mobile"
	a.saveSettingsTo(filepath.Join(t.TempDir(), "settings.json"))
	if !a.pendingTaskDefaults {
		t.Fatal("task defaults were not deferred while running")
	}
	for _, cfg := range a.configs {
		if cfg.resW != 1111 || cfg.resH != 777 {
			t.Fatalf("running task default save reset page resolution to %gx%g", cfg.resW, cfg.resH)
		}
	}
	a.status = "idle"
	a.applyPendingTaskDefaults()
	if a.pendingTaskDefaults {
		t.Fatal("pending task defaults were not cleared")
	}
	for _, cfg := range a.configs {
		if cfg.resW != 390 || cfg.resH != 844 {
			t.Fatalf("deferred task default resolution = %gx%g, want 390x844", cfg.resW, cfg.resH)
		}
	}
}

func TestAppearanceSaveFailureKeepsPreviousDefaults(t *testing.T) {
	a := newApp()
	a.defaults = defaultUserSettings()
	a.lang = "en"
	dir := filepath.Join(t.TempDir(), "blocked")
	if err := os.WriteFile(dir, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "settings.json")
	if err := a.saveAppearanceChanges(path); err == nil {
		t.Fatal("appearance save unexpectedly succeeded")
	}
	if a.defaults != defaultUserSettings() {
		t.Fatalf("failed save changed defaults: %+v", a.defaults)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := a.saveAppearanceChanges(path); err != nil {
		t.Fatal(err)
	}
	if a.defaults.Lang != "en" {
		t.Fatalf("retry did not save appearance: %+v", a.defaults)
	}
}

func TestApplyTaskDefaults(t *testing.T) {
	a := newApp()
	a.applyUserSettings(userSettings{
		Lang:                "en",
		Theme:               "light",
		Scale:               "125",
		DefaultViewMode:     "free",
		DefaultShareStorage: true,
		DefaultPrimaryOnly:  true,
		DefaultRecord:       true,
		DefaultSyncDelay:    "0",
	})
	if a.viewMode != "free" || a.resW != 1280 || a.resH != 800 {
		t.Fatalf("view defaults = %q %gx%g", a.viewMode, a.resW, a.resH)
	}
	for _, cfg := range a.configs {
		if cfg.resW != 1280 || cfg.resH != 800 {
			t.Fatalf("page resolution = %gx%g", cfg.resW, cfg.resH)
		}
	}
	if !a.shareStorage || !a.primaryOnly || !a.record || a.syncDelay != "0" {
		t.Fatalf("task toggles = share:%v primary:%v record:%v delay:%q", a.shareStorage, a.primaryOnly, a.record, a.syncDelay)
	}
	if a.lang != "en" || a.themeMode != "light" || a.scalePct != "125" {
		t.Fatalf("appearance defaults = %q %q %q", a.lang, a.themeMode, a.scalePct)
	}
}

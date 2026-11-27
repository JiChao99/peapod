package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const settingsMaxBytes = 64 << 10

type userSettings struct {
	Lang                string `json:"language"`
	Theme               string `json:"theme"`
	Scale               string `json:"scale"`
	AutoUpdate          bool   `json:"autoUpdate"`
	DefaultViewMode     string `json:"defaultViewMode"`
	DefaultShareStorage bool   `json:"defaultShareStorage"`
	DefaultPrimaryOnly  bool   `json:"defaultPrimaryOnly"`
	DefaultRecord       bool   `json:"defaultRecord"`
	DefaultSyncDelay    string `json:"defaultSyncDelay"`
}

func defaultUserSettings() userSettings {
	return userSettings{
		Lang:             "zh",
		Theme:            "system",
		Scale:            "100",
		DefaultViewMode:  "desktop",
		DefaultSyncDelay: "100",
	}
}

func normalizeSettings(s userSettings) userSettings {
	d := defaultUserSettings()
	if lang := normalizeLang(s.Lang); lang != "" {
		d.Lang = lang
	}
	if s.Theme == "system" || s.Theme == "light" || s.Theme == "dark" {
		d.Theme = s.Theme
	}
	if s.Scale == "100" || s.Scale == "125" || s.Scale == "150" {
		d.Scale = s.Scale
	}
	if s.DefaultViewMode == "desktop" || s.DefaultViewMode == "mobile" || s.DefaultViewMode == "free" {
		d.DefaultViewMode = s.DefaultViewMode
	}
	if slices.Contains(syncDelayOptions, s.DefaultSyncDelay) {
		d.DefaultSyncDelay = s.DefaultSyncDelay
	}
	d.AutoUpdate = s.AutoUpdate
	d.DefaultShareStorage = s.DefaultShareStorage
	d.DefaultPrimaryOnly = s.DefaultPrimaryOnly
	d.DefaultRecord = s.DefaultRecord
	return d
}

func settingsPath() string {
	if dir, err := os.UserConfigDir(); err == nil && dir != "" {
		return filepath.Join(dir, "Peapod", "settings.json")
	}
	return filepath.Join("Peapod", "settings.json")
}

func readSettingsFrom(path string) (userSettings, error) {
	var s userSettings
	data, err := readBoundedFile(path, settingsMaxBytes)
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	return normalizeSettings(s), nil
}

func writeSettingsTo(path string, s userSettings) error {
	data, err := json.MarshalIndent(normalizeSettings(s), "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "settings-*.json")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (a *app) loadSettings() error {
	return a.loadSettingsFrom(settingsPath())
}

func (a *app) loadSettingsFrom(path string) error {
	s, err := readSettingsFrom(path)
	if err != nil {
		if os.IsNotExist(err) {
			a.applyUserSettings(defaultUserSettings())
			return nil
		}
		if backup, backupErr := backupInvalidSettings(path); backupErr == nil {
			return fmt.Errorf("invalid settings moved to %s: %w", backup, err)
		}
		return err
	}
	a.applyUserSettings(s)
	return nil
}

func backupInvalidSettings(path string) (string, error) {
	backup := fmt.Sprintf("%s.invalid-%s", path, time.Now().Format("20060102-150405.000000000"))
	if err := os.Rename(path, backup); err != nil {
		return "", err
	}
	return backup, nil
}

func (a *app) applyUserSettings(s userSettings) {
	s = normalizeSettings(s)
	a.defaults = s
	a.lang = s.Lang
	a.langVal.Store(s.Lang)
	a.themeMode = s.Theme
	a.scalePct = s.Scale
	a.applyTaskDefaults(s)
}

func (a *app) applyTaskDefaults(s userSettings) {
	mode := s.DefaultViewMode
	if mode == "" {
		mode = "desktop"
	}
	if a.resByMode == nil {
		a.resByMode = map[string][2]float64{
			"desktop": {1280, 800},
			"mobile":  {390, 844},
			"free":    {1280, 800},
		}
	}
	res := a.resByMode[mode]
	if res[0] <= 0 || res[1] <= 0 {
		res = [2]float64{1280, 800}
	}
	a.viewMode, a.lastViewMode = mode, mode
	a.resW, a.resH = res[0], res[1]
	for _, cfg := range a.configs {
		cfg.resW, cfg.resH = res[0], res[1]
	}
	a.applyTaskOptionDefaults(s)
}

func (a *app) applyTaskOptionDefaults(s userSettings) {
	a.shareStorage = s.DefaultShareStorage
	a.primaryOnly = s.DefaultPrimaryOnly
	a.record = s.DefaultRecord
	a.syncDelay = s.DefaultSyncDelay
	if a.syncDelay == "" {
		a.syncDelay = "100"
	}
}

func (a *app) applyTaskDefaultsChange(old, next userSettings) {
	if old.DefaultViewMode != next.DefaultViewMode {
		a.applyTaskDefaults(next)
		return
	}
	a.applyTaskOptionDefaults(next)
}

func (a *app) nextAppearanceSettings() userSettings {
	next := normalizeSettings(a.defaults)
	next.Lang = normalizeLang(a.lang)
	if next.Lang == "" {
		next.Lang = defaultUserSettings().Lang
	}
	next.Theme = a.themeMode
	next.Scale = a.scalePct
	return normalizeSettings(next)
}

func (a *app) saveAppearanceChanges(path string) error {
	next := a.nextAppearanceSettings()
	if next == a.defaults {
		return nil
	}
	if err := writeSettingsTo(path, next); err != nil {
		return err
	}
	a.defaults = next
	return nil
}

func (a *app) persistAppearanceChanges() {
	next := a.nextAppearanceSettings()
	if next == a.defaults {
		a.appearanceDirty = false
		return
	}
	now := time.Now()
	if !a.appearanceDirty {
		a.appearanceDirty = true
		a.appearanceRetryAt = time.Time{}
	}
	if now.Before(a.appearanceRetryAt) {
		return
	}
	if err := writeSettingsTo(settingsPath(), next); err != nil {
		a.appearanceRetryAt = now.Add(5 * time.Second)
		a.settingsErr = fmt.Sprintf("%s: %v", a.tr("err.settingsWrite"), err)
		return
	}
	a.defaults = next
	a.appearanceDirty = false
	a.appearanceRetryAt = time.Time{}
	a.settingsErr = ""
}

func (a *app) openSettings() {
	a.settingsDraft = a.defaults
	a.settingsDraft.Lang = a.lang
	a.settingsDraft.Theme = a.themeMode
	a.settingsDraft.Scale = a.scalePct
	a.settingsOpen = true
}

func taskDefaultsChanged(old, next userSettings) bool {
	return old.DefaultViewMode != next.DefaultViewMode ||
		old.DefaultShareStorage != next.DefaultShareStorage ||
		old.DefaultPrimaryOnly != next.DefaultPrimaryOnly ||
		old.DefaultRecord != next.DefaultRecord ||
		old.DefaultSyncDelay != next.DefaultSyncDelay
}

func (a *app) applyPendingTaskDefaults() {
	if !a.pendingTaskDefaults {
		return
	}
	if a.pendingTaskDefaultsMode {
		a.applyTaskDefaults(a.defaults)
	} else {
		a.applyTaskOptionDefaults(a.defaults)
	}
	a.pendingTaskDefaults = false
	a.pendingTaskDefaultsMode = false
}

func (a *app) saveSettings() {
	a.saveSettingsTo(settingsPath())
}

func (a *app) saveSettingsTo(path string) {
	s := normalizeSettings(a.settingsDraft)
	if err := writeSettingsTo(path, s); err != nil {
		a.settingsErr = fmt.Sprintf("%s: %v", a.tr("err.settingsWrite"), err)
		return
	}
	previous := a.defaults
	taskChanged := taskDefaultsChanged(previous, s)
	modeChanged := previous.DefaultViewMode != s.DefaultViewMode
	a.defaults = s
	a.lang = s.Lang
	a.langVal.Store(s.Lang)
	a.themeMode = s.Theme
	a.scalePct = s.Scale
	a.appearanceDirty = false
	a.appearanceRetryAt = time.Time{}
	a.mu.Lock()
	idle := a.status == "idle"
	a.mu.Unlock()
	if taskChanged {
		if idle {
			a.applyTaskDefaultsChange(previous, s)
			a.pendingTaskDefaults = false
			a.pendingTaskDefaultsMode = false
		} else {
			a.pendingTaskDefaults = true
			a.pendingTaskDefaultsMode = a.pendingTaskDefaultsMode || modeChanged
		}
	}
	a.settingsOpen = false
	a.settingsErr = ""
	a.setNoticeOKKey("notice.settingsSaved")
	a.invalidate()
}

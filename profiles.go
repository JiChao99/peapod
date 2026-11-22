package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Setup profiles (P1-8): a named snapshot of the variant combination — pages,
// template, view mode, UA, storage and record toggles — persisted under
// Pictures/Peapod/profiles so a whole task setup can be reloaded in one click.

// profilePage is one setup row as stored on disk.
type profilePage struct {
	Name string  `json:"name"`
	URL  string  `json:"url"`
	Vars string  `json:"vars"`
	ResW float64 `json:"resW"`
	ResH float64 `json:"resH"`
}

// profileData mirrors the raw setup-form fields (not the resolved start
// request), so loading lands exactly what the user had typed.
type profileData struct {
	Template     string        `json:"template"`
	ViewMode     string        `json:"viewMode"`
	UAPreset     string        `json:"uaPreset"`
	UACustom     string        `json:"uaCustom"`
	ShareStorage bool          `json:"shareStorage"`
	Record       bool          `json:"record"`
	SyncDelay    string        `json:"syncDelay"`
	Pages        []profilePage `json:"pages"`
}

const profileMaxBytes = 1 << 20

// profilesRoot is where saved profiles live.
func profilesRoot() string { return filepath.Join(userArtifactRoot(), "profiles") }

func profilePath(name string) string {
	return filepath.Join(profilesRoot(), sanitizeFileName(name)+".json")
}

func writeProfileFile(name string, p profileData) error {
	if err := os.MkdirAll(profilesRoot(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(profilePath(name), data, 0o644)
}

func readProfileFile(name string) (profileData, error) {
	var p profileData
	data, err := readBoundedFile(profilePath(name), profileMaxBytes)
	if err != nil {
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	return p, nil
}

// listProfiles returns saved profile names (file stems), alphabetically.
// Unreadable directories or non-JSON files are ignored.
func listProfiles() []string {
	entries, err := os.ReadDir(profilesRoot())
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(out)
	return out
}

// currentProfile snapshots the setup form.
func (a *app) currentProfile() profileData {
	pages := make([]profilePage, 0, len(a.configs))
	for _, cfg := range a.configs {
		pages = append(pages, profilePage{
			Name: cfg.name, URL: cfg.url, Vars: cfg.vars,
			ResW: cfg.resW, ResH: cfg.resH,
		})
	}
	return profileData{
		Template:     a.template,
		ViewMode:     a.viewMode,
		UAPreset:     a.uaPreset,
		UACustom:     a.uaCustom,
		ShareStorage: a.shareStorage,
		Record:       a.record,
		SyncDelay:    a.syncDelay,
		Pages:        pages,
	}
}

// applyProfile loads a profile into the setup form, replacing the rows.
// Values are treated as untrusted (files can be edited by hand): page count,
// view mode, UA preset and resolutions are validated or clamped. Fresh row ids
// keep the UI's per-row keys unique.
func (a *app) applyProfile(p profileData) error {
	if len(p.Pages) < 2 || len(p.Pages) > 10 {
		return fmt.Errorf("%s", a.tr("err.profilesPages"))
	}
	preset := uaDefault
	for _, up := range uaPresets {
		if up.key == p.UAPreset {
			preset = up.key
			break
		}
	}
	viewMode := "desktop"
	if p.ViewMode == "mobile" {
		viewMode = "mobile"
	}
	configs := make([]*pageConfig, 0, len(p.Pages))
	for _, pg := range p.Pages {
		a.configSeq++
		configs = append(configs, &pageConfig{
			id:   fmt.Sprintf("c%d", a.configSeq),
			name: pg.Name, url: pg.URL, vars: pg.Vars,
			resW: clampRes(pg.ResW, a.resW), resH: clampRes(pg.ResH, a.resH),
		})
	}
	a.configs = configs
	a.template = p.Template
	// lastViewMode = viewMode suppresses the mode-change reseed in the panel,
	// so the profile's per-page resolutions survive.
	a.viewMode, a.lastViewMode = viewMode, viewMode
	a.resW, a.resH = configs[0].resW, configs[0].resH
	// The profile's resolution becomes this mode's remembered preset, so a
	// later mode switch restores it instead of the factory default.
	if a.resByMode == nil {
		a.resByMode = map[string][2]float64{}
	}
	a.resByMode[viewMode] = [2]float64{a.resW, a.resH}
	a.uaPreset = preset
	a.uaCustom = p.UACustom
	a.shareStorage = p.ShareStorage
	a.record = p.Record
	syncDelay := "100"
	for _, v := range syncDelayOptions {
		if v == p.SyncDelay {
			syncDelay = v
			break
		}
	}
	a.syncDelay = syncDelay
	a.invalidate()
	return nil
}

// requestSaveProfile is the panel's save entry point: a new name saves
// directly, while a name that already exists on disk opens the overwrite
// confirm modal first (renderProfileOverwrite in panel.go).
func (a *app) requestSaveProfile(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		a.setNoticeErrKey("err.profilesName")
		return
	}
	if _, err := os.Stat(profilePath(name)); err == nil {
		a.profileOverwriteName = name
		a.profileOverwriteOpen = true
		return
	}
	a.saveProfile(name)
}

// saveProfile writes the current form under name, overwriting an existing
// profile of that name.
func (a *app) saveProfile(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		a.setNoticeErrKey("err.profilesName")
		return
	}
	if err := writeProfileFile(name, a.currentProfile()); err != nil {
		a.setNoticeErrKey("err.profilesWrite", err.Error())
		return
	}
	a.profiles = listProfiles()
	a.profileSel = sanitizeFileName(name)
	a.setNoticeOKKey("notice.profilesSaved", name)
}

// loadProfileIntoForm applies the selected profile to the setup form.
func (a *app) loadProfileIntoForm() {
	if a.profileSel == "" {
		a.setNoticeErrKey("err.profilesPick")
		return
	}
	p, err := readProfileFile(a.profileSel)
	if err != nil {
		a.setNoticeErrKey("err.profilesRead", err.Error())
		return
	}
	if err := a.applyProfile(p); err != nil {
		a.setNoticeErr(err.Error())
		return
	}
	a.setNoticeOKKey("notice.profilesLoaded", a.profileSel)
}

// deleteProfile removes the selected profile file.
func (a *app) deleteProfile() {
	if a.profileSel == "" {
		a.setNoticeErrKey("err.profilesPick")
		return
	}
	name := a.profileSel
	if err := os.Remove(profilePath(name)); err != nil {
		a.setNoticeErrKey("err.profilesDelete", err.Error())
		return
	}
	a.profileSel = ""
	a.profileName = ""
	a.profiles = listProfiles()
	a.setNoticeOKKey("notice.profilesDeleted", name)
}

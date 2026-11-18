package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pinProfileHome redirects userArtifactRoot (and thus profilesRoot) into a
// temp dir for the duration of the test.
func pinProfileHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	return home
}

func sampleProfile() profileData {
	return profileData{
		Template:     "https://{host}/{lang}",
		ViewMode:     "mobile",
		UAPreset:     "iphone",
		ShareStorage: true,
		Record:       true,
		Pages: []profilePage{
			{Name: "primary", Vars: "host=example.com, lang=en-us", ResW: 390, ResH: 844},
			{Name: "de", Vars: "host=example.com, lang=de", ResW: 390, ResH: 844},
		},
	}
}

func TestProfileFileRoundTrip(t *testing.T) {
	pinProfileHome(t)
	want := sampleProfile()
	if err := writeProfileFile("qa", want); err != nil {
		t.Fatalf("writeProfileFile: %v", err)
	}
	got, err := readProfileFile("qa")
	if err != nil {
		t.Fatalf("readProfileFile: %v", err)
	}
	if got.Template != want.Template || got.ViewMode != want.ViewMode || got.UAPreset != want.UAPreset ||
		got.ShareStorage != want.ShareStorage || got.Record != want.Record || len(got.Pages) != 2 ||
		got.Pages[0].Vars != want.Pages[0].Vars || got.Pages[1].ResH != 844 {
		t.Fatalf("round trip mismatch: %+v vs %+v", got, want)
	}
}

func TestListProfilesSortedAndFiltered(t *testing.T) {
	pinProfileHome(t)
	for _, n := range []string{"beta", "alpha"} {
		if err := writeProfileFile(n, sampleProfile()); err != nil {
			t.Fatalf("writeProfileFile(%q): %v", n, err)
		}
	}
	if err := os.MkdirAll(profilesRoot(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesRoot(), "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := listProfiles()
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Fatalf("listProfiles = %v, want [alpha beta]", got)
	}
}

func TestProfileSaveSanitizesName(t *testing.T) {
	home := pinProfileHome(t)
	a := newApp()
	a.profileName = "a/b"
	a.saveProfile(a.profileName)
	want := filepath.Join(home, "Pictures", "Peapod", "profiles", "a_b.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("sanitized file missing: %v", err)
	}
	if len(a.profiles) != 1 || a.profiles[0] != "a_b" {
		t.Fatalf("profiles = %v, want [a_b]", a.profiles)
	}
	if a.profileSel != "a_b" {
		t.Fatalf("profileSel = %q, want a_b", a.profileSel)
	}
}

func TestSaveLoadFormRoundTrip(t *testing.T) {
	pinProfileHome(t)
	a := newApp()
	a.template = "https://{host}/{lang}"
	a.viewMode, a.lastViewMode = "mobile", "desktop"
	a.uaPreset, a.uaCustom = "ipad", ""
	a.shareStorage, a.record = true, true
	a.configs = []*pageConfig{
		{id: "c1", vars: "host=a.com, lang=en", resW: 400, resH: 900},
		{id: "c2", vars: "host=a.com, lang=de", resW: 400, resH: 900},
	}
	a.saveProfile("qa")

	// Scramble the form, then load the profile back.
	b := newApp()
	b.template = ""
	b.viewMode = "desktop"
	b.uaPreset = uaDefault
	b.configs = []*pageConfig{
		{id: "c1", resW: 1280, resH: 800},
		{id: "c2", resW: 1280, resH: 800},
		{id: "c3", resW: 1280, resH: 800},
	}
	b.profileSel = "qa"
	b.loadProfileIntoForm()

	if b.notice != trLang("zh", "notice.profilesLoaded", "qa") {
		t.Fatalf("notice = %q, want loaded", b.notice)
	}
	if b.template != a.template || b.viewMode != "mobile" || b.uaPreset != "ipad" ||
		!b.shareStorage || !b.record {
		t.Fatalf("form not restored: %+v", b.currentProfile())
	}
	if len(b.configs) != 2 {
		t.Fatalf("configs = %d, want 2", len(b.configs))
	}
	if b.configs[0].vars != "host=a.com, lang=en" || b.configs[0].resW != 400 || b.configs[0].resH != 900 {
		t.Fatalf("page 0 not restored: %+v", *b.configs[0])
	}
	if b.configs[0].id == "c1" || b.configs[0].id == b.configs[1].id {
		t.Fatalf("loaded rows need fresh unique ids, got %q/%q", b.configs[0].id, b.configs[1].id)
	}
	// lastViewMode must equal viewMode so the panel does not reseed resolutions.
	if b.lastViewMode != b.viewMode {
		t.Fatalf("lastViewMode = %q, want %q", b.lastViewMode, b.viewMode)
	}
	if b.resW != 400 || b.resH != 900 {
		t.Fatalf("seed res = %v×%v, want the profile's 400×900", b.resW, b.resH)
	}
}

func TestApplyProfileGuards(t *testing.T) {
	pinProfileHome(t)
	base := sampleProfile()

	count := func(n int) profileData {
		p := base
		p.Pages = make([]profilePage, n)
		for i := range p.Pages {
			p.Pages[i] = profilePage{Name: "p"}
		}
		return p
	}
	cases := []struct {
		name string
		p    profileData
	}{
		{"one page", count(1)},
		{"eleven pages", count(11)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a := newApp()
			before := len(a.configs)
			if err := a.applyProfile(c.p); err == nil {
				t.Fatal("want invalid-page-count error")
			}
			if len(a.configs) != before {
				t.Fatalf("form must be untouched on error: %d rows", len(a.configs))
			}
		})
	}

	a := newApp()
	p := count(2)
	p.UAPreset = "nokia-3310"
	p.ViewMode = "tablet"
	p.Pages[0].ResW = 10  // below range
	p.Pages[0].ResH = 9e4 // above range
	p.Pages[1].ResW = 0   // unset falls back to the seed
	p.Pages[1].ResH = 0
	if err := a.applyProfile(p); err != nil {
		t.Fatalf("applyProfile: %v", err)
	}
	if a.uaPreset != uaDefault {
		t.Fatalf("uaPreset = %q, want the default fallback", a.uaPreset)
	}
	if a.viewMode != "desktop" {
		t.Fatalf("viewMode = %q, want desktop fallback", a.viewMode)
	}
	if a.configs[0].resW != 200 || a.configs[0].resH != 3840 {
		t.Fatalf("res not clamped: %v×%v", a.configs[0].resW, a.configs[0].resH)
	}
	if a.configs[1].resW != 1280 || a.configs[1].resH != 800 {
		t.Fatalf("unset res should inherit the seed: %v×%v", a.configs[1].resW, a.configs[1].resH)
	}
}

func TestProfileEmptyNameAndMissingFile(t *testing.T) {
	pinProfileHome(t)
	a := newApp()
	a.profileName = "  "
	a.saveProfile(a.profileName)
	if a.notice != trLang("zh", "err.profilesName") {
		t.Fatalf("notice = %q, want empty-name error", a.notice)
	}

	a.profileSel = ""
	a.loadProfileIntoForm()
	if a.notice != trLang("zh", "err.profilesPick") {
		t.Fatalf("notice = %q, want pick-first error", a.notice)
	}

	a.profileSel = "ghost"
	a.loadProfileIntoForm()
	if !strings.Contains(a.notice, trLang("zh", "err.profilesRead", "")) {
		t.Fatalf("notice = %q, want read-failure error", a.notice)
	}
}

func TestDeleteProfile(t *testing.T) {
	pinProfileHome(t)
	if err := writeProfileFile("gone", sampleProfile()); err != nil {
		t.Fatal(err)
	}
	a := newApp()
	a.profiles = listProfiles()
	a.profileSel = "gone"
	a.deleteProfile()
	if _, err := os.Stat(profilePath("gone")); !os.IsNotExist(err) {
		t.Fatalf("file still present: %v", err)
	}
	if a.profileSel != "" || len(a.profiles) != 0 {
		t.Fatalf("sel=%q profiles=%v, want cleared", a.profileSel, a.profiles)
	}
	if a.notice != trLang("zh", "notice.profilesDeleted", "gone") {
		t.Fatalf("notice = %q, want deleted", a.notice)
	}
}

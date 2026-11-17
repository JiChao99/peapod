package main

import (
	"strings"
	"testing"
)

func TestResolveUA(t *testing.T) {
	cases := []struct {
		name, preset, custom, mode, want string
	}{
		{"default desktop", uaDefault, "", "desktop", ""},
		{"default mobile", uaDefault, "", "mobile", mobileUA},
		{"empty preset mobile", "", "", "mobile", mobileUA},
		{"iphone overrides desktop", "iphone", "", "desktop", mobileUA},
		{"iphone in mobile", "iphone", "", "mobile", mobileUA},
		{"android preset", "android", "", "desktop", "Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36"},
		{"custom trimmed", uaCustom, "  MyUA/1.0  ", "desktop", "MyUA/1.0"},
		{"custom wins over mode", uaCustom, "MyUA/1.0", "mobile", "MyUA/1.0"},
		{"empty custom falls back", uaCustom, "", "mobile", mobileUA},
		{"blank custom falls back", uaCustom, "   ", "desktop", ""},
		{"unknown preset falls back", "nope", "", "mobile", mobileUA},
	}
	for _, c := range cases {
		if got := resolveUA(c.preset, c.custom, c.mode); got != c.want {
			t.Errorf("%s: resolveUA(%q, %q, %q) = %q, want %q", c.name, c.preset, c.custom, c.mode, got, c.want)
		}
	}
}

func TestUAPresets(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range uaPresets {
		if seen[p.key] {
			t.Errorf("duplicate preset key %q", p.key)
		}
		seen[p.key] = true
		// Every preset must be labeled in both catalogs. Read the catalogs
		// directly: trLang falls back to zh, which would mask a missing en key.
		for _, lang := range []string{"zh", "en"} {
			key := "ui.ua." + p.key
			if catalogs[lang][key] == "" {
				t.Errorf("preset %q missing %s label %q", p.key, lang, key)
			}
		}
	}
	for _, key := range []string{uaDefault, "iphone", "ipad", "android", "androidTablet", "chrome", "edge", "firefox", "safari", uaCustom} {
		if !seen[key] {
			t.Errorf("preset %q missing from uaPresets", key)
		}
	}
	// Chrome-based Android UAs tell phones from tablets by the "Mobile" token
	// (Safari iOS UAs carry "Mobile/<build>" on both iPhone and iPad instead).
	for _, p := range uaPresets {
		switch p.key {
		case "android":
			if !strings.Contains(p.ua, " Mobile ") {
				t.Errorf("phone preset %q should carry the Mobile token", p.key)
			}
		case "androidTablet":
			if strings.Contains(p.ua, " Mobile ") {
				t.Errorf("tablet preset %q should not carry the Mobile token", p.key)
			}
		}
	}
}

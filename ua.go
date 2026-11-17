package main

import "strings"

// User-Agent presets for the setup panel's device select. The strings are
// representative device signatures for sniffing tests, not exact current
// releases. Overriding the UA only changes what servers and navigator.userAgent
// see — the rendering engine stays the platform webview (WebView2 / WKWebView /
// WebKitGTK), so engine-specific behavior cannot be simulated this way.
const (
	uaDefault = "default" // follow the view mode: mobile → mobileUA, desktop → engine UA
	uaCustom  = "custom"  // user-typed string; empty falls back to the view mode
)

// uaPresets lists the built-in options in panel order. The label for each key
// lives in the i18n catalogs under "ui.ua.<key>".
var uaPresets = []struct {
	key string
	ua  string
}{
	{uaDefault, ""},
	{"iphone", mobileUA},
	{"ipad", "Mozilla/5.0 (iPad; CPU OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"},
	{"android", "Mozilla/5.0 (Linux; Android 15; Pixel 9) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Mobile Safari/537.36"},
	// Tablet UAs omit the "Mobile" token; sites use it to tell phones from tablets.
	{"androidTablet", "Mozilla/5.0 (Linux; Android 15; SM-X710) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"},
	{"chrome", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36"},
	{"edge", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36 Edg/142.0.0.0"},
	{"firefox", "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:145.0) Gecko/20100101 Firefox/145.0"},
	{"safari", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.0 Safari/605.1.15"},
	{uaCustom, ""},
}

// uaForMode is the implicit default when no explicit UA is chosen: mobile
// tasks impersonate iPhone Safari (the pre-preset behavior), desktop tasks
// keep the engine's own UA.
func uaForMode(viewMode string) string {
	if viewMode == "mobile" {
		return mobileUA
	}
	return ""
}

// resolveUA maps the panel's preset choice, custom text and view mode to the
// UA string applied at page creation. Empty means the engine default. An
// empty custom string or an unknown preset falls back to the view mode.
func resolveUA(preset, custom, viewMode string) string {
	switch preset {
	case uaCustom:
		if s := strings.TrimSpace(custom); s != "" {
			return s
		}
	case uaDefault, "":
	default:
		for _, p := range uaPresets {
			if p.key == preset {
				return p.ua
			}
		}
	}
	return uaForMode(viewMode)
}

package main

import "testing"

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

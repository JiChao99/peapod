package main

import (
	"strings"
	"testing"
)

func TestParseVars(t *testing.T) {
	vars, bad := parseVars("host=a.com, lang=en")
	if bad != "" || vars["host"] != "a.com" || vars["lang"] != "en" {
		t.Fatalf("parseVars = %v, %q", vars, bad)
	}
	vars, bad = parseVars(" host = a.com \n lang=en ")
	if bad != "" || vars["host"] != "a.com" || vars["lang"] != "en" {
		t.Fatalf("parseVars whitespace = %v, %q", vars, bad)
	}
	if _, bad := parseVars("host=a.com, oops"); bad != "oops" {
		t.Errorf("bad token = %q, want oops", bad)
	}
	if _, bad := parseVars("=a.com"); bad != "=a.com" {
		t.Errorf("empty key = %q", bad)
	}
	if vars, bad := parseVars(""); bad != "" || len(vars) != 0 {
		t.Errorf("empty input = %v, %q", vars, bad)
	}
}

func TestRenderTemplate(t *testing.T) {
	out, missing := renderTemplate("https://{host}/{lang}", map[string]string{"host": "a.com", "lang": "en"})
	if out != "https://a.com/en" || len(missing) != 0 {
		t.Fatalf("render = %q, %v", out, missing)
	}
	out, missing = renderTemplate("https://{host}/{lang}", map[string]string{"host": "a.com"})
	if len(missing) != 1 || missing[0] != "lang" {
		t.Fatalf("missing = %v", missing)
	}
	out, missing = renderTemplate("https://a.com", nil)
	if out != "https://a.com" || len(missing) != 0 {
		t.Fatalf("no placeholders = %q, %v", out, missing)
	}
}

func TestBuildRequestTemplateMode(t *testing.T) {
	a := newApp()
	a.template = "https://{host}/{lang}"
	a.configs[0].vars = "host=a.com, lang=en"
	a.configs[1].vars = "host=b.com, lang=de"
	a.configs[2].vars = "host=c.com, lang=en"
	a.configs[2].url = "/shop?x=1"
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	wantURL := []string{"https://a.com/en/", "https://b.com/de/", "https://c.com/en/shop?x=1"}
	wantBase := []string{"https://a.com/en", "https://b.com/de", "https://c.com/en"}
	for i, p := range req.pages {
		if p.url != wantURL[i] || p.base != wantBase[i] {
			t.Errorf("page %d = %q base %q, want %q base %q", i, p.url, p.base, wantURL[i], wantBase[i])
		}
	}
	// Follow maps across template bases: primary /en/cart onto follower /de.
	if got := mapNavigation(req.pages[0].base, req.pages[1].base, "https://a.com/en/cart"); got != "https://b.com/de/cart" {
		t.Errorf("template follow = %q", got)
	}
}

func TestBuildRequestTemplateErrors(t *testing.T) {
	a := newApp()
	a.template = "https://{host}/{lang}"
	a.configs[0].vars = "host=a.com"
	if _, err := a.buildRequest(); err == nil || !strings.Contains(err.Error(), "lang") {
		t.Errorf("missing var error = %v", err)
	}
	a.configs[0].vars = "host=a.com, lang=en, oops"
	if _, err := a.buildRequest(); err == nil {
		t.Error("want bad-var error")
	}
	a.configs[0].vars = "lang=en"
	if _, err := a.buildRequest(); err == nil {
		t.Error("want invalid rendered base error")
	}
}

func TestTemplatePreview(t *testing.T) {
	a := newApp()
	a.template = "https://{host}/{lang}"
	cfg := &pageConfig{vars: "host=a.com, lang=en", url: "shop"}
	if got := a.templatePreview(cfg); got != "https://a.com/en/shop" {
		t.Errorf("preview = %q", got)
	}
	cfg.vars = "host=a.com"
	if got := a.templatePreview(cfg); !strings.Contains(got, "lang") {
		t.Errorf("missing preview = %q", got)
	}
}

func TestCleanTemplateBase(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"https://a.com/en", "https://a.com/en", true},
		{"https://a.com/en/", "https://a.com/en", true},
		{"https://a.com/", "https://a.com", true},
		{"https://a.com/en//", "https://a.com/en", true},
		{"https://a.com/?lang=en", "", false},
		{"https://a.com/en#frag", "", false},
		{"https://a.com/?", "", false},
	}
	for _, c := range cases {
		got, ok := cleanTemplateBase(c.in)
		if ok != c.ok || got != c.want {
			t.Errorf("cleanTemplateBase(%q) = %q,%v want %q,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestBuildRequestTemplateBaseRules(t *testing.T) {
	// Trailing slash in the template must not double the path separator.
	a := newApp()
	a.configs = a.configs[:2]
	a.template = "https://{host}/{lang}/"
	a.configs[0].vars = "host=a.com, lang=en"
	a.configs[0].url = "/shop"
	a.configs[1].vars = "host=b.com, lang=de"
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("trailing slash: %v", err)
	}
	if req.pages[0].url != "https://a.com/en/shop" || req.pages[0].base != "https://a.com/en" {
		t.Errorf("trailing slash page0 = %q base %q", req.pages[0].url, req.pages[0].base)
	}
	if got := mapNavigation(req.pages[0].base, req.pages[1].base, "https://a.com/en/step2"); got != "https://b.com/de/step2" {
		t.Errorf("trailing slash follow = %q", got)
	}

	// Query/fragment in the base is rejected.
	for _, tmpl := range []string{"https://{host}/?lang={lang}", "https://{host}/{lang}#x"} {
		a := newApp()
		a.configs = a.configs[:2]
		a.template = tmpl
		a.configs[0].vars = "host=a.com, lang=en"
		a.configs[1].vars = "host=b.com, lang=de"
		if _, err := a.buildRequest(); err == nil || !strings.Contains(err.Error(), "模板基址") {
			t.Errorf("template %q: want query/fragment rejection, got %v", tmpl, err)
		}
	}

	// Invalid assembled URL (bad path escape) is rejected even with a valid base.
	a = newApp()
	a.configs = a.configs[:2]
	a.template = "https://{host}/{lang}"
	a.configs[0].vars = "host=a.com, lang=en"
	a.configs[0].url = "/shop%zz"
	a.configs[1].vars = "host=b.com, lang=de"
	if _, err := a.buildRequest(); err == nil || !strings.Contains(err.Error(), "URL 无效") {
		t.Errorf("want assembled-URL validation error for bad path escape, got %v", err)
	}
}

func TestTemplatePreviewMatchesBaseRules(t *testing.T) {
	a := newApp()
	a.template = "https://{host}/{lang}/"
	cfg := &pageConfig{vars: "host=a.com, lang=en", url: "/shop"}
	if got := a.templatePreview(cfg); got != "https://a.com/en/shop" {
		t.Errorf("trailing-slash preview = %q", got)
	}
	a.template = "https://{host}/?lang={lang}"
	if got := a.templatePreview(cfg); got == "https://a.com/?lang=en/shop" {
		t.Errorf("query-base preview should report an error, got %q", got)
	}
}

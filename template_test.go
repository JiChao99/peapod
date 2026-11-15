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
package main

import (
	"strings"
	"testing"
)

func TestParseURLLines(t *testing.T) {
	got := parseURLLines(" https://a.example.com \r\n\nhttps://b.example.com/path\r\n  \n")
	want := []string{"https://a.example.com", "https://b.example.com/path"}
	if len(got) != len(want) {
		t.Fatalf("parseURLLines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, got[i], want[i])
		}
	}
	if got := parseURLLines(" \n\t\n"); len(got) != 0 {
		t.Errorf("blank list = %v, want empty", got)
	}
}

func TestBulkAddURLs(t *testing.T) {
	a := newApp()
	a.configs = a.configs[:1]
	a.configs[0].url = "https://primary.example.com"
	a.resW, a.resH = 390, 844
	n, err := a.bulkAddURLs("https://a.example.com\nhttps://b.example.com/x\n")
	if err != nil {
		t.Fatalf("bulkAddURLs: %v", err)
	}
	if n != 2 || len(a.configs) != 3 {
		t.Fatalf("added %d, configs %d, want 2 and 3", n, len(a.configs))
	}
	for i, want := range []string{"a.example.com", "b.example.com/x"} {
		cfg := a.configs[i+1]
		if cfg.name != want {
			t.Errorf("page %d name = %q, want host/path %q", i+1, cfg.name, want)
		}
		if cfg.resW != 390 || cfg.resH != 844 {
			t.Errorf("page %d resolution = %vx%v, want inherited 390x844", i+1, cfg.resW, cfg.resH)
		}
	}
	if a.configs[1].url != "https://a.example.com" || a.configs[2].url != "https://b.example.com/x" {
		t.Errorf("urls not kept verbatim: %q, %q", a.configs[1].url, a.configs[2].url)
	}
}

func TestBulkAddURLsSameHostVariants(t *testing.T) {
	a := newApp()
	a.configs = a.configs[:1]
	a.configs[0].url = "https://primary.example.com"
	n, err := a.bulkAddURLs("https://example.com/en\nhttps://example.com/zh\nhttps://example.com/en\n")
	if err != nil {
		t.Fatalf("bulkAddURLs: %v", err)
	}
	if n != 3 {
		t.Fatalf("added %d, want 3", n)
	}
	want := []string{"example.com/en", "example.com/zh", "example.com/en-2"}
	for i, name := range want {
		if got := a.configs[i+1].name; got != name {
			t.Errorf("page %d name = %q, want %q", i+1, got, name)
		}
	}
}

func TestBulkAddURLsAllOrNothing(t *testing.T) {
	a := newApp()
	before := len(a.configs)
	if _, err := a.bulkAddURLs("https://ok.example.com\nnot a url"); err == nil {
		t.Fatal("want error for invalid line")
	} else if !strings.Contains(err.Error(), "2") {
		t.Errorf("error should point at line 2, got %q", err.Error())
	}
	if len(a.configs) != before {
		t.Errorf("configs mutated on failure: %d, want %d", len(a.configs), before)
	}

	if _, err := a.bulkAddURLs(strings.Repeat("https://ok.example.com\n", 8)); err == nil {
		t.Fatal("want error beyond the ten-page cap")
	}
	if len(a.configs) != before {
		t.Errorf("configs mutated on cap failure: %d, want %d", len(a.configs), before)
	}

	if _, err := a.bulkAddURLs("  \n"); err == nil {
		t.Fatal("want error for empty list")
	}

	a.template = "https://{host}/"
	if _, err := a.bulkAddURLs("https://ok.example.com"); err == nil {
		t.Fatal("want error in template mode")
	}
	if len(a.configs) != before {
		t.Errorf("configs mutated in template mode: %d, want %d", len(a.configs), before)
	}
}

func TestDuplicatePage(t *testing.T) {
	a := newApp()
	src := a.configs[1]
	src.name = "Staging"
	src.url = "https://staging.example.com"
	src.vars = "lang=de"
	src.resW, src.resH = 390, 844
	tail := a.configs[2]
	a.duplicatePage(src.id)
	if len(a.configs) != 4 {
		t.Fatalf("configs = %d, want 4", len(a.configs))
	}
	clone := a.configs[2]
	if clone.id == src.id || clone.id == tail.id {
		t.Errorf("clone id %q collides", clone.id)
	}
	if clone.name != "Staging 副本" {
		t.Errorf("clone name = %q, want suffixed copy", clone.name)
	}
	if clone.url != src.url || clone.vars != src.vars || clone.resW != 390 || clone.resH != 844 {
		t.Errorf("clone fields = %+v, want source fields", clone)
	}
	if a.configs[1] != src || a.configs[3] != tail {
		t.Error("duplicate must insert after the source, keeping order")
	}
	if src.name != "Staging" {
		t.Errorf("source renamed by clone: %q", src.name)
	}
}

func TestDuplicatePageEmptyNameAndCap(t *testing.T) {
	a := newApp()
	a.duplicatePage(a.configs[0].id)
	if a.configs[1].name != "" {
		t.Errorf("empty name gained a suffix: %q", a.configs[1].name)
	}
	for len(a.configs) < 10 {
		a.addPage()
	}
	a.duplicatePage(a.configs[0].id)
	if len(a.configs) != 10 {
		t.Errorf("duplicate past cap: %d, want 10", len(a.configs))
	}
}

func TestBulkAddURLsCapBoundary(t *testing.T) {
	a := newApp()
	n, err := a.bulkAddURLs(strings.Repeat("https://ok.example.com\n", 7))
	if err != nil {
		t.Fatalf("3+7 must fit the cap: %v", err)
	}
	if n != 7 || len(a.configs) != 10 {
		t.Errorf("added %d, configs %d, want 7 and 10", n, len(a.configs))
	}
}

func TestDuplicatePageEnglishSuffix(t *testing.T) {
	a := newApp()
	a.langVal.Store("en")
	a.configs[1].name = "Staging"
	a.duplicatePage(a.configs[1].id)
	if got := a.configs[2].name; got != "Staging copy" {
		t.Errorf("en clone name = %q, want %q", got, "Staging copy")
	}
}

func TestDuplicatePagePositions(t *testing.T) {
	a := newApp()
	primary, last := a.configs[0], a.configs[2]
	a.duplicatePage(primary.id)
	if len(a.configs) != 4 || a.configs[0] != primary || a.configs[1].url != primary.url {
		t.Fatalf("primary duplicate must land at index 1, keeping primary first")
	}
	a.duplicatePage(last.id)
	if len(a.configs) != 5 || a.configs[4].id == last.id || a.configs[3] != last {
		t.Errorf("last-row duplicate must append after the source")
	}
}

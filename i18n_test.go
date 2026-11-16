package main

import (
	"sync"
	"testing"
)

func TestTrLang(t *testing.T) {
	cases := []struct {
		lang, key string
		args      []any
		want      string
	}{
		{"zh", "ui.start", nil, "开始任务"},
		{"en", "ui.start", nil, "Start task"},
		{"fr", "ui.start", nil, "开始任务"},
		{"en", "ui.page.n", []any{3}, "Page 3"},
		{"zh", "ui.page.n", []any{3}, "页面 3"},
		{"en", "no.such.key", nil, "no.such.key"},
	}
	for _, c := range cases {
		if got := trLang(c.lang, c.key, c.args...); got != c.want {
			t.Errorf("trLang(%q, %q, %v) = %q, want %q", c.lang, c.key, c.args, got, c.want)
		}
	}
}

func TestCurrentLang(t *testing.T) {
	a := newApp()
	if got := a.currentLang(); got != "zh" {
		t.Fatalf("default currentLang = %q, want zh", got)
	}
	if got := a.tr("ui.start"); got != "开始任务" {
		t.Fatalf("tr default = %q, want 开始任务", got)
	}
	// The panel publishes a changed language; off-thread readers pick it up.
	a.langVal.Store("en")
	if got := a.currentLang(); got != "en" {
		t.Fatalf("published currentLang = %q, want en", got)
	}
	if got := a.tr("ui.start"); got != "Start task" {
		t.Fatalf("tr after publish = %q, want Start task", got)
	}
	// A zero-value app that has not published yet falls back to zh.
	var zero app
	if got := zero.currentLang(); got != "zh" {
		t.Fatalf("zero currentLang = %q, want zh", got)
	}
}

// TestCurrentLangConcurrent exercises the publisher/reader split that replaces
// the old unsynchronized a.lang access; run with -race to validate it.
func TestCurrentLangConcurrent(t *testing.T) {
	a := newApp()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				a.langVal.Store("en")
				a.langVal.Store("zh")
			}
		}()
	}
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = a.tr("ui.start")
			}
		}()
	}
	wg.Wait()
}

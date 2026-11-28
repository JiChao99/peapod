package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/egoist/mygo"
	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestWebErrorName(t *testing.T) {
	cases := map[int]string{
		13: "NAME_NOT_RESOLVED",
		6:  "SERVER_UNREACHABLE",
		7:  "TIMEOUT",
		12: "CANNOT_CONNECT",
		2:  "CERT_EXPIRED",
		99: "WEB_ERROR_99",
	}
	for code, want := range cases {
		if got := webErrorName(code); got != want {
			t.Errorf("webErrorName(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestMarkNavFailedSetsError(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "ready", url: "https://bad.invalid/"}
	a.mu.Lock()
	a.markNavFailedLocked(p, &mygo.LoadError{URL: "https://bad.invalid/", Code: 13, Description: "navigation failed"})
	a.mu.Unlock()
	if p.status != "error" {
		t.Fatalf("status = %q, want error", p.status)
	}
	if p.navError == nil || p.navError.Name != navErrorName(13) || p.navError.Code != 13 {
		t.Fatalf("navError = %+v", p.navError)
	}
	if !p.navFailed {
		t.Fatal("a failed attempt must set navFailed so a Linux finish-after-fail keeps the error")
	}
}

func TestMarkNavFailedPreservesPaused(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p2", status: "paused"}
	a.mu.Lock()
	a.markNavFailedLocked(p, &mygo.LoadError{URL: "https://bad.invalid/", Code: 13})
	a.mu.Unlock()
	if p.status != "paused" {
		t.Fatalf("paused page should stay paused, got %q", p.status)
	}
	if p.navError == nil {
		t.Fatal("navError should still be recorded on a paused page")
	}
}

func TestMarkNavFailedClosedNoop(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "ready", closed: true}
	a.mu.Lock()
	a.markNavFailedLocked(p, &mygo.LoadError{Code: 13})
	a.mu.Unlock()
	if p.navError != nil || p.status != "ready" {
		t.Fatalf("closed page mutated: %+v", p)
	}
}

func TestMarkNavCommittedClearsError(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "error", navSeq: 2, navError: &navFailure{URL: "https://bad.invalid/", Name: "NAME_NOT_RESOLVED"}}
	a.mu.Lock()
	a.markNavCommittedLocked(p, "https://good.example/")
	a.mu.Unlock()
	if p.navError != nil {
		t.Fatalf("navError should clear on a new document, got %+v", p.navError)
	}
	if p.status != "loading" {
		t.Fatalf("status = %q, want loading", p.status)
	}
	if p.navSeq != 3 {
		t.Fatalf("navSeq = %d, want 3", p.navSeq)
	}
}

func TestMarkNavCommittedKeepsErrorForSameURL(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "error", navError: &navFailure{URL: "https://bad.invalid/", Name: "NAME_NOT_RESOLVED"}}
	a.mu.Lock()
	// WebView2 reports the error document under the same failed URL.
	a.markNavCommittedLocked(p, "https://bad.invalid/")
	a.mu.Unlock()
	if p.navError == nil || p.status != "error" {
		t.Fatalf("error document must not mask the failure: status=%q navError=%+v", p.status, p.navError)
	}
}

func TestMarkNavCommittedPreservesPaused(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p2", status: "paused"}
	a.mu.Lock()
	a.markNavCommittedLocked(p, "https://good.example/")
	a.mu.Unlock()
	if p.status != "paused" {
		t.Fatalf("paused page should stay paused, got %q", p.status)
	}
}

func TestMarkReadySkipsErrorAndPaused(t *testing.T) {
	a := newApp()
	failed := &pageRecord{id: "p1", status: "error", navError: &navFailure{URL: "https://bad.invalid/"}}
	paused := &pageRecord{id: "p2", status: "paused"}
	loading := &pageRecord{id: "p3", status: "loading"}
	a.mu.Lock()
	a.markReadyLocked(failed)
	a.markReadyLocked(paused)
	a.markReadyLocked(loading)
	a.mu.Unlock()
	if failed.status != "error" {
		t.Fatalf("ready must not override error, got %q", failed.status)
	}
	if paused.status != "paused" {
		t.Fatalf("ready must not override paused, got %q", paused.status)
	}
	if loading.status != "ready" {
		t.Fatalf("loading should become ready, got %q", loading.status)
	}
}

func TestMarkNavSucceededRecoversSameURLRetry(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "error", url: "https://bad.invalid/",
		navError: &navFailure{URL: "https://bad.invalid/", Name: "NAME_NOT_RESOLVED"}}
	a.mu.Lock()
	// An in-page retry of the same URL (error-page Refresh / location.reload)
	// commits under the same failed URL, so the commit guard keeps the failure...
	a.markNavCommittedLocked(p, "https://bad.invalid/")
	if p.navError == nil {
		t.Fatal("same-URL commit should keep the failure until the load succeeds")
	}
	// ...but the retry's successful finish (IsSuccess=true) supersedes it.
	a.markNavSucceededLocked(p)
	a.mu.Unlock()
	if p.navError != nil || p.status != "ready" {
		t.Fatalf("recovered retry should be ready, got status=%q navError=%+v", p.status, p.navError)
	}
}

func TestMarkNavSucceededKeepsPaused(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p2", status: "paused", navError: &navFailure{URL: "https://bad.invalid/"}}
	a.mu.Lock()
	a.markNavSucceededLocked(p)
	a.mu.Unlock()
	if p.status != "paused" {
		t.Fatalf("paused should stay paused, got %q", p.status)
	}
	if p.navError != nil {
		t.Fatal("a successful load clears navError even on a paused page")
	}
}

func TestMarkNavSucceededClosedNoop(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "error", closed: true, navError: &navFailure{URL: "https://bad.invalid/"}}
	a.mu.Lock()
	a.markNavSucceededLocked(p)
	a.mu.Unlock()
	if p.navError == nil || p.status != "error" {
		t.Fatalf("closed page mutated: %+v", p)
	}
}

func TestMarkNavSucceededKeepsFailedAttemptError(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "loading", url: "https://bad.invalid/"}
	a.mu.Lock()
	// Linux/WebKitGTK: a failed attempt commits, fails, then still emits
	// WEBKIT_LOAD_FINISHED. The finish of a failed attempt must NOT clear it.
	a.markNavCommittedLocked(p, "https://bad.invalid/")
	a.markNavFailedLocked(p, &mygo.LoadError{URL: "https://bad.invalid/", Code: 13})
	a.markNavSucceededLocked(p)
	a.mu.Unlock()
	if p.navError == nil || p.status != "error" {
		t.Fatalf("finish of a failed attempt must keep the error: status=%q navError=%+v", p.status, p.navError)
	}
}

func TestMarkNavCommittedResetsFailedFlag(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "error", url: "https://bad.invalid/",
		navFailed: true, navError: &navFailure{URL: "https://bad.invalid/", Name: "NAME_NOT_RESOLVED"}}
	a.mu.Lock()
	// A same-URL commit early-returns (keeps navError) but must still reset
	// navFailed so the retry's successful finish can recover the page.
	a.markNavCommittedLocked(p, "https://bad.invalid/")
	a.mu.Unlock()
	if p.navFailed {
		t.Fatal("a new commit must reset navFailed even on the same-URL early return")
	}
	if p.navError == nil {
		t.Fatal("same-URL commit keeps the recorded failure until the load succeeds")
	}
	a.mu.Lock()
	a.markNavSucceededLocked(p)
	a.mu.Unlock()
	if p.navError != nil || p.status != "ready" {
		t.Fatalf("retry after reset should recover: status=%q navError=%+v", p.status, p.navError)
	}
}

func TestNavigatePageFragmentOnlyPreservesState(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "ready", url: "https://x.example/page", pendingNav: "https://stale.example/", navPendingAt: time.Now()}
	a.mu.Lock()
	a.pages = map[string]*pageRecord{"p1": p}
	a.mu.Unlock()
	// view is nil in this unit environment; navigatePage reports unknown page
	// after the lock-held state update. A fragment-only move must not reset a
	// ready page to loading (no new-document callbacks restore it).
	_ = a.navigatePage("p1", "https://x.example/page#section")
	if p.status != "ready" {
		t.Fatalf("fragment-only move must preserve ready, got %q", p.status)
	}
	if p.pendingNav != "" || !p.navPendingAt.IsZero() {
		t.Fatalf("explicit move still discards deferred follow: pendingNav=%q", p.pendingNav)
	}
	// A different-document move does reset to loading.
	_ = a.navigatePage("p1", "https://x.example/other")
	if p.status != "loading" {
		t.Fatalf("new-document move should reset to loading, got %q", p.status)
	}
}

func TestNSErrName(t *testing.T) {
	cases := map[int]string{
		-1003: "NAME_NOT_RESOLVED",
		-1006: "NAME_NOT_RESOLVED",
		-1001: "TIMEOUT",
		-1004: "CANNOT_CONNECT",
		-1200: "CERT_INVALID",
		-1202: "CERT_INVALID",
		-999:  "OPERATION_CANCELED",
		-4242: "NS_ERROR_-4242",
	}
	for code, want := range cases {
		if got := nsErrorName(code); got != want {
			t.Errorf("nsErrorName(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestWebkitErrorName(t *testing.T) {
	cases := map[int]string{
		300:  "LOAD_FAILED",
		301:  "CONNECTION_ABORTED",
		303:  "OPERATION_CANCELED",
		102:  "CANNOT_SHOW_URI",
		9999: "WEBKIT_ERROR_9999",
	}
	for code, want := range cases {
		if got := webkitErrorName(code); got != want {
			t.Errorf("webkitErrorName(%d) = %q, want %q", code, got, want)
		}
	}
}

func TestSameDocumentURL(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://x.example/page", "https://x.example/page#sec", true},
		{"https://x.example/page#a", "https://x.example/page#b", true},
		{"https://x.example/page", "https://x.example/page", true},
		{"https://x.example/page", "https://x.example/other#sec", false},
		{"https://x.example/page", "https://y.example/page#sec", false},
		{"https://x.example/page?q=1", "https://x.example/page?q=2#sec", false},
		{"", "https://x.example/page", false},
	}
	for _, c := range cases {
		if got := sameDocumentURL(c.a, c.b); got != c.want {
			t.Errorf("sameDocumentURL(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestSameFollowURL(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"https://x.com/s?lang=2&page=2", "https://x.com/s?lang=2&page=2", true},
		// Pair order differs between a mapped target and the site-reported URL.
		{"https://x.com/s?lang=2&page=2", "https://x.com/s?page=2&lang=2", true},
		{"https://X.com/s", "https://x.com/s", true},
		{"https://x.com/s?lang=2", "https://x.com/s?lang=0", false},
		{"https://x.com/s?lang=2", "https://x.com/s", false},
		{"https://x.com/s", "https://x.com/t", false},
		{"https://x.com/s#top", "https://x.com/s#bottom", false},
		{"https://x.com/s#top", "https://x.com/s", false},
		// Value order within one key is significant.
		{"https://x.com/?t=a&t=b", "https://x.com/?t=b&t=a", false},
		// Unparseable queries only match verbatim.
		{"https://x.com/s?q=100%", "https://x.com/s?q=100%", true},
		{"https://x.com/s?q=100%&lang=2", "https://x.com/s?lang=2&q=100%", false},
		{"", "https://x.com", false},
	}
	for _, c := range cases {
		if got := sameFollowURL(c.a, c.b); got != c.want {
			t.Errorf("sameFollowURL(%q,%q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestResetNavLocked(t *testing.T) {
	p := &pageRecord{
		status: "paused", navError: &navFailure{URL: "https://bad.invalid/"}, navSeq: 4,
		navFailed: true, lastError: &syncError{Code: "NOT_FOUND"},
	}
	resetNavLocked(p)
	if p.status != "loading" || p.navError != nil || p.navFailed || p.lastError != nil || p.navSeq != 5 {
		t.Fatalf("reset left %+v", p)
	}
}

func TestNavigatePageClearsDeferredFollow(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", status: "paused", pendingNav: "https://stale.example/", navPendingAt: time.Now()}
	a.mu.Lock()
	a.pages = map[string]*pageRecord{"p1": p}
	a.mu.Unlock()
	// view is nil in this unit environment, so navigatePage reports unknown page
	// after performing the lock-held reset; assert that reset discarded the
	// deferred follow destination which would otherwise undo the explicit move.
	_ = a.navigatePage("p1", "https://good.example/")
	if p.pendingNav != "" || !p.navPendingAt.IsZero() {
		t.Fatalf("explicit navigate must discard deferred follow: pendingNav=%q navPendingAt=%v", p.pendingNav, p.navPendingAt)
	}
	if p.status != "loading" {
		t.Fatalf("explicit navigate lifts pause, got %q", p.status)
	}
}

func TestManualNavWaitsForLoadFinish(t *testing.T) {
	a := newApp()
	p := &pageRecord{url: "https://old.example/", manualNav: true, manualNavFrom: "https://old.example/"}
	p.url = "https://old.example/"
	a.finishManualNavLocked(p)
	if !p.manualNav {
		t.Fatal("stale commit cleared the pending manual navigation")
	}
	p.url = "https://new.example/"
	a.finishManualNavLocked(p)
	if p.manualNav || p.manualNavFrom != "" {
		t.Fatalf("finished manual navigation was not cleared: %+v", p)
	}
	p.manualNav = true
	p.manualNavFrom = "https://old.example/"
	a.clearManualNavLocked(p)
	if p.manualNav {
		t.Fatal("failed or superseded manual navigation was not cleared")
	}
}

func TestSnapshotIncludesNavError(t *testing.T) {
	a := newApp()
	a.mu.Lock()
	a.status = "running"
	a.pages = map[string]*pageRecord{
		"p1": {id: "p1", name: "Primary", url: "https://bad.invalid/", status: "error", isPrimary: true,
			navError: &navFailure{URL: "https://bad.invalid/", Code: 13, Name: "NAME_NOT_RESOLVED"}},
	}
	a.order = []string{"p1"}
	a.mu.Unlock()
	st := a.snapshot()
	if len(st.Pages) != 1 || st.Pages[0].NavError == nil || st.Pages[0].NavError.Name != "NAME_NOT_RESOLVED" {
		t.Fatalf("snapshot dropped navError: %+v", st.Pages)
	}
}

func TestMCPStatusNavErrorContract(t *testing.T) {
	a := newApp()
	a.mu.Lock()
	a.status = "running"
	a.pages = map[string]*pageRecord{
		"p1": {id: "p1", name: "Primary", url: "https://bad.invalid/", status: "error", isPrimary: true,
			navError: &navFailure{URL: "https://bad.invalid/", Code: 13, Name: "NAME_NOT_RESOLVED", Description: "navigation failed"}},
	}
	a.order = []string{"p1"}
	a.primaryID = "p1"
	a.mu.Unlock()

	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "peapod_status"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var st struct {
		Pages []struct {
			Status   string `json:"status"`
			NavError *struct {
				URL  string `json:"url"`
				Code int    `json:"code"`
				Name string `json:"name"`
			} `json:"navError"`
		} `json:"pages"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &st); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(st.Pages) != 1 || st.Pages[0].Status != "error" {
		t.Fatalf("page status = %+v", st.Pages)
	}
	ne := st.Pages[0].NavError
	if ne == nil || ne.Name != "NAME_NOT_RESOLVED" || ne.Code != 13 || ne.URL != "https://bad.invalid/" {
		t.Fatalf("navError contract broken: %+v", ne)
	}
}

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
	if p.navError == nil || p.navError.Name != "NAME_NOT_RESOLVED" || p.navError.Code != 13 {
		t.Fatalf("navError = %+v", p.navError)
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
	p := &pageRecord{id: "p1", status: "error", navError: &navFailure{URL: "https://bad.invalid/", Name: "NAME_NOT_RESOLVED"}}
	a.mu.Lock()
	a.markNavCommittedLocked(p, "https://good.example/")
	a.mu.Unlock()
	if p.navError != nil {
		t.Fatalf("navError should clear on a new document, got %+v", p.navError)
	}
	if p.status != "loading" {
		t.Fatalf("status = %q, want loading", p.status)
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

func TestResetNavLocked(t *testing.T) {
	p := &pageRecord{
		status: "paused", navError: &navFailure{URL: "https://bad.invalid/"},
		lastError: &syncError{Code: "NOT_FOUND"},
	}
	resetNavLocked(p)
	if p.status != "loading" || p.navError != nil || p.lastError != nil {
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

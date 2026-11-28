package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestValidateOperation(t *testing.T) {
	cssAnchor := anchor{Type: "css", Selector: "#btn"}
	cases := []struct {
		name    string
		op      operation
		wantErr bool
	}{
		{"click", operation{Type: "click", Anchor: cssAnchor}, false},
		{"input with value", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`"hi"`)}, false},
		{"empty value is kept", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`""`)}, false},
		{"unknown type", operation{Type: "hover", Anchor: cssAnchor}, true},
		{"unknown anchor type", operation{Type: "click", Anchor: anchor{Type: "xpath", Selector: "//x"}}, true},
		{"empty selector", operation{Type: "click", Anchor: anchor{Type: "css"}}, true},
		{"selector at limit", operation{Type: "click", Anchor: anchor{Type: "css", Selector: strings.Repeat("a", 2048)}}, false},
		{"selector over limit", operation{Type: "click", Anchor: anchor{Type: "css", Selector: strings.Repeat("a", 2049)}}, true},
		{"value at limit", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`"` + strings.Repeat("a", 9998) + `"`)}, false},
		{"value over limit", operation{Type: "input", Anchor: cssAnchor, Value: json.RawMessage(`"` + strings.Repeat("a", 9999) + `"`)}, true},
	}
	for _, c := range cases {
		if err := validateOperation(&c.op); (err != nil) != c.wantErr {
			t.Errorf("%s: validateOperation error = %v, wantErr %v", c.name, err, c.wantErr)
		}
	}
}

func TestErrorCode(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{fmt.Errorf("ANCHOR_TIMEOUT: context deadline exceeded"), "ANCHOR_TIMEOUT"},
		{fmt.Errorf("INVALID_LOCATOR"), "INVALID_LOCATOR"},
		{fmt.Errorf("AMBIGUOUS"), "AMBIGUOUS"},
		{fmt.Errorf("NOT_FOUND"), "NOT_FOUND"},
		{fmt.Errorf("SENSITIVE"), "SENSITIVE"},
		{fmt.Errorf("something else"), "REPLAY_FAILED"},
	}
	for _, c := range cases {
		if got := errorCode(c.err); got != c.want {
			t.Errorf("errorCode(%v) = %q, want %q", c.err, got, c.want)
		}
	}
}

func TestReconcileNavMarksNavigationStarted(t *testing.T) {
	a := newApp()
	p := &pageRecord{
		id: "p2", url: "https://staging.example.com/", status: "ready", syncOn: true,
		pendingNav: "https://staging.example.com/cart", navPendingAt: time.Now().Add(-2 * navFollowGrace),
		queue: newOpQueue(0), navSeq: 7,
	}
	a.mu.Lock()
	nav := a.reconcileNavLocked(p, false)
	a.mu.Unlock()
	if nav != "https://staging.example.com/cart" {
		t.Fatalf("navigation = %q", nav)
	}
	if p.navSeq != 8 || p.status != "loading" || p.pendingNav != "" {
		t.Fatalf("navigation start not marked: %+v", p)
	}
}

// TestReplayScriptSensitiveGuard locks in that replay refuses password/file
// targets, mirroring the capture filter, so a host-injected operation cannot
// type into them on a follower.
func TestReplayScriptSensitiveGuard(t *testing.T) {
	s := replayScript(`{"type":"input","anchor":{"type":"id","selector":"#pw"},"value":"x"}`)
	for _, want := range []string{"password", "file", "SENSITIVE"} {
		if !strings.Contains(s, want) {
			t.Errorf("replayScript missing %q guard", want)
		}
	}
}

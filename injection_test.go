package main

import (
	"strings"
	"testing"
	"time"
)

func TestInjectedPreloadEmpty(t *testing.T) {
	if got := injectedPreload("p1", " \n\t"); got != "" {
		t.Fatalf("empty script produced preload: %q", got)
	}
}

func TestInjectedPreloadBridgeAndScript(t *testing.T) {
	got := injectedPreload("p1", "document.body.dataset.qa = 'ready';")
	for _, want := range []string{
		`const PAGE_ID = "p1"`,
		`peapod.emit = function`,
		`Sync.InjectEvent`,
		`Sync.RunInjectedScript`,
		`new Function`,
		`Array.isArray(item) && item.length > 1000`,
		`document.body.dataset.qa = 'ready'`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("preload missing %q", want)
		}
	}
}

func TestInjectedPreloadCompilesUserScriptSeparately(t *testing.T) {
	got := injectedPreload("p1", "if (")
	for _, want := range []string{`const INJECT_SOURCE = "if ("`, `new Function`, `Sync.RunInjectedScript`, `[Peapod inject]`} {
		if !strings.Contains(got, want) {
			t.Errorf("preload missing %q", want)
		}
	}
	if strings.Contains(got, "\nif (\n") {
		t.Fatal("user script was embedded as executable preload source")
	}
}

func TestInjectedEventRateLimitRefills(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1"}
	now := time.Unix(100, 0)
	for i := 0; i < maxInjectedEventBurst; i++ {
		if !a.allowInjectedEventLocked(p, now) {
			t.Fatalf("burst event %d rejected", i)
		}
	}
	if a.allowInjectedEventLocked(p, now) {
		t.Fatal("event above burst limit accepted")
	}
	if !a.allowInjectedEventLocked(p, now.Add(50*time.Millisecond)) {
		t.Fatal("refilled event rejected")
	}
}

func TestInjectedEventRingIsBoundedAndSnapshotsAreCopies(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1"}
	for i := 0; i < maxInjectedEvents+5; i++ {
		a.recordInjectedEventLocked(p, injectedEvent{Name: "event", Payload: string(rune('a' + i%26))})
	}
	if len(p.injectEvents) != maxInjectedEvents {
		t.Fatalf("events = %d, want %d", len(p.injectEvents), maxInjectedEvents)
	}
	snapshot := a.injectedEventsSnapshotLocked(p)
	snapshot[0].Name = "changed"
	if p.injectEvents[0].Name == "changed" {
		t.Fatal("diagnostics snapshot aliases the live event ring")
	}
}

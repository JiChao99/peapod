package main

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMapNavigation(t *testing.T) {
	cases := []struct {
		primaryBase, followerBase, raw, want string
	}{
		{"https://a.com", "https://b.com", "https://a.com/step2?x=1#top", "https://b.com/step2?x=1#top"},
		{"https://a.com", "https://b.com", "https://a.com", "https://b.com"},
		{"https://a.com", "https://b.com", "https://a.com?q=1", "https://b.com?q=1"},
		{"https://a.com", "https://b.com", "https://a.com#frag", "https://b.com#frag"},
		{"http://a.com:8080", "http://b.com:9090", "http://a.com:8080/x", "http://b.com:9090/x"},
		// Same host, different path prefix (template-style bases).
		{"https://x.com/en", "https://x.com/de", "https://x.com/en/shop?x=1", "https://x.com/de/shop?x=1"},
		{"https://x.com/en", "https://x.com/de", "https://x.com/en", "https://x.com/de"},
		// Outside the primary's variant space: no follow.
		{"https://a.com", "https://b.com", "https://elsewhere.com/x", ""},
		{"https://a.com", "https://b.com", "https://a.com.evil.com/x", ""},
		{"https://x.com/en", "https://x.com/de", "https://x.com/english/x", ""},
		{"", "https://b.com", "https://a.com/x", ""},
	}
	for _, c := range cases {
		if got := mapNavigation(c.primaryBase, c.followerBase, c.raw); got != c.want {
			t.Errorf("mapNavigation(%q, %q, %q) = %q, want %q",
				c.primaryBase, c.followerBase, c.raw, got, c.want)
		}
	}
}

func followFixture() (*app, *pageRecord) {
	a := newApp()
	a.status = "running"
	primary := &pageRecord{id: "p1", base: "https://a.com", url: "https://a.com/step2", isPrimary: true}
	f1 := &pageRecord{id: "p2", base: "https://b.com", url: "https://b.com/step1", syncOn: true, status: "ready", queue: make(chan operation, 8)}
	f2 := &pageRecord{id: "p3", base: "https://c.com", url: "https://c.com/step1", syncOn: true, status: "ready", queue: make(chan operation, 8)}
	a.pages = map[string]*pageRecord{"p1": primary, "p2": f1, "p3": f2}
	a.order = []string{"p1", "p2", "p3"}
	a.primaryID = "p1"
	return a, primary
}

func TestFollowTargets(t *testing.T) {
	a, primary := followFixture()
	targets := a.followTargets(primary, "https://a.com/step2")
	if len(targets) != 2 {
		t.Fatalf("got %d targets, want 2", len(targets))
	}
	if targets[0].url != "https://b.com/step2" || targets[1].url != "https://c.com/step2" {
		t.Errorf("targets = %q, %q", targets[0].url, targets[1].url)
	}
}

func TestFollowTargetsSkips(t *testing.T) {
	cases := []struct {
		name  string
		setup func(a *app)
	}{
		{"not running", func(a *app) { a.status = "starting" }},
		{"sync off", func(a *app) { a.pages["p2"].syncOn = false; a.pages["p3"].syncOn = false }},
		{"paused", func(a *app) { a.pages["p2"].status = "paused"; a.pages["p3"].status = "paused" }},
		{"closed", func(a *app) { a.pages["p2"].closed = true; a.pages["p3"].closed = true }},
		{"already at target", func(a *app) {
			a.pages["p2"].url = "https://b.com/step2"
			a.pages["p3"].url = "https://c.com/step2"
		}},
		{"replay queued", func(a *app) {
			a.pages["p2"].queue <- operation{Type: "click"}
			a.pages["p3"].queue <- operation{Type: "click"}
		}},
		{"replay may navigate", func(a *app) {
			a.pages["p2"].navPendingAt = time.Now()
			a.pages["p3"].navPendingAt = time.Now()
		}},
		{"outside variant space", func(a *app) {
			a.pages["p2"].base = "https://b.com"
			a.pages["p3"].base = "https://c.com"
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			a, primary := followFixture()
			c.setup(a)
			raw := "https://a.com/step2"
			if c.name == "outside variant space" {
				raw = "https://elsewhere.com/step2"
			}
			if targets := a.followTargets(primary, raw); len(targets) != 0 {
				t.Errorf("got %d targets, want 0", len(targets))
			}
		})
	}
}

func TestFollowTargetsMixed(t *testing.T) {
	a, primary := followFixture()
	a.pages["p2"].syncOn = false
	a.pages["p3"].navPendingAt = time.Now().Add(-2 * navFollowGrace)
	targets := a.followTargets(primary, "https://a.com/step2")
	if len(targets) != 1 || targets[0].url != "https://c.com/step2" {
		t.Fatalf("got %+v, want only p3", targets)
	}
}

func TestEnqueueOpMarksNavPending(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p2", syncOn: true, status: "ready", queue: make(chan operation, 8)}
	a.enqueueOp(p, operation{Type: "scroll", Anchor: anchor{Type: "css", Selector: "body"}})
	if !p.navPendingAt.IsZero() {
		t.Error("scroll must not mark nav pending")
	}
	a.enqueueOp(p, operation{Type: "click", Anchor: anchor{Type: "id", Selector: "next"}, Value: json.RawMessage(`""`)})
	if p.navPendingAt.IsZero() {
		t.Error("click must mark nav pending")
	}
}

func TestBuildRequestSetsBases(t *testing.T) {
	a := newApp()
	a.configs[0].url = "https://a.com/start"
	a.configs[1].url = "http://b.com:8080/start"
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if req.pages[0].base != "https://a.com" {
		t.Errorf("primary base = %q", req.pages[0].base)
	}
	if req.pages[1].base != "http://b.com:8080" {
		t.Errorf("follower base = %q", req.pages[1].base)
	}
	if req.pages[2].base != "https://a.com" {
		t.Errorf("inheriting follower base = %q", req.pages[2].base)
	}
}
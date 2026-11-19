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
	f1 := &pageRecord{id: "p2", base: "https://b.com", url: "https://b.com/step1", syncOn: true, status: "ready", queue: newOpQueue(0)}
	f2 := &pageRecord{id: "p3", base: "https://c.com", url: "https://c.com/step1", syncOn: true, status: "ready", queue: newOpQueue(0)}
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
			a.pages["p2"].queue.push(operation{Type: "click"})
			a.pages["p3"].queue.push(operation{Type: "click"})
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
	p := &pageRecord{id: "p2", syncOn: true, status: "ready", queue: newOpQueue(0)}
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

func TestOriginOfNormalizes(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://EXAMPLE.com:443/start", "https://example.com"},
		{"http://Example.COM:80/x", "http://example.com"},
		{"https://Example.com:8443/x", "https://example.com:8443"},
		{"http://[::1]:8099/x", "http://[::1]:8099"},
		{"https://a.com/start", "https://a.com"},
	}
	for _, c := range cases {
		if got := originOf(c.in); got != c.want {
			t.Errorf("originOf(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFirstPathSeg(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://x.com/en-us/docs/intro", "en-us"},
		{"https://x.com/zh-cn/", "zh-cn"},
		{"https://x.com/", ""},
		{"https://x.com", ""},
		{"https://x.com/en-us?q=1", "en-us"},
		{"https://x.com/a/b/c", "a"},
	}
	for _, c := range cases {
		if got := firstPathSeg(c.in); got != c.want {
			t.Errorf("firstPathSeg(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestInferPlainBases(t *testing.T) {
	cases := []struct {
		name string
		urls []string
		want []string
	}{
		{
			// Same-origin path variants: the divergent first segment becomes the base.
			"locale variants",
			[]string{"https://x.com/en-us/docs", "https://x.com/zh-cn/docs", "https://x.com/ja-jp/docs"},
			[]string{"https://x.com/en-us", "https://x.com/zh-cn", "https://x.com/ja-jp"},
		},
		{
			// All pages mirror one URL: origin-only base, full mirroring preserved.
			"identical urls",
			[]string{"https://x.com/docs/intro", "https://x.com/docs/intro"},
			[]string{"https://x.com", "https://x.com"},
		},
		{
			// Same first segment across pages: still origin-only (no variant to swap).
			"shared first segment",
			[]string{"https://x.com/docs/a", "https://x.com/docs/b"},
			[]string{"https://x.com", "https://x.com"},
		},
		{
			// Different hosts keep origin bases (host swap already preserves path).
			"different hosts",
			[]string{"https://a.com/en-us/p", "https://b.com/zh-cn/p"},
			[]string{"https://a.com", "https://b.com"},
		},
		{
			// A root page among variants has no prefix of its own.
			"root among variants",
			[]string{"https://x.com/en-us/p", "https://x.com/zh-cn/p", "https://x.com/"},
			[]string{"https://x.com/en-us", "https://x.com/zh-cn", "https://x.com"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := inferPlainBases(c.urls)
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("base[%d] = %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestBuildRequestPreservesPathVariants(t *testing.T) {
	a := newApp()
	a.configs[0].url = "https://x.com/en-us/docs"
	a.configs[1].url = "https://x.com/zh-cn/docs"
	a.configs[2].url = "https://x.com/ja-jp/docs"
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	want := []string{"https://x.com/en-us", "https://x.com/zh-cn", "https://x.com/ja-jp"}
	for i, w := range want {
		if req.pages[i].base != w {
			t.Errorf("page %d base = %q, want %q", i, req.pages[i].base, w)
		}
	}
	// A primary move within its variant maps each follower onto its OWN variant,
	// never copying the primary's en-us path.
	got := mapNavigation(req.pages[0].base, req.pages[1].base, "https://x.com/en-us/products?q=1")
	if got != "https://x.com/zh-cn/products?q=1" {
		t.Errorf("follower mapped to %q, want its own zh-cn variant", got)
	}
}

func TestFollowKeepsFollowerSelfNavigation(t *testing.T) {
	// The follower replays the click and navigates itself to its own variant.
	// Navigation follow's saved destination must match that self-navigation so it
	// clears instead of clobbering the follower back to the primary's variant.
	a := newApp()
	a.status = "running"
	primary := &pageRecord{id: "p1", base: "https://x.com/en-us", url: "https://x.com/en-us/docs", isPrimary: true}
	f := &pageRecord{id: "p2", base: "https://x.com/zh-cn", url: "https://x.com/zh-cn/docs", syncOn: true, status: "ready", queue: newOpQueue(0)}
	a.pages = map[string]*pageRecord{"p1": primary, "p2": f}
	a.order = []string{"p1", "p2"}
	a.primaryID = "p1"

	// Primary clicks a link; the follower is busy replaying it, so follow defers.
	f.navPendingAt = time.Now()
	a.followTargets(primary, "https://x.com/en-us/products")
	if f.pendingNav != "https://x.com/zh-cn/products" {
		t.Fatalf("pendingNav = %q, want the follower's own variant", f.pendingNav)
	}
	// The follower's own replay commits its variant URL: the deferred target equals
	// it, so nothing is left to override the self-navigation.
	f.navPendingAt = time.Time{}
	f.url = "https://x.com/zh-cn/products"
	if nav := a.reconcileNavLocked(f, false); nav != "" {
		t.Errorf("self-navigation must not be clobbered, reconcile returned %q", nav)
	}
}

func TestFollowTargetsRemembersPendingNav(t *testing.T) {
	a, primary := followFixture()
	f := a.pages["p2"]
	f.replaying = true
	targets := a.followTargets(primary, "https://a.com/step2")
	if len(targets) != 1 || targets[0].url != "https://c.com/step2" {
		t.Fatalf("busy follower must be skipped, got %+v", targets)
	}
	if f.pendingNav != "https://b.com/step2" {
		t.Errorf("pendingNav = %q, want remembered destination", f.pendingNav)
	}
}

func TestFollowTargetsClearsPendingNavAtTarget(t *testing.T) {
	a, primary := followFixture()
	f := a.pages["p2"]
	f.pendingNav = "https://b.com/step2"
	f.url = "https://b.com/step2"
	a.followTargets(primary, "https://a.com/step2")
	if f.pendingNav != "" {
		t.Errorf("pendingNav should clear once at target, got %q", f.pendingNav)
	}
}

func TestFinishReplayReconciles(t *testing.T) {
	a, _ := followFixture()
	f := a.pages["p2"]

	// Non-navigating replay reconciles a skipped destination immediately.
	f.replaying = true
	f.pendingNav = "https://b.com/step2"
	a.finishReplay(f, operation{Type: "input"})
	if f.replaying {
		t.Error("replaying flag not cleared")
	}
	if f.pendingNav != "" {
		t.Errorf("input replay should reconcile, pendingNav = %q", f.pendingNav)
	}

	// A navigating replay within the grace window defers to its own navigation.
	f.replaying = true
	f.pendingNav = "https://b.com/step3"
	f.navPendingAt = time.Now()
	a.finishReplay(f, operation{Type: "click"})
	if f.pendingNav != "https://b.com/step3" {
		t.Errorf("click within grace should defer, pendingNav = %q", f.pendingNav)
	}

	// Once the grace expires the skipped destination is reconciled.
	f.navPendingAt = time.Now().Add(-2 * navFollowGrace)
	a.finishReplay(f, operation{Type: "click"})
	if f.pendingNav != "" {
		t.Errorf("click after grace should reconcile, pendingNav = %q", f.pendingNav)
	}
}

func TestFollowTargetsClearsPendingNavWhenUnmappable(t *testing.T) {
	a, primary := followFixture()
	f := a.pages["p2"]
	f.pendingNav = "https://b.com/step2"
	a.followTargets(primary, "https://elsewhere.com/x")
	if f.pendingNav != "" {
		t.Errorf("unmappable primary URL must clear pendingNav, got %q", f.pendingNav)
	}
}

func TestFollowTargetsRetainsPendingNavWhenBusyAtTarget(t *testing.T) {
	a, primary := followFixture()
	f := a.pages["p2"]
	f.replaying = true
	f.url = "https://b.com/step1"
	f.pendingNav = "https://b.com/step2"
	// Primary goes Back to /step1, mapping onto the follower's current URL while
	// its replay is still in flight: the latest target must be retained.
	a.followTargets(primary, "https://a.com/step1")
	if f.pendingNav != "https://b.com/step1" {
		t.Errorf("busy follower must retain the latest target, got %q", f.pendingNav)
	}
}

func TestFollowTargetsEnqueuesNavBarrier(t *testing.T) {
	a, primary := followFixture()
	f := a.pages["p2"]
	f.replaying = true
	a.followTargets(primary, "https://a.com/step2")
	if f.pendingNav != "https://b.com/step2" {
		t.Fatalf("pendingNav = %q, want remembered destination", f.pendingNav)
	}
	if op, ok := f.queue.pop(); !ok || op.Type != navBarrier {
		t.Errorf("queued op = %q (ok=%v), want navBarrier", op.Type, ok)
	}
}

func TestFinishReplayDefersNavigatingInput(t *testing.T) {
	a, _ := followFixture()
	f := a.pages["p2"]
	f.replaying = true
	f.pendingNav = "https://b.com/step2"
	f.navPendingAt = time.Now()
	// input/change/select may navigate too, so they get the same grace as click.
	a.finishReplay(f, operation{Type: "input"})
	if f.pendingNav != "https://b.com/step2" {
		t.Errorf("input within grace must defer, pendingNav = %q", f.pendingNav)
	}
}

func TestReconcileNavLockedEligibility(t *testing.T) {
	a, _ := followFixture()
	f := a.pages["p2"]
	f.pendingNav = "https://b.com/step2"

	f.syncOn = false
	if nav := a.reconcileNavLocked(f, false); nav != "" {
		t.Errorf("sync-off follower must not auto-navigate, got %q", nav)
	}
	f.syncOn = true
	f.status = "paused"
	if nav := a.reconcileNavLocked(f, false); nav != "" {
		t.Errorf("paused follower must not auto-navigate, got %q", nav)
	}
	f.status = "ready"
	f.queue.push(operation{Type: "click"})
	if nav := a.reconcileNavLocked(f, false); nav != "" {
		t.Errorf("busy queue must defer reconciliation, got %q", nav)
	}
	f.queue.pop()
	if nav := a.reconcileNavLocked(f, false); nav != "https://b.com/step2" {
		t.Errorf("eligible follower should reconcile, got %q", nav)
	}
	if f.pendingNav != "" {
		t.Errorf("pendingNav should clear after reconcile, got %q", f.pendingNav)
	}
}

func TestApplyNavBarrierReconcilesAfterGrace(t *testing.T) {
	a, _ := followFixture()
	f := a.pages["p2"]
	f.pendingNav = "https://b.com/step2"
	f.navPendingAt = time.Now().Add(-2 * navFollowGrace)
	a.applyNavBarrier(f)
	if f.pendingNav != "" {
		t.Errorf("barrier should reconcile after grace, pendingNav = %q", f.pendingNav)
	}
}

func TestReplayWorkerAppliesBarrierBeforeOps(t *testing.T) {
	a, _ := followFixture()
	f := a.pages["p2"]
	f.pendingNav = "https://b.com/step2"
	f.navPendingAt = time.Now().Add(-2 * navFollowGrace)
	f.queue.push(operation{Type: navBarrier})
	f.queue.push(operation{Type: "input", Anchor: anchor{Type: "id", Selector: "x"}})
	f.queue.close()
	a.replayWorker(f)
	if f.pendingNav != "" {
		t.Errorf("barrier must reconcile before queued ops, pendingNav = %q", f.pendingNav)
	}
}

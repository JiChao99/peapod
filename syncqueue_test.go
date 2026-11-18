package main

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func inputOp(selector, value string) operation {
	return operation{Type: "input", Anchor: anchor{Type: "id", Selector: selector}, Value: json.RawMessage(`"` + value + `"`)}
}

func scrollOp(selector string, y float64) operation {
	return operation{Type: "scroll", Anchor: anchor{Type: "css", Selector: selector}, ScrollY: &y}
}

func TestOpQueueFIFO(t *testing.T) {
	q := newOpQueue(0)
	ops := []operation{
		{Type: "click", Anchor: anchor{Type: "id", Selector: "a"}},
		{Type: "submit", Anchor: anchor{Type: "css", Selector: "form"}},
		{Type: "click", Anchor: anchor{Type: "id", Selector: "b"}},
	}
	for _, op := range ops {
		if !q.push(op) {
			t.Fatalf("push %s failed", op.Type)
		}
	}
	if q.len() != 3 {
		t.Fatalf("len = %d, want 3", q.len())
	}
	for i, want := range ops {
		got, ok := q.pop()
		if !ok || got.Type != want.Type || got.Anchor != want.Anchor {
			t.Fatalf("pop %d = %+v (ok=%v), want %+v", i, got, ok, want)
		}
	}
}

func TestOpQueueCoalescesInput(t *testing.T) {
	q := newOpQueue(0)
	q.push(inputOp("note", "h"))
	q.push(inputOp("note", "he"))
	q.push(inputOp("note", "hel"))
	if q.len() != 1 {
		t.Fatalf("len = %d, want 1 (same-anchor inputs coalesce)", q.len())
	}
	// A different anchor, a different type and a barrier each break the run.
	q.push(inputOp("other", "x"))
	q.push(operation{Type: "change", Anchor: anchor{Type: "id", Selector: "note"}})
	q.push(inputOp("note", "hello"))
	q.push(operation{Type: navBarrier})
	q.push(inputOp("note", "hello!"))
	if q.len() != 6 {
		t.Fatalf("len = %d, want 6 (no coalescing across anchors/types/barrier)", q.len())
	}
	got, _ := q.pop()
	if string(got.Value) != `"hel"` {
		t.Errorf("coalesced value = %s, want the latest before the run broke", got.Value)
	}
}

func TestOpQueueCoalescesScroll(t *testing.T) {
	q := newOpQueue(0)
	q.push(scrollOp("html", 10))
	q.push(scrollOp("html", 200))
	q.push(scrollOp("html", 900))
	if q.len() != 1 {
		t.Fatalf("len = %d, want 1 (same-anchor scrolls coalesce)", q.len())
	}
	got, _ := q.pop()
	if got.ScrollY == nil || *got.ScrollY != 900 {
		t.Errorf("coalesced scrollY = %v, want the final position 900", got.ScrollY)
	}
	// A different scroll anchor starts a new op.
	q.push(scrollOp("html", 1))
	q.push(scrollOp("div.list", 5))
	if q.len() != 2 {
		t.Fatalf("len = %d, want 2 across scroll anchors", q.len())
	}
}

func TestOpQueueCapDrops(t *testing.T) {
	q := newOpQueue(0)
	for i := 0; i < opQueueCap; i++ {
		if !q.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: fmt.Sprintf("b%d", i)}}) {
			t.Fatalf("push %d failed below cap", i)
		}
	}
	if q.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: "overflow"}}) {
		t.Fatal("push at cap must drop")
	}
	// A coalescing replace still succeeds at cap: it does not grow the queue.
	q2 := newOpQueue(0)
	for i := 0; i < opQueueCap-1; i++ {
		q2.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: fmt.Sprintf("b%d", i)}})
	}
	q2.push(inputOp("note", "a"))
	if !q2.push(inputOp("note", "ab")) {
		t.Fatal("coalescing push at cap must succeed")
	}
	if q2.len() != opQueueCap {
		t.Fatalf("len = %d, want cap %d", q2.len(), opQueueCap)
	}
}

func TestOpQueueCloseDrains(t *testing.T) {
	q := newOpQueue(0)
	q.push(operation{Type: "click"})
	q.push(operation{Type: "submit"})
	q.close()
	q.close() // idempotent
	if q.push(operation{Type: "click"}) {
		t.Fatal("push after close must fail")
	}
	for i := 0; i < 2; i++ {
		if _, ok := q.pop(); !ok {
			t.Fatalf("pop %d after close must drain buffered ops", i)
		}
	}
	if _, ok := q.pop(); ok {
		t.Fatal("pop on a closed, drained queue must report false")
	}
}

func TestOpQueuePopWakesOnPush(t *testing.T) {
	q := newOpQueue(0)
	got := make(chan operation, 1)
	go func() {
		op, _ := q.pop()
		got <- op
	}()
	time.Sleep(10 * time.Millisecond)
	q.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: "go"}})
	select {
	case op := <-got:
		if op.Anchor.Selector != "go" {
			t.Fatalf("popped %+v", op)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pop did not wake on push")
	}
}

func TestOpQueuePopWakesOnClose(t *testing.T) {
	q := newOpQueue(0)
	done := make(chan bool, 1)
	go func() {
		_, ok := q.pop()
		done <- ok
	}()
	time.Sleep(10 * time.Millisecond)
	q.close()
	select {
	case ok := <-done:
		if ok {
			t.Fatal("pop on close must report false")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pop did not wake on close")
	}
}

// TestOpQueuePacesInput: with a delay set, the second same-anchor input waits
// out the interval and pops the coalesced latest value, not each keystroke.
func TestOpQueuePacesInput(t *testing.T) {
	const delay = 80 * time.Millisecond
	q := newOpQueue(delay)
	q.push(inputOp("note", "h"))
	if _, ok := q.pop(); !ok {
		t.Fatal("first input must pop immediately")
	}
	q.push(inputOp("note", "he"))
	q.push(inputOp("note", "hel"))
	start := time.Now()
	got, ok := q.pop()
	elapsed := time.Since(start)
	if !ok {
		t.Fatal("paced pop failed")
	}
	if elapsed < delay-20*time.Millisecond {
		t.Errorf("paced pop waited %s, want ~%s", elapsed, delay)
	}
	if string(got.Value) != `"hel"` {
		t.Errorf("paced value = %s, want the latest coalesced value", got.Value)
	}
	if q.len() != 0 {
		t.Errorf("len = %d, want the burst coalesced into one pop", q.len())
	}
}

// TestOpQueuePacesScroll: a scroll burst replays at the configured rate and
// ends at the final position.
func TestOpQueuePacesScroll(t *testing.T) {
	const delay = 80 * time.Millisecond
	q := newOpQueue(delay)
	q.push(scrollOp("html", 10))
	q.pop()
	q.push(scrollOp("html", 300))
	q.push(scrollOp("html", 800))
	start := time.Now()
	got, _ := q.pop()
	if elapsed := time.Since(start); elapsed < delay-20*time.Millisecond {
		t.Errorf("paced scroll waited %s, want ~%s", elapsed, delay)
	}
	if got.ScrollY == nil || *got.ScrollY != 800 {
		t.Errorf("paced scrollY = %v, want final position 800", got.ScrollY)
	}
}

// TestOpQueueDiscreteNotPaced: clicks and barriers pop immediately even right
// after a paced stream op, and do not reset the stream pace clock.
func TestOpQueueDiscreteNotPaced(t *testing.T) {
	const delay = 200 * time.Millisecond
	q := newOpQueue(delay)
	q.push(inputOp("note", "a"))
	q.pop()
	q.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: "go"}})
	q.push(operation{Type: navBarrier})
	start := time.Now()
	if op, _ := q.pop(); op.Type != "click" {
		t.Fatalf("popped %q, want click", op.Type)
	}
	if op, _ := q.pop(); op.Type != navBarrier {
		t.Fatalf("popped %q, want barrier", op.Type)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Errorf("discrete ops waited %s, want immediate", elapsed)
	}
	// The input pace clock is untouched by the discrete pops: a new input
	// right after them still waits out the remaining interval.
	q.push(inputOp("note", "ab"))
	start = time.Now()
	q.pop()
	if elapsed := time.Since(start); elapsed < delay-100*time.Millisecond {
		t.Errorf("input after discrete ops waited %s, want the stream pace kept", elapsed)
	}
}

func TestSyncDelayFromMs(t *testing.T) {
	cases := []struct {
		ms   *float64
		want time.Duration
	}{
		{nil, defaultSyncDelay},
		{ptr(0), 0},
		{ptr(-5), 0},
		{ptr(250), 250 * time.Millisecond},
		{ptr(99999), 2 * time.Second},
	}
	for _, c := range cases {
		if got := syncDelayFromMs(c.ms); got != c.want {
			t.Errorf("syncDelayFromMs(%v) = %s, want %s", c.ms, got, c.want)
		}
	}
}

func TestFormSyncDelay(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"0", 0},
		{"250", 250 * time.Millisecond},
		{"", defaultSyncDelay},
		{"bogus", defaultSyncDelay},
	}
	for _, c := range cases {
		if got := formSyncDelay(c.in); got != c.want {
			t.Errorf("formSyncDelay(%q) = %s, want %s", c.in, got, c.want)
		}
	}
}

func TestBuildRequestSyncDelay(t *testing.T) {
	a := newApp()
	a.configs[0].url = "https://a.com/"
	req, err := a.buildRequest()
	if err != nil {
		t.Fatalf("buildRequest: %v", err)
	}
	if req.syncDelay != defaultSyncDelay {
		t.Errorf("default syncDelay = %s, want %s", req.syncDelay, defaultSyncDelay)
	}
	a.syncDelay = "0"
	if req, err = a.buildRequest(); err != nil || req.syncDelay != 0 {
		t.Errorf("instant syncDelay = %s, %v; want 0", req.syncDelay, err)
	}
}

func TestProfileSyncDelayRoundTrip(t *testing.T) {
	a := newApp()
	a.syncDelay = "250"
	p := a.currentProfile()
	if p.SyncDelay != "250" {
		t.Fatalf("profile syncDelay = %q, want 250", p.SyncDelay)
	}
	b := newApp()
	if err := b.applyProfile(p); err != nil {
		t.Fatalf("applyProfile: %v", err)
	}
	if b.syncDelay != "250" {
		t.Errorf("loaded syncDelay = %q, want 250", b.syncDelay)
	}
	// Hand-edited or pre-feature profiles fall back to the default pace.
	p.SyncDelay = "9999"
	if err := b.applyProfile(p); err != nil {
		t.Fatalf("applyProfile: %v", err)
	}
	if b.syncDelay != "100" {
		t.Errorf("invalid syncDelay = %q, want the 100 default", b.syncDelay)
	}
}

func TestSnapshotReportsSyncDelay(t *testing.T) {
	a := newApp()
	a.mu.Lock()
	a.taskSyncDelay = 250 * time.Millisecond
	a.mu.Unlock()
	if st := a.snapshot(); st.SyncDelayMs != 250 {
		t.Errorf("snapshot syncDelayMs = %v, want 250", st.SyncDelayMs)
	}
}

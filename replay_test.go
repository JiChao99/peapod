package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript lays down a script.jsonl with n click operations.
func writeScript(t *testing.T, n int) string {
	t.Helper()
	var sb strings.Builder
	for i := 0; i < n; i++ {
		line := scriptLine{
			Seq: i + 1, At: time.Now().Format(time.RFC3339Nano), Source: "guest",
			URL: "https://a.com/",
			Operation: operation{
				Type:   "click",
				Anchor: anchor{Type: "id", Selector: fmt.Sprintf("#b%d", i)},
			},
		}
		data, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		sb.Write(data)
		sb.WriteByte('\n')
	}
	path := filepath.Join(t.TempDir(), "script.jsonl")
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadScript(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	good := `{"seq":1,"operation":{"type":"click","anchor":{"type":"id","selector":"#go"}}}` + "\n"

	t.Run("valid with blank lines", func(t *testing.T) {
		ops, err := loadScript(write("ok.jsonl", good+"\n"+good))
		if err != nil || len(ops) != 2 {
			t.Fatalf("ops = %d, err = %v; want 2, nil", len(ops), err)
		}
	})
	t.Run("malformed line names the line", func(t *testing.T) {
		_, err := loadScript(write("bad.jsonl", good+"{oops\n"))
		if err == nil || !strings.Contains(err.Error(), "line 2") {
			t.Fatalf("err = %v, want line 2", err)
		}
	})
	t.Run("invalid operation rejected", func(t *testing.T) {
		_, err := loadScript(write("badop.jsonl", `{"operation":{"type":"nope","anchor":{"type":"id","selector":"#x"}}}`))
		if err == nil || !strings.Contains(err.Error(), "line 1") {
			t.Fatalf("err = %v, want line 1", err)
		}
	})
	t.Run("empty script", func(t *testing.T) {
		if _, err := loadScript(write("empty.jsonl", "\n\n")); err == nil {
			t.Fatal("want error for a script with no operations")
		}
	})
	t.Run("missing file", func(t *testing.T) {
		if _, err := loadScript(filepath.Join(dir, "nope.jsonl")); err == nil {
			t.Fatal("want error for a missing file")
		}
	})
	t.Run("oversized script", func(t *testing.T) {
		path := write("big.jsonl", strings.Repeat("x", maxScriptBytes+1))
		if _, err := loadScript(path); err == nil || !strings.Contains(err.Error(), "exceeds") {
			t.Fatalf("err = %v, want size bound", err)
		}
	})
}

func TestReplayDelayFromMs(t *testing.T) {
	cases := []struct {
		ms   *float64
		want time.Duration
	}{
		{nil, defaultReplayDelay},
		{ptr(0.0), 0},
		{ptr(-5.0), 0},
		{ptr(250.0), 250 * time.Millisecond},
		{ptr(99999.0), 5 * time.Second},
	}
	for _, c := range cases {
		if got := replayDelayFromMs(c.ms); got != c.want {
			t.Errorf("replayDelayFromMs(%v) = %s, want %s", c.ms, got, c.want)
		}
	}
}

func ptr(v float64) *float64 { return &v }

// startFakeWorkers drains each page's queue like the real replay workers;
// replayOnce is a no-op without a view, which is enough for settle logic.
func startFakeWorkers(t *testing.T, a *app) {
	t.Helper()
	a.mu.Lock()
	pages := make([]*pageRecord, 0, len(a.order))
	for _, id := range a.order {
		pages = append(pages, a.pages[id])
	}
	a.mu.Unlock()
	for _, p := range pages {
		go a.replayWorker(p)
	}
	t.Cleanup(func() {
		a.mu.Lock()
		for _, p := range pages {
			if !p.closed {
				p.closed = true
				p.queue.close()
			}
		}
		a.mu.Unlock()
	})
}

func TestReplayDispatchesEveryPage(t *testing.T) {
	a := runningFixture(t)
	path := writeScript(t, 2)
	n, err := a.startReplay(path, 0)
	if err != nil || n != 2 {
		t.Fatalf("startReplay = %d, %v; want 2, nil", n, err)
	}
	waitFor(t, "replay completion", func() bool {
		st := a.snapshot()
		return st.Replay != nil && !st.Replay.Active
	})
	st := a.snapshot()
	if st.Replay.Done != 2 || st.Replay.Total != 2 || st.Replay.Error != "" {
		t.Fatalf("replay state = %+v", st.Replay)
	}
	// Unlike peapod_send_operation, replay drives the primary too.
	for _, id := range []string{"p1", "p2"} {
		a.mu.Lock()
		queued := a.pages[id].queue.len()
		a.mu.Unlock()
		if queued != 2 {
			t.Fatalf("page %s queued %d ops, want 2", id, queued)
		}
	}
}

func TestReplayRejectsNotRunningAndBusy(t *testing.T) {
	a := newApp()
	path := writeScript(t, 1)
	if _, err := a.startReplay(path, 0); err == nil {
		t.Fatal("replay on an idle task must fail")
	}

	a = runningFixture(t)
	long := writeScript(t, 3)
	if _, err := a.startReplay(long, 5*time.Second); err != nil {
		t.Fatalf("first replay: %v", err)
	}
	if _, err := a.startReplay(long, 0); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("second replay err = %v, want already-running", err)
	}
	// Cancel the first replay through its quit channel (as cleanup does).
	a.mu.Lock()
	rp := a.replay
	close(rp.quit)
	a.mu.Unlock()
	waitFor(t, "replay cancellation", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return !rp.active
	})
	if rp.err != "cancelled" {
		t.Fatalf("replay err = %q, want cancelled", rp.err)
	}
}

func TestReplayCancelOnStop(t *testing.T) {
	a := runningFixture(t)
	path := writeScript(t, 20)
	if _, err := a.startReplay(path, 50*time.Millisecond); err != nil {
		t.Fatalf("startReplay: %v", err)
	}
	a.mu.Lock()
	rp := a.replay
	a.mu.Unlock()
	waitFor(t, "first replayed op", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return rp.done >= 1
	})
	if err := a.stopTask(); err != nil {
		t.Fatalf("stopTask: %v", err)
	}
	waitFor(t, "task idle", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.status == "idle"
	})
	waitFor(t, "replay end", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return !rp.active
	})
	if rp.done >= 20 {
		t.Fatalf("replay finished all %d ops despite the stop", rp.done)
	}
	if rp.err != "cancelled" && rp.err != "task stopped" {
		t.Fatalf("replay err = %q, want cancelled/task stopped", rp.err)
	}
	if st := a.snapshot(); st.Replay != nil {
		t.Fatalf("cleanup should clear the replay state, got %+v", st.Replay)
	}
}

func TestReplayRecordsWhenRecording(t *testing.T) {
	a, r := recordFixture(t)
	startFakeWorkers(t, a)
	path := writeScript(t, 1)
	if _, err := a.startReplay(path, 0); err != nil {
		t.Fatalf("startReplay: %v", err)
	}
	waitFor(t, "replay completion", func() bool {
		st := a.snapshot()
		return st.Replay != nil && !st.Replay.Active
	})
	waitForFile(t, filepath.Join(r.dir, "steps", "0001", "Primary.png"))
	stopRecorder(t, a, r)
	lines := readScript(t, r)
	if len(lines) != 1 || lines[0].Source != "replay" {
		t.Fatalf("script = %+v, want one replay-sourced line", lines)
	}
}

// TestReplaySkipsIneligiblePages locks in that replay reports page-level
// exclusions instead of silently claiming success: a sync-off or paused page
// gets no ops, and skipped counts them while done still advances.
func TestReplaySkipsIneligiblePages(t *testing.T) {
	for name, mute := range map[string]func(*pageRecord){
		"sync off": func(p *pageRecord) { p.syncOn = false },
		"paused":   func(p *pageRecord) { p.status = "paused" },
	} {
		t.Run(name, func(t *testing.T) {
			a := runningFixture(t)
			a.mu.Lock()
			mute(a.pages["p2"])
			a.mu.Unlock()
			path := writeScript(t, 2)
			if _, err := a.startReplay(path, 0); err != nil {
				t.Fatalf("startReplay: %v", err)
			}
			waitFor(t, "replay completion", func() bool {
				st := a.snapshot()
				return st.Replay != nil && !st.Replay.Active
			})
			st := a.snapshot()
			if st.Replay.Done != 2 || st.Replay.Total != 2 || st.Replay.Skipped != 2 || st.Replay.Error != "" {
				t.Fatalf("replay state = %+v, want done 2/2 skipped 2", st.Replay)
			}
			a.mu.Lock()
			q1, q2 := a.pages["p1"].queue.len(), a.pages["p2"].queue.len()
			a.mu.Unlock()
			if q1 != 2 || q2 != 0 {
				t.Fatalf("queues = %d/%d, want 2/0 (ineligible page untouched)", q1, q2)
			}
		})
	}
}

// TestReplayCancelWhileQueueFull parks the retrying enqueue on full queues
// (no workers) and proves stopping the task unblocks it promptly.
func TestReplayCancelWhileQueueFull(t *testing.T) {
	a := runningFixture(t)
	a.mu.Lock()
	for _, id := range a.order {
		// Distinct anchors: same-anchor scrolls would coalesce into one op.
		for i := 0; i < opQueueCap; i++ {
			a.pages[id].queue.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: fmt.Sprintf("#fill%d", i)}})
		}
	}
	a.mu.Unlock()
	path := writeScript(t, 2)
	if _, err := a.startReplay(path, 0); err != nil {
		t.Fatalf("startReplay: %v", err)
	}
	a.mu.Lock()
	rp := a.replay
	a.mu.Unlock()
	if err := a.stopTask(); err != nil {
		t.Fatalf("stopTask: %v", err)
	}
	waitFor(t, "replay unblocked by stop", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return !rp.active
	})
	if rp.err != "cancelled" && rp.err != "task stopped" {
		t.Fatalf("replay err = %q, want cancelled/task stopped", rp.err)
	}
}

// TestReplayCoalescesAtCap: a full queue whose tail is a same-anchor scroll
// accepts the replay op via coalescing replace — no capacity drop, no growth,
// and the tail carries the replayed position.
func TestReplayCoalescesAtCap(t *testing.T) {
	a := runningFixture(t)
	p := a.pages["p2"]
	a.mu.Lock()
	for i := 0; i < opQueueCap-1; i++ {
		p.queue.push(operation{Type: "click", Anchor: anchor{Type: "id", Selector: fmt.Sprintf("#fill%d", i)}})
	}
	p.queue.push(scrollOp("html", 10))
	a.mu.Unlock()

	y := 900.0
	if !a.enqueueReplayOp(p, scrollOp("html", y), nil) {
		t.Fatal("coalescing replay push at cap must succeed")
	}
	if got := p.queue.len(); got != opQueueCap {
		t.Fatalf("len = %d, want cap %d (replace must not grow the queue)", got, opQueueCap)
	}
	// Drain to the tail: the last op is the replayed scroll position.
	var last operation
	for i := 0; i < opQueueCap; i++ {
		last, _ = p.queue.pop()
	}
	if last.Type != "scroll" || last.ScrollY == nil || *last.ScrollY != y {
		t.Fatalf("tail = %+v, want the replayed scroll to 900", last)
	}
}

// TestReplayScriptScrollSuppression locks in the M1 regression guard: a
// replayed scroll fires its (trusted) scroll event in a later task, so the
// replay must arm a suppression window that the capture listener honors.
func TestReplayScriptScrollSuppression(t *testing.T) {
	s := replayScript(`{"type":"scroll","anchor":{"type":"css","selector":"html"},"scrollY":10}`)
	if !strings.Contains(s, "__syncReplayScrollUntil") {
		t.Error("replayScript scroll branch must arm the capture suppression window")
	}
	c := capturePreload("p1")
	if !strings.Contains(c, "__syncReplayScrollUntil") {
		t.Error("capture listener must honor the scroll suppression window")
	}
}

// TestCaptureDedupesSharedEvents locks in the single-capture guard: scroll is
// registered on both document and window, so the same event reaches the
// capture listener twice and must be captured — and mirrored — only once.
func TestCaptureDedupesSharedEvents(t *testing.T) {
	c := capturePreload("p1")
	if !strings.Contains(c, "seenEvents.has(event)") {
		t.Error("capture listener must skip an event the other listener already handled")
	}
}

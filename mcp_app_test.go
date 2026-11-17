package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCaptureFlightSuccessAndError(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1"}
	a.mu.Lock()
	f := a.capturePageFlight(p, func() ([]byte, error) { return []byte("png"), nil })
	a.mu.Unlock()
	png, err := waitCapture(f, time.Second)
	if err != nil || string(png) != "png" {
		t.Fatalf("png = %q, err = %v", png, err)
	}

	want := errors.New("capture failed")
	a.mu.Lock()
	f2 := a.capturePageFlight(p, func() ([]byte, error) { return nil, want })
	a.mu.Unlock()
	if f2 == f {
		t.Fatal("a completed flight should free the slot for a new one")
	}
	_, err = waitCapture(f2, time.Second)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestCaptureFlightCoalescesAndBounds(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1"}
	var calls int32
	release := make(chan struct{})
	capture := func() ([]byte, error) {
		atomic.AddInt32(&calls, 1)
		<-release // simulate a CapturePreview callback that never fires
		return []byte("late"), nil
	}
	a.mu.Lock()
	f1 := a.capturePageFlight(p, capture)
	f2 := a.capturePageFlight(p, capture) // a retry while one is outstanding
	a.mu.Unlock()
	if f1 != f2 {
		t.Fatal("retry should coalesce onto the outstanding flight, not spawn a new capture")
	}

	// Both waiters time out, but only one uncancellable capture was launched.
	_, err1 := waitCapture(f1, 20*time.Millisecond)
	_, err2 := waitCapture(f2, 20*time.Millisecond)
	var to errCaptureTimeout
	if !errors.As(err1, &to) || !errors.As(err2, &to) {
		t.Fatalf("want errCaptureTimeout, got %v / %v", err1, err2)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("capture ran %d times, want 1 (bounded per page)", got)
	}

	// A late completion (window restored) delivers to waiters and frees the slot.
	close(release)
	png, err := waitCapture(f1, time.Second)
	if err != nil || string(png) != "late" {
		t.Fatalf("late png = %q, err = %v", png, err)
	}
	a.mu.Lock()
	slot := p.capture
	f3 := a.capturePageFlight(p, func() ([]byte, error) { return []byte("next"), nil })
	a.mu.Unlock()
	if slot != nil {
		t.Fatal("flight slot should be cleared after completion")
	}
	if f3 == f1 {
		t.Fatal("a new flight should start after the previous completed")
	}
	png3, err3 := waitCapture(f3, time.Second)
	if err3 != nil || string(png3) != "next" {
		t.Fatalf("next png = %q, err = %v", png3, err3)
	}
}

func TestBatchShotValidLocked(t *testing.T) {
	a := newApp()
	a.mu.Lock()
	a.status = "running"
	a.taskID = "task-1"
	a.pages = map[string]*pageRecord{"p1": {id: "p1", status: "ready"}}
	a.mu.Unlock()

	a.mu.Lock()
	valid := a.batchShotValidLocked("task-1", "p1")
	staleTask := a.batchShotValidLocked("task-2", "p1")
	missing := a.batchShotValidLocked("task-1", "p9")
	a.mu.Unlock()
	if !valid {
		t.Fatal("matching running task with an open page should be valid")
	}
	if staleTask {
		t.Fatal("a reused page ID under a new task must be skipped (cross-task capture)")
	}
	if missing {
		t.Fatal("a missing page should be invalid")
	}

	a.mu.Lock()
	a.status = "idle"
	stopped := a.batchShotValidLocked("task-1", "p1")
	a.mu.Unlock()
	if stopped {
		t.Fatal("a stopped task must not be captured")
	}

	a.mu.Lock()
	a.status = "running"
	a.pages["p1"].closed = true
	closed := a.batchShotValidLocked("task-1", "p1")
	a.mu.Unlock()
	if closed {
		t.Fatal("a closed page must be skipped")
	}
}

func TestCaptureTimeoutFromMs(t *testing.T) {
	cases := []struct {
		ms   float64
		want time.Duration
	}{
		{0, captureTimeout},
		{-5, captureTimeout},
		{500, time.Second},
		{1000, time.Second},
		{2500, 2500 * time.Millisecond},
		{60000, 60 * time.Second},
		{99999, 60 * time.Second},
	}
	for _, c := range cases {
		if got := captureTimeoutFromMs(c.ms); got != c.want {
			t.Errorf("captureTimeoutFromMs(%v) = %s, want %s", c.ms, got, c.want)
		}
	}
}

func TestMCPScreenshotUnknownPage(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_screenshot",
		Arguments: map[string]any{"pageId": "p9", "timeoutMs": 2000},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want error result, got %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "unknown page") {
		t.Fatalf("unexpected error text: %s", resultText(t, res))
	}
}

func TestMCPDiagnosticsUnknownPage(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_diagnostics",
		Arguments: map[string]any{"pageId": "p9"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want error result, got %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "unknown page") {
		t.Fatalf("unexpected error text: %s", resultText(t, res))
	}
}

func TestMCPReplayBadInput(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	path := writeScript(t, 1)
	for _, args := range []map[string]any{
		{},                                   // missing scriptPath
		{"scriptPath": path},                 // valid script, but no running task
		{"scriptPath": "no-such-file.jsonl"}, // unreadable script
	} {
		res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
			Name:      "peapod_replay",
			Arguments: args,
		})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if !res.IsError {
			t.Fatalf("want error result for %v, got %s", args, resultText(t, res))
		}
	}
}

func TestMCPReplayFlow(t *testing.T) {
	a := runningFixture(t)
	cs := connectMCP(t, a)
	path := writeScript(t, 2)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_replay",
		Arguments: map[string]any{"scriptPath": path, "delayMs": 0},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("replay returned error: %s", resultText(t, res))
	}
	var started struct {
		Status string `json:"status"`
		Steps  int    `json:"steps"`
	}
	if err := json.Unmarshal([]byte(resultText(t, res)), &started); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if started.Status != "replaying" || started.Steps != 2 {
		t.Fatalf("started = %+v, want replaying/2", started)
	}

	// Poll peapod_status until the replay reports completion.
	deadline := time.Now().Add(3 * time.Second)
	for {
		sres, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "peapod_status"})
		if err != nil {
			t.Fatalf("status call: %v", err)
		}
		var st appState
		if err := json.Unmarshal([]byte(resultText(t, sres)), &st); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		if st.Replay != nil && !st.Replay.Active {
			if st.Replay.Done != 2 || st.Replay.Total != 2 || st.Replay.Error != "" {
				t.Fatalf("replay state = %+v, want done 2/2 without error", st.Replay)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("replay did not finish within 3s")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

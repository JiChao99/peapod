package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// maxScriptBytes bounds a replay script read (~tens of thousands of operations).
const maxScriptBytes = 10 << 20

// defaultReplayDelay paces replayed operations so navigations and renders can
// land between steps; callers can override it (0 = as fast as the queues allow).
const defaultReplayDelay = 500 * time.Millisecond

// loadScript reads a recorded script.jsonl and validates every line up front,
// so a replay never starts half-parsed. Blank lines are skipped; a malformed
// line fails the whole load with its line number. The read is bounded so a
// huge path cannot be pulled into memory before the size check.
func loadScript(path string) ([]operation, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxScriptBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxScriptBytes {
		return nil, fmt.Errorf("script exceeds %d bytes", maxScriptBytes)
	}
	var ops []operation
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var sl scriptLine
		if err := json.Unmarshal([]byte(line), &sl); err != nil {
			return nil, fmt.Errorf("script line %d: %w", i+1, err)
		}
		op := sl.Operation
		if err := validateOperation(&op); err != nil {
			return nil, fmt.Errorf("script line %d: %w", i+1, err)
		}
		ops = append(ops, op)
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("script contains no operations")
	}
	return ops, nil
}

// replayRun is one in-flight peapod_replay. Fields are guarded by a.mu.
type replayRun struct {
	taskID   string
	total    int
	done     int
	skipped  int
	active   bool
	err      string
	quit     chan struct{}
	quitOnce sync.Once
}

// cancel closes the quit channel at most once, so the panel's Cancel button and
// task cleanup can both call it without racing a double close.
func (rp *replayRun) cancel() { rp.quitOnce.Do(func() { close(rp.quit) }) }

// cancelReplay stops an in-flight replay from the panel. It is a no-op when no
// replay is active; the run goroutine reports the cancellation via replayState.
func (a *app) cancelReplay() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if rp := a.replay; rp != nil && rp.active {
		rp.cancel()
	}
}

// replayState is the peapod_status view of a replay run: poll active until it
// turns false, then read done/total and error. skipped counts page-level
// exclusions (an operation not enqueued because that page was closed, switched
// off or paused); done still advances for those operations.
type replayState struct {
	Active  bool   `json:"active"`
	Done    int    `json:"done"`
	Total   int    `json:"total"`
	Skipped int    `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// replayDelayFromMs clamps a caller-supplied pace into 0–5s; nil = default.
func replayDelayFromMs(ms *float64) time.Duration {
	if ms == nil {
		return defaultReplayDelay
	}
	return time.Duration(min(max(*ms, 0), 5000)) * time.Millisecond
}

// startReplay validates the script, then replays it onto EVERY page of the
// running task — including the primary, so the whole task re-enacts the flow
// and navigation follow keeps stragglers aligned. It returns immediately with
// the step count; progress and the final result appear in peapod_status.replay.
// Stopping the task cancels the replay.
func (a *app) startReplay(path string, delay time.Duration) (int, error) {
	ops, err := loadScript(path)
	if err != nil {
		return 0, err
	}
	a.mu.Lock()
	if a.status != "running" {
		a.mu.Unlock()
		return 0, fmt.Errorf("%s", a.tr("err.replayNotRunning"))
	}
	if a.replay != nil && a.replay.active {
		a.mu.Unlock()
		return 0, fmt.Errorf("a replay is already running")
	}
	rp := &replayRun{taskID: a.taskID, total: len(ops), active: true, quit: make(chan struct{})}
	a.replay = rp
	// Tag the "replaying…" notice here, under the same lock that publishes the
	// run, so a fast run cannot finish before the notice exists (which would
	// leave a stale tagged notice that finishReplayRun already cleared).
	a.notice = a.tr("notice.recReplaying", len(ops))
	a.noticeKind = "info"
	a.noticeTag = "replay"
	a.noticePath = ""
	a.noticePathIsDir = false
	a.mu.Unlock()
	a.pushState()
	go a.runReplay(rp, ops, delay)
	return len(ops), nil
}

func (a *app) runReplay(rp *replayRun, ops []operation, delay time.Duration) {
	for i, op := range ops {
		select {
		case <-rp.quit:
			a.finishReplayRun(rp, "cancelled")
			return
		default:
		}
		a.mu.Lock()
		if a.taskID != rp.taskID || a.status != "running" {
			a.mu.Unlock()
			a.finishReplayRun(rp, "task stopped")
			return
		}
		pages := make([]*pageRecord, 0, len(a.order))
		for _, id := range a.order {
			if p := a.pages[id]; p != nil {
				pages = append(pages, p)
			}
		}
		a.mu.Unlock()
		// Enqueue outside the lock: a full queue is retried, not dropped, so a
		// paced replay never loses steps to capacity. Same-anchor input/scroll
		// ops still coalesce to the latest value/position (see opQueue).
		// Ineligible pages are counted.
		skipped := 0
		for _, p := range pages {
			if !a.enqueueReplayOp(p, op, rp.quit) {
				skipped++
			}
			select {
			case <-rp.quit:
				a.finishReplayRun(rp, "cancelled")
				return
			default:
			}
		}
		a.mu.Lock()
		rp.done = i + 1
		rp.skipped += skipped
		a.recordOpLocked(op, "replay")
		a.mu.Unlock()
		a.pushState()
		if i < len(ops)-1 && delay > 0 {
			select {
			case <-rp.quit:
				a.finishReplayRun(rp, "cancelled")
				return
			case <-time.After(delay):
			}
		}
	}
	a.finishReplayRun(rp, "")
}

// enqueueReplayOp queues one replay operation without capacity drops: while
// the target's queue is full it retries (the worker drains it) instead of
// dropping like the capture path does. Same-anchor input/scroll ops still
// coalesce to the latest, so a burst converges to its final state rather than
// replaying every intermediate value. It returns false when the page is
// ineligible — closed, sync off or paused — or the run was cancelled; replay
// counts those skips.
// Every push happens under a.mu with the closed check, so it can never race
// cleanup's queue close. The caller must not hold a.mu.
func (a *app) enqueueReplayOp(p *pageRecord, op operation, quit <-chan struct{}) bool {
	for {
		a.mu.Lock()
		if p.closed || !p.syncOn || p.status == "paused" {
			a.mu.Unlock()
			return false
		}
		if op.Type != "scroll" {
			p.navPendingAt = time.Now()
		}
		if p.queue.push(op) {
			a.mu.Unlock()
			return true
		}
		a.mu.Unlock()
		select {
		case <-quit:
			return false
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (a *app) finishReplayRun(rp *replayRun, errMsg string) {
	a.mu.Lock()
	rp.active = false
	rp.err = errMsg
	done, total := rp.done, rp.total
	// Only touch the notice if it still belongs to this replay, and write it in
	// the same critical section: a notice landing between the check and the
	// write must not be clobbered, and cleanup's reset must not be undone.
	if a.noticeTag == "replay" {
		msg, kind := errMsg, "error"
		switch errMsg {
		case "":
			msg, kind = a.tr("ui.replay.done", done, total), "success"
		case "cancelled", "task stopped":
			// Cancel/stop need no notice — the panel's replay result line states them.
			msg, kind = "", ""
		}
		a.notice, a.noticeKind = msg, kind
		a.noticeTag, a.noticePath, a.noticePathIsDir = "", "", false
	}
	a.mu.Unlock()
	a.pushState()
}

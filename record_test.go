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

// waitFor polls cond until it holds or the deadline passes.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	waitFor(t, path, func() bool { _, err := os.Stat(path); return err == nil })
}

// runningFixture builds a running two-page task without a GUI. Tests inject
// fake captures so the recorder/replay paths run headless.
func runningFixture(t *testing.T) *app {
	t.Helper()
	a := newApp()
	a.mu.Lock()
	a.status = "running"
	a.taskID = "task-1"
	a.primaryID = "p1"
	a.order = []string{"p1", "p2"}
	a.pages = map[string]*pageRecord{
		"p1": {id: "p1", name: "Primary", url: "https://a.com/", status: "ready", syncOn: true, isPrimary: true, queue: make(chan operation, 8), navSignal: make(chan struct{}, 1)},
		"p2": {id: "p2", name: "Page 2", url: "https://b.com/", status: "ready", syncOn: true, queue: make(chan operation, 8), navSignal: make(chan struct{}, 1)},
	}
	a.mu.Unlock()
	return a
}

// recordFixture attaches a recorder rooted at a temp dir with a fake capture.
func recordFixture(t *testing.T) (*app, *recorder) {
	t.Helper()
	a := runningFixture(t)
	req := startRequest{pages: []resolvedPage{
		{name: "Primary", url: "https://a.com/", base: "https://a.com", resW: 1280, resH: 800},
		{name: "Page 2", url: "https://b.com/", base: "https://b.com", resW: 1280, resH: 800},
	}, viewMode: "desktop", record: true}
	a.startRecordingIn(t.TempDir(), "task-1", req, "en")
	a.mu.Lock()
	r := a.rec
	if r == nil {
		a.mu.Unlock()
		t.Fatal("recorder did not attach")
	}
	r.captureFn = func(p *pageRecord) ([]byte, error) { return []byte("png-" + p.id), nil }
	a.mu.Unlock()
	t.Cleanup(func() {
		a.mu.Lock()
		a.stopRecordingLocked()
		a.mu.Unlock()
		<-r.done
	})
	return a, r
}

func dispatchOp(t *testing.T, a *app, op operation, source string) {
	t.Helper()
	a.mu.Lock()
	a.recordOpLocked(op, source)
	a.mu.Unlock()
}

func stopRecorder(t *testing.T, a *app, r *recorder) {
	t.Helper()
	a.mu.Lock()
	a.stopRecordingLocked()
	a.mu.Unlock()
	<-r.done
}

func readScript(t *testing.T, r *recorder) []scriptLine {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.dir, "script.jsonl"))
	if err != nil {
		t.Fatalf("read script: %v", err)
	}
	var lines []scriptLine
	for _, raw := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var sl scriptLine
		if err := json.Unmarshal([]byte(raw), &sl); err != nil {
			t.Fatalf("bad script line %q: %v", raw, err)
		}
		lines = append(lines, sl)
	}
	return lines
}

func readSummary(t *testing.T, r *recorder) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.dir, "summary.json"))
	if err != nil {
		t.Fatalf("read summary: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("bad summary: %v", err)
	}
	return m
}

func TestRecorderScriptAndStepShots(t *testing.T) {
	a, r := recordFixture(t)
	op := operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}, Timestamp: 1}
	dispatchOp(t, a, op, "guest")

	waitForFile(t, filepath.Join(r.dir, "steps", "0001", "Page 2.png"))

	// The panel/MCP status surface reports the live recording.
	st := a.snapshot()
	if !st.Recording || st.RecordSteps != 1 || st.RecordDir != r.dir {
		t.Fatalf("snapshot recording = %v/%d/%q, want true/1/%q", st.Recording, st.RecordSteps, st.RecordDir, r.dir)
	}

	stopRecorder(t, a, r)

	lines := readScript(t, r)
	if len(lines) != 1 {
		t.Fatalf("script has %d lines, want 1", len(lines))
	}
	sl := lines[0]
	if sl.Seq != 1 || sl.Source != "guest" || sl.URL != "https://a.com/" || sl.At == "" {
		t.Fatalf("script line = %+v", sl)
	}
	if sl.Operation.Type != "click" || sl.Operation.Anchor.Selector != "#go" {
		t.Fatalf("operation not preserved: %+v", sl.Operation)
	}

	for name, want := range map[string]string{"Primary.png": "png-p1", "Page 2.png": "png-p2"} {
		got, err := os.ReadFile(filepath.Join(r.dir, "steps", "0001", name))
		if err != nil || string(got) != want {
			t.Fatalf("shot %s = %q, %v; want %q", name, got, err, want)
		}
	}

	// session.json records the setup the evidence came from.
	data, err := os.ReadFile(filepath.Join(r.dir, "session.json"))
	if err != nil {
		t.Fatalf("session.json: %v", err)
	}
	var session struct {
		TaskID string `json:"taskId"`
		Pages  []struct {
			ID        string `json:"id"`
			IsPrimary bool   `json:"isPrimary"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(data, &session); err != nil {
		t.Fatalf("bad session.json: %v", err)
	}
	if session.TaskID != "task-1" || len(session.Pages) != 2 || !session.Pages[0].IsPrimary {
		t.Fatalf("session = %+v", session)
	}

	summary := readSummary(t, r)
	for k, want := range map[string]float64{"operations": 1, "stepsCaptured": 1, "shotsSaved": 2, "shotsFailed": 0, "superseded": 0, "dropped": 0} {
		if got, _ := summary[k].(float64); got != want {
			t.Errorf("summary[%s] = %v, want %v", k, summary[k], want)
		}
	}

	a.mu.Lock()
	notice := a.notice
	a.mu.Unlock()
	if !strings.Contains(notice, r.dir) {
		t.Fatalf("save notice %q should name the recording dir", notice)
	}
}

func TestRecorderCoalescesBurst(t *testing.T) {
	a, r := recordFixture(t)
	// A busy follower keeps the burst from settling.
	a.mu.Lock()
	a.pages["p2"].queue <- operation{Type: "scroll", Anchor: anchor{Type: "css", Selector: "html"}}
	a.mu.Unlock()

	for i := 0; i < 3; i++ {
		op := operation{Type: "input", Anchor: anchor{Type: "id", Selector: "#note"}, Value: json.RawMessage(fmt.Sprintf(`"v%d"`, i))}
		dispatchOp(t, a, op, "guest")
	}

	// Every operation gets its JSONL line even while no capture can happen.
	waitFor(t, "3 script lines", func() bool {
		data, err := os.ReadFile(filepath.Join(r.dir, "script.jsonl"))
		return err == nil && len(strings.Split(strings.TrimSpace(string(data)), "\n")) == 3
	})
	if _, err := os.Stat(filepath.Join(r.dir, "steps")); err == nil {
		t.Fatal("captured while a follower was still busy")
	}

	// Drain the follower: the burst settles and captures under the LAST step.
	<-a.pages["p2"].queue
	waitForFile(t, filepath.Join(r.dir, "steps", "0003", "Primary.png"))
	stopRecorder(t, a, r)

	entries, err := os.ReadDir(filepath.Join(r.dir, "steps"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "0003" {
		t.Fatalf("steps = %v, %v; want only 0003", entries, err)
	}
	summary := readSummary(t, r)
	if got, _ := summary["superseded"].(float64); got != 2 {
		t.Errorf("superseded = %v, want 2", summary["superseded"])
	}
	if got, _ := summary["stepsCaptured"].(float64); got != 1 {
		t.Errorf("stepsCaptured = %v, want 1", summary["stepsCaptured"])
	}
	if got, _ := summary["operations"].(float64); got != 3 {
		t.Errorf("operations = %v, want 3 (every op keeps its line)", summary["operations"])
	}
}

func TestRecorderSettleWaitsForLoad(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	a.pages["p2"].status = "loading"
	a.mu.Unlock()

	dispatchOp(t, a, operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}, "guest")
	waitFor(t, "script line", func() bool {
		data, err := os.ReadFile(filepath.Join(r.dir, "script.jsonl"))
		return err == nil && strings.TrimSpace(string(data)) != ""
	})
	if _, err := os.Stat(filepath.Join(r.dir, "steps")); err == nil {
		t.Fatal("captured while a page was still loading")
	}

	a.mu.Lock()
	a.pages["p2"].status = "ready"
	a.mu.Unlock()
	waitForFile(t, filepath.Join(r.dir, "steps", "0001", "Primary.png"))
	stopRecorder(t, a, r)
}

func TestRecorderStopFlushesAndDetaches(t *testing.T) {
	a, r := recordFixture(t)
	op := operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}
	dispatchOp(t, a, op, "guest")
	dispatchOp(t, a, op, "host")
	stopRecorder(t, a, r)

	// Buffered steps are still written on stop; the summary is final.
	if lines := readScript(t, r); len(lines) != 2 {
		t.Fatalf("script has %d lines, want 2", len(lines))
	}
	summary := readSummary(t, r)
	if got, _ := summary["operations"].(float64); got != 2 {
		t.Errorf("operations = %v, want 2", summary["operations"])
	}
	if _, ok := summary["endedAt"]; !ok {
		t.Error("summary missing endedAt")
	}

	// After detach, dispatch is a no-op and the status surface clears.
	dispatchOp(t, a, op, "guest")
	if lines := readScript(t, r); len(lines) != 2 {
		t.Fatalf("post-stop dispatch leaked into the script: %d lines", len(lines))
	}
	if st := a.snapshot(); st.Recording {
		t.Fatal("snapshot still reports recording after stop")
	}
}

func TestRecorderDropsWhenQueueFull(t *testing.T) {
	a := newApp()
	// A recorder without a run goroutine: the queue fills and overflow drops.
	r := &recorder{steps: make(chan recStep, recordQueueSize)}
	a.mu.Lock()
	a.rec = r
	a.mu.Unlock()
	op := operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}
	for i := 0; i < recordQueueSize+5; i++ {
		dispatchOp(t, a, op, "guest")
	}
	if got := r.dropped.Load(); got != 5 {
		t.Fatalf("dropped = %d, want 5", got)
	}
	a.mu.Lock()
	seq := r.seq
	a.mu.Unlock()
	if seq != recordQueueSize+5 {
		t.Fatalf("seq = %d, want %d (dispatch counts every op)", seq, recordQueueSize+5)
	}
}

func TestRecorderFolderFailureKeepsTask(t *testing.T) {
	a := runningFixture(t)
	// A file where the recordings root should be: MkdirAll must fail.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	a.startRecordingIn(blocker, "task-1", startRequest{}, "en")
	a.mu.Lock()
	rec := a.rec
	notice := a.notice
	a.mu.Unlock()
	if rec != nil {
		t.Fatal("recorder attached despite folder failure")
	}
	if !strings.Contains(notice, "recording folder") {
		t.Fatalf("notice = %q, want the recording-folder failure", notice)
	}
	if st := a.snapshot(); st.Status != "running" {
		t.Fatalf("task status = %q, want running (recording failure must not fail the task)", st.Status)
	}
}

// TestRecorderCaptureFailureCounted: a non-rendering page fails its shot, the
// failure is counted in the summary, and the task keeps running.
func TestRecorderCaptureFailureCounted(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	r.captureFn = func(p *pageRecord) ([]byte, error) {
		if p.id == "p1" {
			return nil, fmt.Errorf("not rendering")
		}
		return []byte("png-" + p.id), nil
	}
	a.mu.Unlock()
	dispatchOp(t, a, operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}, "guest")
	waitForFile(t, filepath.Join(r.dir, "steps", "0001", "Page 2.png"))
	stopRecorder(t, a, r)
	summary := readSummary(t, r)
	if got, _ := summary["shotsSaved"].(float64); got != 1 {
		t.Errorf("shotsSaved = %v, want 1", summary["shotsSaved"])
	}
	if got, _ := summary["shotsFailed"].(float64); got != 1 {
		t.Errorf("shotsFailed = %v, want 1", summary["shotsFailed"])
	}
	if st := a.snapshot(); st.Status != "running" {
		t.Fatalf("task status = %q, want running (capture failure must not fail the task)", st.Status)
	}
}

// TestRecorderDedupesShotNames: two pages with the same name must not overwrite
// each other's step screenshot.
func TestRecorderDedupesShotNames(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	a.pages["p1"].name = "Same"
	a.pages["p2"].name = "Same"
	a.mu.Unlock()
	dispatchOp(t, a, operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}, "guest")
	waitForFile(t, filepath.Join(r.dir, "steps", "0001", "Same_2.png"))
	stopRecorder(t, a, r)
	entries, err := os.ReadDir(filepath.Join(r.dir, "steps", "0001"))
	if err != nil || len(entries) != 2 {
		t.Fatalf("step entries = %v, %v; want 2", entries, err)
	}
	for _, name := range []string{"Same.png", "Same_2.png"} {
		if _, err := os.Stat(filepath.Join(r.dir, "steps", "0001", name)); err != nil {
			t.Errorf("missing deduped shot %s: %v", name, err)
		}
	}
}

// TestRecorderWriteErrorDetaches: once the script can no longer be written the
// recorder detaches (later ops are not queued into a dead recording), reports
// the write error in the summary, and leaves the task running.
func TestRecorderWriteErrorDetaches(t *testing.T) {
	a, r := recordFixture(t)
	// os.File is safe for concurrent Close/Write: the next writeLine fails.
	if err := r.script.Close(); err != nil {
		t.Fatalf("close script: %v", err)
	}
	dispatchOp(t, a, operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}, "guest")
	waitFor(t, "recorder detach", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.rec == nil
	})
	<-r.done
	summary := readSummary(t, r)
	if _, ok := summary["writeError"]; !ok {
		t.Fatalf("summary missing writeError: %v", summary)
	}
	if st := a.snapshot(); st.Status != "running" || st.Recording {
		t.Fatalf("snapshot = %q/%v, want running/not recording", st.Status, st.Recording)
	}
	dispatchOp(t, a, operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}, "guest")
	data, err := os.ReadFile(filepath.Join(r.dir, "script.jsonl"))
	if err != nil || len(data) != 0 {
		t.Fatalf("post-detach dispatch leaked into the script: %q, %v", data, err)
	}
}

// TestStartRecordingAttachesWhileStarting: the recorder must attach before the
// task flips to running, so no dispatched op falls into a recording gap.
func TestStartRecordingAttachesWhileStarting(t *testing.T) {
	a := runningFixture(t)
	a.mu.Lock()
	a.status = "starting"
	a.mu.Unlock()
	a.startRecordingIn(t.TempDir(), "task-1", startRequest{}, "en")
	a.mu.Lock()
	r := a.rec
	a.mu.Unlock()
	if r == nil {
		t.Fatal("recorder did not attach while starting")
	}
	a.mu.Lock()
	a.stopRecordingLocked()
	a.mu.Unlock()
	<-r.done
}

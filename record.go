package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"
)

// Recorder tunables.
const (
	// recordQueueSize buffers dispatched steps; overflow is dropped (and
	// counted) so the capture path never blocks on recording.
	recordQueueSize = 1024
	// recordPollInterval is the settle-wait poll tick.
	recordPollInterval = 50 * time.Millisecond
	// recordBurstTimeout bounds the settle wait for one burst; after it the
	// step is captured in whatever state the pages are in (best effort).
	recordBurstTimeout = 8 * time.Second
	// recordRenderSettle lets the settled state paint before CapturePage.
	recordRenderSettle = 300 * time.Millisecond
)

// recStep is one dispatched operation awaiting its step screenshots.
type recStep struct {
	seq    int
	at     time.Time
	source string // guest | host | replay
	url    string // primary URL at dispatch time
	op     operation
}

// scriptLine is one line of script.jsonl (docs/mcp.md): a recorded operation
// plus its evidence context. peapod_replay reads the operation field back.
type scriptLine struct {
	Seq       int       `json:"seq"`
	At        string    `json:"at"`
	Source    string    `json:"source"`
	URL       string    `json:"url"`
	Operation operation `json:"operation"`
}

// recorder writes one task's evidence chain into a session folder:
//
//	session.json   — the task setup (pages, UA, mode) at start
//	script.jsonl   — every dispatched operation, in order, replayable
//	steps/NNNN/    — one PNG per page after step NNNN settles
//	summary.json   — counts and errors, written when recording ends
//
// One goroutine owns every file write and capture, so steps never interleave.
// Operations that pile up while a capture is in flight are coalesced: each
// still gets its JSONL line, but only the burst's latest step is captured —
// a mid-burst shot would show later operations under an earlier step's label.
// Superseded and dropped counts land in summary.json so the evidence is honest
// about what was and wasn't captured.
type recorder struct {
	a      *app
	taskID string
	dir    string
	lang   string
	script *os.File
	seq    int // guarded by a.mu; assigned in recordOpLocked
	steps  chan recStep
	quit   chan struct{}
	done   chan struct{} // closed when run exits (tests wait on it)

	dropped atomic.Int64 // steps lost to a full queue

	// captureFn is p.view.CapturePage in production; tests inject a fake so
	// the whole recorder runs without a GUI.
	captureFn func(p *pageRecord) ([]byte, error)

	// The stats below are touched only by the run goroutine.
	written    int // JSONL lines actually persisted
	superseded int
	captured   int
	saved      int
	failed     int
	writeErr   error
	sessionErr error // set before run starts; metadata is best-effort
}

// startRecording opens a session folder under the user's recordings root and
// starts the recorder goroutine. A folder failure disables recording for this
// task (with a notice) instead of failing the task itself.
func (a *app) startRecording(taskID string, req startRequest, lang string) {
	a.startRecordingIn(filepath.Join(userArtifactRoot(), "recordings"), taskID, req, lang)
}

// startRecordingIn is startRecording with an explicit root, for tests.
func (a *app) startRecordingIn(root, taskID string, req startRequest, lang string) {
	dir, err := batchDir(root, "recording")
	if err != nil {
		a.setNoticeWarn(trLang(lang, "err.recordDir", err.Error()))
		return
	}
	f, err := os.Create(filepath.Join(dir, "script.jsonl"))
	if err != nil {
		a.setNoticeWarn(trLang(lang, "err.recordDir", err.Error()))
		return
	}
	r := &recorder{
		a: a, taskID: taskID, dir: dir, lang: lang, script: f,
		steps: make(chan recStep, recordQueueSize),
		quit:  make(chan struct{}),
		done:  make(chan struct{}),
		captureFn: func(p *pageRecord) ([]byte, error) {
			return p.view.CapturePage()
		},
	}
	r.sessionErr = writeSessionJSON(dir, taskID, req)
	a.mu.Lock()
	if a.taskID != taskID || (a.status != "running" && a.status != "starting") {
		// The task stopped while the folder was being created.
		a.mu.Unlock()
		f.Close()
		os.RemoveAll(dir)
		return
	}
	a.rec = r
	a.mu.Unlock()
	go r.run()
}

// recordOpLocked appends one dispatched operation to the active recording.
// The caller holds a.mu. The send never blocks: a full queue drops the step
// (counted in the summary) so capture latency is unaffected by recording.
func (a *app) recordOpLocked(op operation, source string) {
	r := a.rec
	if r == nil {
		return
	}
	r.seq++
	url := ""
	if p := a.pages[a.primaryID]; p != nil {
		url = p.url
	}
	select {
	case r.steps <- recStep{seq: r.seq, at: time.Now(), source: source, url: url, op: op}:
	default:
		r.dropped.Add(1)
	}
}

// stopRecordingLocked detaches the recorder and tells its goroutine to finish.
// The caller holds a.mu; clearing a.rec under the same lock guarantees no
// sender can race the channel closes.
func (a *app) stopRecordingLocked() {
	r := a.rec
	if r == nil {
		return
	}
	a.rec = nil
	close(r.quit)
	close(r.steps)
}

func (r *recorder) run() {
	defer close(r.done)
	defer r.finish()
	for step := range r.steps {
		if err := r.writeLine(step); err != nil {
			r.writeErr = err
			return
		}
		r.captureBurst(step)
		if r.writeErr != nil {
			return
		}
	}
}

func (r *recorder) writeLine(step recStep) error {
	data, err := json.Marshal(scriptLine{
		Seq: step.seq, At: step.at.Format(time.RFC3339Nano),
		Source: step.source, URL: step.url, Operation: step.op,
	})
	if err != nil {
		return err
	}
	if _, err := r.script.Write(append(data, '\n')); err != nil {
		return err
	}
	r.written++
	return nil
}

// captureBurst settles the task, then captures every page for the burst's
// latest step. Steps arriving during the wait are absorbed (their JSONL lines
// are written) and supersede the capture label.
func (r *recorder) captureBurst(step recStep) {
	latest := step
	deadline := time.Now().Add(recordBurstTimeout)
	for {
		r.drain(&latest)
		if r.writeErr != nil {
			return
		}
		expired := time.Now().After(deadline)
		if r.settled() || expired {
			// Label the capture with the newest dispatched operation; if one
			// landed just now, settle again for it (unless the burst expired).
			seq := latest.seq
			r.drain(&latest)
			if r.writeErr != nil {
				return
			}
			if latest.seq == seq || expired {
				break
			}
			continue
		}
		select {
		case <-r.quit:
			return
		case <-time.After(recordPollInterval):
		}
	}
	select {
	case <-r.quit:
		return
	case <-time.After(recordRenderSettle):
	}
	r.captureStep(latest)
}

// drain absorbs queued steps into latest, writing each one's JSONL line.
func (r *recorder) drain(latest *recStep) {
	for {
		select {
		case s, ok := <-r.steps:
			if !ok {
				return // stopped; run's range ends next
			}
			if err := r.writeLine(s); err != nil {
				r.writeErr = err
				return
			}
			r.superseded++
			*latest = s
		default:
			return
		}
	}
}

// settled reports whether every live page has loaded and every synced page
// has drained its replay queue — the state a step screenshot should show.
// A stopped or replaced task is never settled; quit ends the wait instead.
func (r *recorder) settled() bool {
	a := r.a
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.taskID != r.taskID || a.status != "running" {
		return false
	}
	for _, id := range a.order {
		p := a.pages[id]
		if p == nil || p.closed {
			continue
		}
		if p.status == "loading" {
			return false
		}
		if p.syncOn && (p.queue.len() > 0 || p.replaying) {
			return false
		}
	}
	return true
}

// captureStep saves one PNG per running page into steps/{seq:04d}/, reusing
// the batch path's cross-task guard and coalesced capture flights. Failures
// are counted, not fatal: a non-rendering page (minimized window) must not
// stop the recording.
func (r *recorder) captureStep(step recStep) {
	a := r.a
	a.mu.Lock()
	if a.taskID != r.taskID || a.status != "running" {
		a.mu.Unlock()
		return
	}
	type shot struct{ name, id string }
	shots := make([]shot, 0, len(a.order))
	for _, id := range a.order {
		if p := a.pages[id]; p != nil && !p.closed {
			shots = append(shots, shot{name: p.name, id: id})
		}
	}
	a.mu.Unlock()
	if len(shots) == 0 {
		return
	}
	dir := filepath.Join(r.dir, "steps", fmt.Sprintf("%04d", step.seq))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		r.failed += len(shots)
		return
	}
	r.captured++
	used := map[string]bool{}
	for _, s := range shots {
		a.mu.Lock()
		var f *captureFlight
		if a.batchShotValidLocked(r.taskID, s.id) {
			p := a.pages[s.id]
			f = a.capturePageFlight(p, func() ([]byte, error) { return r.captureFn(p) })
		}
		a.mu.Unlock()
		if f == nil {
			r.failed++
			continue
		}
		png, err := waitCapture(f, captureTimeout)
		if err != nil {
			r.failed++
			continue
		}
		stem := sanitizeFileName(s.name)
		fname := stem + ".png"
		for i := 2; used[fname]; i++ {
			fname = fmt.Sprintf("%s_%d.png", stem, i)
		}
		used[fname] = true
		if err := os.WriteFile(filepath.Join(dir, fname), png, 0o644); err != nil {
			r.failed++
			continue
		}
		r.saved++
	}
}

// finish closes the script and writes summary.json. It runs on the recorder
// goroutine after run exits, so the stats are final; seq is read under a.mu
// because a write-error exit can detach while dispatch is still counting.
func (r *recorder) finish() {
	if r.writeErr != nil {
		// The script can no longer be written: detach so later operations do
		// not queue into a dead recording.
		r.a.mu.Lock()
		if r.a.rec == r {
			r.a.stopRecordingLocked()
		}
		r.a.mu.Unlock()
	}
	r.a.mu.Lock()
	dispatched := r.seq
	r.a.mu.Unlock()
	if r.script != nil {
		r.script.Close()
	}
	summary := map[string]any{
		"endedAt":       time.Now().Format(time.RFC3339Nano),
		"operations":    r.written,
		"dispatched":    dispatched,
		"stepsCaptured": r.captured,
		"shotsSaved":    r.saved,
		"shotsFailed":   r.failed,
		"superseded":    r.superseded,
		"dropped":       r.dropped.Load(),
	}
	if r.writeErr != nil {
		summary["writeError"] = r.writeErr.Error()
	}
	if r.sessionErr != nil {
		summary["sessionError"] = r.sessionErr.Error()
	}
	if data, err := json.MarshalIndent(summary, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(r.dir, "summary.json"), data, 0o644)
	}
	if dispatched > 0 {
		r.a.setNoticeOK(trLang(r.lang, "notice.recordSaved", dispatched, r.dir))
	}
}

// writeSessionJSON records the task setup alongside the script so the evidence
// stands alone: which pages/variants, UA and mode produced these operations.
func writeSessionJSON(dir, taskID string, req startRequest) error {
	type sessionPage struct {
		ID        string  `json:"id"`
		Name      string  `json:"name"`
		URL       string  `json:"url"`
		Base      string  `json:"base"`
		ResW      float64 `json:"resW"`
		ResH      float64 `json:"resH"`
		IsPrimary bool    `json:"isPrimary"`
	}
	pages := make([]sessionPage, 0, len(req.pages))
	for i, rp := range req.pages {
		pages = append(pages, sessionPage{
			ID: fmt.Sprintf("p%d", i+1), Name: rp.name, URL: rp.url, Base: rp.base,
			ResW: rp.resW, ResH: rp.resH, IsPrimary: i == 0,
		})
	}
	data, err := json.MarshalIndent(map[string]any{
		"taskId":       taskID,
		"startedAt":    time.Now().Format(time.RFC3339Nano),
		"viewMode":     req.viewMode,
		"userAgent":    req.userAgent,
		"shareStorage": req.shareStorage,
		"diagnostics":  req.diagnostics,
		"syncDelayMs":  float64(req.syncDelay) / float64(time.Millisecond),
		"pages":        pages,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "session.json"), data, 0o644)
}

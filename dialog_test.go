package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// dispatchDialog records one page-reported popup through the app's locked path.
func dispatchDialog(t *testing.T, a *app, pageID string, d dialogPayload) {
	t.Helper()
	a.mu.Lock()
	a.recordDialogLocked(a.pages[pageID], d)
	a.mu.Unlock()
}

func readDialogs(t *testing.T, r *recorder) []dialogLine {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(r.dir, "dialogs.jsonl"))
	if err != nil {
		t.Fatalf("read dialogs: %v", err)
	}
	var lines []dialogLine
	for _, raw := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if raw == "" {
			continue
		}
		var dl dialogLine
		if err := json.Unmarshal([]byte(raw), &dl); err != nil {
			t.Fatalf("bad dialog line %q: %v", raw, err)
		}
		lines = append(lines, dl)
	}
	return lines
}

func TestDialogPreloadReportsPopups(t *testing.T) {
	s := dialogPreload("p2")
	for _, want := range []string{
		`"p2"`,
		"window.__peapodDialog",
		"Sync.Dialog",
		"dialog[open]",
		`[role="alertdialog"]`,
		`[aria-modal="true"]`,
		`[role="alert"]`,
		`[role="status"]`,
		"MutationObserver",
		"characterData: true",
		"wrapNative('alert', 'alert')",
		"wrapNative('confirm', 'confirm')",
		"wrapNative('prompt', 'prompt')",
		"MAX_TEXT = 4000",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("dialog preload missing %q", want)
		}
	}
	// A page id is %q-quoted, so a quote in it cannot break out of the literal.
	if got := dialogPreload(`p"x`); !strings.Contains(got, `"p\"x"`) {
		t.Fatalf("page id not escaped: %q", got)
	}
}

func TestValidateDialog(t *testing.T) {
	ok := []dialogPayload{
		{Kind: "dialog", Text: "hi"},
		{Kind: "modal", Role: "alertdialog", Text: "hi"},
		{Kind: "toast", Role: "status", Text: "saved"},
		{Kind: "alert", Text: "boom"},
		{Kind: "confirm", Text: "sure?"},
		{Kind: "prompt", Text: "name?", Value: "default"},
	}
	for _, p := range ok {
		if err := validateDialog(&p); err != nil {
			t.Errorf("valid payload %+v rejected: %v", p, err)
		}
	}
	if err := validateDialog(&dialogPayload{Kind: "popup"}); err == nil {
		t.Error("unknown kind accepted")
	}
}

// TestValidateDialogClips: the in-page clip counts UTF-16 units, the Go bound
// counts bytes, so CJK text is clipped to a valid UTF-8 prefix, never rejected.
func TestValidateDialogClips(t *testing.T) {
	p := dialogPayload{
		Kind:     "dialog",
		Text:     strings.Repeat("确", 2000), // 6000 bytes
		Role:     strings.Repeat("确", 40),   // 120 bytes
		Selector: strings.Repeat("确", 200),
		Value:    strings.Repeat("默", 100), // 300 bytes
	}
	if err := validateDialog(&p); err != nil {
		t.Fatalf("CJK payload rejected: %v", err)
	}
	for name, s := range map[string]string{"text": p.Text, "role": p.Role, "selector": p.Selector, "value": p.Value} {
		if !utf8.ValidString(s) {
			t.Errorf("%s split a UTF-8 sequence", name)
		}
	}
	if len(p.Text) > 4000 || len(p.Role) > 64 || len(p.Selector) > 512 || len(p.Value) > 200 {
		t.Fatalf("clip over bound: %d/%d/%d/%d", len(p.Text), len(p.Role), len(p.Selector), len(p.Value))
	}
	if len(p.Text) < 3990 {
		t.Errorf("text clipped too aggressively: %d bytes", len(p.Text))
	}
}

// TestRecorderDialogEvidence: a DOM popup gets a text line plus a page shot; a
// native alert gets a text line marked native with no shot. Neither leaks into
// script.jsonl (which replay consumes), and the counts land in summary.json.
func TestRecorderDialogEvidence(t *testing.T) {
	a, r := recordFixture(t)
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "dialog", Role: "dialog", Text: "确定要删除吗？", Selector: "div.confirm"})
	dispatchDialog(t, a, "p2", dialogPayload{Kind: "alert", Text: "不能为空"})

	waitFor(t, "two dialog lines", func() bool { return len(readDialogsQuiet(r)) == 2 })

	st := a.snapshot()
	if !st.Recording || st.RecordDialogs != 2 || st.RecordDir != r.dir {
		t.Fatalf("snapshot = recording %v, dialogs %d, dir %q; want true/2/%q", st.Recording, st.RecordDialogs, st.RecordDir, r.dir)
	}
	stopRecorder(t, a, r)

	lines := readDialogs(t, r)
	if len(lines) != 2 {
		t.Fatalf("dialogs = %d, want 2", len(lines))
	}
	dom := lines[0]
	if dom.Seq != 1 || dom.Kind != "dialog" || dom.Text != "确定要删除吗？" || dom.PageID != "p1" || dom.PageName != "Primary" {
		t.Fatalf("dom line = %+v", dom)
	}
	if dom.Shot != "dialogs/0001-Primary.png" {
		t.Fatalf("dom shot = %q, want dialogs/0001-Primary.png", dom.Shot)
	}
	got, err := os.ReadFile(filepath.Join(r.dir, filepath.FromSlash(dom.Shot)))
	if err != nil || string(got) != "png-p1" {
		t.Fatalf("shot = %q, %v; want png-p1", got, err)
	}

	native := lines[1]
	if native.Kind != "alert" || !native.Native || native.Shot != "" || native.ShotSkipped == "" {
		t.Fatalf("native line = %+v", native)
	}

	summary := readSummary(t, r)
	for k, want := range map[string]float64{
		"dialogs": 2, "dialogShots": 1, "dialogShotsFailed": 0, "nativeDialogs": 1, "dialogsDropped": 0,
	} {
		if got, _ := summary[k].(float64); got != want {
			t.Errorf("summary[%s] = %v, want %v", k, summary[k], want)
		}
	}

	// Popups are evidence, not replayable operations.
	if data, err := os.ReadFile(filepath.Join(r.dir, "script.jsonl")); err != nil || len(data) != 0 {
		t.Fatalf("script.jsonl = %q, %v; want empty", data, err)
	}
}

// readDialogsQuiet is the polling variant: a missing or partially written file
// reads as fewer lines instead of failing, so waitFor can retry it.
func readDialogsQuiet(r *recorder) []dialogLine {
	data, err := os.ReadFile(filepath.Join(r.dir, "dialogs.jsonl"))
	if err != nil {
		return nil
	}
	var out []dialogLine
	for _, raw := range strings.Split(string(data), "\n") {
		var dl dialogLine
		if json.Unmarshal([]byte(raw), &dl) == nil && dl.Seq > 0 {
			out = append(out, dl)
		}
	}
	return out
}

// TestRecorderDialogShotFailureCounted: a capture failure keeps the popup's text
// and is counted, without failing the task.
func TestRecorderDialogShotFailureCounted(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	r.captureFn = func(p *pageRecord) ([]byte, error) { return nil, os.ErrDeadlineExceeded }
	a.mu.Unlock()
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "modal", Text: "async error"})
	waitFor(t, "dialog line", func() bool { return len(readDialogsQuiet(r)) == 1 })
	stopRecorder(t, a, r)

	line := readDialogs(t, r)[0]
	if line.Text != "async error" || line.Shot != "" || line.ShotError == "" {
		t.Fatalf("line = %+v", line)
	}
	summary := readSummary(t, r)
	if got, _ := summary["dialogShotsFailed"].(float64); got != 1 {
		t.Errorf("dialogShotsFailed = %v, want 1", summary["dialogShotsFailed"])
	}
	if got, _ := summary["dialogShots"].(float64); got != 0 {
		t.Errorf("dialogShots = %v, want 0", summary["dialogShots"])
	}
	if st := a.snapshot(); st.Status != "running" {
		t.Fatalf("task status = %q, want running", st.Status)
	}
}

// TestRecorderDialogWriteErrorCounted: a dialogs.jsonl write failure is
// reported in the summary; recording of operations still ends normally.
func TestRecorderDialogWriteErrorCounted(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	r.dialogFile.Close()
	a.mu.Unlock()
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "toast", Text: "saved"})
	stopRecorder(t, a, r)

	summary := readSummary(t, r)
	if _, ok := summary["dialogError"]; !ok {
		t.Fatalf("summary missing dialogError: %v", summary)
	}
	if got, _ := summary["dialogs"].(float64); got != 1 {
		t.Errorf("dialogs = %v, want 1 (seen even though the line failed)", summary["dialogs"])
	}
}

// TestRecorderDialogDropsWhenQueueFull: popup reports never block a page's IPC
// path; overflow is dropped and counted.
func TestRecorderDialogDropsWhenQueueFull(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", name: "Primary", url: "https://a.com/"}
	a.pages = map[string]*pageRecord{"p1": p}
	r := &recorder{dialogs: make(chan dialogEvent, 2)}
	a.rec = r
	for i := 0; i < 5; i++ {
		a.recordDialogLocked(p, dialogPayload{Kind: "toast", Text: "x"})
	}
	if got := r.dialogDropped.Load(); got != 3 {
		t.Fatalf("dialogDropped = %d, want 3", got)
	}
	if r.dialogSeq != 5 {
		t.Fatalf("dialogSeq = %d, want 5 (dispatch counts every report)", r.dialogSeq)
	}
}

func writeRecDialogs(t *testing.T, dir string, lines []dialogLine) {
	t.Helper()
	var buf bytes.Buffer
	for _, dl := range lines {
		data, err := json.Marshal(dl)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "dialogs.jsonl"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// dialogSession writes a two-step session with popups before, between and after
// the steps, including one shot path that tries to escape the session folder.
func dialogSession(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	base := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	writeRecScript(t, dir, []scriptLine{
		{Seq: 1, At: base.Add(time.Second).Format(time.RFC3339Nano), Source: "guest", URL: "https://a.com/", Operation: operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}},
		{Seq: 2, At: base.Add(3 * time.Second).Format(time.RFC3339Nano), Source: "guest", URL: "https://a.com/", Operation: operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#ok"}}},
	})
	for _, seq := range []string{"0001", "0002"} {
		sd := filepath.Join(dir, "steps", seq)
		if err := os.MkdirAll(sd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sd, "Primary.png"), testPNG(t, 16, 16), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	popupDir := filepath.Join(dir, "dialogs")
	if err := os.MkdirAll(popupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(popupDir, "0001-Primary.png"), testPNG(t, 16, 16), 0o644); err != nil {
		t.Fatal(err)
	}
	writeRecDialogs(t, dir, []dialogLine{
		{Seq: 1, At: base.Add(500 * time.Millisecond).Format(time.RFC3339Nano), PageID: "p1", PageName: "Primary", PageURL: "https://a.com/", Kind: "dialog", Text: "删除 <b>“x”</b>", Shot: "dialogs/0001-Primary.png"},
		{Seq: 2, At: base.Add(2 * time.Second).Format(time.RFC3339Nano), PageID: "p1", PageName: "Primary", PageURL: "https://a.com/", Kind: "alert", Text: "不能为空", Native: true, ShotSkipped: "native"},
		{Seq: 3, At: base.Add(4 * time.Second).Format(time.RFC3339Nano), PageID: "p2", PageName: "Page 2", PageURL: "https://b.com/", Kind: "toast", Text: "已保存", Shot: "../escape.png"},
	})
	return dir
}

func TestLoadRecDialogs(t *testing.T) {
	dir := dialogSession(t)
	d, err := loadRecDetail(dir, 0)
	if err != nil {
		t.Fatalf("loadRecDetail: %v", err)
	}
	if len(d.dialogs) != 3 {
		t.Fatalf("dialogs = %d, want 3", len(d.dialogs))
	}
	if d.dialogs[0].shotPath != filepath.Join(dir, "dialogs", "0001-Primary.png") {
		t.Fatalf("shot path = %q", d.dialogs[0].shotPath)
	}
	if !d.dialogs[1].native || d.dialogs[1].pageName != "Primary" {
		t.Fatalf("native dialog = %+v", d.dialogs[1])
	}
	// An escaping shot path is rejected, not followed.
	if d.dialogs[2].shotPath != "" {
		t.Fatalf("escaping shot path accepted: %q", d.dialogs[2].shotPath)
	}
	// Absent dialogs.jsonl (older recordings) loads as none.
	if got := loadRecDialogs(t.TempDir()); got != nil {
		t.Fatalf("loadRecDialogs on an empty folder = %v, want nil", got)
	}
}

func TestGroupRecTimelineInterleavesDialogs(t *testing.T) {
	dir := dialogSession(t)
	d, err := loadRecDetail(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	entries := groupRecTimeline(d.steps, d.dialogs, false)
	if len(entries) != 5 {
		t.Fatalf("entries = %d, want 5 (dialog, step, dialog, step, dialog)", len(entries))
	}
	want := []struct {
		seq    int // dialog seq, or step seq when dialog is nil
		dialog bool
	}{
		{1, true}, {1, false}, {2, true}, {2, false}, {3, true},
	}
	for i, w := range want {
		e := entries[i]
		if w.dialog {
			if e.dialog == nil || e.dialog.seq != w.seq {
				t.Fatalf("entry %d = %+v, want dialog %d", i, e, w.seq)
			}
			continue
		}
		if e.dialog != nil || len(e.steps) != 1 || e.steps[0].line.Seq != w.seq {
			t.Fatalf("entry %d = %+v, want step %d", i, e, w.seq)
		}
	}
}

func TestGroupRecTimelineDialogBreaksScrollRun(t *testing.T) {
	base := time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local)
	stepAt := func(seq int, offset time.Duration) recDetailStep {
		st := recShotStep(seq, "scroll", true)
		st.line.At = base.Add(offset).Format(time.RFC3339Nano)
		return st
	}
	steps := []recDetailStep{stepAt(1, time.Second), stepAt(2, 3*time.Second)}
	dialogs := []recDialog{{seq: 1, at: base.Add(2 * time.Second).Format(time.RFC3339Nano), kind: "toast", text: "saved"}}
	entries := groupRecTimeline(steps, dialogs, false)
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (scroll, popup, scroll)", len(entries))
	}
	if !entries[0].run || len(entries[0].steps) != 1 || entries[1].dialog == nil || entries[2].dialog != nil || !entries[2].run {
		t.Fatalf("popup did not break the scroll run: %+v / %+v / %+v", entries[0], entries[1], entries[2])
	}
}

// TestRecorderDialogStopSkipsQueuedShots: a popup already being captured keeps
// its shot, but popups still queued when recording stops skip the capture and
// say so — a clean stop must not wait on (or count) more captures.
func TestRecorderDialogStopSkipsQueuedShots(t *testing.T) {
	a, r := recordFixture(t)
	release := make(chan struct{})
	a.mu.Lock()
	r.captureFn = func(p *pageRecord) ([]byte, error) {
		<-release
		return []byte("png-" + p.id), nil
	}
	a.mu.Unlock()
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "dialog", Text: "first"})
	waitFor(t, "in-flight capture", func() bool {
		a.mu.Lock()
		defer a.mu.Unlock()
		return a.pages["p1"].capture != nil
	})
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "dialog", Text: "second"})
	a.mu.Lock()
	a.stopRecordingLocked()
	a.mu.Unlock()
	close(release)
	<-r.done

	lines := readDialogs(t, r)
	if len(lines) != 2 {
		t.Fatalf("dialogs = %d, want 2", len(lines))
	}
	if lines[0].Shot == "" {
		t.Fatalf("in-flight popup lost its shot: %+v", lines[0])
	}
	if lines[1].Shot != "" || lines[1].ShotSkipped != dialogSkipStopped {
		t.Fatalf("queued popup should skip its shot: %+v", lines[1])
	}
	summary := readSummary(t, r)
	if got, _ := summary["dialogShots"].(float64); got != 1 {
		t.Errorf("dialogShots = %v, want 1", summary["dialogShots"])
	}
	if got, _ := summary["dialogShotsFailed"].(float64); got != 0 {
		t.Errorf("dialogShotsFailed = %v, want 0 on a clean stop", summary["dialogShotsFailed"])
	}
}

// TestCanRecordDialogsStates: popup reports are accepted while a recording is
// attached, including the starting phase (pages load before status is running)
// and the stopping phase (queued text is still evidence).
func TestCanRecordDialogsStates(t *testing.T) {
	a := newApp()
	a.rec = &recorder{}
	for status, want := range map[string]bool{
		"idle": false, "starting": true, "running": true, "stopping": true, "cleanup-error": false,
	} {
		a.status = status
		if got := a.canRecordDialogsLocked(); got != want {
			t.Errorf("status %q: canRecordDialogsLocked = %v, want %v", status, got, want)
		}
	}
	a.rec = nil
	a.status = "running"
	if a.canRecordDialogsLocked() {
		t.Error("no recorder: reports must be ignored")
	}
}

// TestRecorderDialogStartingCaptures: pages raise popups while the task is still
// starting (the recorder attaches before they load); those get a shot too.
func TestRecorderDialogStartingCaptures(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	a.status = "starting"
	a.mu.Unlock()
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "modal", Text: "本地化启动弹窗"})
	waitFor(t, "dialog line", func() bool { return len(readDialogsQuiet(r)) == 1 })
	stopRecorder(t, a, r)

	line := readDialogs(t, r)[0]
	if line.Shot == "" || line.ShotError != "" {
		t.Fatalf("starting popup shot = %q error = %q, want a shot", line.Shot, line.ShotError)
	}
	summary := readSummary(t, r)
	if got, _ := summary["dialogShots"].(float64); got != 1 {
		t.Errorf("dialogShots = %v, want 1", summary["dialogShots"])
	}
}

// TestRecorderDialogStoppingSkipsShots: stopTask flips the status to stopping
// before pages are destroyed. A popup queued at that moment must be recorded as
// "stopped" (text kept, no shot), not as a capture failure against a page that
// cleanup is about to close.
func TestRecorderDialogStoppingSkipsShots(t *testing.T) {
	a, r := recordFixture(t)
	a.mu.Lock()
	a.status = "stopping"
	a.mu.Unlock()
	dispatchDialog(t, a, "p1", dialogPayload{Kind: "toast", Text: "closed now"})
	waitFor(t, "dialog line", func() bool { return len(readDialogsQuiet(r)) == 1 })
	stopRecorder(t, a, r)

	line := readDialogs(t, r)[0]
	if line.Text != "closed now" || line.Shot != "" || line.ShotSkipped != dialogSkipStopped || line.ShotError != "" {
		t.Fatalf("stopping popup line = %+v", line)
	}
	summary := readSummary(t, r)
	if got, _ := summary["dialogShots"].(float64); got != 0 {
		t.Errorf("dialogShots = %v, want 0", summary["dialogShots"])
	}
	if got, _ := summary["dialogShotsFailed"].(float64); got != 0 {
		t.Errorf("dialogShotsFailed = %v, want 0 on a stop", summary["dialogShotsFailed"])
	}
}

// TestLoadRecDialogsToleratesTornAndOversized: a live write and a file over the
// read cap both yield the readable prefix instead of nothing.
func TestLoadRecDialogsToleratesTornAndOversized(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	good, _ := json.Marshal(dialogLine{Seq: 1, At: time.Now().Format(time.RFC3339Nano), Kind: "toast", Text: "ok"})
	if err := os.WriteFile(filepath.Join(dir, "dialogs.jsonl"), append(append(good, '\n'), []byte(`{"seq":2,"pa`)...), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadRecDialogs(dir); len(got) != 1 || got[0].text != "ok" {
		t.Fatalf("torn line not tolerated: %+v", got)
	}

	var buf bytes.Buffer
	line, _ := json.Marshal(dialogLine{Seq: 1, At: time.Now().Format(time.RFC3339Nano), Kind: "toast", Text: strings.Repeat("x", 900)})
	for buf.Len() <= recDialogMaxBytes+len(line) {
		buf.Write(line)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "dialogs.jsonl"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := loadRecDialogs(dir); len(got) == 0 {
		t.Fatal("oversized dialogs.jsonl loaded as empty")
	}
}

// TestExportReportDialogs: the HTML report carries each popup's text (escaped)
// and its shot, or a note when no shot exists.
func TestExportReportDialogs(t *testing.T) {
	dir := dialogSession(t)
	path, err := exportReport(dir, "en")
	if err != nil {
		t.Fatalf("exportReport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		trLang("en", "rec.popup.title"),
		"删除 &lt;b&gt;“x”&lt;/b&gt;",
		`src="dialogs/0001-Primary.png"`,
		trLang("en", "rec.popup.kind.alert"),
		`已保存`,
		trLang("en", "rec.popup.native"),
		trLang("en", "rec.popup.noShot"),
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if strings.Contains(html, "escape.png") {
		t.Error("report referenced a path outside the session folder")
	}
	if strings.Contains(html, "<b>“x”</b>") {
		t.Error("popup text embedded unescaped")
	}
}

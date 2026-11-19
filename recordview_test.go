package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// testPNG encodes a solid-color PNG so viewer tests have real, decodable shots.
func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 80, G: 120, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

// writeSession creates a session folder with session.json and, optionally,
// summary.json, mirroring what the recorder writes.
func writeSession(t *testing.T, root, name string, started time.Time, withSummary bool) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sess := map[string]any{
		"taskId":    "task-x",
		"startedAt": started.Format(time.RFC3339Nano),
		"viewMode":  "mobile",
		"pages": []map[string]any{
			{"id": "p1", "name": "Primary", "url": "https://a.com/", "isPrimary": true},
			{"id": "p2", "name": "Page 2", "url": "https://b.com/", "isPrimary": false},
		},
	}
	data, _ := json.MarshalIndent(sess, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "session.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if withSummary {
		sum := map[string]any{
			"endedAt":    started.Add(time.Minute).Format(time.RFC3339Nano),
			"operations": 2, "dispatched": 2, "stepsCaptured": 1,
			"shotsSaved": 2, "shotsFailed": 0, "superseded": 1, "dropped": 0,
		}
		sd, _ := json.MarshalIndent(sum, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, "summary.json"), sd, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func writeRecScript(t *testing.T, dir string, lines []scriptLine) {
	t.Helper()
	var buf bytes.Buffer
	for _, sl := range lines {
		data, err := json.Marshal(sl)
		if err != nil {
			t.Fatal(err)
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(dir, "script.jsonl"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestScanRecordingsNewestFirst(t *testing.T) {
	root := t.TempDir()
	old := writeSession(t, root, "recording_20261117-101500", time.Date(2026, 11, 17, 10, 15, 0, 0, time.Local), true)
	newer := writeSession(t, root, "recording_20261118-120000", time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local), false)
	// A folder with no metadata and a non-session folder are both ignored.
	if err := os.MkdirAll(filepath.Join(root, "recording_junk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "screenshots"), 0o755); err != nil {
		t.Fatal(err)
	}

	list := scanRecordings(root)
	if len(list) != 2 {
		t.Fatalf("scan returned %d sessions, want 2", len(list))
	}
	if list[0].dir != newer || !list[0].torn || list[0].recording {
		t.Fatalf("newest = %q torn=%v recording=%v, want %q torn=true recording=false", list[0].dir, list[0].torn, list[0].recording, newer)
	}
	if list[1].dir != old || list[1].torn || list[1].recording {
		t.Fatalf("older = %q torn=%v recording=%v, want %q torn=false recording=false", list[1].dir, list[1].torn, list[1].recording, old)
	}
	if list[1].ops != 2 || list[1].shots != 2 || len(list[1].pages) != 2 || list[1].viewMode != "mobile" {
		t.Fatalf("older meta = %+v", list[1])
	}
}

func TestRecMarkLive(t *testing.T) {
	root := t.TempDir()
	live := writeSession(t, root, "recording_20261118-120000", time.Now(), false)
	torn := writeSession(t, root, "recording_20261117-101500", time.Now(), false)
	done := writeSession(t, root, "recording_20261116-101500", time.Now(), true)
	for _, dir := range []string{live, torn, done} {
		m, ok := readSessionMeta(dir)
		if !ok {
			t.Fatalf("no meta for %s", dir)
		}
		m.markLive(live)
		switch dir {
		case live:
			if !m.recording {
				t.Fatalf("live session not flagged recording: %+v", m)
			}
		case torn:
			if m.recording || !m.torn {
				t.Fatalf("interrupted session = %+v, want torn and not recording", m)
			}
		case done:
			if m.recording || m.torn {
				t.Fatalf("completed session = %+v, want no flags", m)
			}
		}
	}
}

func TestReadSessionMetaDirNameFallback(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "recording_20261117-101500")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Only summary.json: start time falls back to the folder-name timestamp.
	sd, _ := json.Marshal(map[string]any{"operations": 1, "endedAt": time.Now().Format(time.RFC3339Nano)})
	if err := os.WriteFile(filepath.Join(dir, "summary.json"), sd, 0o644); err != nil {
		t.Fatal(err)
	}
	m, ok := readSessionMeta(dir)
	if !ok {
		t.Fatal("summary-only folder should be a session")
	}
	want := time.Date(2026, 11, 17, 10, 15, 0, 0, time.Local)
	if !m.startedAt.Equal(want) {
		t.Fatalf("startedAt = %v, want %v", m.startedAt, want)
	}
	// Neither metadata file: not a session.
	empty := filepath.Join(root, "recording_empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := readSessionMeta(empty); ok {
		t.Fatal("folder with no metadata should not be a session")
	}
}

func TestLoadRecDetailMapsShots(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	now := time.Now().Format(time.RFC3339Nano)
	writeRecScript(t, dir, []scriptLine{
		{Seq: 1, At: now, Source: "guest", URL: "https://a.com/", Operation: operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}},
		{Seq: 2, At: now, Source: "guest", URL: "https://a.com/", Operation: operation{Type: "input", Anchor: anchor{Type: "id", Selector: "#note"}, Value: json.RawMessage(`"hello"`)}},
	})
	// Step 1 was superseded (no folder); step 2 captured both pages.
	stepDir := filepath.Join(dir, "steps", "0002")
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Primary.png", "Page 2.png"} {
		if err := os.WriteFile(filepath.Join(stepDir, name), testPNG(t, 390, 844), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	d, err := loadRecDetail(dir, 0)
	if err != nil {
		t.Fatalf("loadRecDetail: %v", err)
	}
	if len(d.steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(d.steps))
	}
	if len(d.steps[0].shots) != 0 {
		t.Fatalf("superseded step 1 has %d shots, want 0", len(d.steps[0].shots))
	}
	if len(d.steps[1].shots) != 2 {
		t.Fatalf("step 2 has %d shots, want 2", len(d.steps[1].shots))
	}
}

func TestLoadRecDetailToleratesTornLine(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), false)
	good, _ := json.Marshal(scriptLine{Seq: 1, At: time.Now().Format(time.RFC3339Nano), Source: "guest", Operation: operation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}})
	// A half-written last line (live recording) must not fail the load.
	if err := os.WriteFile(filepath.Join(dir, "script.jsonl"), append(good, []byte("\n{\"seq\":2,\"ope")...), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := loadRecDetail(dir, 0)
	if err != nil {
		t.Fatalf("torn line should be tolerated: %v", err)
	}
	if len(d.steps) != 1 {
		t.Fatalf("steps = %d, want 1 (torn line skipped)", len(d.steps))
	}
}

func TestDescribeOp(t *testing.T) {
	yes := true
	y := 300.0
	cases := []struct {
		op   operation
		lang string
		want string
	}{
		{operation{Type: "click", Anchor: anchor{Selector: "#go"}}, "en", "Click #go"},
		{operation{Type: "click", Anchor: anchor{Selector: "#go"}}, "zh", "点击 #go"},
		{operation{Type: "input", Anchor: anchor{Selector: "#note"}, Value: json.RawMessage(`"hi"`)}, "en", "Type “hi” into #note"},
		{operation{Type: "input", Anchor: anchor{Selector: "#note"}, Value: json.RawMessage(`"hi"`)}, "zh", "在 #note 输入 “hi”"},
		{operation{Type: "scroll", ScrollY: &y}, "en", "Scroll to 300"},
		{operation{Type: "submit", Anchor: anchor{Selector: "form"}}, "en", "Submit form"},
		{operation{Type: "input", Anchor: anchor{Selector: "#agree"}, Checked: &yes}, "en", "Check #agree"},
		{operation{Type: "change", Anchor: anchor{Selector: "#sel"}, Value: json.RawMessage(`42`)}, "en", "Change #sel to “42”"},
	}
	for _, tc := range cases {
		if got := describeOp(tc.op, tc.lang); got != tc.want {
			t.Errorf("describeOp(%s,%s) = %q, want %q", tc.op.Type, tc.lang, got, tc.want)
		}
	}
}

func TestDescribeOpTruncates(t *testing.T) {
	long := strings.Repeat("x", 100)
	op := operation{Type: "input", Anchor: anchor{Selector: "#" + long}, Value: json.RawMessage(`"` + long + `"`)}
	got := describeOp(op, "en")
	if strings.Contains(got, long) {
		t.Fatalf("long selector/value not truncated: %q", got)
	}
	if !strings.Contains(got, "…") {
		t.Fatalf("truncation ellipsis missing: %q", got)
	}
}

func TestExportReport(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	writeRecScript(t, dir, []scriptLine{
		{Seq: 1, At: time.Now().Format(time.RFC3339Nano), Source: "guest", URL: "https://a.com/",
			Operation: operation{Type: "input", Anchor: anchor{Type: "id", Selector: "#note"}, Value: json.RawMessage(`"a<b>&c"`)}},
	})
	stepDir := filepath.Join(dir, "steps", "0001")
	if err := os.MkdirAll(stepDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stepDir, "Primary.png"), testPNG(t, 390, 844), 0o644); err != nil {
		t.Fatal(err)
	}

	path, err := exportReport(dir, "en")
	if err != nil {
		t.Fatalf("exportReport: %v", err)
	}
	if path != filepath.Join(dir, "report.html") {
		t.Fatalf("path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	for _, want := range []string{
		`<html lang="en">`,
		"data:image/png;base64,",
		"Type “a&lt;b&gt;&amp;c” into #note", // value HTML-escaped
		"Primary",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if strings.Contains(html, "a<b>&c") {
		t.Error("report contains unescaped value text")
	}
}

func TestFileURL(t *testing.T) {
	p := filepath.Join(t.TempDir(), "report.html")
	u := fileURL(p)
	if !strings.HasPrefix(u, "file:///") {
		t.Fatalf("fileURL = %q, want file:/// prefix", u)
	}
	if !strings.HasSuffix(u, "report.html") {
		t.Fatalf("fileURL = %q, want report.html suffix", u)
	}
}

func TestDecodeThumbDownscales(t *testing.T) {
	src := decodeThumb(testPNG(t, 400, 900), 200)
	if src == nil {
		t.Fatal("decodeThumb returned nil for a valid PNG")
	}
	w, h := ui.ImageSize(src)
	if max(w, h) > 201 {
		t.Fatalf("thumbnail %vx%v not downscaled to <=200", w, h)
	}
	if decodeThumb([]byte("not a png"), 200) != nil {
		t.Fatal("decodeThumb should return nil for invalid data")
	}
}

func TestRecImageCaches(t *testing.T) {
	a := newApp()
	dir := t.TempDir()
	path := filepath.Join(dir, "Primary.png")
	if err := os.WriteFile(path, testPNG(t, 390, 844), 0o644); err != nil {
		t.Fatal(err)
	}
	first := a.recImage(path)
	if first == nil {
		t.Fatal("recImage returned nil for a valid PNG")
	}
	if second := a.recImage(path); second != first {
		t.Fatal("recImage did not return the cached source")
	}
	// A missing file caches nil so it is not retried every frame.
	missing := filepath.Join(dir, "nope.png")
	if a.recImage(missing) != nil {
		t.Fatal("recImage should return nil for a missing file")
	}
	if _, ok := a.recView.imgs[missing]; !ok {
		t.Fatal("nil result was not cached")
	}
}

func TestLoadRecDetailShotLimit(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	now := time.Now().Format(time.RFC3339Nano)
	writeRecScript(t, dir, []scriptLine{
		{Seq: 1, At: now, Source: "guest", Operation: operation{Type: "click", Anchor: anchor{Selector: "#a"}}},
		{Seq: 2, At: now, Source: "guest", Operation: operation{Type: "click", Anchor: anchor{Selector: "#b"}}},
	})
	for _, seq := range []string{"0001", "0002"} {
		sd := filepath.Join(dir, "steps", seq)
		if err := os.MkdirAll(sd, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sd, "Primary.png"), testPNG(t, 8, 8), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	d, err := loadRecDetail(dir, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.steps) != 2 || len(d.steps[0].shots) != 1 || len(d.steps[1].shots) != 0 {
		t.Fatalf("shotLimit not applied: %d steps, shots %d/%d", len(d.steps), len(d.steps[0].shots), len(d.steps[1].shots))
	}
}

func TestReadBoundedFileRefusesOversized(t *testing.T) {
	path := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readBoundedFile(path, 1024); err == nil {
		t.Fatal("oversized file accepted")
	}
	if _, err := readBoundedFile(path, 2048); err != nil {
		t.Fatalf("exact-limit file refused: %v", err)
	}
}

func TestDecodeThumbRefusesHugeIHDR(t *testing.T) {
	data := testPNG(t, 4, 4)
	// Claim 100000x100000 in IHDR and fix the chunk CRC: decoding must refuse
	// before allocating the pixel buffer.
	binary.BigEndian.PutUint32(data[16:20], 100000)
	binary.BigEndian.PutUint32(data[20:24], 100000)
	binary.BigEndian.PutUint32(data[29:33], crc32.ChecksumIEEE(data[12:29]))
	if decodeThumb(data, recThumbMaxDim) != nil {
		t.Fatal("crafted IHDR was decoded")
	}
}

func TestFileURLEscapes(t *testing.T) {
	got := fileURL(filepath.Join(t.TempDir(), "a b#c", "报告.html"))
	if !strings.HasPrefix(got, "file:///") {
		t.Fatalf("prefix missing: %q", got)
	}
	if strings.ContainsAny(got, " #") || strings.Contains(got, "报") {
		t.Fatalf("space/hash/CJK not escaped: %q", got)
	}
	if !strings.Contains(got, "%20") || !strings.Contains(got, "%23") {
		t.Fatalf("percent escapes missing: %q", got)
	}
}

func TestExportReportMissingShot(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	writeRecScript(t, dir, []scriptLine{
		{Seq: 1, At: time.Now().Format(time.RFC3339Nano), Source: "guest", Operation: operation{Type: "click", Anchor: anchor{Selector: "#go"}}},
	})
	sd := filepath.Join(dir, "steps", "0001")
	if err := os.MkdirAll(sd, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sd, "Primary.png"), []byte("not a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	path, err := exportReport(dir, "en")
	if err != nil {
		t.Fatalf("exportReport: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	html := string(data)
	if strings.Contains(html, "<img") {
		t.Fatal("unreadable shot embedded as img")
	}
	if !strings.Contains(html, ">"+trLang("en", "rec.shotMissing")+"<") {
		t.Fatalf("missing-shot placeholder absent: %s", html)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp report left behind")
	}
}

func TestRecMetaLineTornOmitsCounts(t *testing.T) {
	torn := recSessionMeta{torn: true, pages: []string{"a", "b"}, viewMode: "mobile"}
	if got := recMetaLine(torn, "en"); strings.Contains(got, trLang("en", "rec.ops", 0)) {
		t.Fatalf("torn meta shows unknown counts: %q", got)
	}
	done := recSessionMeta{pages: []string{"a"}, ops: 2, steps: 1, shots: 2}
	if got := recMetaLine(done, "en"); !strings.Contains(got, trLang("en", "rec.ops", 2)) {
		t.Fatalf("completed meta lost counts: %q", got)
	}
}

func recShotStep(seq int, typ string, withShot bool) recDetailStep {
	st := recDetailStep{line: scriptLine{Seq: seq, Operation: operation{Type: typ}}}
	if withShot {
		st.shots = []recStepShot{{name: "Primary", path: "x.png"}}
	}
	return st
}

func TestGroupRecStepsCoalescesScrolls(t *testing.T) {
	steps := []recDetailStep{
		recShotStep(1, "click", true),
		recShotStep(2, "scroll", true),
		recShotStep(3, "scroll", false), // superseded: hidden by default
		recShotStep(4, "scroll", true),
		recShotStep(5, "input", true),
		recShotStep(6, "scroll", true),
	}
	entries := groupRecSteps(steps, false)
	if len(entries) != 4 {
		t.Fatalf("entries = %d, want 4 (click, scroll run, input, scroll)", len(entries))
	}
	if entries[0].run || len(entries[0].steps) != 1 {
		t.Fatalf("click entry = %+v", entries[0])
	}
	run := entries[1]
	if !run.run || len(run.steps) != 2 || run.steps[0].line.Seq != 2 || run.steps[1].line.Seq != 4 {
		t.Fatalf("scroll run = %+v, want steps 2 and 4 coalesced", run)
	}
	if entries[2].run || entries[3].run == false || len(entries[3].steps) != 1 {
		t.Fatalf("trailing entries = %+v / %+v", entries[2], entries[3])
	}
	// Show-all keeps the no-shot scroll inside the run: 2,3,4 coalesce.
	all := groupRecSteps(steps, true)
	if len(all) != 4 || len(all[1].steps) != 3 {
		t.Fatalf("show-all entries = %d, run len = %d, want 4 / 3", len(all), len(all[1].steps))
	}
	// Underlying steps are untouched: the lightbox still sees real shots.
	if len(steps[2].shots) != 0 || len(steps[1].shots) != 1 {
		t.Fatal("grouping mutated the underlying steps")
	}
}

func TestRecSessionNamePersist(t *testing.T) {
	root := t.TempDir()
	dir := writeSession(t, root, "recording_20261118-120000", time.Now(), true)
	m, ok := readSessionMeta(dir)
	if !ok || m.name != "" {
		t.Fatalf("fresh session name = %q, want empty", m.name)
	}
	if m.title() != formatRecTitle(m.startedAt) {
		t.Fatalf("unnamed title = %q, want timestamp", m.title())
	}
	if err := renameRecSession(dir, "  登录流程  "); err != nil {
		t.Fatal(err)
	}
	m, _ = readSessionMeta(dir)
	if m.name != "登录流程" || m.title() != "登录流程" {
		t.Fatalf("renamed meta = %q / %q", m.name, m.title())
	}
	// An empty name clears the custom title.
	if err := renameRecSession(dir, "   "); err != nil {
		t.Fatal(err)
	}
	m, _ = readSessionMeta(dir)
	if m.name != "" || m.title() != formatRecTitle(m.startedAt) {
		t.Fatalf("cleared meta = %q / %q", m.name, m.title())
	}
	// Overlong names are bounded.
	if err := renameRecSession(dir, strings.Repeat("名", 200)); err != nil {
		t.Fatal(err)
	}
	m, _ = readSessionMeta(dir)
	if len([]rune(m.name)) > recNameMaxRunes+1 {
		t.Fatalf("name not bounded: %d runes", len([]rune(m.name)))
	}
}

func TestRecSessionMatches(t *testing.T) {
	m := recSessionMeta{
		dir:       filepath.Join("x", "recording_20261118-120000"),
		name:      "Login Flow",
		startedAt: time.Date(2026, 11, 18, 12, 0, 0, 0, time.Local),
		pages:     []string{"Primary"},
		urls:      []string{"https://example.com/login"},
	}
	for _, q := range []string{"", "login", "LOGIN", "example.com", "2026-11-18", "20261118", "primary"} {
		if !recSessionMatches(m, q) {
			t.Errorf("query %q should match", q)
		}
	}
	for _, q := range []string{"nomatch", "别的"} {
		if recSessionMatches(m, q) {
			t.Errorf("query %q should not match", q)
		}
	}
}

func TestRecOpIconCoversTypes(t *testing.T) {
	yes := true
	for _, op := range []operation{
		{Type: "click"}, {Type: "submit"}, {Type: "scroll"},
		{Type: "input"}, {Type: "change"}, {Type: "select"},
		{Type: "input", Checked: &yes}, {Type: "unknown"},
	} {
		if recOpIcon(op) == nil {
			t.Errorf("no icon for %s (checked=%v)", op.Type, op.Checked != nil)
		}
	}
}

func TestDeleteRecSessionRefusesLive(t *testing.T) {
	root := t.TempDir()
	live := writeSession(t, root, "recording_20261118-120000", time.Now(), false)
	a := newApp()
	a.rec = &recorder{dir: live}
	a.deleteRecSession(live)
	if _, err := os.Stat(live); err != nil {
		t.Fatalf("live session was deleted: %v", err)
	}
	a.deleteRecSession("") // empty dir is a no-op, not a RemoveAll("")
}

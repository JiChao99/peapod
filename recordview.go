package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"image"
	"image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
	"golang.org/x/image/draw"
)

// The recordings browser is a lightweight, pinky-Activity-style view of the
// evidence chain the recorder writes: a session list beside a step timeline
// with plain-language operations and per-page thumbnails, plus replay and a
// folder-relative HTML report. It deliberately has no video player — Peapod
// captures per-step PNGs, not a movie — and reads only local artifacts.
const (
	recThumbW      = 100  // thumbnail box width (DIPs); image is contained
	recThumbH      = 140  // thumbnail box height
	recThumbMaxDim = 200  // decode downscale bound, so thumbnails stay small
	recLightboxDim = 1600 // decode downscale bound for the full-size lightbox
	recImgCacheMax = 512  // decoded thumbnails kept before the cache is cleared
	recMaxSteps    = 100  // steps rendered in the timeline; export includes all

	recShotMaxBytes  = 32 << 20 // one screenshot read cap, so corrupt files stay bounded
	recShotMaxPixels = 32 << 20 // width*height cap before decode: a crafted IHDR must not OOM
	recMetaMaxBytes  = 1 << 20  // session.json / summary.json read cap
	recNameMaxBytes  = 4096     // name.txt read cap: a custom session name is short
	recNameMaxRunes  = 80       // custom name length bound, so list rows stay tidy
)

// recBrowser is the viewer's UI-owned state, touched only on the main thread
// in view(). It lives on app as a value so app.go needs no ui import.
type recBrowser struct {
	open     bool                      // stage shows the browser instead of pages
	sel      string                    // selected session dir
	list     []recSessionMeta          // scanned sessions, newest first
	detail   *recDetail                // lazy-loaded steps for sel
	imgs     map[string]ui.ImageSource // decoded thumbnail cache (nil = unreadable)
	lightbox *recLightbox              // full-size shot viewer, nil when closed

	search      string // session list filter query (name / pages / date)
	showNoShots bool   // timeline filter: also show steps without screenshots
	renameDir   string // session with the inline rename input open ("" = none)
	renameText  string // draft custom name bound to the rename input
	deleteDir   string // session pending the delete/discard confirm modal
	deleteOpen  bool   // confirm modal open flag, owned by feedback.Modal
}

// recLightbox holds one step's shots opened at a clicked thumbnail. The decoded
// full-size source is cached on the struct and reloaded only when the shot
// changes, so a large PNG is not decoded every frame. UI-owned, main thread only.
type recLightbox struct {
	shots []recStepShot
	index int
	desc  string
	seq   int
	at    string // step's recorded time (RFC3339), shown as a caption
	src   ui.ImageSource
	path  string
}

// recSummary mirrors summary.json (record.go finish).
type recSummary struct {
	EndedAt       string `json:"endedAt"`
	Operations    int    `json:"operations"`
	Dispatched    int    `json:"dispatched"`
	StepsCaptured int    `json:"stepsCaptured"`
	ShotsSaved    int    `json:"shotsSaved"`
	ShotsFailed   int    `json:"shotsFailed"`
	Superseded    int    `json:"superseded"`
	Dropped       int    `json:"dropped"`
	WriteError    string `json:"writeError"`
	SessionError  string `json:"sessionError"`
}

// recSessionInfo mirrors session.json (record.go writeSessionJSON).
type recSessionInfo struct {
	TaskID    string `json:"taskId"`
	StartedAt string `json:"startedAt"`
	ViewMode  string `json:"viewMode"`
	UserAgent string `json:"userAgent"`
	Pages     []struct {
		ID        string `json:"id"`
		Name      string `json:"name"`
		URL       string `json:"url"`
		IsPrimary bool   `json:"isPrimary"`
	} `json:"pages"`
}

// recSessionMeta is one session folder's listing entry, read from the small
// metadata files only — never the script or screenshots — so the list stays
// cheap however many recordings exist.
type recSessionMeta struct {
	dir       string
	name      string // custom name from name.txt, preferred over the timestamp
	startedAt time.Time
	endedAt   time.Time
	ops       int
	steps     int
	shots     int
	failed    int
	pages     []string
	urls      []string // page URLs, for the list search
	viewMode  string
	torn      bool // summary.json missing: live or interrupted
	recording bool // torn and the recorder's current session
	writeErr  string
	sessErr   string
}

// title prefers the user's custom name over the recorded start timestamp.
func (m recSessionMeta) title() string {
	if m.name != "" {
		return m.name
	}
	return formatRecTitle(m.startedAt)
}

// markLive distinguishes the recorder's current session from older sessions
// interrupted before they could write a summary.
func (m *recSessionMeta) markLive(live string) {
	m.recording = m.torn && m.dir == live
}

// renameRecSession persists a custom session name in name.txt. An empty name
// clears it, falling back to the timestamp title. The write is atomic (tmp +
// rename) so a crash can't leave a partial name, and it never touches the
// recorder's own files.
func renameRecSession(dir, name string) error {
	name = strings.TrimSpace(name)
	path := filepath.Join(dir, "name.txt")
	if name == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(truncateRunes(name, recNameMaxRunes)), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// recSessionMatches reports whether a session matches the list search query
// (case-insensitive): custom name, timestamp title, folder date, page names
// or page URLs. An empty query matches everything.
func recSessionMatches(m recSessionMeta, q string) bool {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return true
	}
	for _, s := range []string{m.name, formatRecTitle(m.startedAt), filepath.Base(m.dir)} {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	for _, s := range m.pages {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	for _, s := range m.urls {
		if strings.Contains(strings.ToLower(s), q) {
			return true
		}
	}
	return false
}

// recStepShot is one page's screenshot for a step.
type recStepShot struct {
	name string // page name (file stem)
	path string
}

// recDetailStep is one script line plus the screenshots captured for it. A
// superseded or dropped step has no shots but still appears in the timeline.
type recDetailStep struct {
	line  scriptLine
	shots []recStepShot
}

// recDetail is a session's full step timeline, loaded on selection.
type recDetail struct {
	meta  recSessionMeta
	steps []recDetailStep
}

// recordingsRoot is where the recorder writes sessions (record.go).
func recordingsRoot() string { return filepath.Join(userArtifactRoot(), "recordings") }

// scanRecordings lists session folders under root, newest first. Folders that
// are not recognizable sessions are skipped rather than failing the list.
func scanRecordings(root string) []recSessionMeta {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []recSessionMeta
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "recording_") {
			continue
		}
		if m, ok := readSessionMeta(filepath.Join(root, e.Name())); ok {
			out = append(out, m)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].startedAt.After(out[j].startedAt) })
	return out
}

// readSessionMeta reads summary.json and session.json, tolerating either being
// absent (a live recording has no summary yet). A folder with neither is not a
// session. The folder-name timestamp is the fallback start time.
func readSessionMeta(dir string) (recSessionMeta, bool) {
	m := recSessionMeta{dir: dir}
	sessData, sessErr := readBoundedFile(filepath.Join(dir, "session.json"), recMetaMaxBytes)
	sumData, sumErr := readBoundedFile(filepath.Join(dir, "summary.json"), recMetaMaxBytes)
	if sessErr != nil && sumErr != nil {
		return m, false
	}
	if sessErr == nil {
		var si recSessionInfo
		if json.Unmarshal(sessData, &si) == nil {
			m.startedAt = parseRecTime(si.StartedAt)
			m.viewMode = si.ViewMode
			for _, p := range si.Pages {
				m.pages = append(m.pages, p.Name)
				m.urls = append(m.urls, p.URL)
			}
		}
	}
	// A custom name lives in its own small file so renaming never rewrites
	// the recorder's session.json (which a live recorder still owns).
	if data, err := readBoundedFile(filepath.Join(dir, "name.txt"), recNameMaxBytes); err == nil {
		m.name = strings.TrimSpace(string(data))
	}
	if sumErr == nil {
		var s recSummary
		if json.Unmarshal(sumData, &s) == nil {
			m.ops, m.steps, m.shots, m.failed = s.Operations, s.StepsCaptured, s.ShotsSaved, s.ShotsFailed
			m.writeErr, m.sessErr = s.WriteError, s.SessionError
			m.endedAt = parseRecTime(s.EndedAt)
		}
	} else {
		m.torn = true
	}
	if m.startedAt.IsZero() {
		m.startedAt = timeFromDirName(filepath.Base(dir))
	}
	return m, true
}

// loadRecDetail reads the script and maps step screenshots. A torn last line
// (the recorder is mid-write) is skipped, so a live session still loads. The
// read is bounded like loadScript so a huge file cannot exhaust memory.
// shotLimit caps how many steps get their shots listed (0 = all): the timeline
// renders only recMaxSteps, while the export embeds every step.
func loadRecDetail(dir string, shotLimit int) (*recDetail, error) {
	meta, ok := readSessionMeta(dir)
	if !ok {
		return nil, fmt.Errorf("not a recording session: %s", dir)
	}
	f, err := os.Open(filepath.Join(dir, "script.jsonl"))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := readAllBounded(f, maxScriptBytes)
	if err != nil {
		return nil, err
	}
	d := &recDetail{meta: meta}
	for _, raw := range strings.Split(string(data), "\n") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var sl scriptLine
		if json.Unmarshal([]byte(raw), &sl) != nil {
			continue
		}
		step := recDetailStep{line: sl}
		if shotLimit > 0 && len(d.steps) >= shotLimit {
			d.steps = append(d.steps, step)
			continue
		}
		stepDir := filepath.Join(dir, "steps", fmt.Sprintf("%04d", sl.Seq))
		if entries, err := os.ReadDir(stepDir); err == nil {
			for _, e := range entries {
				if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".png") {
					continue
				}
				name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
				step.shots = append(step.shots, recStepShot{name: name, path: filepath.Join(stepDir, e.Name())})
			}
		}
		d.steps = append(d.steps, step)
	}
	return d, nil
}

// readAllBounded reads up to limit bytes, failing if the file is larger.
func readAllBounded(f *os.File, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d bytes", limit)
	}
	return data, nil
}

// readBoundedFile reads a whole file, refusing ones over limit.
func readBoundedFile(path string, limit int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readAllBounded(f, limit)
}

func parseRecTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func timeFromDirName(base string) time.Time {
	t, err := time.ParseInLocation("20060102-150405", strings.TrimPrefix(base, "recording_"), time.Local)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatRecTitle(t time.Time) string {
	if t.IsZero() {
		return "—"
	}
	return t.Local().Format("2006-01-02 15:04:05")
}

func recClock(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("15:04:05")
}

// recMetaLine is the compact counts line shown in the list and detail header.
func recMetaLine(m recSessionMeta, lang string) string {
	parts := []string{trLang(lang, "rec.pages", len(m.pages))}
	if m.viewMode != "" {
		mode := "ui.viewMode.desktop"
		if m.viewMode == "mobile" {
			mode = "ui.viewMode.mobile"
		}
		parts = append(parts, trLang(lang, mode))
	}
	if !m.torn { // counts are unknown until the summary lands
		parts = append(parts,
			trLang(lang, "rec.ops", m.ops),
			trLang(lang, "rec.steps", m.steps),
			trLang(lang, "rec.shots", m.shots),
		)
		if m.failed > 0 {
			parts = append(parts, trLang(lang, "rec.failed", m.failed))
		}
	}
	return strings.Join(parts, " · ")
}

// describeOp renders an operation in plain words for the timeline and report.
func describeOp(op operation, lang string) string {
	sel := truncateRunes(op.Anchor.Selector, 48)
	switch op.Type {
	case "click":
		return trLang(lang, "op.click", sel)
	case "submit":
		return trLang(lang, "op.submit", sel)
	case "scroll":
		return trLang(lang, "op.scroll", scrollCoords(op))
	case "input", "change", "select":
		if op.Checked != nil {
			if *op.Checked {
				return trLang(lang, "op.check", sel)
			}
			return trLang(lang, "op.uncheck", sel)
		}
		return trLang(lang, "op."+op.Type, opValueString(op.Value), sel)
	default:
		return op.Type + " " + sel
	}
}

// opValueString decodes a raw JSON value to readable text: a string is
// unquoted, anything else shown as its JSON literal, both bounded.
func opValueString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return truncateRunes(s, 40)
	}
	return truncateRunes(strings.TrimSpace(string(raw)), 40)
}

func scrollCoords(op operation) string {
	var x, y float64
	if op.ScrollX != nil {
		x = *op.ScrollX
	}
	if op.ScrollY != nil {
		y = *op.ScrollY
	}
	if x != 0 {
		return fmt.Sprintf("%.0f, %.0f", x, y)
	}
	return fmt.Sprintf("%.0f", y)
}

// recOpIcon maps an operation to a timeline type icon, mirroring describeOp's
// type switch so the icon and the plain-language label never disagree.
func recOpIcon(op operation) *ui.SVG {
	switch op.Type {
	case "click":
		return lucide.MousePointerClick()
	case "submit":
		return lucide.Send()
	case "scroll":
		return lucide.MoveVertical()
	case "input", "change", "select":
		if op.Checked != nil {
			return lucide.SquareCheck()
		}
		switch op.Type {
		case "input":
			return lucide.TextCursorInput()
		case "select":
			return lucide.ChevronsUpDown()
		default:
			return lucide.Type()
		}
	default:
		return lucide.CircleDot()
	}
}

// recTimelineEntry is a VIEW-layer grouping of timeline steps: consecutive
// scroll operations coalesce into one entry, everything else stays individual.
// The underlying recDetail steps are untouched, so the lightbox still opens
// and navigates the real per-step screenshots.
type recTimelineEntry struct {
	steps []recDetailStep
	run   bool // a scroll group (rendered coalesced only when len(steps) > 1)
}

// groupRecSteps builds the timeline entries. Steps without screenshots are
// dropped unless showNoShots is set; consecutive scrolls among the remaining
// steps merge into one entry.
func groupRecSteps(steps []recDetailStep, showNoShots bool) []recTimelineEntry {
	var out []recTimelineEntry
	gap := false
	for _, st := range steps {
		scroll := st.line.Operation.Type == "scroll"
		if !showNoShots && len(st.shots) == 0 {
			// A hidden non-scroll op breaks consecutiveness: merging across it
			// would label two separate scroll runs as one.
			if !scroll {
				gap = true
			}
			continue
		}
		if scroll && !gap && len(out) > 0 && out[len(out)-1].run {
			out[len(out)-1].steps = append(out[len(out)-1].steps, st)
		} else {
			out = append(out, recTimelineEntry{steps: []recDetailStep{st}, run: scroll})
		}
		gap = false
	}
	return out
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func recSourceLabel(src, lang string) string {
	switch src {
	case "host":
		return trLang(lang, "rec.src.host")
	case "replay":
		return trLang(lang, "rec.src.replay")
	case "guest":
		return trLang(lang, "rec.src.guest")
	default:
		return src
	}
}

func recSourceColor(src string) display.TagColor {
	switch src {
	case "host":
		return display.TagProcessing
	case "replay":
		return display.TagSuccess
	default:
		return display.TagGray
	}
}

// decodeThumb decodes a PNG and downscales it so its longest side is at most
// maxDim, keeping thumbnail memory bounded regardless of the page resolution.
func decodeThumb(data []byte, maxDim int) ui.ImageSource {
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > recShotMaxPixels {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return nil
	}
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if m := max(w, h); m > maxDim {
		scale := float64(maxDim) / float64(m)
		tw := max(1, int(float64(w)*scale))
		th := max(1, int(float64(h)*scale))
		dst := image.NewRGBA(image.Rect(0, 0, tw, th))
		draw.ApproxBiLinear.Scale(dst, dst.Bounds(), img, b, draw.Over, nil)
		img = dst
	}
	return ui.NewBitmap(img)
}

// recImage returns a cached, downscaled thumbnail for a screenshot path, or
// nil when it cannot be read/decoded. Nil results are cached too so a broken
// file is not retried every frame. Main thread only.
func (a *app) recImage(path string) ui.ImageSource {
	if a.recView.imgs == nil {
		a.recView.imgs = map[string]ui.ImageSource{}
	}
	if src, ok := a.recView.imgs[path]; ok {
		return src
	}
	var src ui.ImageSource
	if data, err := readBoundedFile(path, recShotMaxBytes); err == nil {
		src = decodeThumb(data, recThumbMaxDim)
	}
	if len(a.recView.imgs) >= recImgCacheMax {
		a.recView.imgs = map[string]ui.ImageSource{}
	}
	a.recView.imgs[path] = src
	return src
}

// openRecLightbox opens the full-size viewer on one step's shots at index.
func (a *app) openRecLightbox(st recDetailStep, index int) {
	if index < 0 || index >= len(st.shots) {
		return
	}
	a.recView.lightbox = &recLightbox{
		shots: st.shots, index: index,
		desc: describeOp(st.line.Operation, a.currentLang()), seq: st.line.Seq,
		at: st.line.At,
	}
	a.loadLightboxImage()
	a.invalidate()
}

// loadLightboxImage decodes the current shot at full size, once per shot change.
func (a *app) loadLightboxImage() {
	lb := a.recView.lightbox
	if lb == nil || lb.index < 0 || lb.index >= len(lb.shots) {
		return
	}
	sh := lb.shots[lb.index]
	if lb.path == sh.path {
		return
	}
	lb.path = sh.path
	lb.src = nil
	if data, err := readBoundedFile(sh.path, recShotMaxBytes); err == nil {
		lb.src = decodeThumb(data, recLightboxDim)
	}
}

// lightboxStep moves to the previous/next shot, wrapping around the step.
func (a *app) lightboxStep(delta int) {
	lb := a.recView.lightbox
	if lb == nil || len(lb.shots) == 0 {
		return
	}
	lb.index = (lb.index + delta + len(lb.shots)) % len(lb.shots)
	lb.path = "" // force a reload for the new shot
	a.loadLightboxImage()
	a.invalidate()
}

func (a *app) closeRecLightbox() {
	a.recView.lightbox = nil
	a.invalidate()
}

// renderRecLightbox paints the full-size shot viewer over the window. Escape
// closes it and Left/Right move between the step's shots; the prev/next/close
// buttons are focusable so the whole viewer is keyboard reachable.
func (a *app) renderRecLightbox(c *ui.Context, tk *theme.Tokens) {
	lb := a.recView.lightbox
	if lb == nil || len(lb.shots) == 0 {
		return
	}
	winW, winH := c.Size()
	boxW := min(winW-220, 1100)
	boxH := min(winH-220, 820)
	if boxW < 80 {
		boxW = 80
	}
	if boxH < 80 {
		boxH = 80
	}
	ui.Overlay(c, func() {
		back := ui.Column(c).Fill().Center().Padding(24).Background(tk.BgContainer.Alpha(0.72))
		if back.OverlayShortcut(0, ui.KeyEscape) {
			a.closeRecLightbox()
			return
		}
		if back.OverlayShortcut(0, ui.KeyLeft) {
			a.lightboxStep(-1)
		}
		if back.OverlayShortcut(0, ui.KeyRight) {
			a.lightboxStep(1)
		}
		panel := layout.Block(c, layout.BlockProps{Variant: layout.BlockFilled})
		panel.Radius(12).Padding(16).Gap(12).Background(tk.BgElevated)
		// Reclaim the keyboard from a guest WebView2 that may hold it: the
		// panel is focusable (but not a Tab stop before its own buttons) and
		// takes the focus in the frame it appears, so Esc/Left/Right land on
		// the overlay instead of the page underneath.
		panel.TabStop(false).AutoFocus()
		tk.Elevate(panel, theme.BoxShadow)
		panel.Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, lb.desc).FontSize(14).Bold().TextColor(tk.TextHeading).SingleLine()
					caption := a.tr("rec.lightbox.counter", lb.index+1, len(lb.shots), lb.shots[lb.index].name)
					if clk := recClock(parseRecTime(lb.at)); clk != "" {
						caption += " · " + clk
					}
					ui.Text(c, caption).FontSize(12).TextColor(tk.TextTertiary).SingleLine()
				})
				a.iconButton(c, tk, lucide.X(), a.tr("rec.lightbox.close"), func() { a.closeRecLightbox() })
			})
			ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
				a.iconButton(c, tk, lucide.ChevronLeft(), a.tr("rec.lightbox.prev"), func() { a.lightboxStep(-1) })
				ui.Column(c).Width(boxW).Height(boxH).Center().Radius(8).Clip().
					Background(tk.BgContainerSecondary).Children(func() {
					if lb.src != nil {
						ui.Image(c, lb.src).Size(boxW, boxH).Fit(ui.Contain)
					} else {
						ui.Text(c, a.tr("rec.shotMissing")).FontSize(13).TextColor(tk.TextQuaternary)
					}
				})
				a.iconButton(c, tk, lucide.ChevronRight(), a.tr("rec.lightbox.next"), func() { a.lightboxStep(1) })
			})
		})
	})
}

func recListHas(list []recSessionMeta, dir string) bool {
	for _, m := range list {
		if m.dir == dir {
			return true
		}
	}
	return false
}

// toggleRecordings swaps the stage between the guest pages and the browser.
func (a *app) toggleRecordings() {
	a.recView.open = !a.recView.open
	if a.recView.open {
		a.refreshRecList()
	}
	a.invalidate()
}

// refreshRecList rescans the recordings root and selects the live recording if
// one is running, else keeps a still-present selection, else the newest.
func (a *app) refreshRecList() {
	a.recView.list = scanRecordings(recordingsRoot())
	// A live session's shots land after first paint; a cached nil must not
	// pin them as missing until the cache evicts.
	for p, src := range a.recView.imgs {
		if src == nil {
			delete(a.recView.imgs, p)
		}
	}
	live := a.liveRecDir()
	for i := range a.recView.list {
		a.recView.list[i].markLive(live)
	}
	switch {
	case live != "" && recListHas(a.recView.list, live):
		a.openRecSession(live)
	case recListHas(a.recView.list, a.recView.sel):
		a.openRecSession(a.recView.sel)
	case len(a.recView.list) > 0:
		a.openRecSession(a.recView.list[0].dir)
	default:
		a.recView.sel = ""
		a.recView.detail = nil
	}
}

// liveRecDir is the session the recorder is writing right now, if any.
func (a *app) liveRecDir() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.rec != nil {
		return a.rec.dir
	}
	return ""
}

// openRecSession selects a session and loads its timeline.
func (a *app) openRecSession(dir string) {
	a.recView.sel = dir
	a.recView.detail = nil
	if dir == "" {
		return
	}
	d, err := loadRecDetail(dir, recMaxSteps)
	if err != nil {
		a.setNoticeErr(err.Error())
		return
	}
	d.meta.markLive(a.liveRecDir())
	a.recView.detail = d
}

func (a *app) replayRecording(dir string) {
	// A recording re-enacts operations against whatever the pages show now. If
	// the primary has drifted from the recorded entry URL, the first anchors may
	// not exist, so offer to return to the entry before replaying.
	if entry := recEntryURL(dir); entry != "" {
		if cur := a.primaryURL(); cur != "" && !replayEntryMatches(cur, entry) {
			a.replayPendingDir = dir
			a.replayEntryURL = entry
			a.replayConfirmOpen = true
			return
		}
	}
	a.beginReplay(dir)
}

// beginReplay starts the replay and switches back to the pages so it is visible.
func (a *app) beginReplay(dir string) {
	_, err := a.startReplay(filepath.Join(dir, "script.jsonl"), defaultReplayDelay)
	if err != nil {
		a.setNoticeErr(err.Error())
		return
	}
	a.recView.open = false // switch back to the pages so the replay is visible
	a.invalidate()
}

// primaryURL reads the primary page's current URL under the task lock.
func (a *app) primaryURL() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if p := a.pages[a.primaryID]; p != nil {
		return p.url
	}
	return ""
}

// recEntryURL reads the recorded primary entry URL from session.json. An absent
// or unreadable session yields "" so the precheck is skipped rather than guessed.
func recEntryURL(dir string) string {
	data, err := readBoundedFile(filepath.Join(dir, "session.json"), recMetaMaxBytes)
	if err != nil {
		return ""
	}
	var si recSessionInfo
	if json.Unmarshal(data, &si) != nil {
		return ""
	}
	for _, p := range si.Pages {
		if p.IsPrimary && p.URL != "" {
			return p.URL
		}
	}
	return ""
}

// replayEntryMatches compares two URLs by scheme, host and path (ignoring query,
// fragment and a trailing slash), so a harmless query difference does not trip
// the precheck while a genuinely different page does.
func replayEntryMatches(cur, entry string) bool {
	cu, err1 := url.Parse(cur)
	eu, err2 := url.Parse(entry)
	if err1 != nil || err2 != nil {
		return cur == entry
	}
	norm := func(u *url.URL) string {
		return strings.ToLower(u.Scheme) + "://" + strings.ToLower(u.Host) + strings.TrimSuffix(u.Path, "/")
	}
	return norm(cu) == norm(eu)
}

func (a *app) exportRecording(c *ui.Context, dir string) {
	path, err := exportReport(dir, a.currentLang())
	if err != nil {
		a.setNoticeErr(err.Error())
		return
	}
	a.setNoticeOpenKey("success", "notice.recExported", path, false, path)
	c.OpenURL(fileURL(path))
}

// fileURL turns a local path into a file:// URL the OS can open in a browser.
func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	p := filepath.ToSlash(abs)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p // Windows drive path needs the empty authority slash
	}
	u := url.URL{Scheme: "file", Path: p}
	return u.String()
}

// recToggleButton is the stage toolbar switch into the recordings browser.
func (a *app) recToggleButton(c *ui.Context, tk *theme.Tokens) {
	b := ui.ButtonBase(c).Label(a.tr("ui.rec.toggle")).Tooltip(a.tr("ui.rec.toggle")).Size(40, 40).Center().
		Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer)
	if a.recView.open {
		b.TextColor(tk.Primary).Background(tk.PrimaryFillTertiary)
	} else {
		b.TextColor(tk.TextSecondary)
		if b.Hovered() {
			b.TextColor(tk.Primary).Background(tk.FillTertiary)
		}
	}
	b.Children(func() { ui.Icon(c, lucide.History()).FontSize(16) })
	if b.Clicked() {
		a.toggleRecordings()
	}
}

// renderRecordings draws the master-detail browser into the stage area.
func (a *app) renderRecordings(c *ui.Context, tk *theme.Tokens) {
	ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
		a.renderRecList(c, tk)
		a.renderRecDetail(c, tk)
	})
	a.renderRecDeleteConfirm(c, tk)
}

func (a *app) renderRecList(c *ui.Context, tk *theme.Tokens) {
	ui.Column(c).Width(300).Shrink(0).BorderWidth(0, 1, 0, 0).BorderColor(tk.BorderSecondary).
		Background(tk.BgContainer).Children(func() {
		ui.Row(c).Height(44).Padding(0, 12).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("rec.list")).FontSize(13).Bold().TextColor(tk.TextHeading).Grow(1)
			a.iconButton(c, tk, lucide.RefreshCw(), a.tr("rec.refresh"), func() {
				a.refreshRecList()
				a.invalidate()
			})
		})
		ui.Row(c).Padding(0, 12, 8, 12).Children(func() {
			field.Input(c, &a.recView.search, field.InputProps{
				Placeholder: a.tr("rec.search.placeholder"),
				Prefix:      lucide.Search(),
				Size:        theme.Small,
			}).Grow(1)
		})
		ui.Scroll(c).Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).Gap(8).Padding(12).AlignItems(ui.Stretch).Children(func() {
				if len(a.recView.list) == 0 {
					ui.Text(c, a.tr("rec.empty")).FontSize(12).TextColor(tk.TextTertiary)
					return
				}
				shown := 0
				for _, m := range a.recView.list {
					m := m
					if !recSessionMatches(m, a.recView.search) {
						continue
					}
					shown++
					a.renderRecCard(c, tk, m)
				}
				if shown == 0 {
					ui.Text(c, a.tr("rec.search.empty")).FontSize(12).TextColor(tk.TextTertiary)
				}
			})
		})
	})
}

// renderRecCard draws one session's listing card: title (custom name preferred),
// status, counts, the torn/live reason, and the rename / delete actions. Only
// metadata is read here — never scripts or screenshots.
func (a *app) renderRecCard(c *ui.Context, tk *theme.Tokens, m recSessionMeta) {
	ck := c.Key(m.dir)
	card := clickableCard(ck, tk, m.title(), m.dir == a.recView.sel, func() {
		a.openRecSession(m.dir)
		a.invalidate()
	})
	card.AlignSelf(ui.Stretch).Padding(12).Gap(6)
	card.Children(func() {
		if a.recView.renameDir == m.dir {
			ui.Row(ck).Gap(4).AlignItems(ui.Center).Children(func() {
				field.Input(ck, &a.recView.renameText, field.InputProps{
					Placeholder: a.tr("rec.rename.placeholder"),
					Size:        theme.Small,
				}).Grow(1).MinWidth(0).OnShortcut(0, ui.KeyEscape, func() {
					// Esc cancels the rename, not the whole overlay: the input's
					// focus subtree claims the key before the panel's handler.
					a.recView.renameDir = ""
					a.invalidate()
				})
				a.recSmallIcon(ck, tk, lucide.Check(), a.tr("rec.rename.save"), func() {
					if err := renameRecSession(m.dir, a.recView.renameText); err != nil {
						a.setNoticeErr(err.Error())
						return
					}
					a.recView.renameDir = ""
					a.refreshRecList()
					a.setNoticeOKKey("notice.recRenamed")
					a.invalidate()
				})
				a.recSmallIcon(ck, tk, lucide.X(), a.tr("rec.rename.cancel"), func() {
					a.recView.renameDir = ""
					a.invalidate()
				})
			})
		} else {
			ui.Row(ck).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(ck, m.title()).FontSize(13).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
				a.recStatusTag(ck, m)
			})
		}
		ui.Text(ck, recMetaLine(m, a.currentLang())).FontSize(11).TextColor(tk.TextTertiary).SingleLine()
		if m.recording {
			ui.Text(ck, a.tr("rec.reason.live")).FontSize(11).TextColor(tk.Error).Wrap()
		} else if m.torn {
			ui.Text(ck, a.tr("rec.reason.torn")).FontSize(11).TextColor(tk.Warning).Wrap()
		}
		ui.Row(ck).Gap(4).AlignItems(ui.Center).Children(func() {
			a.recSmallIcon(ck, tk, lucide.Pencil(), a.tr("rec.rename"), func() {
				a.recView.renameDir = m.dir
				a.recView.renameText = m.name
				a.invalidate()
			})
			// The live recording is never deletable: its evidence is still
			// being written. Torn non-live sessions offer 丢弃 instead.
			if !m.recording {
				label := a.tr("rec.delete")
				if m.torn {
					label = a.tr("rec.discard")
				}
				if action.Button(ck, label, action.ButtonProps{Variant: action.Text, Size: theme.Small}).Clicked() {
					a.recView.deleteDir = m.dir
					a.recView.deleteOpen = true
					a.invalidate()
				}
			}
		})
	})
}

// recSmallIcon is a compact 32px icon button for the dense session cards.
func (a *app) recSmallIcon(c *ui.Context, tk *theme.Tokens, svg *ui.SVG, label string, onClick func()) {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(32, 32).Center().
		Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextSecondary)
	if b.Hovered() {
		b.TextColor(tk.Primary).Background(tk.FillTertiary)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(14) })
	if b.Clicked() {
		onClick()
	}
}

// renderRecDeleteConfirm gates deleting (or discarding a torn) session behind
// a confirm modal; only its OK path ever reaches os.RemoveAll.
func (a *app) renderRecDeleteConfirm(c *ui.Context, tk *theme.Tokens) {
	if !a.recView.deleteOpen {
		return
	}
	dir := a.recView.deleteDir
	torn := false
	for _, m := range a.recView.list {
		if m.dir == dir {
			torn = m.torn
			break
		}
	}
	title, okText := a.tr("rec.delete.title"), a.tr("rec.delete.ok")
	if torn {
		title, okText = a.tr("rec.discard.title"), a.tr("rec.discard.ok")
	}
	if feedback.Modal(c, &a.recView.deleteOpen, feedback.ModalProps{
		Title:      title,
		OkText:     okText,
		CancelText: a.tr("ui.cancel"),
		Width:      440,
	}, func() {
		ui.Text(c, a.tr("rec.delete.body", filepath.Base(dir))).FontSize(13).TextColor(tk.TextSecondary).Wrap()
	}) {
		a.recView.deleteOpen = false
		a.deleteRecSession(dir)
	}
}

// deleteRecSession removes a session folder after the confirm modal. The live
// recording is refused again here even if the UI somehow offered it.
func (a *app) deleteRecSession(dir string) {
	if dir == "" || dir == a.liveRecDir() {
		a.setNoticeWarnKey("rec.delete.refused")
		return
	}
	if err := os.RemoveAll(dir); err != nil {
		a.setNoticeErr(err.Error())
		return
	}
	if a.recView.renameDir == dir {
		a.recView.renameDir = ""
	}
	if a.recView.sel == dir {
		a.recView.sel = ""
	}
	// Drop the deleted session's decoded thumbnails so they do not linger.
	prefix := dir + string(filepath.Separator)
	for p := range a.recView.imgs {
		if strings.HasPrefix(p, prefix) {
			delete(a.recView.imgs, p)
		}
	}
	a.refreshRecList()
	a.setNoticeOKKey("notice.recDeleted")
	a.invalidate()
}

// recStatusTag tags sessions without a summary: the live recording, or one
// interrupted before it could finish. Completed sessions get no tag.
func (a *app) recStatusTag(c *ui.Context, m recSessionMeta) {
	switch {
	case m.recording:
		display.Tag(c, a.tr("rec.recording"), display.TagProps{Color: display.TagError, Size: theme.Small, Round: true})
	case m.torn:
		display.Tag(c, a.tr("rec.unfinished"), display.TagProps{Color: display.TagWarning, Size: theme.Small, Round: true})
	}
	if m.writeErr != "" || m.sessErr != "" {
		display.Tag(c, a.tr("rec.errTag"), display.TagProps{Color: display.TagWarning, Size: theme.Small, Round: true})
	}
}

func (a *app) renderRecDetail(c *ui.Context, tk *theme.Tokens) {
	ui.Scroll(c).Grow(1).MinWidth(0).Children(func() {
		d := a.recView.detail
		if d == nil {
			ui.Column(c).Fill().Center().Padding(24).Gap(10).Children(func() {
				ui.Icon(c, lucide.History()).FontSize(40).TextColor(tk.TextQuaternary)
				ui.Text(c, a.tr("rec.noDetail")).FontSize(13).TextColor(tk.TextTertiary)
			})
			return
		}
		ui.Column(c).Gap(12).Padding(16).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Gap(8).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, d.meta.title()).FontSize(18).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
					a.recStatusTag(c, d.meta)
				})
				ui.Text(c, recMetaLine(d.meta, a.currentLang())).FontSize(12).TextColor(tk.TextTertiary)
				if d.meta.recording {
					ui.Text(c, a.tr("rec.reason.live")).FontSize(12).TextColor(tk.Error).Wrap()
				} else if d.meta.torn {
					ui.Text(c, a.tr("rec.reason.torn")).FontSize(12).TextColor(tk.Warning).Wrap()
				}
				if d.meta.writeErr != "" {
					ui.Text(c, a.tr("rec.writeErr", d.meta.writeErr)).FontSize(12).TextColor(tk.Warning).Wrap()
				}
				if d.meta.sessErr != "" {
					ui.Text(c, a.tr("rec.sessErr", d.meta.sessErr)).FontSize(12).TextColor(tk.Warning).Wrap()
				}
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					if action.Button(c, a.tr("rec.replay"), action.ButtonProps{Variant: action.Primary, Size: theme.Small}).Clicked() {
						a.replayRecording(d.meta.dir)
					}
					if action.Button(c, a.tr("rec.export"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
						a.exportRecording(c, d.meta.dir)
					}
					filterLabel := a.tr("rec.showAllSteps")
					if a.recView.showNoShots {
						filterLabel = a.tr("rec.onlyShots")
					}
					if action.Button(c, filterLabel, action.ButtonProps{Variant: action.Text, Size: theme.Small}).Clicked() {
						a.recView.showNoShots = !a.recView.showNoShots
						a.invalidate()
					}
				})
			})
			shown := min(len(d.steps), recMaxSteps)
			entries := groupRecSteps(d.steps[:shown], a.recView.showNoShots)
			if !a.recView.showNoShots {
				hidden := 0
				for _, st := range d.steps[:shown] {
					if len(st.shots) == 0 {
						hidden++
					}
				}
				if hidden > 0 {
					ui.Text(c, a.tr("rec.hiddenNoShots", hidden)).FontSize(12).TextColor(tk.TextTertiary)
				}
			}
			for _, e := range entries {
				a.renderRecEntry(c, tk, e)
			}
			if len(d.steps) > shown {
				ui.Text(c, a.tr("rec.truncated", shown, len(d.steps))).FontSize(12).TextColor(tk.TextTertiary)
			}
		})
	})
}

// renderRecEntry draws one timeline entry: a single step, or a coalesced run
// of consecutive scrolls shown as "滚动 ×N · y0→y1". Every thumbnail still
// opens the lightbox on its own real step, so shot navigation is unchanged.
func (a *app) renderRecEntry(c *ui.Context, tk *theme.Tokens, e recTimelineEntry) {
	first := e.steps[0]
	last := e.steps[len(e.steps)-1]
	c = c.Key(strconv.Itoa(first.line.Seq))
	card := layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined})
	card.AlignSelf(ui.Stretch).Padding(12).Gap(10)
	card.Children(func() {
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Size(26, 26).Center().Radius(13).Background(tk.FillTertiary).Shrink(0).Children(func() {
				ui.Text(c, strconv.Itoa(first.line.Seq)).FontSize(12).Bold().TextColor(tk.TextSecondary)
			})
			ui.Icon(c, recOpIcon(first.line.Operation)).FontSize(16).TextColor(tk.TextSecondary).Shrink(0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
				title := describeOp(first.line.Operation, a.currentLang())
				clock := recClock(parseRecTime(first.line.At))
				if e.run && len(e.steps) > 1 {
					title = a.tr("rec.scrollRun", len(e.steps),
						scrollCoords(first.line.Operation), scrollCoords(last.line.Operation))
					if end := recClock(parseRecTime(last.line.At)); end != "" && end != clock {
						if clock == "" {
							clock = end
						} else {
							clock += "→" + end
						}
					}
				}
				ui.Text(c, title).FontSize(14).Bold().TextColor(tk.TextHeading).SingleLine()
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					display.Tag(c, recSourceLabel(first.line.Source, a.currentLang()), display.TagProps{Color: recSourceColor(first.line.Source), Size: theme.Small, Round: true})
					ui.Text(c, clock).FontSize(12).TextColor(tk.TextTertiary).Shrink(0)
					ui.Text(c, first.line.URL).FontSize(12).TextColor(tk.TextTertiary).Grow(1).MinWidth(0).SingleLine()
				})
			})
		})
		shots := 0
		for _, st := range e.steps {
			shots += len(st.shots)
		}
		if shots == 0 {
			ui.Text(c, a.tr("rec.noShots")).FontSize(12).TextColor(tk.TextQuaternary)
			return
		}
		ui.ScrollHorizontal(c).AlignSelf(ui.Stretch).Children(func() {
			ui.Row(c).Gap(10).AlignItems(ui.Start).Children(func() {
				for _, st := range e.steps {
					st := st
					for i, sh := range st.shots {
						sh := sh
						idx := i
						ui.Column(c).Gap(4).AlignItems(ui.Center).Children(func() {
							btn := ui.ButtonBase(c).Label(sh.name).Tooltip(a.tr("rec.zoom")).
								Size(recThumbW, recThumbH).Center().Radius(4).Clip().Cursor(ui.CursorPointer).
								Border(1, tk.BorderSecondary).Background(tk.BgContainerSecondary)
							if src := a.recImage(sh.path); src != nil {
								btn.Children(func() { ui.Image(c, src).Size(recThumbW, recThumbH).Fit(ui.Contain) })
							} else {
								btn.Children(func() {
									ui.Text(c, a.tr("rec.shotMissing")).FontSize(11).TextColor(tk.TextQuaternary)
								})
							}
							if btn.Clicked() {
								a.openRecLightbox(st, idx)
							}
							ui.Text(c, sh.name).FontSize(11).TextColor(tk.TextTertiary).SingleLine().MaxWidth(recThumbW)
						})
					}
				}
			})
		})
	})
}

// reportData is the HTML report's template payload. Image data URIs are
// template.URL so html/template embeds them verbatim; all text is escaped.
type reportData struct {
	Lang        string
	Title       string
	Meta        string
	Generated   string
	NoShots     string
	ShotMissing string
	Steps       []reportStep
}

type reportStep struct {
	Seq    int
	Desc   string
	Source string
	Time   string
	URL    string
	Shots  []reportShot
}

type reportShot struct {
	Name string
	Src  template.URL
}

var reportTmpl = template.Must(template.New("report").Parse(`<!doctype html>
<html lang="{{.Lang}}">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}}</title>
<style>
body{font-family:system-ui,-apple-system,"Segoe UI",Roboto,"PingFang SC","Microsoft YaHei",sans-serif;margin:0;background:#f6f7f9;color:#1f2328;line-height:1.5}
.wrap{max-width:1100px;margin:0 auto;padding:32px 20px}
h1{font-size:22px;margin:0 0 4px}
.meta{color:#656d76;font-size:13px;margin-bottom:24px}
.step{background:#fff;border:1px solid #d0d7de;border-radius:10px;padding:16px;margin-bottom:14px}
.stephead{display:flex;align-items:center;gap:10px;margin-bottom:10px}
.seq{flex:0 0 auto;width:26px;height:26px;border-radius:13px;background:#eaeef2;color:#424a53;font-size:12px;font-weight:700;display:flex;align-items:center;justify-content:center}
.desc{font-weight:600;font-size:15px}
.sub{color:#656d76;font-size:12px;margin-top:2px;word-break:break-all}
.tag{display:inline-block;padding:1px 8px;border-radius:10px;background:#eaeef2;color:#424a53;font-size:11px;margin-right:6px}
.shots{display:flex;flex-wrap:wrap;gap:12px}
.shot{margin:0}
.shot img{max-width:220px;max-height:300px;border:1px solid #d0d7de;border-radius:6px;display:block;background:#fff}
.shot figcaption{font-size:11px;color:#656d76;text-align:center;margin-top:4px;max-width:220px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.none{color:#8c959f;font-size:12px}
@media (prefers-color-scheme:dark){body{background:#0d1117;color:#e6edf3}.step{background:#161b22;border-color:#30363d}.seq,.tag{background:#21262d;color:#adbac7}.meta,.sub,.shot figcaption{color:#768390}.shot img{border-color:#30363d;background:#0d1117}.none{color:#636e7b}}
</style>
</head>
<body><div class="wrap">
<h1>{{.Title}}</h1>
<div class="meta">{{.Meta}}<br>{{.Generated}}</div>
{{range .Steps}}<div class="step">
<div class="stephead"><div class="seq">{{.Seq}}</div><div><div class="desc">{{.Desc}}</div><div class="sub"><span class="tag">{{.Source}}</span>{{.Time}} · {{.URL}}</div></div></div>
{{if .Shots}}<div class="shots">{{range .Shots}}<figure class="shot">{{if .Src}}<img src="{{.Src}}" alt="{{.Name}}" loading="lazy" decoding="async">{{else}}<div class="none">{{$.ShotMissing}}</div>{{end}}<figcaption>{{.Name}}</figcaption></figure>{{end}}</div>{{else}}<div class="none">{{$.NoShots}}</div>{{end}}
</div>
{{end}}</div></body></html>
`))

// exportReport writes report.html into the session folder, referencing each
// step screenshot by its relative path with lazy loading so the HTML stays
// small and thumbnails decode only as they scroll into view. The report thus
// travels with its session folder, not alone. It returns the written path.
func exportReport(dir, lang string) (string, error) {
	d, err := loadRecDetail(dir, 0)
	if err != nil {
		return "", err
	}
	data := reportData{
		Lang:        lang,
		Title:       trLang(lang, "rec.report.title"),
		Meta:        recMetaLine(d.meta, lang),
		Generated:   trLang(lang, "rec.report.generated", formatRecTitle(time.Now())),
		NoShots:     trLang(lang, "rec.noShots"),
		ShotMissing: trLang(lang, "rec.shotMissing"),
	}
	for _, st := range d.steps {
		rs := reportStep{
			Seq:    st.line.Seq,
			Desc:   describeOp(st.line.Operation, lang),
			Source: recSourceLabel(st.line.Source, lang),
			Time:   recClock(parseRecTime(st.line.At)),
			URL:    st.line.URL,
		}
		for _, sh := range st.shots {
			shot := reportShot{Name: sh.name}
			raw, err := readBoundedFile(sh.path, recShotMaxBytes)
			if err == nil {
				_, err = png.DecodeConfig(bytes.NewReader(raw))
			}
			if err == nil {
				if rel, rerr := filepath.Rel(dir, sh.path); rerr == nil {
					parts := strings.Split(filepath.ToSlash(rel), "/")
					for i, p := range parts {
						parts[i] = url.PathEscape(p)
					}
					shot.Src = template.URL(strings.Join(parts, "/"))
				}
			}
			rs.Shots = append(rs.Shots, shot)
		}
		data.Steps = append(data.Steps, rs)
	}
	out := filepath.Join(dir, "report.html")
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if err := reportTmpl.Execute(f, data); err != nil {
		f.Close()
		os.Remove(tmp)
		return "", err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return "", err
	}
	if err := os.Rename(tmp, out); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return out, nil
}

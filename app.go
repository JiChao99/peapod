package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const mobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"

// countdownGrace is the cancel window between Start and page creation.
const countdownGrace = 3 * time.Second

// pageConfig is one editable row of the setup form: a name and an optional
// URL. A non-primary page left empty inherits the primary page's URL.
type pageConfig struct {
	id         string
	name       string
	url        string
	vars       string // template-mode placeholder assignments
	resW, resH float64
}

// pageRecord is one guest page: its web view, ephemeral session, sync status
// and the ordered queue of operations replayed onto it.
type pageRecord struct {
	id         string
	name       string
	url        string
	base       string // variant prefix swapped when following navigations
	resW, resH float64
	view       *mygo.WebView
	session    *mygo.Session
	status     string // loading, ready, error, closed, paused
	syncOn     bool   // guarded by mu; read by the replay path
	isPrimary  bool
	closed     bool
	lastError  *syncError
	navError   *navFailure    // failed navigation (DNS/TLS/timeout…); guarded by mu
	navFailed  bool           // current load attempt failed; gates Linux finish-after-fail; guarded by mu
	capture    *captureFlight // outstanding CapturePage coalescing concurrent shots; guarded by mu
	queue      *opQueue

	// navPendingAt is guarded by mu: when an operation that may navigate was
	// enqueued, so navigation follow lets the replay navigate this page itself.
	navPendingAt time.Time

	// replaying is guarded by mu: true while replayOnce is resolving/replaying
	// an operation, so navigation follow never replaces the document mid-replay.
	replaying bool

	// pendingNav is guarded by mu: the latest follow destination skipped while
	// this page was busy replaying, reconciled once replay is safe.
	pendingNav string

	// navSignal is a buffered ping sent on every committed navigation so a
	// queued navigation barrier can wait for the follower to reach its target.
	navSignal chan struct{}

	// appliedZoom is touched only on the main thread by the native panel: it
	// tracks the last zoom set to fit the configured resolution.
	appliedZoom float64
}

// pageState is the panel's read-only view of one page. The json tags are the
// MCP peapod_status contract (docs/mcp.md); the panel reads the Go fields.
type pageState struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	URL       string  `json:"url"`
	Status    string  `json:"status"`
	SyncOn    bool    `json:"syncOn"`
	IsPrimary bool    `json:"isPrimary"`
	ResW      float64 `json:"resW"`
	ResH      float64 `json:"resH"`
	// Syncing/Pending expose the replay queue so the panel can show "syncing"
	// as soon as an operation is queued, instead of leaving the card "ready"
	// while the follower waits for an anchor.
	Syncing  bool        `json:"syncing,omitempty"`
	Pending  int         `json:"pending,omitempty"`
	Error    *syncError  `json:"error"`
	NavError *navFailure `json:"navError,omitempty"`
}

// appState is everything the native panel renders, and the peapod_status payload.
type appState struct {
	Status   string      `json:"status"` // idle, starting, running, stopping, cleanup-error
	Pages    []pageState `json:"pages"`
	ActiveID string      `json:"activeId"`
	Errors   []syncError `json:"errors"`
	Message  string      `json:"message"`
	Notice   string      `json:"notice"`
	// NoticeKey/NoticeArgs are set for notices built from an i18n key, so the
	// panel re-renders them in the current language. Notice keeps the text
	// already resolved when the notice was published (MCP status contract).
	NoticeKey  string `json:"noticeKey,omitempty"`
	NoticeArgs []any  `json:"noticeArgs,omitempty"`
	// NoticeKind tiers Notice for the panel: info | success | warning | error.
	NoticeKind string `json:"noticeKind,omitempty"`
	// NoticePath/NoticePathIsDir drive the notice's open-folder/open-file action.
	NoticePath      string `json:"noticePath,omitempty"`
	NoticePathIsDir bool   `json:"noticePathIsDir,omitempty"`
	ShotBusy        bool   `json:"shotBusy,omitempty"`
	DiffBusy        bool   `json:"diffBusy,omitempty"`
	// Diff is the latest per-page batch-diff result, shown in the runtime panel.
	Diff *diffReportState `json:"diff,omitempty"`

	// Recording/replay evidence surface (docs/mcp.md). Recording is true while
	// a recorder is attached; Replay appears once a peapod_replay has started
	// and keeps its final result until the task stops or a new replay begins.
	Recording     bool         `json:"recording"`
	RecordSteps   int          `json:"recordSteps,omitempty"`
	RecordDialogs int          `json:"recordDialogs,omitempty"`
	RecordDir     string       `json:"recordDir,omitempty"`
	Replay        *replayState `json:"replay,omitempty"`

	// SyncDelayMs is the running task's pace for continuous operations
	// (input/scroll); 0 replays them instantly.
	SyncDelayMs float64 `json:"syncDelayMs"`

	// ViewMode is the running task's resolved view mode; the stage and size
	// warnings read it from this snapshot instead of the guarded app field.
	ViewMode string `json:"viewMode,omitempty"`
}

// app holds the task state. mu guards the task/page fields, which services and
// page handlers mutate off the main thread. The setup-form fields (configs,
// viewMode, resW/resH, shareStorage, syncUI, notice source) are touched only on
// the main thread by the native panel.
type app struct {
	mu  sync.Mutex
	win *mygo.Window

	status  string
	taskID  string
	message string
	notice  string
	// noticeKind tiers the notice for the panel: info | success | warning | error.
	// Guarded by mu like notice; setNotice* write both together.
	noticeKind string
	// noticeKey/noticeArgs let the panel re-render a localized notice after a
	// language switch; notice keeps the text resolved at publish time.
	noticeKey  string
	noticeArgs []any
	// noticeSeq invalidates the auto-dismiss timer of an older notice.
	noticeSeq uint64
	// noticeTag marks a notice tied to a transient activity ("replay") so the
	// activity's end can replace or clear it instead of leaving it stale.
	noticeTag string
	// noticePath/noticePathIsDir attach an "open" action to a result notice
	// (screenshot folder, diff evidence, exported report).
	noticePath      string
	noticePathIsDir bool
	// shotBusy/diffBusy mark an in-flight batch so the panel can spin and
	// disable the trigger instead of letting a second batch stack on it.
	shotBusy  bool
	diffBusy  bool
	pages     map[string]*pageRecord
	order     []string
	activeID  string
	primaryID string
	cancel    chan struct{}
	seq       int
	// startAt marks when the countdown began, guarded by mu; the panel derives
	// the visible 3→2→1 countdown and ring progress from it.
	startAt time.Time

	// rec is the running task's operation recorder (record.go), nil when the
	// task does not record. replay is the latest peapod_replay run (replay.go).
	// Both are guarded by mu.
	rec    *recorder
	replay *replayRun

	// taskViewMode is the resolved view mode of the running task, guarded by mu.
	// The stage renders from it, not the setup form's viewMode, so an MCP-started
	// task lays out by the mode it actually requested.
	taskViewMode string

	// taskSyncDelay is the running task's continuous-operation pace, guarded by
	// mu; each page's opQueue carries its own copy.
	taskSyncDelay time.Duration

	configs   []*pageConfig
	configSeq int
	// fieldErr maps a setup-field id ("primary", "page:<id>", "template") to its
	// validation error; the form outlines the field and scrolls the first one
	// into view. Main-thread only, like configs; revalidated each render so a
	// corrected field clears its own outline.
	fieldErr       map[string]string
	scrollErrField string
	// lastScrolledErr is the field already scrolled into view, so the reveal
	// fires once per error rather than every frame (which would pin the scroll
	// and block the user from scrolling away). Main-thread only.
	lastScrolledErr string
	template        string // optional URL template with {placeholders}; empty = plain URLs
	viewMode        string // desktop | mobile
	lastViewMode    string
	resW, resH      float64 // preset seeded by viewMode; each page copies it
	// resByMode remembers each view mode's last-used preset resolution so
	// switching back restores it instead of the factory preset. resByPage
	// additionally remembers each page's own resolution per mode, so a mode
	// switch no longer flattens customized page sizes.
	resByMode map[string][2]float64
	resByPage map[string]map[string][2]float64
	// diffReport is the latest per-page diff result, cleared on a new launch.
	diffReport *diffReportState
	// advancedOpen expands the advanced-settings group (UA, sync pace, shared
	// storage, recording); default collapsed. Main-thread only.
	advancedOpen bool
	// lastStatus/configScroll reset the setup form's scroll to the top whenever
	// the task status changes, so a state change never leaves the user scrolled
	// mid-form. Main-thread only.
	lastStatus   string
	configScroll ui.ScrollState
	// stageStopped distinguishes "never run" from "run and stopped" for the
	// stage's empty-state copy. Main-thread only.
	stageStopped bool
	// mcp is the live loopback server, nil when disabled or failed to start.
	// Assigned once in WhenReady before the first frame; read by the title bar.
	mcp          *mcpServer
	uaPreset     string // built-in UA option key (ua.go); UI-owned like viewMode
	uaCustom     string // full UA string when uaPreset == uaCustom
	shareStorage bool
	record       bool   // record operations + per-step screenshots; UI-owned like shareStorage
	syncDelay    string // continuous-operation pace in ms ("0" = instant); UI-owned like shareStorage
	syncUI       map[string]*bool

	// Setup profiles (profiles.go): profileName is the save-as input, profileSel
	// the picked existing profile, profiles the cached list refreshed on
	// startup/save/delete. All UI-owned like the fields above.
	profileName string
	profileSel  string
	profiles    []string
	// profileOverwriteOpen/profileOverwriteName gate saving over an existing
	// profile behind a confirm modal. UI-owned like the fields above.
	profileOverwriteOpen bool
	profileOverwriteName string
	// profileDeleteOpen/profileDeleteName gate deleting a profile behind a
	// confirm modal, since the file cannot be recovered.
	profileDeleteOpen bool
	profileDeleteName string

	themeMode string // system | light | dark
	// captionTheme is the theme last given to the native caption buttons, so
	// syncCaptionTheme only repaints them when the panel theme changes.
	captionTheme string
	scalePct     string // UI zoom percent: 100 | 125 | 150
	shotName     string // base name for batch screenshots
	bulkOpen     bool   // URL-list bulk add area expanded; UI-owned like shotName
	bulkText     string // pasted URL list, one per line; UI-owned like shotName
	// pendingAdd/pendingDup/pendingRemove defer row mutations clicked during
	// the setup range loop until after it, so the loop never reads a shifted
	// slice.
	pendingAdd    bool
	pendingDup    string
	pendingRemove string
	// confirmStop gates the irreversible stop behind a confirmation modal;
	// replayPendingDir/replayEntryURL/replayConfirmOpen hold a replay awaiting the
	// entry-URL precheck choice. All UI-owned, touched only on the main thread.
	confirmStop       bool
	replayPendingDir  string
	replayEntryURL    string
	replayConfirmOpen bool
	lang              string // zh | en; bound to the panel Select, touched only on the main thread

	// langVal publishes lang for off-thread readers (MCP handlers, cleanup and
	// screenshot goroutines). The panel stores it after each frame; readers load
	// it through currentLang so they never race the Select's write to lang.
	langVal atomic.Value

	// recView is the in-app recordings browser (recordview.go), UI-owned like
	// the setup form: touched only on the main thread in view().
	recView recBrowser
}

func newApp() *app {
	a := &app{
		status:       "idle",
		pages:        map[string]*pageRecord{},
		viewMode:     "desktop",
		lastViewMode: "desktop",
		resW:         1280,
		resH:         800,
		resByMode: map[string][2]float64{
			"desktop": {1280, 800},
			"mobile":  {390, 844},
		},
		resByPage:  map[string]map[string][2]float64{},
		lastStatus: "idle",
		uaPreset:   uaDefault,
		themeMode:  "system",
		scalePct:   "100",
		syncDelay:  "100",
		lang:       "zh",
		syncUI:     map[string]*bool{},
		configs: []*pageConfig{
			{id: "c1", resW: 1280, resH: 800},
			{id: "c2", resW: 1280, resH: 800},
			{id: "c3", resW: 1280, resH: 800},
		},
		configSeq: 3,
	}
	a.langVal.Store(a.lang)
	a.profiles = listProfiles()
	return a
}

// currentLang returns the published UI language for off-thread readers. It
// falls back to zh before the panel has published a value.
func (a *app) currentLang() string {
	if v, ok := a.langVal.Load().(string); ok && v != "" {
		return v
	}
	return "zh"
}

// resolvedPage is a setup row after URL inheritance and validation. base is
// the page's variant prefix: its origin in plain mode, the rendered template
// in template mode.
type resolvedPage struct {
	name, url  string
	base       string
	resW, resH float64
}

type startRequest struct {
	pages        []resolvedPage
	shareStorage bool
	viewMode     string
	userAgent    string        // resolved UA applied to every page; empty = engine default
	diagnostics  bool          // inject the opt-in diagnostics collector at document start
	record       bool          // record dispatched operations + per-step screenshots
	syncDelay    time.Duration // pace between continuous (input/scroll) replays; 0 = instant
}

// capturePayload is what a guest page's preload sends.
type capturePayload struct {
	PageID    string    `json:"pageId"`
	Operation operation `json:"operation"`
}

// dialogPayload is what a guest page's recording-only preload sends when a
// popup appears. Text is untrusted page content, bounded at the source and
// again here.
type dialogPayload struct {
	PageID   string `json:"pageId"`
	Kind     string `json:"kind"` // dialog | modal | toast | alert | confirm | prompt
	Role     string `json:"role,omitempty"`
	Text     string `json:"text"`
	Selector string `json:"selector,omitempty"`
	Value    string `json:"value,omitempty"` // prompt() default, otherwise empty
}

// validateDialog mirrors validateOperation's bounds for popup evidence. Text
// and metadata are clipped by UTF-8 bytes, not rejected: the in-page clip counts
// UTF-16 units, so a long CJK message would otherwise be dropped silently.
func validateDialog(d *dialogPayload) error {
	switch d.Kind {
	case "dialog", "modal", "toast", "alert", "confirm", "prompt":
	default:
		return fmt.Errorf("unknown dialog kind %q", d.Kind)
	}
	d.Text = clipUTF8(d.Text, 4000)
	d.Role = clipUTF8(d.Role, 64)
	d.Selector = clipUTF8(d.Selector, 512)
	d.Value = clipUTF8(d.Value, 200)
	return nil
}

// clipUTF8 cuts s to at most n bytes without splitting a UTF-8 sequence.
func clipUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut]
}

// Dialog accepts a popup report from a guest page. Any page may report its own
// popup (unlike Capture, which is primary-only): each variant's popup is
// recorded separately. It is a no-op when no recording is attached.
func (s *SyncService) Dialog(ctx context.Context, payload dialogPayload) error {
	a := s.a
	caller := mygo.CallerPage(ctx)
	if err := validateDialog(&payload); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != "running" || a.rec == nil {
		return nil
	}
	p := a.pages[payload.PageID]
	if p == nil || p.closed || caller != p.view.Page() {
		return fmt.Errorf("forbidden")
	}
	a.recordDialogLocked(p, payload)
	return nil
}

// SyncService is the bridge guest pages call to report a captured operation.
// It is the only IPC service left: the panel is native UI calling app directly.
type SyncService struct{ a *app }

// Capture accepts a trusted operation from the primary page and queues it onto
// every other synced target. Only the primary page may call it.
func (s *SyncService) Capture(ctx context.Context, payload capturePayload) error {
	a := s.a
	caller := mygo.CallerPage(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != "running" || a.primaryID == "" {
		return nil
	}
	primary := a.pages[a.primaryID]
	if primary == nil || caller != primary.view.Page() {
		return fmt.Errorf("forbidden")
	}
	if payload.PageID != a.primaryID {
		return fmt.Errorf("forbidden")
	}
	op := payload.Operation
	if err := validateOperation(&op); err != nil {
		return err
	}
	a.recordOpLocked(op, "guest")
	for _, id := range a.order {
		if id == a.primaryID {
			continue
		}
		if p := a.pages[id]; p != nil {
			a.enqueueOp(p, op)
		}
	}
	return nil
}

// addPage appends a setup row, up to ten pages.
func (a *app) addPage() {
	if len(a.configs) >= 10 {
		return
	}
	a.configSeq++
	a.configs = append(a.configs, &pageConfig{
		id:   fmt.Sprintf("c%d", a.configSeq),
		resW: a.resW,
		resH: a.resH,
	})
}

// removePage drops a setup row, keeping at least two and never the primary.
func (a *app) removePage(id string) {
	if len(a.configs) <= 2 {
		return
	}
	for i, cfg := range a.configs {
		if cfg.id == id && i > 0 {
			a.configs = append(a.configs[:i], a.configs[i+1:]...)
			return
		}
	}
}

// duplicatePage clones a setup row right after its source, so a variant that
// differs in one field starts from a filled row instead of an empty one.
func (a *app) duplicatePage(id string) {
	if len(a.configs) >= 10 {
		return
	}
	for i, cfg := range a.configs {
		if cfg.id != id {
			continue
		}
		a.configSeq++
		clone := *cfg
		clone.id = fmt.Sprintf("c%d", a.configSeq)
		if clone.name != "" {
			clone.name = a.tr("ui.page.copyName", clone.name)
		}
		a.configs = append(a.configs, nil)
		copy(a.configs[i+2:], a.configs[i+1:])
		a.configs[i+1] = &clone
		return
	}
}

// parseURLLines splits a pasted URL list into trimmed, non-empty lines.
func parseURLLines(raw string) []string {
	var out []string
	for _, line := range strings.Split(raw, "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// defaultPageName derives a setup-row name from a URL. Same-host variants are
// the common case (example.com/en vs example.com/zh), so the path is part of
// the name; the host alone cannot tell them apart in cards or screenshots.
func defaultPageName(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return strings.TrimSpace(raw)
	}
	name := u.Host
	if path := strings.Trim(u.Path, "/"); path != "" {
		name += "/" + path
	}
	return name
}

// uniquePageName keeps generated names distinct against the rows already in
// the form (and earlier rows of the same bulk batch).
func (a *app) uniquePageName(name string) string {
	used := map[string]bool{}
	for _, cfg := range a.configs {
		if n := strings.TrimSpace(cfg.name); n != "" {
			used[n] = true
		}
	}
	base := name
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	return name
}

// bulkAddURLs appends one setup row per URL line, all or nothing: every line
// must validate and the total must stay within the ten-page cap. Generated
// rows inherit the preset resolution and are named host/path, with a numeric
// suffix when a name would repeat, so same-host variants stay tellable apart.
func (a *app) bulkAddURLs(raw string) (int, error) {
	if a.templateMode() {
		return 0, fmt.Errorf("%s", a.tr("err.bulkPlain"))
	}
	lines := parseURLLines(raw)
	if len(lines) == 0 {
		return 0, fmt.Errorf("%s", a.tr("err.bulkEmpty"))
	}
	if len(a.configs)+len(lines) > 10 {
		return 0, fmt.Errorf("%s", a.tr("err.bulkTooMany", len(lines), len(a.configs)))
	}
	for i, line := range lines {
		if err := a.validateURL(line); err != nil {
			return 0, fmt.Errorf("%s", a.tr("err.bulkURL", i+1, err.Error()))
		}
	}
	for _, line := range lines {
		a.configSeq++
		cfg := &pageConfig{
			id:   fmt.Sprintf("c%d", a.configSeq),
			url:  line,
			name: a.uniquePageName(defaultPageName(line)),
			resW: a.resW,
			resH: a.resH,
		}
		a.configs = append(a.configs, cfg)
	}
	return len(lines), nil
}

// buildRequest resolves the setup form. In plain mode the primary URL is
// required and empty non-primary URLs inherit it. In template mode each page's
// variant base is rendered from the shared template plus its own vars, and the
// URL field becomes an optional path appended to that base.
func (a *app) buildRequest() (startRequest, error) {
	a.fieldErr = map[string]string{}
	a.scrollErrField = ""
	if len(a.configs) < 2 || len(a.configs) > 10 {
		return startRequest{}, fmt.Errorf("%s", a.tr("err.pageCount"))
	}
	tmpl := strings.TrimSpace(a.template)
	primaryURL := ""
	if tmpl == "" {
		primaryURL = strings.TrimSpace(a.configs[0].url)
		if primaryURL == "" {
			return startRequest{}, a.fieldError("page:"+a.configs[0].id, a.tr("err.primaryURLEmpty"))
		}
		if err := a.validateURL(primaryURL); err != nil {
			return startRequest{}, a.fieldError("page:"+a.configs[0].id, a.tr("err.primaryURL", err.Error()))
		}
	}
	pages := make([]resolvedPage, 0, len(a.configs))
	plainURLs := make([]string, 0, len(a.configs))
	for i, cfg := range a.configs {
		var raw, base string
		if tmpl == "" {
			raw = strings.TrimSpace(cfg.url)
			if i > 0 && raw == "" {
				raw = primaryURL
			}
			if err := a.validateURL(raw); err != nil {
				return startRequest{}, a.fieldError("page:"+cfg.id, a.tr("err.pageURL", a.cfgName(cfg, i), err.Error()))
			}
			base = originOf(raw)
			plainURLs = append(plainURLs, raw)
		} else {
			vars, bad := parseVars(cfg.vars)
			if bad != "" {
				return startRequest{}, a.fieldError("vars:"+cfg.id, a.tr("err.vars", a.cfgName(cfg, i), bad))
			}
			b, missing := renderTemplate(tmpl, vars)
			if len(missing) > 0 {
				return startRequest{}, a.fieldError("vars:"+cfg.id, a.tr("err.template", a.cfgName(cfg, i), strings.Join(missing, ", ")))
			}
			if err := a.validateURL(b); err != nil {
				return startRequest{}, a.fieldError("page:"+cfg.id, a.tr("err.pageURL", a.cfgName(cfg, i), err.Error()))
			}
			clean, ok := cleanTemplateBase(b)
			if !ok {
				return startRequest{}, a.fieldError("page:"+cfg.id, a.tr("err.templateBase", a.cfgName(cfg, i)))
			}
			base = clean
			var pathErr error
			raw, pathErr = appendTemplatePath(base, cfg.url)
			if pathErr != nil {
				return startRequest{}, a.fieldError("page:"+cfg.id, a.tr("err.pageURL", a.cfgName(cfg, i), a.tr("err.urlInvalid")))
			}
			if err := a.validateURL(raw); err != nil {
				return startRequest{}, a.fieldError("page:"+cfg.id, a.tr("err.pageURL", a.cfgName(cfg, i), err.Error()))
			}
		}
		pages = append(pages, resolvedPage{
			name: a.cfgName(cfg, i), url: raw, base: base,
			resW: clampRes(cfg.resW, a.resW), resH: clampRes(cfg.resH, a.resH),
		})
	}
	if tmpl == "" {
		bases := inferPlainBases(plainURLs)
		for i := range pages {
			pages[i].base = bases[i]
		}
	}
	return startRequest{pages: pages, shareStorage: a.shareStorage, viewMode: a.viewMode,
		userAgent: resolveUA(a.uaPreset, a.uaCustom, a.viewMode), record: a.record,
		syncDelay: formSyncDelay(a.syncDelay)}, nil
}

// fieldError records a per-field validation error for the setup form's outline
// and auto-scroll, remembering the first failing field, and returns it as the
// build error. Main-thread only, like the form fields.
func (a *app) fieldError(id, msg string) error {
	if a.fieldErr == nil {
		a.fieldErr = map[string]string{}
	}
	a.fieldErr[id] = msg
	if a.scrollErrField == "" {
		a.scrollErrField = id
	}
	return fmt.Errorf("%s", msg)
}

func (a *app) cfgName(cfg *pageConfig, i int) string {
	if n := strings.TrimSpace(cfg.name); n != "" {
		return n
	}
	if i == 0 {
		return a.tr("ui.page.primary")
	}
	return a.tr("ui.page.n", i+1)
}

// clampRes keeps a resolution within the supported range, falling back to the
// preset when unset.
func clampRes(v, fallback float64) float64 {
	if v <= 0 {
		v = fallback
	}
	return min(max(v, 200), 3840)
}

// startTask validates the form, then counts down before creating pages.
func (a *app) startTask() {
	req, err := a.buildRequest()
	if err != nil {
		// Only a field failure is cleared by fixing the form; anything else
		// (page count) has no field to correct and must stay visible.
		if len(a.fieldErr) > 0 {
			a.setNoticeErrTag("form", err.Error())
		} else {
			a.setNoticeErr(err.Error())
		}
		return
	}
	if err := a.launch(req); err != nil {
		a.setNoticeErr(err.Error())
	}
}

// launch begins a resolved request: it marks the task starting and counts
// down before creating pages. It is shared by the panel and the MCP server.
func (a *app) launch(req startRequest) error {
	a.mu.Lock()
	if a.status == "cleanup-error" {
		a.mu.Unlock()
		return fmt.Errorf("%s", a.tr("err.cleanupBlocked"))
	}
	if a.status != "idle" {
		a.mu.Unlock()
		return fmt.Errorf("%s", a.tr("err.notIdle"))
	}
	a.seq++
	a.taskID = fmt.Sprintf("task-%d", a.seq)
	a.status = "starting"
	a.startAt = time.Now()
	a.message = ""
	a.clearNoticeLocked()
	a.diffReport = nil
	a.cancel = make(chan struct{})
	cancel := a.cancel
	taskID := a.taskID
	lang := a.currentLang()
	a.mu.Unlock()
	a.invalidate()
	go a.runCountdown(taskID, cancel, req, lang)
	return nil
}

// cancelStart aborts the countdown before any page is created.
func (a *app) cancelStart() {
	a.mu.Lock()
	if a.status == "starting" && a.cancel != nil {
		close(a.cancel)
		a.cancel = nil
	}
	a.mu.Unlock()
	a.invalidate()
}

// stopTask tears down every page and clears its session data. cleanup-error is
// rejected: cleanup already emptied a.pages, so re-running it would reset to
// idle without retrying the failed wipe and silently clear the restart lockout.
func (a *app) stopTask() error {
	a.mu.Lock()
	if a.status == "cleanup-error" {
		a.mu.Unlock()
		return fmt.Errorf("%s", a.tr("err.cleanupBlocked"))
	}
	if a.status == "idle" || a.status == "stopping" {
		a.mu.Unlock()
		return nil
	}
	if a.status == "starting" && a.cancel != nil {
		close(a.cancel)
		a.cancel = nil
	}
	a.status = "stopping"
	pages := make([]*pageRecord, 0, len(a.order))
	for _, id := range a.order {
		if p := a.pages[id]; p != nil {
			pages = append(pages, p)
		}
	}
	a.mu.Unlock()
	a.invalidate()
	go a.cleanup(pages, a.currentLang())
	return nil
}

// setActive switches which page the stage shows.
func (a *app) setActive(id string) {
	a.mu.Lock()
	if p := a.pages[id]; p != nil && !p.closed {
		a.activeID = id
	}
	a.mu.Unlock()
	a.invalidate()
}

// reloadPage reloads a page and clears its paused state. A running follower
// reloads to the primary's current URL mapped onto its own variant, so one
// click also repairs a desynced page.
func (a *app) reloadPage(id string) {
	a.mu.Lock()
	p := a.pages[id]
	var view *mygo.WebView
	resync := ""
	if p != nil && !p.closed {
		resetNavLocked(p)
		// An explicit reload/resync supersedes any deferred follow destination;
		// leaving it would let the navigation handler undo the repair.
		p.pendingNav = ""
		p.navPendingAt = time.Time{}
		view = p.view
		if a.status == "running" && !p.isPrimary {
			if primary := a.pages[a.primaryID]; primary != nil && !primary.closed {
				resync = mapNavigation(primary.base, p.base, primary.url)
			}
		}
	}
	a.mu.Unlock()
	if view != nil {
		if resync != "" {
			view.Page().LoadURL(resync)
		} else {
			view.Page().Reload()
		}
	}
	a.invalidate()
}

// resumePage lifts a pause by skipping the failed step: the sync queue continues
// with the next operation instead of retrying the one that desynced this variant.
func (a *app) resumePage(id string) {
	a.mu.Lock()
	p := a.pages[id]
	if p != nil && !p.closed && p.status == "paused" {
		p.status = "ready"
		p.lastError = nil
		// A pause can coexist with a nav failure; leaving navFailed/pendingNav
		// set would block markNavSucceededLocked and re-apply the failed URL on
		// the next reconcile, looping straight back into the pause.
		p.navError = nil
		p.navFailed = false
		p.pendingNav = ""
		p.navPendingAt = time.Time{}
	}
	a.mu.Unlock()
	a.pushState()
}

// gotoPageHome navigates a page back to its own variant origin — the recovery
// when a followed path does not exist under that variant.
func (a *app) gotoPageHome(id string) {
	a.mu.Lock()
	p := a.pages[id]
	var view *mygo.WebView
	home := ""
	if p != nil && !p.closed {
		resetNavLocked(p)
		p.pendingNav = ""
		p.navPendingAt = time.Time{}
		view = p.view
		home = p.base
	}
	a.mu.Unlock()
	if view != nil && home != "" {
		view.Page().LoadURL(home)
	}
	a.invalidate()
}

// screenshotAll captures every running page to a PNG named
// {base}_{pageName}.png inside a per-batch folder under the user's
// Pictures/Peapod screenshots root.
// The stage keeps every page placed (stacked under the active one), so each
// view renders without being brought to the front; CapturePage is
// goroutine-safe.
func (a *app) screenshotAll() {
	a.mu.Lock()
	if a.status != "running" {
		a.mu.Unlock()
		a.setNoticeErrKey("err.notRunning")
		return
	}
	if a.shotBusy {
		a.mu.Unlock()
		return
	}
	a.shotBusy = true
	type shot struct {
		name string
		id   string
	}
	shots := make([]shot, 0, len(a.order))
	for _, id := range a.order {
		if p := a.pages[id]; p != nil && !p.closed {
			shots = append(shots, shot{name: p.name, id: id})
		}
	}
	base := strings.TrimSpace(a.shotName)
	taskID := a.taskID
	a.mu.Unlock()
	if len(shots) == 0 {
		a.mu.Lock()
		a.shotBusy = false
		a.mu.Unlock()
		a.setNoticeErrKey("err.noPages")
		return
	}
	if base == "" {
		base = "screenshot"
	}
	a.pushState()
	go func() {
		defer func() {
			a.mu.Lock()
			a.shotBusy = false
			a.mu.Unlock()
			a.pushState()
		}()
		dir, err := screenshotDir(base)
		if err != nil {
			if !a.batchStale(taskID) {
				a.setNoticeErrKey("err.shotDir", err.Error())
			}
			return
		}
		used := map[string]bool{}
		saved, failed := 0, 0
		for _, s := range shots {
			a.mu.Lock()
			var f *captureFlight
			if a.batchShotValidLocked(taskID, s.id) {
				p := a.pages[s.id]
				f = a.capturePageFlight(p, p.view.CapturePage)
			}
			a.mu.Unlock()
			if f == nil {
				failed++
				continue
			}
			png, err := waitCapture(f, captureTimeout)
			if err != nil {
				failed++
				continue
			}
			stem := fmt.Sprintf("%s_%s", sanitizeFileName(base), sanitizeFileName(s.name))
			fname := stem + ".png"
			for i := 2; used[fname]; i++ {
				fname = fmt.Sprintf("%s_%d.png", stem, i)
			}
			used[fname] = true
			if err := os.WriteFile(filepath.Join(dir, fname), png, 0o644); err != nil {
				failed++
				continue
			}
			saved++
		}
		if a.batchStale(taskID) {
			return
		}
		if failed > 0 {
			a.setNoticeOpenKey("warning", "notice.shotPartial", dir, true, saved, failed, dir)
			return
		}
		a.setNoticeOpenKey("success", "notice.shotDone", dir, true, saved, dir)
	}()
}

// batchShotValidLocked reports whether a batch screenshot shot still belongs to
// the originating, running task. Page IDs (p1, p2, …) are reused across tasks, so
// a batch goroutine that outlives its task — e.g. while waiting on a capture
// timeout — must not capture a newly started task's pages under the old batch's
// names. The caller holds mu.
func (a *app) batchShotValidLocked(taskID, id string) bool {
	if a.taskID != taskID || a.status != "running" {
		return false
	}
	p := a.pages[id]
	return p != nil && !p.closed
}

// batchStale reports whether a batch goroutine outlived its task, so its result
// notice (and open-folder action) must not land on a newer task or idle screen.
func (a *app) batchStale(taskID string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.taskID != taskID || a.status != "running"
}

// setNoticeErr tiers a notice as an error so the panel renders it as an alert
// instead of undifferentiated gray text.
func (a *app) setNoticeErr(msg string) { a.setNoticeFull(msg, "error", "", "", false) }

// setNoticeTextLocked publishes a literal notice, dropping any keyed notice it
// replaces and moving the sequence so a pending TTL from an older notice cannot
// dismiss this one. The caller holds a.mu.
func (a *app) setNoticeTextLocked(msg, kind, tag string) {
	a.notice = msg
	a.noticeKind = kind
	a.noticeKey = ""
	a.noticeArgs = nil
	a.noticeTag = tag
	a.noticePath = ""
	a.noticePathIsDir = false
	a.noticeSeq++
}

// setNoticeErrTag publishes a literal error tied to a transient form; the
// setup form clears it once its validation errors are gone.
func (a *app) setNoticeErrTag(tag, msg string) {
	a.setNoticeFull(msg, "error", tag, "", false)
}

// setNoticeKey* publish a notice from an i18n key and arguments, so the panel
// re-renders it in the language selected later instead of freezing the text.
func (a *app) setNoticeKey(kind, key string, args ...any) {
	a.setNoticeKeyFull(kind, key, "", "", false, args...)
}
func (a *app) setNoticeOKKey(key string, args ...any)   { a.setNoticeKey("success", key, args...) }
func (a *app) setNoticeWarnKey(key string, args ...any) { a.setNoticeKey("warning", key, args...) }
func (a *app) setNoticeErrKey(key string, args ...any)  { a.setNoticeKey("error", key, args...) }
func (a *app) setNoticeOpenKey(kind, key, path string, isDir bool, args ...any) {
	a.setNoticeKeyFull(kind, key, "", path, isDir, args...)
}

func (a *app) setNoticeFull(msg, kind, tag, path string, isDir bool) {
	a.publishNotice(msg, kind, tag, path, isDir, "", nil)
}

func (a *app) setNoticeKeyFull(kind, key, tag, path string, isDir bool, args ...any) {
	a.publishNotice(a.tr(key, args...), kind, tag, path, isDir, key, args)
}

// noticeTTL is how long a transient success/info notice stays before it
// auto-dismisses; errors and warnings stay until replaced or closed.
const noticeTTL = 5 * time.Second

// noticeAutoExpires reports whether a notice may dismiss itself. Result notices
// that carry an artifact path stay up: the open-folder button is the only
// in-app route back to the diff/screenshot/recording they produced.
func noticeAutoExpires(kind, path string) bool {
	if path != "" {
		return false
	}
	return kind == "success" || kind == "info"
}

func (a *app) publishNotice(msg, kind, tag, path string, isDir bool, key string, args []any) {
	a.mu.Lock()
	a.notice = msg
	a.noticeKind = kind
	a.noticeKey = key
	a.noticeArgs = append([]any(nil), args...)
	a.noticeTag = tag
	a.noticePath = path
	a.noticePathIsDir = isDir
	a.noticeSeq++
	seq := a.noticeSeq
	a.mu.Unlock()
	if noticeAutoExpires(kind, path) {
		time.AfterFunc(noticeTTL, func() { a.expireNotice(seq) })
	}
	a.pushState()
}

// expireNotice clears a success/info notice only if it is still the one the
// timer was scheduled for, so a newer notice is never dismissed early.
func (a *app) expireNotice(seq uint64) {
	a.mu.Lock()
	if a.noticeSeq != seq || (a.noticeKind != "success" && a.noticeKind != "info") {
		a.mu.Unlock()
		return
	}
	a.clearNoticeLocked()
	a.mu.Unlock()
	a.pushState()
}

func (a *app) clearNoticeLocked() {
	a.notice = ""
	a.noticeKind = ""
	a.noticeKey = ""
	a.noticeArgs = nil
	a.noticeTag = ""
	a.noticePath = ""
	a.noticePathIsDir = false
}

// clearNoticeTag drops the notice only if it still carries tag, so a newer
// notice set while the activity ran is not clobbered when the activity ends.
func (a *app) clearNoticeTag(tag string) {
	a.mu.Lock()
	if a.noticeTag != tag {
		a.mu.Unlock()
		return
	}
	a.clearNoticeLocked()
	a.mu.Unlock()
	a.pushState()
}

// userArtifactRoot is the user-writable root for saved artifacts (screenshots,
// recordings): Pictures/Peapod under the user's home, falling back to the
// config dir and then a relative folder. It is never the executable's own
// directory — an installed app lives somewhere read-only (e.g. Program Files),
// which is exactly where captures used to fail.
func userArtifactRoot() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Pictures", "Peapod")
	}
	if cfg, err := os.UserConfigDir(); err == nil && cfg != "" {
		return filepath.Join(cfg, "Peapod")
	}
	return "Peapod"
}

// screenshotDir creates a per-batch folder under the user's Pictures/Peapod
// screenshots root so repeated captures never overwrite earlier ones, and
// returns its absolute path.
func screenshotDir(base string) (string, error) {
	return batchDir(filepath.Join(userArtifactRoot(), "screenshots"), base)
}

// batchDir creates root/{base}_{timestamp} and returns its absolute path. The
// timestamp keeps repeated batches apart; base is sanitized for the filesystem.
func batchDir(root, base string) (string, error) {
	dir := filepath.Join(root, fmt.Sprintf("%s_%s", sanitizeFileName(base), time.Now().Format("20060102-150405")))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if abs, err := filepath.Abs(dir); err == nil {
		return abs, nil
	}
	return dir, nil
}

func sanitizeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch r {
		case '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return '_'
		}
		return r
	}, strings.TrimSpace(s))
}

// runCountdown waits the grace period, then creates the pages, unless the
// user cancels first. It repaints on a short tick so the panel's countdown
// digit and ring advance even without other events.
func (a *app) runCountdown(taskID string, cancel chan struct{}, req startRequest, lang string) {
	deadline := time.NewTimer(countdownGrace)
	defer deadline.Stop()
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-cancel:
			a.mu.Lock()
			if a.taskID == taskID && a.status == "starting" {
				a.status = "idle"
			}
			a.mu.Unlock()
			a.invalidate()
			return
		case <-deadline.C:
			a.createPages(taskID, req, lang)
			return
		case <-tick.C:
			a.invalidate()
		}
	}
}

// countdownProgress derives the visible countdown from the mu-guarded start
// timestamp: the remaining whole seconds (3→2→1) and the elapsed percent for
// the progress ring. Both are zero when not starting.
func (a *app) countdownProgress() (sec int, pct float32) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != "starting" || a.startAt.IsZero() {
		return 0, 0
	}
	elapsed := time.Since(a.startAt)
	rem := countdownGrace - elapsed
	sec = min(max(int(rem.Seconds()+0.999), 1), 3)
	pct = min(max(float32(elapsed.Seconds()/countdownGrace.Seconds()*100), 0), 100)
	return sec, pct
}

// reseedViewMode applies a view-mode change on the main thread. Each page's
// resolution is remembered under the mode it was edited in, and restored when
// the user returns to that mode; pages without a saved value for the entering
// mode take that mode's preset. Switching modes therefore never flattens the
// per-page sizes the user configured for the other mode.
func (a *app) reseedViewMode() {
	if a.viewMode == a.lastViewMode {
		return
	}
	if a.resByMode == nil {
		a.resByMode = map[string][2]float64{}
	}
	if a.resByPage == nil {
		a.resByPage = map[string]map[string][2]float64{}
	}
	if a.resByPage[a.lastViewMode] == nil {
		a.resByPage[a.lastViewMode] = map[string][2]float64{}
	}
	w, h := a.resW, a.resH
	if len(a.configs) > 0 && a.configs[0].resW > 0 && a.configs[0].resH > 0 {
		w, h = a.configs[0].resW, a.configs[0].resH
	}
	a.resByMode[a.lastViewMode] = [2]float64{w, h}
	for _, cfg := range a.configs {
		a.resByPage[a.lastViewMode][cfg.id] = [2]float64{cfg.resW, cfg.resH}
	}
	r, ok := a.resByMode[a.viewMode]
	if !ok {
		if a.viewMode == "mobile" {
			r = [2]float64{390, 844}
		} else {
			r = [2]float64{1280, 800}
		}
	}
	a.resW, a.resH = r[0], r[1]
	saved := a.resByPage[a.viewMode]
	for _, cfg := range a.configs {
		if p, ok := saved[cfg.id]; ok && p[0] > 0 && p[1] > 0 {
			cfg.resW, cfg.resH = p[0], p[1]
			continue
		}
		cfg.resW, cfg.resH = a.resW, a.resH
	}
	a.lastViewMode = a.viewMode
}

// createPages builds one web view per resolved page with capture preload,
// lockdown handlers and a replay worker, then marks the task running.
func (a *app) createPages(taskID string, req startRequest, lang string) {
	var shared *mygo.Session
	if req.shareStorage {
		shared = mygo.NewEphemeralSession()
	}

	for i, rp := range req.pages {
		a.mu.Lock()
		if a.taskID != taskID || a.status != "starting" {
			a.mu.Unlock()
			return
		}
		a.mu.Unlock()

		id := fmt.Sprintf("p%d", i+1)
		session := shared
		if session == nil {
			session = mygo.NewEphemeralSession()
		}
		pageOpts := mygo.PageOptions{
			TrustedOrigins:       []string{originOf(rp.url)},
			BackgroundThrottling: mygo.ThrottlingOff,
			Session:              session,
			UserAgent:            req.userAgent,
		}
		preload := capturePreload(id)
		if req.record {
			preload += "\n" + dialogPreload(id)
		}
		if req.diagnostics {
			preload += "\n" + diagPreload()
		}
		pageOpts.PreloadScript = preload
		view, err := a.win.NewWebView(mygo.WebViewOptions{URL: rp.url, Page: pageOpts})
		if err != nil {
			a.mu.Lock()
			a.message = trLang(lang, "err.createPage", rp.name, err)
			a.mu.Unlock()
			continue
		}
		p := &pageRecord{
			id:        id,
			name:      rp.name,
			url:       rp.url,
			base:      rp.base,
			resW:      rp.resW,
			resH:      rp.resH,
			view:      view,
			session:   session,
			status:    "loading",
			syncOn:    true,
			isPrimary: i == 0,
			queue:     newOpQueue(req.syncDelay),
			navSignal: make(chan struct{}, 1),
		}
		a.attachHandlers(p)

		a.mu.Lock()
		if a.taskID != taskID || a.status != "starting" {
			a.mu.Unlock()
			view.Destroy()
			return
		}
		a.pages[id] = p
		a.order = append(a.order, id)
		if i == 0 {
			a.primaryID = id
		}
		a.mu.Unlock()
		go a.replayWorker(p)
	}

	// Attach the recorder while still starting: once status flips to running,
	// operations can dispatch, and none may fall into a recording gap.
	if req.record {
		a.startRecording(taskID, req)
	}
	a.mu.Lock()
	if a.taskID != taskID || a.status != "starting" {
		if a.rec != nil && a.rec.taskID == taskID {
			a.stopRecordingLocked()
		}
		a.mu.Unlock()
		return
	}
	a.status = "running"
	a.activeID = a.primaryID
	a.taskViewMode = req.viewMode
	a.taskSyncDelay = req.syncDelay
	a.mu.Unlock()
	a.invalidate()
}

// attachHandlers locks the page down (no popups, no permissions) and tracks
// its load status for the panel.
func (a *app) attachHandlers(p *pageRecord) {
	page := p.view.Page()
	page.SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
	page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	page.OnDOMReady(func() {
		a.mu.Lock()
		a.markReadyLocked(p)
		a.mu.Unlock()
		a.pushState()
	})
	page.OnDidFinishLoad(func() {
		a.mu.Lock()
		a.markNavSucceededLocked(p)
		a.mu.Unlock()
		a.pushState()
	})
	page.OnDidFailLoad(func(e *mygo.LoadError) {
		a.mu.Lock()
		a.markNavFailedLocked(p, e)
		a.mu.Unlock()
		a.pushState()
	})
	page.OnDidNavigate(func(u string) {
		a.mu.Lock()
		var targets []followTarget
		var nav string
		view := p.view
		if !p.closed {
			p.url = u
			a.markNavCommittedLocked(p, u)
			select {
			case p.navSignal <- struct{}{}:
			default:
			}
			if p.isPrimary {
				targets = a.followTargets(p, u)
			} else {
				// This page navigated; any replay that was expected to cause
				// it has landed, so follow may act on the next primary move.
				p.navPendingAt = time.Time{}
				if p.pendingNav != "" && sameFollowURL(p.pendingNav, u) {
					p.pendingNav = ""
				} else {
					nav = a.reconcileNavLocked(p, false)
				}
			}
		}
		a.mu.Unlock()
		for _, t := range targets {
			t.view.Page().LoadURL(t.url)
		}
		if nav != "" && view != nil {
			view.Page().LoadURL(nav)
		}
		a.pushState()
	})
}

// cleanup clears each page's session data and destroys its view. A failure
// leaves the task in cleanup-error, which blocks new tasks.
func (a *app) cleanup(pages []*pageRecord, lang string) {
	var failed bool
	for _, p := range pages {
		a.mu.Lock()
		if !p.closed {
			p.closed = true
			p.queue.close()
		}
		session := p.session
		view := p.view
		a.mu.Unlock()

		if session != nil {
			if err := session.ClearData(mygo.SiteDataAll); err != nil {
				failed = true
			}
		}
		if view != nil {
			view.Destroy()
		}
	}
	a.mu.Lock()
	// Detach the recorder (its goroutine flushes the script and writes the
	// summary asynchronously) and cancel any in-flight replay before the page
	// map is cleared.
	a.stopRecordingLocked()
	if rp := a.replay; rp != nil {
		if rp.active {
			rp.cancel()
		}
		a.replay = nil
	}
	a.pages = map[string]*pageRecord{}
	a.order = nil
	a.activeID = ""
	a.primaryID = ""
	a.cancel = nil
	a.taskViewMode = ""
	a.taskSyncDelay = 0
	// Drop the previous task's result notice so a stale screenshot/diff path does
	// not linger in the footer after stop; the recorder's save notice (if any)
	// lands afterwards as a fresh result. Clear the tag/path/key too, so a replay
	// finishing concurrently cannot resurrect its result over this reset.
	a.clearNoticeLocked()
	// A batch goroutine may outlive the task (capture timeouts); release its
	// busy flags so the next task's buttons are not disabled by a dead batch.
	a.shotBusy = false
	a.diffBusy = false
	if failed {
		a.status = "cleanup-error"
		a.message = trLang(lang, "err.cleanupFailed")
	} else {
		a.status = "idle"
		a.message = ""
	}
	a.mu.Unlock()
	a.invalidate()
}

// snapshot builds the panel state under the lock.
func (a *app) snapshot() appState {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := appState{
		Status:          a.status,
		ActiveID:        a.activeID,
		Message:         a.message,
		Notice:          a.notice,
		NoticeKey:       a.noticeKey,
		NoticeArgs:      append([]any(nil), a.noticeArgs...),
		NoticeKind:      a.noticeKind,
		NoticePath:      a.noticePath,
		NoticePathIsDir: a.noticePathIsDir,
		ShotBusy:        a.shotBusy,
		DiffBusy:        a.diffBusy,
		Pages:           make([]pageState, 0, len(a.order)),
		Errors:          []syncError{},
		SyncDelayMs:     float64(a.taskSyncDelay) / float64(time.Millisecond),
		ViewMode:        a.taskViewMode,
	}
	for _, id := range a.order {
		p := a.pages[id]
		if p == nil {
			continue
		}
		pending := 0
		if p.queue != nil {
			pending = p.queue.len()
		}
		if p.replaying {
			pending++
		}
		syncing := p.status == "ready" && pending > 0
		st.Pages = append(st.Pages, pageState{
			ID: p.id, Name: p.name, URL: p.url, Status: p.status,
			SyncOn: p.syncOn, IsPrimary: p.isPrimary, Error: p.lastError, NavError: p.navError,
			ResW: p.resW, ResH: p.resH, Syncing: syncing, Pending: pending,
		})
		if p.lastError != nil {
			st.Errors = append(st.Errors, *p.lastError)
		}
	}
	if a.diffReport != nil {
		rep := *a.diffReport
		rep.Entries = append([]diffReportEntry(nil), a.diffReport.Entries...)
		st.Diff = &rep
	}
	if r := a.rec; r != nil {
		st.Recording = true
		st.RecordSteps = r.seq
		st.RecordDialogs = r.dialogSeq
		st.RecordDir = r.dir
	}
	if rp := a.replay; rp != nil {
		st.Replay = &replayState{Active: rp.active, Done: rp.done, Total: rp.total, Skipped: rp.skipped, Error: rp.err}
	}
	return st
}

// pushState redraws the native panel. Page handlers call it off the main
// thread; Invalidate is safe from any goroutine.
func (a *app) pushState() {
	a.invalidate()
}

// invalidate redraws the window when one exists (tests run without a window).
func (a *app) invalidate() {
	if a.win != nil {
		a.win.Invalidate()
	}
}

func (a *app) validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("%s", a.tr("err.urlInvalid"))
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s", a.tr("err.urlScheme"))
	}
	if u.Host == "" {
		return fmt.Errorf("%s", a.tr("err.urlHost"))
	}
	if u.User != nil {
		return fmt.Errorf("%s", a.tr("err.urlCreds"))
	}
	return nil
}

// originOf returns the scheme://host[:port] prefix used as a variant base. It
// normalizes the way browsers report origins (lowercase host, no default port)
// so a configured URL like https://EXAMPLE.com:443 still matches the URL the
// web view reports after navigating.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	port := u.Port()
	if port == "" || (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		return u.Scheme + "://" + host
	}
	return u.Scheme + "://" + host + ":" + port
}

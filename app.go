package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
)

const mobileUA = "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1"

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
	queue      chan operation

	// navPendingAt is guarded by mu: when an operation that may navigate was
	// enqueued, so navigation follow lets the replay navigate this page itself.
	navPendingAt time.Time

	// replaying is guarded by mu: true while replayOnce is resolving/replaying
	// an operation, so navigation follow never replaces the document mid-replay.
	replaying bool

	// pendingNav is guarded by mu: the latest follow destination skipped while
	// this page was busy replaying, reconciled once replay is safe.
	pendingNav string

	// appliedZoom is touched only on the main thread by the native panel: it
	// tracks the last zoom set to fit the configured resolution.
	appliedZoom float64
}

// pageState is the panel's read-only view of one page.
type pageState struct {
	ID        string
	Name      string
	URL       string
	Status    string
	SyncOn    bool
	IsPrimary bool
	Error     *syncError
}

// appState is everything the native panel renders.
type appState struct {
	Status   string // idle, starting, running, stopping, cleanup-error
	Pages    []pageState
	ActiveID string
	Errors   []syncError
	Message  string
	Notice   string
}

// app holds the task state. mu guards the task/page fields, which services and
// page handlers mutate off the main thread. The setup-form fields (configs,
// viewMode, resW/resH, shareStorage, syncUI, notice source) are touched only on
// the main thread by the native panel.
type app struct {
	mu  sync.Mutex
	win *mygo.Window

	status    string
	taskID    string
	message   string
	notice    string
	pages     map[string]*pageRecord
	order     []string
	activeID  string
	primaryID string
	cancel    chan struct{}
	seq       int

	configs      []*pageConfig
	configSeq    int
	template     string // optional URL template with {placeholders}; empty = plain URLs
	viewMode     string // desktop | mobile
	lastViewMode string
	resW, resH   float64 // preset seeded by viewMode; each page copies it
	shareStorage bool
	syncUI       map[string]*bool

	themeMode string // system | light | dark
	scalePct  string // UI zoom percent: 100 | 125 | 150
	shotName  string // base name for batch screenshots
	lang      string // zh | en
}

func newApp() *app {
	a := &app{
		status:       "idle",
		pages:        map[string]*pageRecord{},
		viewMode:     "desktop",
		lastViewMode: "desktop",
		resW:         1280,
		resH:         800,
		themeMode:    "system",
		scalePct:     "100",
		lang:         "zh",
		syncUI:       map[string]*bool{},
		configs: []*pageConfig{
			{id: "c1", resW: 1280, resH: 800},
			{id: "c2", resW: 1280, resH: 800},
			{id: "c3", resW: 1280, resH: 800},
		},
		configSeq: 3,
	}
	return a
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
}

// capturePayload is what a guest page's preload sends.
type capturePayload struct {
	PageID    string    `json:"pageId"`
	Operation operation `json:"operation"`
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

// buildRequest resolves the setup form. In plain mode the primary URL is
// required and empty non-primary URLs inherit it. In template mode each page's
// variant base is rendered from the shared template plus its own vars, and the
// URL field becomes an optional path appended to that base.
func (a *app) buildRequest() (startRequest, error) {
	if len(a.configs) < 2 || len(a.configs) > 10 {
		return startRequest{}, fmt.Errorf("%s", a.tr("err.pageCount"))
	}
	tmpl := strings.TrimSpace(a.template)
	primaryURL := ""
	if tmpl == "" {
		primaryURL = strings.TrimSpace(a.configs[0].url)
		if err := a.validateURL(primaryURL); err != nil {
			return startRequest{}, fmt.Errorf("%s", a.tr("err.primaryURL", err.Error()))
		}
	}
	pages := make([]resolvedPage, 0, len(a.configs))
	for i, cfg := range a.configs {
		var raw, base string
		if tmpl == "" {
			raw = strings.TrimSpace(cfg.url)
			if i > 0 && raw == "" {
				raw = primaryURL
			}
			if err := a.validateURL(raw); err != nil {
				return startRequest{}, fmt.Errorf("%s", a.tr("err.pageURL", a.cfgName(cfg, i), err.Error()))
			}
			base = originOf(raw)
		} else {
			vars, bad := parseVars(cfg.vars)
			if bad != "" {
				return startRequest{}, fmt.Errorf("%s", a.tr("err.vars", a.cfgName(cfg, i), bad))
			}
			b, missing := renderTemplate(tmpl, vars)
			if len(missing) > 0 {
				return startRequest{}, fmt.Errorf("%s", a.tr("err.template", a.cfgName(cfg, i), strings.Join(missing, ", ")))
			}
			if err := a.validateURL(b); err != nil {
				return startRequest{}, fmt.Errorf("%s", a.tr("err.pageURL", a.cfgName(cfg, i), err.Error()))
			}
			base = b
			path := strings.TrimSpace(cfg.url)
			if path == "" {
				path = "/"
			}
			if !strings.HasPrefix(path, "/") {
				path = "/" + path
			}
			raw = base + path
		}
		pages = append(pages, resolvedPage{
			name: a.cfgName(cfg, i), url: raw, base: base,
			resW: clampRes(cfg.resW, a.resW), resH: clampRes(cfg.resH, a.resH),
		})
	}
	return startRequest{pages: pages, shareStorage: a.shareStorage, viewMode: a.viewMode}, nil
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
		a.setNotice(err.Error())
		return
	}
	a.mu.Lock()
	if a.status == "cleanup-error" {
		a.mu.Unlock()
		a.setNotice(a.tr("err.cleanupBlocked"))
		return
	}
	if a.status != "idle" {
		a.mu.Unlock()
		return
	}
	a.seq++
	a.taskID = fmt.Sprintf("task-%d", a.seq)
	a.status = "starting"
	a.message = ""
	a.notice = ""
	a.syncUI = map[string]*bool{}
	a.cancel = make(chan struct{})
	cancel := a.cancel
	taskID := a.taskID
	a.mu.Unlock()
	a.win.Invalidate()
	go a.runCountdown(taskID, cancel, req, a.lang)
}

// cancelStart aborts the countdown before any page is created.
func (a *app) cancelStart() {
	a.mu.Lock()
	if a.status == "starting" && a.cancel != nil {
		close(a.cancel)
		a.cancel = nil
	}
	a.mu.Unlock()
	a.win.Invalidate()
}

// stopTask tears down every page and clears its session data.
func (a *app) stopTask() {
	a.mu.Lock()
	if a.status == "idle" || a.status == "stopping" {
		a.mu.Unlock()
		return
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
	a.win.Invalidate()
	go a.cleanup(pages, a.lang)
}

// setActive switches which page the stage shows.
func (a *app) setActive(id string) {
	a.mu.Lock()
	if p := a.pages[id]; p != nil && !p.closed {
		a.activeID = id
	}
	a.mu.Unlock()
	a.win.Invalidate()
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
		p.status = "loading"
		p.lastError = nil
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
	a.win.Invalidate()
}

// screenshotAll captures every running page to a PNG named
// {base}_{pageName}.png inside a per-batch folder next to the executable.
// The stage keeps every page placed (stacked under the active one), so each
// view renders without being brought to the front; CapturePage is
// goroutine-safe.
func (a *app) screenshotAll() {
	a.mu.Lock()
	if a.status != "running" {
		a.mu.Unlock()
		a.setNotice(a.tr("err.notRunning"))
		return
	}
	type shot struct {
		name string
		view *mygo.WebView
	}
	shots := make([]shot, 0, len(a.order))
	for _, id := range a.order {
		if p := a.pages[id]; p != nil && !p.closed {
			shots = append(shots, shot{name: p.name, view: p.view})
		}
	}
	base := strings.TrimSpace(a.shotName)
	a.mu.Unlock()
	if len(shots) == 0 {
		a.setNotice(a.tr("err.noPages"))
		return
	}
	if base == "" {
		base = "screenshot"
	}
	lang := a.lang
	go func() {
		dir, err := screenshotDir(base)
		if err != nil {
			a.setNotice(trLang(lang, "err.shotDir", err.Error()))
			return
		}
		used := map[string]bool{}
		saved, failed := 0, 0
		for _, s := range shots {
			png, err := s.view.CapturePage()
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
		if failed > 0 {
			a.setNotice(trLang(lang, "notice.shotPartial", saved, failed, dir))
			return
		}
		a.setNotice(trLang(lang, "notice.shotDone", saved, dir))
	}()
}

func (a *app) setNotice(msg string) {
	a.mu.Lock()
	a.notice = msg
	a.mu.Unlock()
	a.win.Invalidate()
}

// screenshotDir creates a per-batch folder next to the executable so repeated
// captures never overwrite earlier ones, and returns its absolute path.
func screenshotDir(base string) (string, error) {
	root := "screenshots"
	if exe, err := os.Executable(); err == nil {
		root = filepath.Join(filepath.Dir(exe), "screenshots")
	}
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

// runCountdown waits the 3s grace period, then creates the pages, unless the
// user cancels first.
func (a *app) runCountdown(taskID string, cancel chan struct{}, req startRequest, lang string) {
	select {
	case <-cancel:
		a.mu.Lock()
		if a.taskID == taskID && a.status == "starting" {
			a.status = "idle"
		}
		a.mu.Unlock()
		a.win.Invalidate()
		return
	case <-time.After(3 * time.Second):
	}
	a.createPages(taskID, req, lang)
}

// createPages builds one web view per resolved page with capture preload,
// lockdown handlers and a replay worker, then marks the task running.
func (a *app) createPages(taskID string, req startRequest, lang string) {
	var shared *mygo.Session
	if req.shareStorage {
		shared = mygo.NewEphemeralSession()
	}
	mobile := req.viewMode == "mobile"

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
			PreloadScript:        capturePreload(id),
			TrustedOrigins:       []string{originOf(rp.url)},
			BackgroundThrottling: mygo.ThrottlingOff,
			Session:              session,
		}
		if mobile {
			pageOpts.UserAgent = mobileUA
		}
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
			queue:     make(chan operation, 256),
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

	a.mu.Lock()
	if a.taskID != taskID || a.status != "starting" {
		a.mu.Unlock()
		return
	}
	a.status = "running"
	a.activeID = a.primaryID
	a.mu.Unlock()
	a.win.Invalidate()
}

// attachHandlers locks the page down (no popups, no permissions) and tracks
// its load status for the panel.
func (a *app) attachHandlers(p *pageRecord) {
	page := p.view.Page()
	page.SetWindowOpenHandler(func(mygo.WindowOpenRequest) *mygo.WindowOptions { return nil })
	page.SetPermissionHandler(func(mygo.PermissionRequest) bool { return false })
	page.OnDOMReady(func() {
		a.mu.Lock()
		if !p.closed && p.status != "paused" {
			p.status = "ready"
		}
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
			if p.isPrimary {
				targets = a.followTargets(p, u)
			} else {
				// This page navigated; any replay that was expected to cause
				// it has landed, so follow may act on the next primary move.
				p.navPendingAt = time.Time{}
				if p.pendingNav == u {
					p.pendingNav = ""
				} else if p.pendingNav != "" && len(p.queue) == 0 && !p.replaying {
					nav = p.pendingNav
					p.pendingNav = ""
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
			close(p.queue)
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
	a.pages = map[string]*pageRecord{}
	a.order = nil
	a.activeID = ""
	a.primaryID = ""
	a.cancel = nil
	if failed {
		a.status = "cleanup-error"
		a.message = trLang(lang, "err.cleanupFailed")
	} else {
		a.status = "idle"
		a.message = ""
	}
	a.mu.Unlock()
	a.win.Invalidate()
}

// snapshot builds the panel state under the lock.
func (a *app) snapshot() appState {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := appState{
		Status:   a.status,
		ActiveID: a.activeID,
		Message:  a.message,
		Notice:   a.notice,
		Pages:    make([]pageState, 0, len(a.order)),
		Errors:   []syncError{},
	}
	for _, id := range a.order {
		p := a.pages[id]
		if p == nil {
			continue
		}
		st.Pages = append(st.Pages, pageState{
			ID: p.id, Name: p.name, URL: p.url, Status: p.status,
			SyncOn: p.syncOn, IsPrimary: p.isPrimary, Error: p.lastError,
		})
		if p.lastError != nil {
			st.Errors = append(st.Errors, *p.lastError)
		}
	}
	return st
}

// pushState redraws the native panel. Page handlers call it off the main
// thread; Invalidate is safe from any goroutine.
func (a *app) pushState() {
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

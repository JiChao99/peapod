package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestResolveMCPRequestPlain(t *testing.T) {
	a := newApp()
	req, err := a.resolveMCPRequest(startIn{Pages: []mcpPage{
		{URL: "https://example.com/a"},
		{URL: ""},
	}})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if len(req.pages) != 2 {
		t.Fatalf("want 2 pages, got %d", len(req.pages))
	}
	if req.pages[0].url != "https://example.com/a" || req.pages[0].base != "https://example.com" {
		t.Fatalf("primary = %+v", req.pages[0])
	}
	if req.pages[1].url != "https://example.com/a" {
		t.Fatalf("follower should inherit primary URL, got %q", req.pages[1].url)
	}
	if req.pages[0].name != "Primary" || req.pages[1].name != "Page 2" {
		t.Fatalf("default names wrong: %q %q", req.pages[0].name, req.pages[1].name)
	}
	if req.viewMode != "desktop" {
		t.Fatalf("default viewMode = %q", req.viewMode)
	}
}

func TestResolveMCPRequestTemplate(t *testing.T) {
	a := newApp()
	req, err := a.resolveMCPRequest(startIn{
		Template: "https://{host}/{lang}",
		Pages: []mcpPage{
			{Vars: "host=a.com, lang=en", Path: "/cart"},
			{Vars: "host=b.com, lang=de"},
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if req.pages[0].base != "https://a.com/en" || req.pages[0].url != "https://a.com/en/cart" {
		t.Fatalf("page0 = %+v", req.pages[0])
	}
	if req.pages[1].base != "https://b.com/de" || req.pages[1].url != "https://b.com/de/" {
		t.Fatalf("page1 = %+v", req.pages[1])
	}
}

func TestResolveMCPRequestTemplateQuery(t *testing.T) {
	a := newApp()
	req, err := a.resolveMCPRequest(startIn{
		Template: "https://{host}/?lang={lang}",
		Pages: []mcpPage{
			{Vars: "host=a.com, lang=0", Path: "/shop?page=1"},
			{Vars: "host=a.com, lang=2"},
		},
	})
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if req.pages[0].base != "https://a.com?lang=0" ||
		req.pages[0].url != "https://a.com/shop?lang=0&page=1" {
		t.Errorf("page0 = %+v", req.pages[0])
	}
	if req.pages[1].base != "https://a.com?lang=2" ||
		req.pages[1].url != "https://a.com/?lang=2" {
		t.Errorf("page1 = %+v", req.pages[1])
	}
}

func TestResolveMCPRequestErrors(t *testing.T) {
	a := newApp()
	cases := []struct {
		name string
		in   startIn
	}{
		{"too few", startIn{Pages: []mcpPage{{URL: "https://a.com"}}}},
		{"bad primary", startIn{Pages: []mcpPage{{URL: "ftp://a.com"}, {URL: ""}}}},
		{"missing var", startIn{Template: "https://{host}", Pages: []mcpPage{{Vars: ""}, {Vars: ""}}}},
		{"bad var", startIn{Template: "https://{host}", Pages: []mcpPage{{Vars: "nope"}, {Vars: ""}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := a.resolveMCPRequest(c.in); err == nil {
				t.Fatalf("expected error for %s", c.name)
			}
		})
	}
}

func TestMCPAuth(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := mcpAuth("secret-token", next)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/mcp", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: want 401, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token: want 401, got %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	r.Header.Set("Authorization", "Bearer secret-token")
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: want 200, got %d", rec.Code)
	}
}

// connectMCP wires an app's tools to an in-memory client for integration tests.
func connectMCP(t *testing.T, a *app) *sdk.ClientSession {
	t.Helper()
	srv := sdk.NewServer(&sdk.Implementation{Name: "peapod-test", Version: "0"}, nil)
	a.registerMCPTools(srv)
	ct, st := sdk.NewInMemoryTransports()
	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := sdk.NewClient(&sdk.Implementation{Name: "test", Version: "0"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func resultText(t *testing.T, res *sdk.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatalf("empty result content")
	}
	tc, ok := res.Content[0].(*sdk.TextContent)
	if !ok {
		t.Fatalf("first content is %T, want TextContent", res.Content[0])
	}
	return tc.Text
}

func TestMCPStatusTool(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "peapod_status"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("status returned error: %s", resultText(t, res))
	}
	var st appState
	if err := json.Unmarshal([]byte(resultText(t, res)), &st); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if st.Status != "idle" {
		t.Fatalf("want idle, got %q", st.Status)
	}
}

func TestStateJSONContract(t *testing.T) {
	st := appState{
		Status: "running", ActiveID: "p1",
		Pages:  []pageState{{ID: "p1", Name: "Primary", URL: "https://a.com/", Status: "ready", SyncOn: true, IsPrimary: true}},
		Errors: []syncError{},
	}
	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, k := range []string{"status", "pages", "activeId", "errors", "message", "notice", "recording"} {
		if _, ok := top[k]; !ok {
			t.Fatalf("missing documented key %q in %s", k, raw)
		}
	}
	for _, k := range []string{"Status", "Pages", "ActiveID"} {
		if _, ok := top[k]; ok {
			t.Fatalf("unexpected capitalized key %q in %s", k, raw)
		}
	}
	var pages []map[string]json.RawMessage
	if err := json.Unmarshal(top["pages"], &pages); err != nil {
		t.Fatalf("decode pages: %v", err)
	}
	for _, k := range []string{"id", "name", "url", "status", "syncOn", "isPrimary", "error"} {
		if _, ok := pages[0][k]; !ok {
			t.Fatalf("missing page key %q in %s", k, raw)
		}
	}
}

func TestStopTaskRejectsCleanupError(t *testing.T) {
	a := newApp()
	a.mu.Lock()
	a.status = "cleanup-error"
	a.mu.Unlock()
	if err := a.stopTask(); err == nil {
		t.Fatal("stopTask must reject cleanup-error, got nil")
	}
	a.mu.Lock()
	got := a.status
	a.mu.Unlock()
	if got != "cleanup-error" {
		t.Fatalf("stopTask cleared the restart lockout: status = %q", got)
	}
	a.mu.Lock()
	a.status = "idle"
	a.mu.Unlock()
	if err := a.stopTask(); err != nil {
		t.Fatalf("idle stopTask should be a no-op, got %v", err)
	}
}

func TestMCPStopCleanupError(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	a.mu.Lock()
	a.status = "cleanup-error"
	a.mu.Unlock()
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "peapod_stop"})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("peapod_stop must surface the lockout, got %s", resultText(t, res))
	}
	a.mu.Lock()
	got := a.status
	a.mu.Unlock()
	if got != "cleanup-error" {
		t.Fatalf("lockout cleared via MCP: status = %q", got)
	}
}

func TestMCPStartRejectsBadInput(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_start",
		Arguments: map[string]any{"pages": []map[string]any{{"url": "https://a.com"}}},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError for a single page, got %s", resultText(t, res))
	}
}

func TestMCPSendOperationNotRunning(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	// A string value must pass schema validation (json.RawMessage would infer
	// []byte and reject it); the tool then reports not-running as IsError.
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name: "peapod_send_operation",
		Arguments: map[string]any{"operation": map[string]any{
			"type":   "input",
			"anchor": map[string]any{"type": "id", "selector": "#note"},
			"value":  "hello",
		}},
	})
	if err != nil {
		t.Fatalf("call (schema should accept string value): %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError when not running, got %s", resultText(t, res))
	}
}

func TestMCPOperationToOperation(t *testing.T) {
	m := mcpOperation{
		Type:   "input",
		Anchor: anchor{Type: "id", Selector: "#note"},
		Value:  "hello",
	}
	op, err := m.toOperation()
	if err != nil {
		t.Fatalf("toOperation: %v", err)
	}
	if string(op.Value) != `"hello"` {
		t.Fatalf("value marshaled to %s, want \"hello\"", op.Value)
	}
	if err := validateOperation(&op); err != nil {
		t.Fatalf("validate: %v", err)
	}
	// nil value stays absent so replay does not clear the field.
	m2 := mcpOperation{Type: "click", Anchor: anchor{Type: "id", Selector: "#go"}}
	op2, _ := m2.toOperation()
	if op2.Value != nil {
		t.Fatalf("nil value should stay nil, got %s", op2.Value)
	}
}

func TestMCPToolsListed(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	want := map[string]bool{
		"peapod_status": false, "peapod_start": false, "peapod_stop": false,
		"peapod_navigate": false, "peapod_reload": false, "peapod_set_sync": false,
		"peapod_send_operation": false, "peapod_replay": false, "peapod_screenshot": false, "peapod_diagnostics": false,
	}
	for _, tool := range res.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Errorf("tool %q not registered", name)
		}
	}
}

func TestResolveMCPRequestViewportDefaults(t *testing.T) {
	a := newApp()
	// The panel preset must not leak into MCP defaults.
	a.resW, a.resH = 999, 999

	desktop, err := a.resolveMCPRequest(startIn{Pages: []mcpPage{{URL: "https://a.com"}, {URL: ""}}})
	if err != nil {
		t.Fatalf("desktop: %v", err)
	}
	for i, p := range desktop.pages {
		if p.resW != 1280 || p.resH != 800 {
			t.Errorf("desktop page %d = %vx%v, want 1280x800", i, p.resW, p.resH)
		}
	}

	mobile, err := a.resolveMCPRequest(startIn{ViewMode: "mobile", Pages: []mcpPage{{URL: "https://a.com"}, {URL: ""}}})
	if err != nil {
		t.Fatalf("mobile: %v", err)
	}
	for i, p := range mobile.pages {
		if p.resW != 390 || p.resH != 844 {
			t.Errorf("mobile page %d = %vx%v, want 390x844", i, p.resW, p.resH)
		}
	}

	// Explicit dimensions override the mode default and are clamped.
	custom, err := a.resolveMCPRequest(startIn{ViewMode: "mobile", Pages: []mcpPage{
		{URL: "https://a.com", ResW: 1024, ResH: 768},
		{URL: "", ResW: 5000, ResH: 100},
	}})
	if err != nil {
		t.Fatalf("custom: %v", err)
	}
	if custom.pages[0].resW != 1024 || custom.pages[0].resH != 768 {
		t.Errorf("explicit = %vx%v, want 1024x768", custom.pages[0].resW, custom.pages[0].resH)
	}
	if custom.pages[1].resW != 3840 || custom.pages[1].resH != 200 {
		t.Errorf("clamped = %vx%v, want 3840x200", custom.pages[1].resW, custom.pages[1].resH)
	}
}

func TestResolveMCPRequestDiagnostics(t *testing.T) {
	a := newApp()
	pages := []mcpPage{{URL: "https://a.com"}, {URL: ""}}

	// Default off: the invasive collector must not be injected unless asked.
	off, err := a.resolveMCPRequest(startIn{Pages: pages})
	if err != nil {
		t.Fatalf("off: %v", err)
	}
	if off.diagnostics {
		t.Errorf("diagnostics should default to false")
	}

	on, err := a.resolveMCPRequest(startIn{Pages: pages, Diagnostics: true})
	if err != nil {
		t.Fatalf("on: %v", err)
	}
	if !on.diagnostics {
		t.Errorf("diagnostics:true should propagate to startRequest")
	}
}

func TestResolveMCPRequestRecord(t *testing.T) {
	a := newApp()
	pages := []mcpPage{{URL: "https://a.com"}, {URL: ""}}

	off, err := a.resolveMCPRequest(startIn{Pages: pages})
	if err != nil {
		t.Fatalf("off: %v", err)
	}
	if off.record {
		t.Errorf("record should default to false")
	}

	on, err := a.resolveMCPRequest(startIn{Pages: pages, Record: true})
	if err != nil {
		t.Fatalf("on: %v", err)
	}
	if !on.record {
		t.Errorf("record:true should propagate to startRequest")
	}
}

func TestWriteDiscovery(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "mcp.json")
	// A pre-existing world-readable file must not keep its mode or content.
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	m := &mcpServer{url: "http://127.0.0.1:1234/mcp", token: "tok"}
	if err := m.writeDiscoveryTo(dir); err != nil {
		t.Fatalf("writeDiscoveryTo: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got map[string]string
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got["url"] != m.url || got["token"] != m.token {
		t.Fatalf("discovery = %v, want url/token", got)
	}
	// The temp file was renamed away, leaving only mcp.json.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "mcp.json" {
			t.Errorf("unexpected leftover file %q", e.Name())
		}
	}
	// Windows does not honor Unix mode bits; assert them only where meaningful.
	if runtime.GOOS != "windows" {
		if fi, err := os.Stat(target); err == nil && fi.Mode().Perm() != 0o600 {
			t.Errorf("file mode = %v, want 0600", fi.Mode().Perm())
		}
		if di, err := os.Stat(dir); err == nil && di.Mode().Perm() != 0o700 {
			t.Errorf("dir mode = %v, want 0700", di.Mode().Perm())
		}
	}
}

func TestResolveMCPRequestUserAgent(t *testing.T) {
	a := newApp()
	// The panel's preset and view mode must not leak into MCP defaults.
	a.uaPreset = "firefox"
	a.uaCustom = "PanelUA/1.0"
	a.viewMode = "mobile"
	pages := []mcpPage{{URL: "https://a.com"}, {URL: ""}}

	desktop, err := a.resolveMCPRequest(startIn{Pages: pages})
	if err != nil {
		t.Fatalf("desktop: %v", err)
	}
	if desktop.userAgent != "" {
		t.Errorf("desktop default userAgent = %q, want engine default", desktop.userAgent)
	}

	mobile, err := a.resolveMCPRequest(startIn{ViewMode: "mobile", Pages: pages})
	if err != nil {
		t.Fatalf("mobile: %v", err)
	}
	if mobile.userAgent != mobileUA {
		t.Errorf("mobile default userAgent = %q, want mobileUA", mobile.userAgent)
	}

	// An explicit UA wins over the mode default and is trimmed.
	explicit, err := a.resolveMCPRequest(startIn{ViewMode: "mobile", UserAgent: "  MyUA/1.0  ", Pages: pages})
	if err != nil {
		t.Fatalf("explicit: %v", err)
	}
	if explicit.userAgent != "MyUA/1.0" {
		t.Errorf("explicit userAgent = %q, want trimmed MyUA/1.0", explicit.userAgent)
	}
}

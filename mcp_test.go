package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		"peapod_send_operation": false, "peapod_screenshot": false,
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
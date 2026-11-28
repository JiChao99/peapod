package main

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestNormalizeVerificationRequest(t *testing.T) {
	a := newApp()
	checks, timeout, err := a.normalizeVerificationRequest(verificationRequest{
		Checks: []verificationCheck{
			{Kind: " TEXT ", Selector: " #total ", Value: " 10.99 "},
			{Kind: "dom", Selector: "#pay"},
			{Kind: "url", Operator: "not_contains", Value: "/error"},
			{Kind: "http", Operator: "equals", Value: "0200"},
			{Kind: "console"},
		},
		TimeoutMs: 999999,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(checks) != 5 {
		t.Fatalf("checks = %d, want 5", len(checks))
	}
	if checks[0].Kind != "text" || checks[0].Operator != "contains" || checks[0].Selector != "#total" || checks[0].Value != "10.99" {
		t.Fatalf("text check not normalized: %+v", checks[0])
	}
	if checks[1].Operator != "visible" || checks[3].Value != "200" || checks[4].Operator != "clean" {
		t.Fatalf("default operators/status not normalized: %+v", checks)
	}
	if timeout != maxVerificationTimeout {
		t.Fatalf("timeout = %s, want %s", timeout, maxVerificationTimeout)
	}
	_, timeout, err = a.normalizeVerificationRequest(verificationRequest{Checks: []verificationCheck{{Kind: "console"}}, TimeoutMs: math.MaxFloat64})
	if err != nil || timeout != maxVerificationTimeout {
		t.Fatalf("overflowing timeout = %s, err = %v", timeout, err)
	}

	_, timeout, err = a.normalizeVerificationRequest(verificationRequest{Checks: []verificationCheck{{Kind: "console"}}, TimeoutMs: 1})
	if err != nil || timeout != minVerificationTimeout {
		t.Fatalf("minimum timeout = %s, err = %v", timeout, err)
	}
	_, timeout, err = a.normalizeVerificationRequest(verificationRequest{Checks: []verificationCheck{{Kind: "console"}}})
	if err != nil || timeout != defaultVerificationTimeout {
		t.Fatalf("default timeout = %s, err = %v", timeout, err)
	}
}

func TestNormalizeVerificationRequestRejectsInvalidInput(t *testing.T) {
	a := newApp()
	cases := []struct {
		name   string
		checks []verificationCheck
	}{
		{"empty", nil},
		{"too many", make([]verificationCheck, maxVerificationChecks+1)},
		{"kind", []verificationCheck{{Kind: "script", Value: "alert(1)"}}},
		{"operator", []verificationCheck{{Kind: "text", Operator: "regex", Value: "x"}}},
		{"text value", []verificationCheck{{Kind: "text"}}},
		{"dom selector", []verificationCheck{{Kind: "dom"}}},
		{"dom value", []verificationCheck{{Kind: "dom", Selector: "#x", Value: "x"}}},
		{"url value", []verificationCheck{{Kind: "url"}}},
		{"url selector", []verificationCheck{{Kind: "url", Selector: "#x", Value: "/x"}}},
		{"http selector", []verificationCheck{{Kind: "http", Selector: "#x"}}},
		{"http status", []verificationCheck{{Kind: "http", Operator: "equals", Value: "999"}}},
		{"console value", []verificationCheck{{Kind: "console", Value: "x"}}},
		{"selector size", []verificationCheck{{Kind: "dom", Selector: strings.Repeat("x", maxVerificationSelectorSize+1)}}},
		{"value size", []verificationCheck{{Kind: "text", Value: strings.Repeat("x", maxVerificationValueSize+1)}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := a.normalizeVerificationRequest(verificationRequest{Checks: tc.checks}); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestEvaluateVerificationChecks(t *testing.T) {
	checks := []verificationCheck{
		{Kind: "text", Operator: "contains", Value: "Order total 10.99"},
		{Kind: "dom", Selector: "#pay", Operator: "visible"},
		{Kind: "url", Operator: "contains", Value: "/checkout"},
		{Kind: "http", Operator: "success"},
		{Kind: "console", Operator: "clean"},
	}
	observation := verificationObservation{
		Href:                 "https://dev.example.com/checkout?lang=en",
		ReadyState:           "complete",
		HTTPStatus:           200,
		Text:                 "Welcome\nOrder   total 10.99\nThank you",
		DiagnosticsAvailable: true,
		Checks: []verificationObservedCheck{
			{Index: 0, Exists: true, Text: "Order total 10.99"},
			{Index: 1, Exists: true, Visible: true},
		},
	}
	results := evaluateVerificationChecks(checks, observation)
	if !verificationResultsPass(results) {
		t.Fatalf("expected all checks to pass: %+v", results)
	}
	if results[0].Actual != "Welcome Order total 10.99 Thank you" {
		t.Fatalf("text was not normalized: %q", results[0].Actual)
	}

	observation.HTTPStatus = 500
	observation.ConsoleErrors = []string{"boom"}
	observation.Checks[1].Visible = false
	results = evaluateVerificationChecks(checks, observation)
	for i, want := range []bool{true, false, true, false, false} {
		if results[i].Pass != want {
			t.Errorf("check %d pass = %v, want %v: %+v", i, results[i].Pass, want, results[i])
		}
	}
}

func TestEvaluateVerificationChecksWaitsForDocument(t *testing.T) {
	results := evaluateVerificationChecks([]verificationCheck{{Kind: "url", Value: "/ready"}}, verificationObservation{
		Href: "/ready", ReadyState: "loading",
	})
	if results[0].Pass || results[0].Actual != "document loading" {
		t.Fatalf("loading document should not pass: %+v", results[0])
	}
}

func TestEvaluateVerificationChecksSelectorError(t *testing.T) {
	results := evaluateVerificationChecks([]verificationCheck{{Kind: "dom", Selector: "["}}, verificationObservation{
		ReadyState: "complete",
		Checks:     []verificationObservedCheck{{Error: "invalid selector"}},
	})
	if results[0].Pass || results[0].Error != "invalid selector" {
		t.Fatalf("selector error not surfaced: %+v", results[0])
	}
}

func TestVerificationTextOperators(t *testing.T) {
	cases := []struct {
		operator, actual, expected string
		want                       bool
	}{
		{"equals", "Order   10.99", "order 10.99", false},
		{"equals", "Order   10.99", "Order 10.99", true},
		{"not_contains", "Paid", "failed", true},
		{"not_contains", "Payment failed", "failed", false},
		{"contains", "Payment failed", "failed", true},
	}
	for _, tc := range cases {
		if got := verificationTextMatch(tc.operator, tc.actual, tc.expected); got != tc.want {
			t.Errorf("verificationTextMatch(%q, %q, %q) = %v, want %v", tc.operator, tc.actual, tc.expected, got, tc.want)
		}
	}
}

func TestBuildVerificationDifferences(t *testing.T) {
	checks := []verificationCheck{{Kind: "text", Value: "10.99"}, {Kind: "http"}}
	pages := []verificationPageResult{
		{PageID: "p1", Name: "Primary", Results: []verificationCheckResult{{Actual: "10.99", Pass: true}, {Actual: "200", Pass: true}}},
		{PageID: "p2", Name: "Staging", Results: []verificationCheckResult{{Actual: "9.99", Pass: false}, {Actual: "200", Pass: true}}},
	}
	differences := buildVerificationDifferences(checks, pages)
	if len(differences) != 1 || differences[0].Check != 1 || differences[0].Kind != "text" {
		t.Fatalf("differences = %+v", differences)
	}
	if len(differences[0].Values) != 2 || differences[0].Values[1].Page != "Staging" || differences[0].Values[1].Pass {
		t.Fatalf("difference values = %+v", differences[0].Values)
	}
}

func TestDecodeVerificationEval(t *testing.T) {
	var got verificationObservation
	if err := decodeVerificationEval(`{"href":"https://example.com","readyState":"complete"}`, &got); err != nil {
		t.Fatal(err)
	}
	if got.Href != "https://example.com" {
		t.Fatalf("href = %q", got.Href)
	}
	if err := decodeVerificationEval(map[string]any{"href": "https://example.org"}, &got); err != nil {
		t.Fatal(err)
	}
	if got.Href != "https://example.org" {
		t.Fatalf("map href = %q", got.Href)
	}
}

func TestVerificationScriptUsesBoundedDataProbe(t *testing.T) {
	selector := `img[src="x"]); alert(1); //`
	script := verificationScript([]verificationCheck{{Kind: "dom", Selector: selector}})
	if !strings.Contains(script, "document.querySelector(check.selector)") {
		t.Fatal("script must use selectors as querySelector data")
	}
	if strings.Contains(script, "const MAX = 200") {
		t.Fatal("verification must not install diagnostics hooks for ordinary checks")
	}
	if !strings.Contains(script, "diagnosticsAvailable") || !strings.Contains(script, "JSON.stringify") || !strings.Contains(script, ".slice(-20)") {
		t.Fatal("script must report diagnostics availability and return bounded JSON evidence")
	}
	payload, err := json.Marshal([]verificationCheck{{Kind: "dom", Selector: selector}})
	if err != nil || !strings.Contains(script, string(payload)) {
		t.Fatal("selector payload was not JSON encoded")
	}
}

func TestConsoleCheckRequiresDocumentStartDiagnostics(t *testing.T) {
	results := evaluateVerificationChecks([]verificationCheck{{Kind: "console", Operator: "clean"}}, verificationObservation{ReadyState: "complete"})
	if results[0].Pass || results[0].Actual != "document-start diagnostics unavailable" {
		t.Fatalf("console check without evidence must fail: %+v", results[0])
	}
	results = evaluateVerificationChecks([]verificationCheck{{Kind: "console", Operator: "clean"}}, verificationObservation{
		ReadyState: "complete", DiagnosticsAvailable: true,
	})
	if !results[0].Pass {
		t.Fatalf("console check with empty collected evidence should pass: %+v", results[0])
	}
}

func TestDiagnosticsPreloadExposesSnapshots(t *testing.T) {
	script := diagPreload()
	if !strings.Contains(script, "Object.defineProperty(window, '__peapodDiag'") || !strings.Contains(script, "console: diag.console.map(item => ({ ...item }))") {
		t.Fatal("diagnostics global must expose deep snapshots instead of mutable buffers")
	}
}

func TestNormalizeVerificationCheckForUIClearsHiddenFields(t *testing.T) {
	check := verificationCheck{Kind: "text", Selector: "#total", Operator: "contains", Value: "10.99"}
	check.Kind = "dom"
	check.Operator = "visible"
	normalizeVerificationCheckForUI(&check)
	if check.Value != "" || check.Selector != "#total" {
		t.Fatalf("dom check = %+v", check)
	}
	check.Kind = "url"
	check.Operator = "contains"
	check.Value = "/checkout"
	normalizeVerificationCheckForUI(&check)
	if check.Selector != "" {
		t.Fatalf("url check retained selector: %+v", check)
	}
	check.Kind = "http"
	check.Operator = "success"
	check.Value = "200"
	normalizeVerificationCheckForUI(&check)
	if check.Value != "" {
		t.Fatalf("http success retained status: %+v", check)
	}
}

func TestRunVerificationCanceledContextDoesNotBusy(t *testing.T) {
	a := newApp()
	a.status = "running"
	a.taskID = "task-1"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := a.runVerification(ctx, verificationRequest{Checks: []verificationCheck{{Kind: "console"}}}); err == nil {
		t.Fatal("expected cancellation error")
	}
	if a.verificationBusy {
		t.Fatal("canceled verification left busy set")
	}
}

func TestRunVerificationNoPagesDoesNotBusy(t *testing.T) {
	a := newApp()
	a.status = "running"
	a.taskID = "task-1"
	if _, err := a.runVerification(context.Background(), verificationRequest{Checks: []verificationCheck{{Kind: "console"}}}); err == nil {
		t.Fatal("expected no-page error")
	}
	if a.verificationBusy {
		t.Fatal("no-page verification left busy set")
	}
}

func TestMCPVerifyRequiresRunningTask(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_verify",
		Arguments: map[string]any{"checks": []any{map[string]any{"kind": "console"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected error result, got %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), a.tr("err.verifyNotRunning")) {
		t.Fatalf("unexpected error: %s", resultText(t, res))
	}
}

func TestVerificationReportInStatusSnapshot(t *testing.T) {
	a := newApp()
	report := &verificationReport{TaskID: "task-1", RunID: "run-1", Status: "pass", Pages: []verificationPageResult{}}
	a.verification = report
	a.verificationBusy = true
	st := a.snapshot()
	if st.Verification != report || !st.VerifyBusy {
		t.Fatalf("snapshot missing verification state: %+v", st)
	}
}

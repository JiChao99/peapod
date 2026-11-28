package main

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxVerificationChecks       = 5
	maxVerificationSelectorSize = 512
	maxVerificationValueSize    = 2000
	defaultVerificationTimeout  = 5 * time.Second
	minVerificationTimeout      = 250 * time.Millisecond
	maxVerificationTimeout      = 30 * time.Second
	verificationPollInterval    = 100 * time.Millisecond
	verificationEvalTimeout     = 2 * time.Second
)

type verificationCheck struct {
	Kind     string `json:"kind" jsonschema:"text, dom, url, http or console."`
	Selector string `json:"selector,omitempty" jsonschema:"CSS selector for text/dom checks; max 512 bytes. Text checks may omit it to use body text."`
	Operator string `json:"operator,omitempty" jsonschema:"text/url: contains, equals, not_contains; dom: visible, exists; http: success, equals; console: clean."`
	Value    string `json:"value,omitempty" jsonschema:"Expected text, URL or HTTP status; max 2000 bytes."`
}

type verificationRequest struct {
	Checks    []verificationCheck `json:"checks"`
	TimeoutMs float64             `json:"timeoutMs,omitempty"`
}

type verificationReport struct {
	TaskID      string                   `json:"taskId"`
	RunID       string                   `json:"runId"`
	StartedAt   string                   `json:"startedAt"`
	EndedAt     string                   `json:"endedAt"`
	TimeoutMs   int                      `json:"timeoutMs"`
	Status      string                   `json:"status"`
	Checks      []verificationCheck      `json:"checks"`
	Pages       []verificationPageResult `json:"pages"`
	Differences []verificationDifference `json:"differences"`
	ReportPath  string                   `json:"reportPath,omitempty"`
	ReportError string                   `json:"reportError,omitempty"`
	Error       string                   `json:"error,omitempty"`
}

type verificationPageResult struct {
	PageID          string                    `json:"pageId"`
	Name            string                    `json:"name"`
	URL             string                    `json:"url"`
	Status          string                    `json:"status"`
	Results         []verificationCheckResult `json:"results"`
	Screenshot      string                    `json:"screenshot,omitempty"`
	ScreenshotError string                    `json:"screenshotError,omitempty"`
	Error           string                    `json:"error,omitempty"`
}

type verificationCheckResult struct {
	Kind     string `json:"kind"`
	Selector string `json:"selector,omitempty"`
	Operator string `json:"operator"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Pass     bool   `json:"pass"`
	Error    string `json:"error,omitempty"`
}

type verificationDifference struct {
	Check  int                   `json:"check"`
	Kind   string                `json:"kind"`
	Values []verificationValueAt `json:"values"`
}

type verificationValueAt struct {
	PageID string `json:"pageId"`
	Page   string `json:"page"`
	Actual string `json:"actual"`
	Pass   bool   `json:"pass"`
}

type verificationObservation struct {
	Href                 string                      `json:"href"`
	ReadyState           string                      `json:"readyState"`
	HTTPStatus           int                         `json:"httpStatus"`
	Text                 string                      `json:"text"`
	ConsoleErrors        []string                    `json:"consoleErrors"`
	DiagnosticsAvailable bool                        `json:"diagnosticsAvailable"`
	Checks               []verificationObservedCheck `json:"checks"`
}

type verificationObservedCheck struct {
	Index   int    `json:"index"`
	Exists  bool   `json:"exists"`
	Visible bool   `json:"visible"`
	Text    string `json:"text"`
	Error   string `json:"error"`
}

func (a *app) normalizeVerificationRequest(req verificationRequest) ([]verificationCheck, time.Duration, error) {
	if len(req.Checks) == 0 || len(req.Checks) > maxVerificationChecks {
		return nil, 0, fmt.Errorf("%s", a.tr("err.verifyChecks", maxVerificationChecks))
	}
	checks := make([]verificationCheck, 0, len(req.Checks))
	for i, raw := range req.Checks {
		c := verificationCheck{
			Kind:     strings.ToLower(strings.TrimSpace(raw.Kind)),
			Selector: strings.TrimSpace(raw.Selector),
			Operator: strings.ToLower(strings.TrimSpace(raw.Operator)),
			Value:    strings.TrimSpace(raw.Value),
		}
		switch c.Kind {
		case "text", "dom", "url", "http", "console":
		default:
			return nil, 0, fmt.Errorf("%s", a.tr("err.verifyKind", i+1))
		}
		if c.Operator == "" {
			c.Operator = defaultVerificationOperator(c.Kind)
		}
		if !validVerificationOperator(c.Kind, c.Operator) {
			return nil, 0, fmt.Errorf("%s", a.tr("err.verifyOperator", i+1))
		}
		if len(c.Selector) > maxVerificationSelectorSize {
			return nil, 0, fmt.Errorf("%s", a.tr("err.verifySelector", i+1, maxVerificationSelectorSize))
		}
		if len(c.Value) > maxVerificationValueSize {
			return nil, 0, fmt.Errorf("%s", a.tr("err.verifyValue", i+1, maxVerificationValueSize))
		}
		switch c.Kind {
		case "text":
			if c.Value == "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifyValueRequired", i+1))
			}
		case "dom":
			if c.Selector == "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifySelectorRequired", i+1))
			}
			if c.Value != "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifyValueUnexpected", i+1))
			}
		case "url":
			if c.Value == "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifyValueRequired", i+1))
			}
			if c.Selector != "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifySelectorUnexpected", i+1))
			}
		case "http":
			if c.Selector != "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifySelectorUnexpected", i+1))
			}
			if c.Operator == "equals" {
				status, err := strconv.Atoi(c.Value)
				if err != nil || status < 100 || status > 599 {
					return nil, 0, fmt.Errorf("%s", a.tr("err.verifyHTTP", i+1))
				}
				c.Value = strconv.Itoa(status)
			} else if c.Value != "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifyValueUnexpected", i+1))
			}
		case "console":
			if c.Value != "" || c.Selector != "" {
				return nil, 0, fmt.Errorf("%s", a.tr("err.verifyConsole", i+1))
			}
		default:
			return nil, 0, fmt.Errorf("%s", a.tr("err.verifyKind", i+1))
		}
		checks = append(checks, c)
	}
	timeout := defaultVerificationTimeout
	if req.TimeoutMs > 0 {
		ms := min(max(req.TimeoutMs, float64(minVerificationTimeout/time.Millisecond)), float64(maxVerificationTimeout/time.Millisecond))
		timeout = time.Duration(ms) * time.Millisecond
	}
	timeout = min(max(timeout, minVerificationTimeout), maxVerificationTimeout)
	return checks, timeout, nil
}

func defaultVerificationOperator(kind string) string {
	switch kind {
	case "text", "url":
		return "contains"
	case "dom":
		return "visible"
	case "http":
		return "success"
	case "console":
		return "clean"
	default:
		return ""
	}
}

func validVerificationOperator(kind, operator string) bool {
	switch kind {
	case "text", "url":
		return operator == "contains" || operator == "equals" || operator == "not_contains"
	case "dom":
		return operator == "visible" || operator == "exists"
	case "http":
		return operator == "success" || operator == "equals"
	case "console":
		return operator == "clean"
	default:
		return false
	}
}

func (a *app) runVerification(ctx context.Context, req verificationRequest) (*verificationReport, error) {
	checks, timeout, err := a.normalizeVerificationRequest(req)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	a.mu.Lock()
	if a.status != "running" {
		a.mu.Unlock()
		return nil, fmt.Errorf("%s", a.tr("err.verifyNotRunning"))
	}
	if a.verificationBusy {
		a.mu.Unlock()
		return nil, fmt.Errorf("%s", a.tr("err.verifyBusy"))
	}
	taskID := a.taskID
	taskCancel := a.cancel
	pages := make([]*pageRecord, 0, len(a.order))
	for _, id := range a.order {
		if p := a.pages[id]; p != nil && !p.closed {
			pages = append(pages, p)
		}
	}
	if len(pages) == 0 {
		a.mu.Unlock()
		return nil, fmt.Errorf("%s", a.tr("err.verifyNotRunning"))
	}
	a.verificationBusy = true
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		a.verificationBusy = false
		a.mu.Unlock()
		a.invalidate()
	}()

	runCtx, cancel := context.WithCancel(ctx)
	ctx = runCtx
	defer cancel()
	if taskCancel != nil {
		go func() {
			select {
			case <-taskCancel:
				cancel()
			case <-ctx.Done():
			}
		}()
	}

	dir, err := newVerificationDir()
	if err != nil {
		return nil, err
	}
	report := &verificationReport{
		TaskID:    taskID,
		RunID:     filepath.Base(dir),
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		TimeoutMs: int(timeout / time.Millisecond),
		Status:    "pass",
		Checks:    checks,
		Pages:     make([]verificationPageResult, len(pages)),
	}

	var wg sync.WaitGroup
	for i, p := range pages {
		wg.Add(1)
		go func(i int, p *pageRecord) {
			defer wg.Done()
			report.Pages[i] = a.verifyPage(ctx, p, i, checks, timeout, dir)
		}(i, p)
	}
	wg.Wait()
	report.EndedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := ctx.Err(); err != nil {
		report.Status = "fail"
		report.Error = err.Error()
	}
	for _, page := range report.Pages {
		if page.Status != "pass" {
			report.Status = "fail"
		}
	}
	report.Differences = buildVerificationDifferences(report.Checks, report.Pages)
	path, err := writeVerificationReport(dir, report, a.currentLang())
	if err != nil {
		report.ReportError = err.Error()
	} else {
		report.ReportPath = path
	}

	a.mu.Lock()
	if a.taskID == taskID && a.status == "running" {
		a.verification = report
	}
	a.mu.Unlock()
	a.invalidate()
	return report, nil
}

func (a *app) verifyPage(ctx context.Context, p *pageRecord, index int, checks []verificationCheck, timeout time.Duration, dir string) verificationPageResult {
	a.mu.Lock()
	page := verificationPageResult{PageID: p.id, Name: p.name, URL: p.url, Status: "fail"}
	navError := p.navError
	pageStatus := p.status
	closed := p.closed
	diagnostics := p.diagnostics
	view := p.view
	a.mu.Unlock()
	fail := func(reason string) {
		page.Error = reason
		page.Results = failedVerificationResults(checks, reason)
	}
	if closed || view == nil {
		fail("page closed")
		return page
	}
	if navError != nil {
		fail(navError.Name)
	} else if pageStatus == "error" {
		fail("page navigation failed")
	} else {
		script := verificationScript(checks)
		deadline := time.Now().Add(timeout)
		var observation *verificationObservation
		var evalErr error
		for time.Now().Before(deadline) {
			a.mu.Lock()
			currentStatus := p.status
			currentNavError := p.navError
			currentClosed := p.closed
			navSeq := p.navSeq
			a.mu.Unlock()
			if currentClosed {
				fail("page closed")
				break
			}
			if currentNavError != nil {
				fail(currentNavError.Name)
				break
			}
			if currentStatus == "error" {
				fail("page navigation failed")
				break
			}
			if currentStatus == "loading" || view.Page().IsLoading() {
				if ctx.Err() != nil {
					evalErr = ctx.Err()
					break
				}
				time.Sleep(min(verificationPollInterval, max(time.Until(deadline), 0)))
				continue
			}

			remaining := time.Until(deadline)
			evalCtx, cancel := context.WithTimeout(ctx, min(verificationEvalTimeout, remaining))
			raw, err := view.Page().EvalContext(evalCtx, script)
			cancel()

			a.mu.Lock()
			postStatus := p.status
			postNavError := p.navError
			postClosed := p.closed
			postNavSeq := p.navSeq
			a.mu.Unlock()
			if postClosed {
				fail("page closed")
				break
			}
			if postNavError != nil {
				fail(postNavError.Name)
				break
			}
			if postStatus == "error" {
				fail("page navigation failed")
				break
			}
			if postNavSeq != navSeq || postStatus == "loading" || view.Page().IsLoading() {
				observation = nil
				continue
			}
			if ctx.Err() != nil {
				evalErr = ctx.Err()
				break
			}
			if err == nil {
				var got verificationObservation
				if decodeVerificationEval(raw, &got) == nil {
					observation = &got
					page.URL = got.Href
					page.Results = evaluateVerificationChecks(checks, got)
					loading := view.Page().IsLoading()
					a.mu.Lock()
					stable := p.navSeq == navSeq && p.status != "loading" && p.status != "error" && p.navError == nil && !p.closed && !loading
					a.mu.Unlock()
					if !stable {
						observation = nil
						continue
					}
					if verificationHasConsoleCheck(checks) && got.ReadyState != "loading" && (!diagnostics || !got.DiagnosticsAvailable) {
						page.Error = "document-start diagnostics not enabled"
						break
					}
					if verificationResultsPass(page.Results) {
						loading := view.Page().IsLoading()
						a.mu.Lock()
						if p.navSeq == navSeq && p.status != "loading" && p.status != "error" && p.navError == nil && !p.closed && !loading {
							page.Status = "pass"
						}
						a.mu.Unlock()
						if page.Status == "pass" {
							break
						}
						observation = nil
						continue
					}
				} else {
					evalErr = fmt.Errorf("invalid verification response")
				}
			} else {
				evalErr = err
			}
			if ctx.Err() != nil {
				evalErr = ctx.Err()
				break
			}
			time.Sleep(min(verificationPollInterval, max(time.Until(deadline), 0)))
		}
		if page.Error == "" {
			if observation == nil {
				if evalErr == nil {
					evalErr = fmt.Errorf("timed out after %s", timeout)
				}
				fail(evalErr.Error())
			} else if page.Status != "pass" {
				if ctx.Err() != nil {
					page.Error = ctx.Err().Error()
				} else {
					page.Error = fmt.Sprintf("timed out after %s", timeout)
				}
			}
		}
	}

	if ctx.Err() != nil {
		page.ScreenshotError = "capture skipped: " + ctx.Err().Error()
		return page
	}
	png, err := a.captureAtConfiguredSizeContext(ctx, p, captureTimeout)
	if err != nil {
		page.ScreenshotError = err.Error()
	} else if rel, err := writeVerificationScreenshot(dir, index, png); err != nil {
		page.ScreenshotError = err.Error()
	} else {
		page.Screenshot = rel
	}
	return page
}

func failedVerificationResults(checks []verificationCheck, reason string) []verificationCheckResult {
	out := make([]verificationCheckResult, 0, len(checks))
	for _, c := range checks {
		out = append(out, verificationCheckResult{
			Kind: c.Kind, Selector: c.Selector, Operator: c.Operator,
			Expected: verificationExpected(c), Error: reason,
		})
	}
	return out
}

func verificationHasConsoleCheck(checks []verificationCheck) bool {
	for _, check := range checks {
		if check.Kind == "console" {
			return true
		}
	}
	return false
}

func verificationResultsPass(results []verificationCheckResult) bool {
	if len(results) == 0 {
		return false
	}
	for _, result := range results {
		if !result.Pass {
			return false
		}
	}
	return true
}

func evaluateVerificationChecks(checks []verificationCheck, observation verificationObservation) []verificationCheckResult {
	out := make([]verificationCheckResult, 0, len(checks))
	for i, check := range checks {
		result := verificationCheckResult{
			Kind: check.Kind, Selector: check.Selector, Operator: check.Operator,
			Expected: verificationExpected(check),
		}
		if observation.ReadyState == "loading" {
			result.Actual = "document loading"
			out = append(out, result)
			continue
		}
		switch check.Kind {
		case "text":
			actual := observation.Text
			var observed verificationObservedCheck
			if check.Selector != "" {
				observed = verificationObservedAt(observation, i)
				if observed.Error != "" {
					result.Error = observed.Error
					result.Actual = "selector error"
					out = append(out, result)
					continue
				}
				if !observed.Exists {
					result.Actual = "element not found"
					out = append(out, result)
					continue
				}
				actual = observed.Text
			}
			result.Actual = verificationActualText(actual)
			result.Pass = verificationTextMatch(check.Operator, actual, check.Value)
		case "dom":
			observed := verificationObservedAt(observation, i)
			if observed.Error != "" {
				result.Error = observed.Error
				result.Actual = "selector error"
			} else if !observed.Exists {
				result.Actual = "not found"
			} else if check.Operator == "exists" {
				result.Actual = "exists"
				result.Pass = true
			} else if observed.Visible {
				result.Actual = "visible"
				result.Pass = true
			} else {
				result.Actual = "hidden"
			}
		case "url":
			result.Actual = verificationActualText(observation.Href)
			result.Pass = verificationTextMatch(check.Operator, observation.Href, check.Value)
		case "http":
			result.Actual = strconv.Itoa(observation.HTTPStatus)
			if check.Operator == "success" {
				result.Pass = observation.HTTPStatus >= 200 && observation.HTTPStatus < 300
			} else {
				status, _ := strconv.Atoi(check.Value)
				result.Pass = observation.HTTPStatus == status
			}
		case "console":
			if !observation.DiagnosticsAvailable {
				result.Actual = "document-start diagnostics unavailable"
			} else {
				errors := observation.ConsoleErrors
				result.Pass = len(errors) == 0
				if len(errors) == 0 {
					result.Actual = "0 errors"
				} else {
					result.Actual = fmt.Sprintf("%d errors: %s", len(errors), strings.Join(errors[:min(len(errors), 3)], " | "))
				}
			}
		}
		out = append(out, result)
	}
	return out
}

func verificationExpected(check verificationCheck) string {
	switch check.Kind {
	case "dom":
		return check.Operator
	case "http":
		if check.Operator == "success" {
			return "2xx"
		}
		return check.Value
	case "console":
		return "no console errors"
	default:
		return check.Value
	}
}

func verificationObservedAt(observation verificationObservation, index int) verificationObservedCheck {
	if index >= 0 && index < len(observation.Checks) {
		return observation.Checks[index]
	}
	return verificationObservedCheck{Index: index, Error: "missing probe result"}
}

func verificationTextMatch(operator, actual, expected string) bool {
	actual = normalizeVerificationText(actual)
	expected = normalizeVerificationText(expected)
	switch operator {
	case "equals":
		return actual == expected
	case "not_contains":
		return !strings.Contains(actual, expected)
	default:
		return strings.Contains(actual, expected)
	}
}

func normalizeVerificationText(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func verificationActualText(s string) string {
	return truncateRunes(normalizeVerificationText(s), 500)
}

func buildVerificationDifferences(checks []verificationCheck, pages []verificationPageResult) []verificationDifference {
	var out []verificationDifference
	for i, check := range checks {
		values := make([]verificationValueAt, 0, len(pages))
		different := false
		var first verificationValueAt
		for pageIndex, page := range pages {
			value := verificationValueAt{PageID: page.PageID, Page: page.Name}
			if i < len(page.Results) {
				value.Actual = page.Results[i].Actual
				value.Pass = page.Results[i].Pass
			} else {
				value.Actual = page.Error
			}
			if pageIndex == 0 {
				first = value
			} else if value.Actual != first.Actual || value.Pass != first.Pass {
				different = true
			}
			values = append(values, value)
		}
		if different {
			out = append(out, verificationDifference{Check: i + 1, Kind: check.Kind, Values: values})
		}
	}
	return out
}

func decodeVerificationEval(raw any, dst any) error {
	switch value := raw.(type) {
	case string:
		return json.Unmarshal([]byte(value), dst)
	case []byte:
		return json.Unmarshal(value, dst)
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		return json.Unmarshal(data, dst)
	}
}

func verificationScript(checks []verificationCheck) string {
	payload, _ := json.Marshal(checks)
	return `(() => {
  const checks = ` + string(payload) + `;
  const d = window.__peapodDiag || { console: [], errors: [] };
  let httpStatus = 0;
  try {
    const nav = performance.getEntriesByType('navigation')[0];
    if (nav && typeof nav.responseStatus === 'number') httpStatus = nav.responseStatus;
  } catch {}
  let text = '';
  try { text = ((document.body && document.body.innerText) || '').slice(0, 4000); } catch {}
  const rows = [];
  for (let i = 0; i < checks.length; i++) {
    const check = checks[i];
    const row = { index: i, exists: false, visible: false, text: '', error: '' };
    if (check.kind === 'text' || check.kind === 'dom') {
      try {
        const el = check.selector ? document.querySelector(check.selector) : document.body;
        row.exists = !!el;
        if (el) {
          try { row.text = (el.innerText || el.textContent || '').slice(0, 4000); } catch {}
          try {
            const rect = el.getBoundingClientRect();
            const style = getComputedStyle(el);
            row.visible = !!(rect.width || rect.height) && style.display !== 'none' && style.visibility !== 'hidden';
          } catch {}
        }
      } catch (err) { row.error = String(err && err.message || err).slice(0, 500); }
    }
    rows.push(row);
  }
  const consoleErrors = d.console.filter(item => item.level === 'error').map(item => item.text)
    .concat(d.errors.map(item => item.message || 'error')).slice(-20);
  return JSON.stringify({
    href: location.href,
    readyState: document.readyState,
    httpStatus,
    text,
    consoleErrors,
    diagnosticsAvailable: !!window.__peapodDiag,
    checks: rows,
  });
})();`
}

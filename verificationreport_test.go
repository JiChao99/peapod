package main

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteVerificationReport(t *testing.T) {
	dir := t.TempDir()
	pngData := verificationPNG(t)
	rel, err := writeVerificationScreenshot(dir, 0, pngData)
	if err != nil {
		t.Fatal(err)
	}
	report := &verificationReport{
		TaskID:    "task-1",
		RunID:     "run-1",
		StartedAt: "2026-11-28T10:00:00Z",
		EndedAt:   "2026-11-28T10:00:01Z",
		TimeoutMs: 5000,
		Status:    "fail",
		Checks:    []verificationCheck{{Kind: "text", Operator: "contains", Value: "10.99"}},
		Pages: []verificationPageResult{{
			PageID: "p1", Name: `<img src=x onerror="alert(1)">`, URL: "https://example.com/?q=<script>",
			Status: "fail", Screenshot: rel,
			Results: []verificationCheckResult{{Kind: "text", Operator: "contains", Expected: "10.99", Actual: "9.99", Pass: false}},
		}},
		Differences: []verificationDifference{{Check: 1, Kind: "text", Values: []verificationValueAt{{PageID: "p1", Page: "Primary", Actual: "9.99", Pass: false}}}},
	}
	path, err := writeVerificationReport(dir, report, "en")
	if err != nil {
		t.Fatal(err)
	}
	if path != filepath.Join(dir, verificationHTMLName) || report.ReportPath != path {
		t.Fatalf("report path = %q, want %q", path, filepath.Join(dir, verificationHTMLName))
	}
	html, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), `<img src=x onerror="alert(1)">`) || strings.Contains(string(html), `<script>alert(1)</script>`) {
		t.Fatal("report contains unescaped page content")
	}
	if !strings.Contains(string(html), "&lt;img src=x onerror=") || !strings.Contains(string(html), "Cross-page differences") {
		t.Fatalf("report missing escaped content or sections: %s", html)
	}
	if !strings.Contains(string(html), "screenshots/page-01.png") {
		t.Fatal("report does not reference the captured screenshot")
	}

	raw, err := os.ReadFile(filepath.Join(dir, verificationJSONName))
	if err != nil {
		t.Fatal(err)
	}
	var saved verificationReport
	if err := json.Unmarshal(raw, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.TaskID != "task-1" || saved.RunID != "run-1" || saved.ReportPath != path || len(saved.Pages) != 1 {
		t.Fatalf("saved report = %+v", saved)
	}
}

func TestWriteVerificationReportRejectsScreenshotTraversal(t *testing.T) {
	dir := t.TempDir()
	report := &verificationReport{
		TaskID: "task-1", RunID: "run-1", Status: "pass",
		Checks: []verificationCheck{{Kind: "console", Operator: "clean"}},
		Pages:  []verificationPageResult{{PageID: "p1", Name: "Primary", Status: "pass", Screenshot: "../../outside.png"}},
	}
	path, err := writeVerificationReport(dir, report, "en")
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(html), "<figure class=\"shot\">") || strings.Contains(string(html), "../../outside.png") {
		t.Fatal("report referenced a screenshot outside its run folder")
	}
}

func TestWriteVerificationReportFailureDoesNotSetPath(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, verificationHTMLName), 0o755); err != nil {
		t.Fatal(err)
	}
	report := &verificationReport{TaskID: "task-1", RunID: "run-1", Status: "pass", Checks: []verificationCheck{{Kind: "console"}}}
	if _, err := writeVerificationReport(dir, report, "en"); err == nil {
		t.Fatal("expected HTML write failure")
	}
	if report.ReportPath != "" {
		t.Fatalf("failed report exposed path %q", report.ReportPath)
	}
}

func TestWriteVerificationScreenshotRejectsInvalidPNG(t *testing.T) {
	dir := t.TempDir()
	if _, err := writeVerificationScreenshot(dir, 0, []byte("not a png")); err == nil {
		t.Fatal("expected invalid PNG error")
	}
}

func verificationPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

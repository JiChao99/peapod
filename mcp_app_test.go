package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCaptureWithTimeoutSuccess(t *testing.T) {
	png, err := captureWithTimeout(func() ([]byte, error) { return []byte("png"), nil }, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(png) != "png" {
		t.Fatalf("png = %q", png)
	}
}

func TestCaptureWithTimeoutError(t *testing.T) {
	want := errors.New("capture failed")
	_, err := captureWithTimeout(func() ([]byte, error) { return nil, want }, time.Second)
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}

func TestCaptureWithTimeoutHangs(t *testing.T) {
	release := make(chan struct{})
	done := make(chan struct{})
	start := time.Now()
	_, err := captureWithTimeout(func() ([]byte, error) {
		<-release // simulate a CapturePreview callback that never fires
		close(done)
		return []byte("late"), nil
	}, 30*time.Millisecond)
	var to errCaptureTimeout
	if !errors.As(err, &to) {
		t.Fatalf("err = %v, want errCaptureTimeout", err)
	}
	if elapsed := time.Since(start); elapsed < 30*time.Millisecond {
		t.Fatalf("returned too early: %s", elapsed)
	}
	// A late completion must be discarded, not block or panic.
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("capture goroutine never finished after release")
	}
}

func TestCaptureTimeoutFromMs(t *testing.T) {
	cases := []struct {
		ms   float64
		want time.Duration
	}{
		{0, captureTimeout},
		{-5, captureTimeout},
		{500, time.Second},
		{1000, time.Second},
		{2500, 2500 * time.Millisecond},
		{60000, 60 * time.Second},
		{99999, 60 * time.Second},
	}
	for _, c := range cases {
		if got := captureTimeoutFromMs(c.ms); got != c.want {
			t.Errorf("captureTimeoutFromMs(%v) = %s, want %s", c.ms, got, c.want)
		}
	}
}

func TestMCPScreenshotUnknownPage(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_screenshot",
		Arguments: map[string]any{"pageId": "p9", "timeoutMs": 2000},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want error result, got %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "unknown page") {
		t.Fatalf("unexpected error text: %s", resultText(t, res))
	}
}

func TestMCPDiagnosticsUnknownPage(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{
		Name:      "peapod_diagnostics",
		Arguments: map[string]any{"pageId": "p9"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("want error result, got %s", resultText(t, res))
	}
	if !strings.Contains(resultText(t, res), "unknown page") {
		t.Fatalf("unexpected error text: %s", resultText(t, res))
	}
}

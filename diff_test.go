package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func solidPNG(t *testing.T, w, h int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func pngWithPixel(t *testing.T, w, h int, base color.RGBA, x, y int, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for yy := 0; yy < h; yy++ {
		for xx := 0; xx < w; xx++ {
			img.SetRGBA(xx, yy, base)
		}
	}
	img.SetRGBA(x, y, c)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode: %v", err)
	}
	return buf.Bytes()
}

func decodePNG(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return img
}

var white = color.RGBA{R: 255, G: 255, B: 255, A: 255}

func TestDiffImagesIdentical(t *testing.T) {
	a := solidPNG(t, 3, 2, white)
	st, outPNG, err := diffPNG(a, a, 0)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if st.Different != 0 || st.Fraction != 0 || st.MaxDelta != 0 || st.MeanDelta != 0 {
		t.Fatalf("identical images must diff zero: %+v", st)
	}
	if st.Width != 3 || st.Height != 2 || st.Pixels != 6 {
		t.Fatalf("dimensions wrong: %+v", st)
	}
	out := decodePNG(t, outPNG)
	// No red highlight anywhere; the base is dimmed grayscale.
	for y := 0; y < 2; y++ {
		for x := 0; x < 3; x++ {
			r, g, b, _ := out.At(x, y).RGBA()
			if r != g || g != b || r == 255<<8 {
				t.Fatalf("pixel (%d,%d) = %d/%d/%d, want dimmed grayscale", x, y, r>>8, g>>8, b>>8)
			}
		}
	}
}

func TestDiffImagesKnownDelta(t *testing.T) {
	ref := solidPNG(t, 2, 2, white)
	page := pngWithPixel(t, 2, 2, white, 1, 1, color.RGBA{R: 200, G: 255, B: 255, A: 255})
	st, outPNG, err := diffPNG(ref, page, 0)
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	if st.Different != 1 || st.Fraction != 0.25 {
		t.Fatalf("different/fraction = %d/%v, want 1/0.25", st.Different, st.Fraction)
	}
	if st.MaxDelta != 55 {
		t.Fatalf("maxDelta = %d, want 55", st.MaxDelta)
	}
	if st.MeanDelta != 55.0/16.0 {
		t.Fatalf("meanDelta = %v, want 55/16 (one channel of 16)", st.MeanDelta)
	}
	out := decodePNG(t, outPNG)
	r, g, b, _ := out.At(1, 1).RGBA()
	if r>>8 != 255 || g>>8 != 0 || b>>8 != 0 {
		t.Fatalf("differing pixel = %d/%d/%d, want red", r>>8, g>>8, b>>8)
	}
	r, g, b, _ = out.At(0, 0).RGBA()
	if r>>8 != 89 || g>>8 != 89 || b>>8 != 89 {
		t.Fatalf("equal pixel = %d/%d/%d, want dimmed white 89", r>>8, g>>8, b>>8)
	}
}

func TestDiffImagesTolerance(t *testing.T) {
	ref := solidPNG(t, 1, 1, white)
	page := solidPNG(t, 1, 1, color.RGBA{R: 250, G: 255, B: 255, A: 255})
	if st, _, _ := diffPNG(ref, page, 5); st.Different != 0 {
		t.Fatalf("delta 5 within tolerance 5 must not count: %+v", st)
	}
	if st, _, _ := diffPNG(ref, page, 4); st.Different != 1 {
		t.Fatalf("delta 5 beyond tolerance 4 must count: %+v", st)
	}
}

func TestDiffPNGSizeMismatch(t *testing.T) {
	_, _, err := diffPNG(solidPNG(t, 2, 2, white), solidPNG(t, 3, 2, white), 0)
	var sz errDiffSize
	if !errors.As(err, &sz) {
		t.Fatalf("want errDiffSize, got %v", err)
	}
	if sz.ref.X != 2 || sz.page.X != 3 {
		t.Fatalf("sizes = %+v", sz)
	}
}

func TestDiffPNGDecodeError(t *testing.T) {
	if _, _, err := diffPNG([]byte("not a png"), solidPNG(t, 1, 1, white), 0); err == nil {
		t.Fatal("garbage reference must fail decode")
	}
	if _, _, err := diffPNG(solidPNG(t, 1, 1, white), []byte("not a png"), 0); err == nil {
		t.Fatal("garbage page must fail decode")
	}
}

func TestClampTolerance(t *testing.T) {
	cases := []struct {
		in   float64
		want int
	}{
		{0, 0}, {-3, 0}, {12.9, 12}, {255, 255}, {9999, 255},
	}
	for _, c := range cases {
		if got := clampTolerance(c.in); got != c.want {
			t.Errorf("clampTolerance(%v) = %d, want %d", c.in, got, c.want)
		}
	}
	if got := clampTolerance(math.NaN()); got != 0 {
		t.Errorf("clampTolerance(NaN) = %d, want 0", got)
	}
}

func TestDiffPairNotRunning(t *testing.T) {
	a := newApp()
	if _, err := a.diffPair("p1", "p2", 0, time.Second, nil); err == nil {
		t.Fatal("idle task must refuse diff")
	}
}

func TestDiffPairUnknownPage(t *testing.T) {
	a := runningFixture(t)
	if _, err := a.diffPair("nope", "p1", 0, time.Second, nil); err == nil || !strings.Contains(err.Error(), "unknown page") {
		t.Fatalf("want unknown page error, got %v", err)
	}
	if _, err := a.diffPair("p1", "nope", 0, time.Second, nil); err == nil || !strings.Contains(err.Error(), "unknown page") {
		t.Fatalf("want unknown page error, got %v", err)
	}
}

func TestDiffPairSuccess(t *testing.T) {
	a := runningFixture(t)
	ref := solidPNG(t, 2, 2, white)
	page := pngWithPixel(t, 2, 2, white, 0, 0, color.RGBA{R: 0, G: 0, B: 0, A: 255})
	capture := func(p *pageRecord) ([]byte, error) {
		if p.id == "p1" {
			return ref, nil
		}
		return page, nil
	}
	d, err := a.diffPair("p2", "p1", 0, time.Second, capture)
	if err != nil {
		t.Fatalf("diffPair: %v", err)
	}
	if d.PageName != "Page 2" || d.AgainstName != "Primary" {
		t.Fatalf("names = %q/%q", d.PageName, d.AgainstName)
	}
	if d.Stats.Different != 1 || d.Stats.MaxDelta != 255 {
		t.Fatalf("stats = %+v, want 1 differing pixel at delta 255", d.Stats)
	}
	decodePNG(t, d.PNG)
}

func TestDiffPairSizeMismatch(t *testing.T) {
	a := runningFixture(t)
	a.langVal.Store("en")
	capture := func(p *pageRecord) ([]byte, error) {
		if p.id == "p1" {
			return solidPNG(t, 2, 2, white), nil
		}
		return solidPNG(t, 3, 2, white), nil
	}
	_, err := a.diffPair("p2", "p1", 0, time.Second, capture)
	if err == nil {
		t.Fatal("mismatched resolutions must fail")
	}
	// The inspected page's dimensions come first, the reference's second.
	want := "Page 2: resolution 3×2 differs from Primary (2×2); set the same resolution to diff"
	if err.Error() != want {
		t.Fatalf("size error = %q, want %q", err.Error(), want)
	}
}

func TestDiffPairCaptureTimeout(t *testing.T) {
	a := runningFixture(t)
	capture := func(*pageRecord) ([]byte, error) {
		time.Sleep(time.Second)
		return solidPNG(t, 1, 1, white), nil
	}
	start := time.Now()
	_, err := a.diffPair("p2", "p1", 0, 50*time.Millisecond, capture)
	if err == nil {
		t.Fatal("a non-rendering capture must time out")
	}
	if !strings.Contains(err.Error(), "Page 2") {
		t.Fatalf("timeout must name the inspected page: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("timeout took %s, want bounded", elapsed)
	}
}

func TestDiffPairAgainstTimeout(t *testing.T) {
	a := runningFixture(t)
	capture := func(p *pageRecord) ([]byte, error) {
		if p.id == "p1" {
			time.Sleep(time.Second)
		}
		return solidPNG(t, 1, 1, white), nil
	}
	_, err := a.diffPair("p2", "p1", 0, 50*time.Millisecond, capture)
	if err == nil {
		t.Fatal("a non-rendering reference must time out")
	}
	if !strings.Contains(err.Error(), "Primary") {
		t.Fatalf("timeout must name the reference page: %v", err)
	}
}

func TestRunBatchDiff(t *testing.T) {
	a := runningFixture(t)
	a.mu.Lock()
	a.pages["p3"] = &pageRecord{id: "p3", name: "Page 3", url: "https://c.com/", status: "ready", syncOn: true, queue: newOpQueue(0), navSignal: make(chan struct{}, 1)}
	a.order = append(a.order, "p3")
	a.mu.Unlock()

	dir := t.TempDir()
	capture := func(p *pageRecord) ([]byte, error) {
		switch p.id {
		case "p1":
			return solidPNG(t, 2, 2, white), nil
		case "p2":
			return pngWithPixel(t, 2, 2, white, 1, 0, color.RGBA{R: 0, G: 0, B: 0, A: 255}), nil
		default:
			return solidPNG(t, 3, 2, white), nil // size mismatch → failed entry
		}
	}
	ok, failed, differ := a.runBatchDiff("task-1", []diffTarget{{name: "Page 2", id: "p2"}, {name: "Page 3", id: "p3"}}, dir, "en", capture)
	if ok != 1 || failed != 1 || differ != 1 {
		t.Fatalf("counts = %d/%d/%d, want 1/1/1", ok, failed, differ)
	}
	if _, err := os.Stat(filepath.Join(dir, "Page 2_diff.png")); err != nil {
		t.Fatalf("highlight PNG missing: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "diff.json"))
	if err != nil {
		t.Fatalf("diff.json missing: %v", err)
	}
	var summary struct {
		Primary string `json:"primary"`
		Pages   []struct {
			Name  string     `json:"name"`
			Stats *diffStats `json:"stats"`
			Error string     `json:"error"`
		} `json:"pages"`
	}
	if err := json.Unmarshal(data, &summary); err != nil {
		t.Fatalf("diff.json invalid: %v", err)
	}
	if summary.Primary != "Primary" || len(summary.Pages) != 2 {
		t.Fatalf("summary = %+v", summary)
	}
	if summary.Pages[0].Stats == nil || summary.Pages[0].Stats.Different != 1 {
		t.Fatalf("page 2 entry = %+v, want stats with 1 differing pixel", summary.Pages[0])
	}
	if summary.Pages[1].Error == "" || summary.Pages[1].Stats != nil {
		t.Fatalf("page 3 entry = %+v, want an error and no stats", summary.Pages[1])
	}
	want := "Page 3: resolution 3×2 differs from Primary (2×2); set the same resolution to diff"
	if summary.Pages[1].Error != want {
		t.Fatalf("page 3 error = %q, want %q", summary.Pages[1].Error, want)
	}
}

func TestRunBatchDiffDuplicateNames(t *testing.T) {
	a := runningFixture(t)
	a.mu.Lock()
	a.pages["p3"] = &pageRecord{id: "p3", name: "Page 2", url: "https://c.com/", status: "ready", syncOn: true, queue: newOpQueue(0), navSignal: make(chan struct{}, 1)}
	a.order = append(a.order, "p3")
	a.mu.Unlock()

	dir := t.TempDir()
	capture := func(p *pageRecord) ([]byte, error) {
		if p.id == "p1" {
			return solidPNG(t, 2, 2, white), nil
		}
		return pngWithPixel(t, 2, 2, white, 0, 0, color.RGBA{R: 0, G: 0, B: 0, A: 255}), nil
	}
	ok, failed, _ := a.runBatchDiff("task-1", []diffTarget{{name: "Page 2", id: "p2"}, {name: "Page 2", id: "p3"}}, dir, "en", capture)
	if ok != 2 || failed != 0 {
		t.Fatalf("counts = %d/%d, want 2/0", ok, failed)
	}
	// Same-named pages must not overwrite each other's highlight PNG.
	for _, name := range []string{"Page 2_diff.png", "Page 2_diff_2.png"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
}

func TestRunBatchDiffPrimaryCaptureFails(t *testing.T) {
	a := runningFixture(t)
	dir := t.TempDir()
	capture := func(*pageRecord) ([]byte, error) { return nil, errors.New("not rendering") }
	ok, failed, differ := a.runBatchDiff("task-1", []diffTarget{{name: "Page 2", id: "p2"}}, dir, "en", capture)
	if ok != 0 || failed != 1 || differ != 0 {
		t.Fatalf("counts = %d/%d/%d, want 0/1/0", ok, failed, differ)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "diff.json"))
	if !strings.Contains(string(data), "primaryError") {
		t.Fatalf("diff.json must record the primary failure: %s", data)
	}
}

func TestRunBatchDiffTaskStopped(t *testing.T) {
	a := runningFixture(t)
	dir := t.TempDir()
	capture := func(*pageRecord) ([]byte, error) { return solidPNG(t, 1, 1, white), nil }
	ok, failed, _ := a.runBatchDiff("task-OTHER", []diffTarget{{name: "Page 2", id: "p2"}}, dir, "en", capture)
	if ok != 0 || failed != 1 {
		t.Fatalf("a replaced task must fail every entry: %d/%d", ok, failed)
	}
}

func TestMCPDiffSamePage(t *testing.T) {
	a := runningFixture(t)
	cs := connectMCP(t, a)
	res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "peapod_diff", Arguments: map[string]any{}})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !res.IsError {
		t.Fatalf("both defaults resolve to the primary; want error, got %s", resultText(t, res))
	}
}

func TestMCPDiffNotRunning(t *testing.T) {
	a := newApp()
	cs := connectMCP(t, a)
	for _, args := range []map[string]any{
		{"pageId": "p2"},
		{}, // both defaults resolve to the empty primaryID; not-running must win over same-page
	} {
		res, err := cs.CallTool(context.Background(), &sdk.CallToolParams{Name: "peapod_diff", Arguments: args})
		if err != nil {
			t.Fatalf("call: %v", err)
		}
		if !res.IsError {
			t.Fatalf("idle task must refuse diff, got %s", resultText(t, res))
		}
	}
}

func TestPageDiffJSONExcludesPNG(t *testing.T) {
	d := pageDiff{PageID: "p2", PageName: "Page 2", AgainstID: "p1", AgainstName: "Primary",
		Stats: diffStats{Width: 2, Height: 2, Pixels: 4, Different: 1}, PNG: []byte{0x89, 0x50, 0x4e, 0x47}}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := m["stats"]; !ok {
		t.Fatalf("stats missing from JSON: %s", data)
	}
	if strings.Contains(string(data), "PNG") || strings.Contains(string(data), "iVBOR") {
		t.Fatalf("PNG bytes must stay out of the JSON: %s", data)
	}
}

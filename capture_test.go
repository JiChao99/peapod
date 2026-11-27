package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"runtime"
	"testing"

	"golang.org/x/image/font/sfnt"
)

func solidPNG(width, height int, c color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		panic(err)
	}
	return out.Bytes()
}

func TestNormalizeCapture(t *testing.T) {
	got, err := normalizeCapture(solidPNG(600, 400, color.RGBA{10, 20, 30, 255}), 300, 200)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(got))
	if err != nil || cfg.Width != 300 || cfg.Height != 200 {
		t.Fatalf("normalized size = %dx%d, err=%v", cfg.Width, cfg.Height, err)
	}
	if _, err := normalizeCapture(solidPNG(200, 100, color.RGBA{}), 300, 200); err == nil {
		t.Fatal("small captures must not be upscaled")
	}
}

func TestRenderCanvasShot(t *testing.T) {
	items := []canvasShotItem{
		{id: "p1", name: "Primary", x: 0, y: 0, w: 200, h: 100},
		{id: "p2", name: "Follower", x: 220, y: 0, w: 200, h: 100},
	}
	shots := map[string][]byte{
		"p1": solidPNG(400, 200, color.RGBA{255, 0, 0, 255}),
	}
	got, failed, err := renderCanvasShot(items, shots)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 1 {
		t.Fatalf("failed = %d, want 1", failed)
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(got))
	if err != nil || cfg.Width != 432 || cfg.Height != 140 {
		t.Fatalf("canvas size = %dx%d, err=%v", cfg.Width, cfg.Height, err)
	}
}

func TestRenderCanvasShotWithCJKNames(t *testing.T) {
	items := []canvasShotItem{
		{id: "p1", name: "日本語 QA", x: 0, y: 0, w: 200, h: 100, lang: "ja"},
		{id: "p2", name: "繁體中文 QA", x: 220, y: 0, w: 200, h: 100, lang: "zh-Hant"},
	}
	shots := map[string][]byte{
		"p1": solidPNG(400, 200, color.RGBA{255, 0, 0, 255}),
		"p2": solidPNG(400, 200, color.RGBA{0, 255, 0, 255}),
	}
	if _, failed, err := renderCanvasShot(items, shots); err != nil || failed != 0 {
		t.Fatalf("CJK canvas render: failed=%d err=%v", failed, err)
	}
}

func TestCanvasLabelKindUsesPageScript(t *testing.T) {
	tests := []struct {
		name  string
		items []canvasShotItem
		want  string
	}{
		{name: "latin", items: []canvasShotItem{{name: "English QA", lang: "ja"}}, want: "latin"},
		{name: "japanese", items: []canvasShotItem{{name: "にほんご QA", lang: "ja"}}, want: "ja"},
		{name: "traditional", items: []canvasShotItem{{name: "繁體中文 QA", lang: "zh-Hant"}}, want: "zh-Hant"},
		{name: "traditional on japanese ui", items: []canvasShotItem{{name: "繁體中文 QA", lang: "ja"}}, want: "cjk"},
		{name: "simplified", items: []canvasShotItem{{name: "简体中文 QA", lang: "ja"}}, want: "cjk"},
		{name: "mixed", items: []canvasShotItem{{name: "にほんご", lang: "ja"}, {name: "繁體中文", lang: "zh-Hant"}}, want: "ja"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := canvasLabelKind(tt.items); got != tt.want {
				t.Fatalf("canvas label kind = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCanvasLabelFontsCoverRepresentativeScripts(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("checks the bundled Windows CJK fonts")
	}
	items := []canvasShotItem{
		{name: "日本語 QA", lang: "ja"},
		{name: "繁體中文 QA", lang: "zh-Hant"},
		{name: "简体中文 QA", lang: "zh"},
		{name: "English QA", lang: "en"},
	}
	fonts := canvasLabelFonts(items, 1)
	if len(fonts) == 0 {
		t.Skip("Windows CJK fonts are unavailable")
	}
	defer func() {
		for _, candidate := range fonts {
			candidate.face.Close()
		}
	}()
	var buffer sfnt.Buffer
	for _, item := range items {
		for _, r := range item.name {
			covered := false
			for _, candidate := range fonts {
				glyph, err := candidate.font.GlyphIndex(&buffer, r)
				if err == nil && glyph != 0 {
					covered = true
					break
				}
			}
			if !covered {
				t.Fatalf("no canvas font covers %q in %q", r, item.name)
			}
		}
	}
}

func TestCanvasWidthClamp(t *testing.T) {
	a := newApp()
	a.resizeCanvasPage("p1", -1000)
	if got := a.canvasWidthForID("p1"); got != canvasMinWidth {
		t.Fatalf("minimum width = %v", got)
	}
	a.resizeCanvasPage("p1", 1000)
	if got := a.canvasWidthForID("p1"); got != canvasMaxWidth {
		t.Fatalf("maximum width = %v", got)
	}
}

func TestCanvasResizePreservesAspectRatioAndOtherPages(t *testing.T) {
	a := newApp()
	p := &pageRecord{id: "p1", resW: 1920, resH: 1080}
	for _, width := range []float32{canvasMinWidth, 320, canvasMaxWidth} {
		h := canvasPageHeight(p, width)
		if h != width*9/16 {
			t.Fatalf("height at width %v = %v, want %v", width, h, width*9/16)
		}
	}
	a.canvasWidths["p2"] = 500
	a.resizeCanvasPage("p1", 80)
	if got := a.canvasWidthForID("p2"); got != 500 {
		t.Fatalf("resizing p1 changed p2 width to %v", got)
	}
}

func TestCanvasResizeDeltaUsesDominantAxis(t *testing.T) {
	if got := canvasResizeDelta(60, 10, 1920, 1080); got != 60 {
		t.Fatalf("horizontal delta = %v, want 60", got)
	}
	if got := canvasResizeDelta(10, 60, 1920, 1080); got != 60*1920.0/1080.0 {
		t.Fatalf("vertical delta = %v, want %v", got, 60*1920.0/1080.0)
	}
	if got := canvasResizeDelta(-40, -5, 390, 844); got != -40 {
		t.Fatalf("shrink delta = %v, want -40", got)
	}
	if got := canvasResizeDelta(60, 60, 390, 844); got != 60 {
		t.Fatalf("normalized portrait delta = %v, want 60", got)
	}
	if got := canvasResizeDelta(60, 60, 1920, 1080); got != 60*1920.0/1080.0 {
		t.Fatalf("normalized landscape delta = %v, want %v", got, 60*1920.0/1080.0)
	}
}

func TestRecMetaLineNamesFreeCanvas(t *testing.T) {
	got := recMetaLine(recSessionMeta{viewMode: "free", pages: []string{"Primary", "Follower"}, torn: true}, "en")
	if got != "2 pages · Free canvas" {
		t.Fatalf("free-canvas recording meta = %q", got)
	}
}

func TestCaptureRequestRejectsReusedPageID(t *testing.T) {
	a := newApp()
	oldPage := &pageRecord{id: "p1", resW: 390, resH: 844}
	a.mu.Lock()
	a.status = "running"
	a.taskID = "task-1"
	a.pages[oldPage.id] = oldPage
	req := &captureLayoutRequest{taskID: a.taskID, page: oldPage}
	if got := a.capturePageForRequestLocked(req); got != oldPage {
		t.Fatalf("current request resolved to %p, want %p", got, oldPage)
	}
	a.taskID = "task-2"
	newPage := &pageRecord{id: "p1", resW: 390, resH: 844}
	a.pages[newPage.id] = newPage
	if got := a.capturePageForRequestLocked(req); got != nil {
		t.Fatalf("stale request resolved to new task page %p", got)
	}
	a.mu.Unlock()
}

func TestCanvasResizeMovesOverlappingPage(t *testing.T) {
	a := newApp()
	p1 := &pageRecord{id: "p1", resW: 1280, resH: 800}
	p2 := &pageRecord{id: "p2", resW: 1280, resH: 800}
	a.mu.Lock()
	a.pages = map[string]*pageRecord{"p1": p1, "p2": p2}
	a.order = []string{"p1", "p2"}
	a.canvasPos = map[string][2]float64{"p1": {0, 0}, "p2": {344, 0}}
	a.mu.Unlock()

	a.resizeCanvasPage("p1", 400)
	a.mu.Lock()
	pos := a.canvasPos["p2"]
	a.mu.Unlock()
	wantY := float64(canvasMaxWidth) * 800 / 1280
	if pos[1] < wantY+canvasLabelH+canvasGap {
		t.Fatalf("overlapping page y = %v, want at least %v", pos[1], wantY+canvasLabelH+canvasGap)
	}
}

func TestFinishBatchShotKeepsNewTaskBusy(t *testing.T) {
	a := newApp()
	a.mu.Lock()
	a.taskID = "task-2"
	a.shotBusy = true
	a.mu.Unlock()
	a.finishBatchShot("task-1")
	a.mu.Lock()
	busy := a.shotBusy
	a.mu.Unlock()
	if !busy {
		t.Fatal("old batch cleared the new task's busy flag")
	}
	a.finishBatchShot("task-2")
	a.mu.Lock()
	busy = a.shotBusy
	a.mu.Unlock()
	if busy {
		t.Fatal("current batch did not release busy flag")
	}
}

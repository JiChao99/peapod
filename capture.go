package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	drawx "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

type captureLayoutRequest struct {
	taskID string
	page   *pageRecord
	ready  chan struct{}
	cancel context.CancelFunc
	once   sync.Once
}

type captureResult struct {
	png []byte
	err error
}

func (a *app) captureAtConfiguredSize(p *pageRecord, timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = captureTimeout
	}
	if a.win == nil {
		return nil, fmt.Errorf("capture requires a visible window")
	}
	if a.captureGate == nil {
		return nil, fmt.Errorf("capture gate is not initialized")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	select {
	case a.captureGate <- struct{}{}:
	case <-ctx.Done():
		return nil, captureContextError(ctx, timeout)
	}
	releaseOnReturn := true
	defer func() {
		if releaseOnReturn {
			<-a.captureGate
		}
	}()

	a.mu.Lock()
	if p == nil || p.closed || a.pages[p.id] != p || (a.status != "running" && a.status != "starting") {
		a.mu.Unlock()
		return nil, fmt.Errorf("page is no longer running")
	}
	taskID := a.taskID
	req := &captureLayoutRequest{taskID: taskID, page: p, ready: make(chan struct{}), cancel: cancel}
	a.captureReq = req
	view := p.view
	resW, resH := p.resW, p.resH
	a.mu.Unlock()

	restore := func() {
		a.mu.Lock()
		if a.captureReq == req {
			a.captureReq = nil
		}
		a.mu.Unlock()
		cancel()
		a.invalidate()
	}
	defer restore()
	a.invalidate()

	select {
	case <-req.ready:
	case <-ctx.Done():
		return nil, captureContextError(ctx, timeout)
	}
	if err := waitForViewport(ctx, view, resW, resH); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, captureContextError(ctx, timeout)
	}

	result := make(chan captureResult, 1)
	releaseOnReturn = false
	go func() {
		pngBytes, err := view.CapturePage()
		result <- captureResult{png: pngBytes, err: err}
		// CapturePage cannot be cancelled. Keep the gate until the native call
		// actually returns so a timeout never overlaps two native captures.
		<-a.captureGate
	}()
	select {
	case got := <-result:
		if err := ctx.Err(); err != nil {
			return nil, captureContextError(ctx, timeout)
		}
		if got.err != nil {
			return nil, got.err
		}
		return normalizeCapture(got.png, int(math.Round(resW)), int(math.Round(resH)))
	case <-ctx.Done():
		return nil, captureContextError(ctx, timeout)
	}
}

func captureContextError(ctx context.Context, timeout time.Duration) error {
	if ctx.Err() == context.DeadlineExceeded {
		return errCaptureTimeout{timeout: timeout}
	}
	return fmt.Errorf("capture cancelled: page is no longer running")
}

func waitForViewport(ctx context.Context, view *mygo.WebView, resW, resH float64) error {
	stable := 0
	for {
		value, err := view.Page().EvalContext(ctx, `JSON.stringify({w:window.innerWidth,h:window.innerHeight,dpr:window.devicePixelRatio})`)
		if err == nil {
			raw, _ := value.(string)
			var size struct {
				W float64 `json:"w"`
				H float64 `json:"h"`
			}
			if json.Unmarshal([]byte(raw), &size) == nil && math.Abs(size.W-resW) <= 1 && math.Abs(size.H-resH) <= 1 {
				stable++
				if stable == 2 {
					return nil
				}
			} else {
				stable = 0
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("capture viewport did not reach %.0f×%.0f: restore the Peapod window and retry", resW, resH)
		case <-time.After(60 * time.Millisecond):
		}
	}
}

func normalizeCapture(pngBytes []byte, width, height int) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode capture: %w", err)
	}
	if cfg.Width < width || cfg.Height < height {
		return nil, fmt.Errorf("capture is %d×%d, smaller than configured %d×%d; restore the Peapod window and retry", cfg.Width, cfg.Height, width, height)
	}
	if cfg.Width == width && cfg.Height == height {
		return pngBytes, nil
	}
	src, _, err := image.Decode(bytes.NewReader(pngBytes))
	if err != nil {
		return nil, fmt.Errorf("decode capture: %w", err)
	}
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	drawx.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return nil, fmt.Errorf("encode capture: %w", err)
	}
	return out.Bytes(), nil
}

type canvasShotItem struct {
	id, name string
	x, y     float64
	w, h     float64
	lang     string
}

func (a *app) screenshotCanvas() {
	a.mu.Lock()
	if a.status != "running" || a.taskViewMode != "free" {
		a.mu.Unlock()
		a.setNoticeErrKey("err.canvasShotMode")
		return
	}
	if a.shotBusy {
		a.mu.Unlock()
		return
	}
	a.shotBusy = true
	items := make([]canvasShotItem, 0, len(a.order))
	for _, id := range a.order {
		p := a.pages[id]
		if p == nil || p.closed {
			continue
		}
		pos := a.canvasPos[id]
		w := a.canvasWidthForID(id)
		items = append(items, canvasShotItem{id: id, name: p.name, x: pos[0], y: pos[1], w: float64(w), h: float64(w) * p.resH / p.resW, lang: a.currentLang()})
	}
	base := strings.TrimSpace(a.shotName)
	if base == "" {
		base = "screenshot"
	}
	taskID := a.taskID
	a.mu.Unlock()
	if len(items) == 0 {
		a.finishCanvasShot(taskID)
		a.setNoticeErrKey("err.noPages")
		return
	}
	a.pushState()
	go func() {
		dir, err := screenshotDir(base)
		if err != nil {
			if !a.batchStale(taskID) {
				a.setNoticeErrKey("err.shotDir", err.Error())
			}
			a.finishCanvasShot(taskID)
			return
		}
		shots := make(map[string][]byte, len(items))
		for _, item := range items {
			_, shot, captureErr := a.capturePNGForTask(taskID, item.id, captureTimeout)
			if captureErr == nil {
				shots[item.id] = shot
			}
			if a.batchStale(taskID) {
				a.finishCanvasShot(taskID)
				return
			}
		}
		canvas, failed, renderErr := renderCanvasShot(items, shots)
		if renderErr != nil {
			if !a.batchStale(taskID) {
				a.setNoticeErr(renderErr.Error())
			}
			a.finishCanvasShot(taskID)
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "canvas.png"), canvas, 0o644); err != nil {
			if !a.batchStale(taskID) {
				a.setNoticeErr(err.Error())
			}
			a.finishCanvasShot(taskID)
			return
		}
		if a.batchStale(taskID) {
			a.finishCanvasShot(taskID)
			return
		}
		if failed > 0 {
			a.setNoticeOpenKey("warning", "notice.canvasPartial", dir, true, len(items)-failed, failed, dir)
		} else {
			a.setNoticeOpenKey("success", "notice.canvasDone", dir, true, len(items), dir)
		}
		a.finishCanvasShot(taskID)
	}()
}

func (a *app) finishCanvasShot(taskID string) {
	a.mu.Lock()
	if a.taskID == taskID {
		a.shotBusy = false
	}
	a.mu.Unlock()
	a.pushState()
}

func renderCanvasShot(items []canvasShotItem, shots map[string][]byte) ([]byte, int, error) {
	const maxDimension = 6144
	contentW, contentH := 0.0, 0.0
	for _, item := range items {
		contentW = math.Max(contentW, item.x+item.w+stagePadding)
		contentH = math.Max(contentH, item.y+item.h+canvasLabelH+stagePadding)
	}
	scale := math.Min(1, math.Min(maxDimension/math.Max(contentW, 1), maxDimension/math.Max(contentH, 1)))
	width, height := int(math.Ceil(contentW*scale)), int(math.Ceil(contentH*scale))
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{240, 243, 237, 255}), image.Point{}, draw.Src)
	labelFonts := canvasLabelFonts(items, scale)
	defer func() {
		for _, labelFont := range labelFonts {
			labelFont.face.Close()
		}
	}()
	failed := 0
	for _, item := range items {
		x, y := int(item.x*scale), int(item.y*scale)
		w, h := int(math.Round(item.w*scale)), int(math.Round(item.h*scale))
		headerH := int(math.Round(canvasLabelH * scale))
		draw.Draw(canvas, image.Rect(x, y, x+w, y+headerH), image.NewUniform(color.RGBA{255, 255, 255, 255}), image.Point{}, draw.Src)
		label := &font.Drawer{Dst: canvas, Src: image.NewUniform(color.RGBA{35, 49, 40, 255}), Face: basicfont.Face7x13}
		label.Dot = fixed.P(x+int(8*scale), y+max(headerH-6, 12))
		drawCanvasLabel(label, labelFonts, item.name)
		src, _, err := image.Decode(bytes.NewReader(shots[item.id]))
		if err != nil {
			failed++
			draw.Draw(canvas, image.Rect(x, y+headerH, x+w, y+headerH+h), image.NewUniform(color.RGBA{35, 49, 40, 255}), image.Point{}, draw.Src)
			continue
		}
		drawx.CatmullRom.Scale(canvas, image.Rect(x, y+headerH, x+w, y+headerH+h), src, src.Bounds(), draw.Src, nil)
	}
	var out bytes.Buffer
	if err := png.Encode(&out, canvas); err != nil {
		return nil, failed, fmt.Errorf("encode canvas: %w", err)
	}
	return out.Bytes(), failed, nil
}

type canvasLabelFont struct {
	face font.Face
	font *sfnt.Font
}

func drawCanvasLabel(drawer *font.Drawer, fonts []canvasLabelFont, text string) {
	if len(fonts) == 0 {
		drawer.DrawString(text)
		return
	}
	var buffer sfnt.Buffer
	for _, r := range text {
		selected := -1
		for i, candidate := range fonts {
			glyph, err := candidate.font.GlyphIndex(&buffer, r)
			if err == nil && glyph != 0 {
				selected = i
				break
			}
		}
		if selected >= 0 {
			drawer.Face = fonts[selected].face
		}
		drawer.DrawString(string(r))
	}
}

func canvasLabelKind(items []canvasShotItem) string {
	kind := "latin"
	traditional := false
	for _, item := range items {
		for _, r := range item.name {
			switch {
			case isCanvasKana(r):
				return "ja"
			case isCanvasHan(r):
				if kind == "latin" {
					kind = "cjk"
				}
				traditional = traditional || normalizeLang(item.lang) == "zh-Hant"
			case r > 127 && kind == "latin":
				kind = "cjk"
			}
		}
	}
	if traditional && kind == "cjk" {
		return "zh-Hant"
	}
	return kind
}

func isCanvasKana(r rune) bool {
	return r >= 0x3040 && r <= 0x30ff || r >= 0x31f0 && r <= 0x31ff || r >= 0xff66 && r <= 0xff9d
}

func isCanvasHan(r rune) bool {
	return r >= 0x3400 && r <= 0x4dbf || r >= 0x4e00 && r <= 0x9fff || r >= 0xf900 && r <= 0xfaff ||
		r >= 0x20000 && r <= 0x2fa1f || r >= 0x30000 && r <= 0x323af
}

func canvasLabelFontPaths(kind string) []string {
	var paths []string
	switch runtime.GOOS {
	case "windows":
		dir := os.Getenv("WINDIR")
		if dir == "" {
			dir = `C:\Windows`
		}
		fonts := filepath.Join(dir, "Fonts")
		switch kind {
		case "ja":
			paths = []string{"meiryo.ttc", "yugothic.ttc", "msjh.ttc", "msyh.ttc", "segoeui.ttf"}
		case "zh-Hant":
			paths = []string{"msjh.ttc", "mingliu.ttc", "meiryo.ttc", "msyh.ttc", "segoeui.ttf"}
		case "cjk":
			paths = []string{"msyh.ttc", "simsun.ttc", "msjh.ttc", "meiryo.ttc", "segoeui.ttf"}
		default:
			paths = []string{"segoeui.ttf"}
		}
		for i := range paths {
			paths[i] = filepath.Join(fonts, paths[i])
		}
	case "darwin":
		if kind != "latin" {
			paths = []string{"/System/Library/Fonts/PingFang.ttc", "/System/Library/Fonts/Hiragino Sans GB.ttc"}
		}
		paths = append(paths, "/System/Library/Fonts/Supplemental/Arial.ttf")
	default:
		if kind != "latin" {
			paths = []string{
				"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
				"/usr/share/fonts/opentype/noto/NotoSansCJKjp-Regular.otf",
				"/usr/share/fonts/truetype/wqy/wqy-microhei.ttc",
			}
		}
		paths = append(paths, "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf")
	}
	return paths
}

func canvasLabelFonts(items []canvasShotItem, scale float64) []canvasLabelFont {
	var fonts []canvasLabelFont
	for _, path := range canvasLabelFontPaths(canvasLabelKind(items)) {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		collection, err := opentype.ParseCollection(data)
		if err != nil {
			continue
		}
		for i := 0; i < collection.NumFonts(); i++ {
			parsed, err := collection.Font(i)
			if err != nil {
				continue
			}
			face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: max(8, 12*scale), DPI: 72, Hinting: font.HintingFull})
			if err == nil {
				fonts = append(fonts, canvasLabelFont{face: face, font: parsed})
			}
		}
	}
	return fonts
}

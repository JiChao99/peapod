package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"time"
)

// diffStats quantifies one variant comparison: how much of the inspected page
// differs from the reference, and by how much per channel (0–255 scale).
type diffStats struct {
	Width     int     `json:"width"`
	Height    int     `json:"height"`
	Pixels    int     `json:"pixels"`
	Different int     `json:"different"`
	Fraction  float64 `json:"fraction"`
	MaxDelta  int     `json:"maxDelta"`
	MeanDelta float64 `json:"meanDelta"`
	Tolerance int     `json:"tolerance"`
}

// errDiffSize reports incomparable captures: a pixel diff needs equal
// resolutions, so mismatched pages fail loudly instead of silently cropping
// or scaling one variant onto another.
type errDiffSize struct{ ref, page image.Point }

func (e errDiffSize) Error() string {
	return fmt.Sprintf("image sizes differ: %dx%d vs %dx%d", e.ref.X, e.ref.Y, e.page.X, e.page.Y)
}

// clampTolerance keeps a caller-supplied per-channel tolerance in 0–255.
func clampTolerance(v float64) int {
	if math.IsNaN(v) || v <= 0 {
		return 0
	}
	return int(min(v, 255))
}

// diffImages compares two equal-bounds images channel by channel. A pixel
// counts as different when any channel differs by more than tolerance. The
// returned image dims the inspected page to grayscale and paints differing
// pixels red, so a reviewer sees exactly where the variants diverge.
func diffImages(ref, page image.Image, tolerance int) (diffStats, *image.RGBA) {
	b := page.Bounds()
	st := diffStats{Width: b.Dx(), Height: b.Dy(), Pixels: b.Dx() * b.Dy(), Tolerance: tolerance}
	out := image.NewRGBA(b)
	var sum int64
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r1, g1, b1, a1 := ref.At(x, y).RGBA()
			r2, g2, b2, a2 := page.At(x, y).RGBA()
			d0 := abs8(int(r1>>8) - int(r2>>8))
			d1 := abs8(int(g1>>8) - int(g2>>8))
			d2 := abs8(int(b1>>8) - int(b2>>8))
			d3 := abs8(int(a1>>8) - int(a2>>8))
			maxd := max(d0, d1, d2, d3)
			st.MaxDelta = max(st.MaxDelta, maxd)
			sum += int64(d0 + d1 + d2 + d3)
			c := color.RGBA{A: 255}
			if maxd > tolerance {
				st.Different++
				c.R = 255
			} else {
				g := (int(r2>>8)*299 + int(g2>>8)*587 + int(b2>>8)*114) / 1000
				g = g * 35 / 100
				c.R, c.G, c.B = uint8(g), uint8(g), uint8(g)
			}
			out.SetRGBA(x, y, c)
		}
	}
	if st.Pixels > 0 {
		st.Fraction = float64(st.Different) / float64(st.Pixels)
		st.MeanDelta = float64(sum) / float64(st.Pixels*4)
	}
	return st, out
}

func abs8(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// diffPNG decodes two captures and diffs them; ref is the reference page
// (usually the primary), page the inspected variant. Equal dimensions are
// required (errDiffSize otherwise).
func diffPNG(refPNG, pagePNG []byte, tolerance int) (diffStats, []byte, error) {
	ref, err := png.Decode(bytes.NewReader(refPNG))
	if err != nil {
		return diffStats{}, nil, fmt.Errorf("decode reference image: %w", err)
	}
	page, err := png.Decode(bytes.NewReader(pagePNG))
	if err != nil {
		return diffStats{}, nil, fmt.Errorf("decode page image: %w", err)
	}
	if ref.Bounds() != page.Bounds() {
		return diffStats{}, nil, errDiffSize{ref: ref.Bounds().Size(), page: page.Bounds().Size()}
	}
	st, out := diffImages(ref, page, tolerance)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return st, nil, err
	}
	return st, buf.Bytes(), nil
}

// pageDiff is one variant comparison: the stats plus the highlight PNG.
type pageDiff struct {
	PageID      string    `json:"pageId"`
	PageName    string    `json:"pageName"`
	AgainstID   string    `json:"againstId"`
	AgainstName string    `json:"againstName"`
	Stats       diffStats `json:"stats"`
	PNG         []byte    `json:"-"`
}

// diffPair captures two running pages and compares them (idA inspected, idB
// reference). capture is injectable so tests run without a GUI; production
// passes p.view.CapturePage. Both captures share the batch path's coalesced
// flights and timeout, so a non-rendering page fails fast.
func (a *app) diffPair(idA, idB string, tolerance int, timeout time.Duration, capture func(*pageRecord) ([]byte, error)) (pageDiff, error) {
	a.mu.Lock()
	if a.status != "running" {
		a.mu.Unlock()
		return pageDiff{}, fmt.Errorf("%s", a.tr("err.notRunning"))
	}
	if idA == idB {
		a.mu.Unlock()
		return pageDiff{}, fmt.Errorf("%s", a.tr("err.diffSame"))
	}
	pa, pb := a.pages[idA], a.pages[idB]
	if pa == nil || pa.closed {
		a.mu.Unlock()
		return pageDiff{}, fmt.Errorf("unknown page %q", idA)
	}
	if pb == nil || pb.closed {
		a.mu.Unlock()
		return pageDiff{}, fmt.Errorf("unknown page %q", idB)
	}
	d := pageDiff{PageID: idA, PageName: pa.name, AgainstID: idB, AgainstName: pb.name}
	fa := a.capturePageFlight(pa, func() ([]byte, error) { return capture(pa) })
	fb := a.capturePageFlight(pb, func() ([]byte, error) { return capture(pb) })
	a.mu.Unlock()

	pngA, err := waitCapture(fa, timeout)
	if err != nil {
		return d, fmt.Errorf("%s: %w", d.PageName, err)
	}
	pngB, err := waitCapture(fb, timeout)
	if err != nil {
		return d, fmt.Errorf("%s: %w", d.AgainstName, err)
	}
	st, diff, err := diffPNG(pngB, pngA, tolerance)
	if err != nil {
		return d, diffError(a.currentLang(), d.PageName, d.AgainstName, err)
	}
	d.Stats, d.PNG = st, diff
	return d, nil
}

// diffError localizes a size mismatch with both page names; other errors
// (decode, capture) pass through unchanged. lang is explicit so batch work
// keeps the language captured when it was launched.
func diffError(lang, pageName, againstName string, err error) error {
	var sz errDiffSize
	if errors.As(err, &sz) {
		return fmt.Errorf("%s", trLang(lang, "err.diffSize", pageName, sz.page.X, sz.page.Y, againstName, sz.ref.X, sz.ref.Y))
	}
	return err
}

// diffTarget is one follower queued for a batch diff.
type diffTarget struct{ name, id string }

// batchDiffEntry is one page's line in diff.json: stats on success, the
// reason on failure — the evidence stays honest about what was compared.
type batchDiffEntry struct {
	Name   string     `json:"name"`
	PageID string     `json:"pageId"`
	Stats  *diffStats `json:"stats,omitempty"`
	Error  string     `json:"error,omitempty"`
}

// diffAll compares every follower against the primary and saves the evidence
// under Pictures/Peapod/diffs: one highlight PNG per page plus diff.json.
func (a *app) diffAll() {
	a.mu.Lock()
	if a.status != "running" {
		a.mu.Unlock()
		a.setNoticeErr(a.tr("err.notRunning"))
		return
	}
	if a.diffBusy {
		a.mu.Unlock()
		return
	}
	a.diffBusy = true
	var targets []diffTarget
	for _, id := range a.order {
		if id == a.primaryID {
			continue
		}
		if p := a.pages[id]; p != nil && !p.closed {
			targets = append(targets, diffTarget{name: p.name, id: id})
		}
	}
	taskID := a.taskID
	a.mu.Unlock()
	if len(targets) == 0 {
		a.mu.Lock()
		a.diffBusy = false
		a.mu.Unlock()
		a.setNoticeErr(a.tr("err.noPages"))
		return
	}
	lang := a.currentLang()
	a.pushState()
	go func() {
		defer func() {
			a.mu.Lock()
			a.diffBusy = false
			a.mu.Unlock()
			a.pushState()
		}()
		dir, err := batchDir(filepath.Join(userArtifactRoot(), "diffs"), "diff")
		if err != nil {
			if !a.batchStale(taskID) {
				a.setNoticeErr(trLang(lang, "err.diffDir", err.Error()))
			}
			return
		}
		ok, failed, differ := a.runBatchDiff(taskID, targets, dir, lang, func(p *pageRecord) ([]byte, error) {
			return p.view.CapturePage()
		})
		if a.batchStale(taskID) {
			return
		}
		if failed > 0 {
			a.setNoticeOpen(trLang(lang, "notice.diffPartial", ok, failed, dir), "warning", dir, true)
			return
		}
		a.setNoticeOpen(trLang(lang, "notice.diffDone", len(targets), differ, dir), "success", dir, true)
	}()
}

// runBatchDiff captures the primary once, then diffs each target against it,
// writing {page}_diff.png per success and a diff.json summary. It returns
// ok/failed/differ counts for the notice. capture is injectable for tests.
func (a *app) runBatchDiff(taskID string, targets []diffTarget, dir, lang string, capture func(*pageRecord) ([]byte, error)) (ok, failed, differ int) {
	entries := make([]batchDiffEntry, 0, len(targets))
	used := map[string]bool{}
	writeSummary := func(primaryName, primaryErr string) {
		summary := map[string]any{
			"generatedAt": time.Now().Format(time.RFC3339Nano),
			"primary":     primaryName,
			"tolerance":   0,
			"pages":       entries,
		}
		if primaryErr != "" {
			summary["primaryError"] = primaryErr
		}
		data, _ := json.MarshalIndent(summary, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, "diff.json"), data, 0o644)
	}
	failAll := func(primaryName, reason string) {
		for _, t := range targets {
			entries = append(entries, batchDiffEntry{Name: t.name, PageID: t.id, Error: reason})
		}
		writeSummary(primaryName, reason)
		failed = len(targets)
	}

	a.mu.Lock()
	var pf *captureFlight
	primaryName := ""
	if a.batchShotValidLocked(taskID, a.primaryID) {
		p := a.pages[a.primaryID]
		primaryName = p.name
		pf = a.capturePageFlight(p, func() ([]byte, error) { return capture(p) })
	}
	a.mu.Unlock()
	if pf == nil {
		failAll("", "task stopped")
		return
	}
	refPNG, err := waitCapture(pf, captureTimeout)
	if err != nil {
		failAll(primaryName, err.Error())
		return
	}

	for _, t := range targets {
		a.mu.Lock()
		var f *captureFlight
		if a.batchShotValidLocked(taskID, t.id) {
			p := a.pages[t.id]
			f = a.capturePageFlight(p, func() ([]byte, error) { return capture(p) })
		}
		a.mu.Unlock()
		if f == nil {
			entries = append(entries, batchDiffEntry{Name: t.name, PageID: t.id, Error: "task stopped"})
			failed++
			continue
		}
		png, err := waitCapture(f, captureTimeout)
		if err != nil {
			entries = append(entries, batchDiffEntry{Name: t.name, PageID: t.id, Error: err.Error()})
			failed++
			continue
		}
		st, diff, err := diffPNG(refPNG, png, 0)
		if err != nil {
			entries = append(entries, batchDiffEntry{Name: t.name, PageID: t.id, Error: diffError(lang, t.name, primaryName, err).Error()})
			failed++
			continue
		}
		stem := sanitizeFileName(t.name) + "_diff"
		fname := stem + ".png"
		for i := 2; used[fname]; i++ {
			fname = fmt.Sprintf("%s_%d.png", stem, i)
		}
		used[fname] = true
		if err := os.WriteFile(filepath.Join(dir, fname), diff, 0o644); err != nil {
			entries = append(entries, batchDiffEntry{Name: t.name, PageID: t.id, Error: err.Error()})
			failed++
			continue
		}
		entries = append(entries, batchDiffEntry{Name: t.name, PageID: t.id, Stats: &st})
		ok++
		if st.Different > 0 {
			differ++
		}
	}
	writeSummary(primaryName, "")
	return
}

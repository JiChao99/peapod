package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// anchor identifies a single element on a page.
type anchor struct {
	Type     string `json:"type"`
	Selector string `json:"selector"`
}

// operation is a captured user action. Value is raw JSON so an empty string
// stays present (clearing an input), matching hasOwnProperty('value') in the
// replay script.
type operation struct {
	Type      string          `json:"type"`
	Anchor    anchor          `json:"anchor"`
	Value     json.RawMessage `json:"value,omitempty"`
	Checked   *bool           `json:"checked,omitempty"`
	ScrollX   *float64        `json:"scrollX,omitempty"`
	ScrollY   *float64        `json:"scrollY,omitempty"`
	Timestamp float64         `json:"timestamp"`
}

var operationTypes = map[string]bool{
	"click": true, "input": true, "change": true,
	"select": true, "scroll": true, "submit": true,
}

// validateOperation mirrors the host policy: known type, a bounded selector,
// and a bounded value.
func validateOperation(op *operation) error {
	if !operationTypes[op.Type] {
		return fmt.Errorf("unknown operation type %q", op.Type)
	}
	if op.Anchor.Type != "id" && op.Anchor.Type != "name" && op.Anchor.Type != "css" {
		return fmt.Errorf("unknown anchor type %q", op.Anchor.Type)
	}
	if op.Anchor.Selector == "" || len(op.Anchor.Selector) > 2048 {
		return fmt.Errorf("selector out of bounds")
	}
	if len(op.Value) > 10000 {
		return fmt.Errorf("value out of bounds")
	}
	return nil
}

// enqueueOp queues an operation for one target page. The caller holds a.mu and
// has checked that the task is running and the page is alive.
func (a *app) enqueueOp(p *pageRecord, op operation) {
	if p.closed || !p.syncOn || p.status == "paused" {
		return
	}
	// Anything but a scroll may navigate the page when replayed; navigation
	// follow must not race that with a LoadURL of its own.
	if op.Type != "scroll" {
		p.navPendingAt = time.Now()
	}
	select {
	case p.queue <- op:
	default:
		// Queue full: drop rather than block the capture path.
	}
}

// replayWorker drains a page's queue in order, one operation at a time, so
// replays on a target keep the sequence the primary page produced.
func (a *app) replayWorker(p *pageRecord) {
	for op := range p.queue {
		a.replayOnce(p, op)
	}
}

// replayOnce resolves the anchor then performs the operation on a target page.
// A failure pauses that page until the user reloads it.
func (a *app) replayOnce(p *pageRecord, op operation) {
	a.mu.Lock()
	paused := p.status == "paused" || !p.syncOn || p.closed
	view := p.view
	if !paused && view != nil {
		p.replaying = true
	}
	a.mu.Unlock()
	if paused || view == nil {
		return
	}
	defer a.finishReplay(p, op)

	if err := a.resolveAnchor(view, op.Anchor); err != nil {
		a.pausePage(p, op, err)
		return
	}
	opJSON, err := json.Marshal(op)
	if err != nil {
		a.pausePage(p, op, err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := view.Page().EvalContext(ctx, replayScript(string(opJSON)))
	if err != nil {
		a.pausePage(p, op, err)
		return
	}
	if m, ok := result.(map[string]any); ok {
		if completed, _ := m["completed"].(bool); !completed {
			if code, _ := m["errorCode"].(string); code != "" {
				a.pausePage(p, op, fmt.Errorf("%s", code))
			} else {
				a.pausePage(p, op, fmt.Errorf("replay did not complete"))
			}
			return
		}
	}
}

// finishReplay clears the in-flight flag and reconciles a follow destination
// that was skipped while this page was busy. A navigating replay (click/submit)
// may still commit its own navigation within the grace window, so in that case
// reconciliation waits for the grace to expire or the follower's own navigate.
func (a *app) finishReplay(p *pageRecord, op operation) {
	a.mu.Lock()
	p.replaying = false
	nav := ""
	if !p.closed && p.syncOn && p.status != "paused" && len(p.queue) == 0 &&
		p.pendingNav != "" && p.pendingNav != p.url {
		navigating := op.Type == "click" || op.Type == "submit"
		if !navigating || time.Since(p.navPendingAt) >= navFollowGrace {
			nav = p.pendingNav
			p.pendingNav = ""
		}
	}
	view := p.view
	a.mu.Unlock()
	if nav != "" && view != nil {
		view.Page().LoadURL(nav)
	}
}

// resolveAnchor waits for the selector to match exactly one element, with a
// 10s budget.
func (a *app) resolveAnchor(view *mygo.WebView, an anchor) error {
	anchorJSON, err := json.Marshal(an)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	result, err := view.Page().EvalContext(ctx, resolveScript(string(anchorJSON)))
	if err != nil {
		return fmt.Errorf("ANCHOR_TIMEOUT: %w", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		return fmt.Errorf("INVALID_LOCATOR")
	}
	if found, _ := m["found"].(bool); found {
		return nil
	}
	code, _ := m["errorCode"].(string)
	if code == "" {
		code = "NOT_FOUND"
	}
	return fmt.Errorf("%s", code)
}

// pausePage marks a target paused and records the failure for the panel.
func (a *app) pausePage(p *pageRecord, op operation, cause error) {
	a.mu.Lock()
	if p.closed {
		a.mu.Unlock()
		return
	}
	p.status = "paused"
	p.lastError = &syncError{
		TargetID:  p.id,
		Operation: op.Type,
		Code:      errorCode(cause),
		Message:   cause.Error(),
	}
	a.mu.Unlock()
	a.pushState()
}

func errorCode(err error) string {
	msg := err.Error()
	for _, code := range []string{"ANCHOR_TIMEOUT", "INVALID_LOCATOR", "AMBIGUOUS", "NOT_FOUND", "SENSITIVE"} {
		if strings.Contains(msg, code) {
			return code
		}
	}
	return "REPLAY_FAILED"
}

// syncError is one paused-target failure shown in the panel.
type syncError struct {
	TargetID  string `json:"targetId"`
	Operation string `json:"operation"`
	Code      string `json:"code"`
	Message   string `json:"message"`
}

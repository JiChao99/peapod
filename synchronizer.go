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

// navBarrier is an internal queue sentinel (never captured or validated) that
// orders a deferred follow navigation ahead of actions captured on the primary's
// new document, so those actions replay against the right page.
const navBarrier = "__follow_nav__"

// navBarrierTimeout bounds how long a barrier waits for the follower to commit
// the follow navigation before letting the queue drain again.
const navBarrierTimeout = 8 * time.Second

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
// replays on a target keep the sequence the primary page produced. A navigation
// barrier is applied (and waited on) before the actions queued behind it.
func (a *app) replayWorker(p *pageRecord) {
	for op := range p.queue {
		if op.Type == navBarrier {
			a.applyNavBarrier(p)
			continue
		}
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
			a.pausePage(p, op, fmt.Errorf("replay did not complete"))
			return
		}
	}
}

// finishReplay clears the in-flight flag and reconciles a follow destination
// that was skipped while this page was busy. Any replay but a scroll may still
// commit its own navigation within the grace window, so reconciliation then
// waits for the grace to expire (via the queued barrier) or the follower's own
// navigate.
func (a *app) finishReplay(p *pageRecord, op operation) {
	a.mu.Lock()
	p.replaying = false
	nav := a.reconcileNavLocked(p, op.Type != "scroll")
	view := p.view
	a.mu.Unlock()
	if nav != "" && view != nil {
		view.Page().LoadURL(nav)
	}
}

// reconcileNavLocked decides whether a deferred follow destination can be loaded
// now; the caller holds a.mu. It rechecks page eligibility, queue/replay state
// and the grace window so a paused, switched-off or still-busy follower is never
// auto-navigated. navigating is true when the last replay may commit its own
// navigation (anything but a scroll), matching enqueueOp's classification.
func (a *app) reconcileNavLocked(p *pageRecord, navigating bool) string {
	if p.closed || !p.syncOn || p.status == "paused" || p.replaying || len(p.queue) > 0 {
		return ""
	}
	if p.pendingNav == "" || p.pendingNav == p.url {
		return ""
	}
	if navigating && time.Since(p.navPendingAt) < navFollowGrace {
		return ""
	}
	nav := p.pendingNav
	p.pendingNav = ""
	p.navPendingAt = time.Time{}
	return nav
}

// applyNavBarrier loads the deferred follow destination and waits for the
// follower to commit it before the worker replays actions captured on the new
// document. It honors the grace window so a replay's own navigation lands first,
// and rechecks eligibility because the task may have stopped meanwhile.
func (a *app) applyNavBarrier(p *pageRecord) {
	deadline := time.Now().Add(navBarrierTimeout)
	for {
		a.mu.Lock()
		if p.closed || !p.syncOn || p.status == "paused" {
			a.mu.Unlock()
			return
		}
		target := p.pendingNav
		if target == "" {
			a.mu.Unlock()
			return
		}
		if !p.replaying && time.Since(p.navPendingAt) >= navFollowGrace {
			p.pendingNav = ""
			p.navPendingAt = time.Time{}
			view := p.view
			reached := target == p.url
			a.mu.Unlock()
			if reached || view == nil {
				return
			}
			view.Page().LoadURL(target)
			a.waitNavCommit(p, target, deadline)
			return
		}
		a.mu.Unlock()
		if !a.waitNavSignal(p, deadline) {
			return
		}
	}
}

// waitNavCommit blocks until the follower commits target, pauses, closes, or the
// deadline passes.
func (a *app) waitNavCommit(p *pageRecord, target string, deadline time.Time) {
	for {
		a.mu.Lock()
		done := p.closed || p.status == "paused" || p.url == target
		a.mu.Unlock()
		if done || !a.waitNavSignal(p, deadline) {
			return
		}
	}
}

// waitNavSignal waits for a navigation signal or a short poll tick, returning
// false once the deadline passes.
func (a *app) waitNavSignal(p *pageRecord, deadline time.Time) bool {
	if time.Now().After(deadline) {
		return false
	}
	select {
	case <-p.navSignal:
	case <-time.After(20 * time.Millisecond):
	}
	return true
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
	for _, code := range []string{"ANCHOR_TIMEOUT", "INVALID_LOCATOR", "AMBIGUOUS", "NOT_FOUND"} {
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

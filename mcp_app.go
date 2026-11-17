package main

import (
	"context"
	"fmt"
	"time"

	"github.com/egoist/mygo"
)

// hostCapture injects a host-trusted operation (from MCP) onto every synced
// follower, mirroring SyncService.Capture without the guest caller check.
func (a *app) hostCapture(op operation) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != "running" || a.primaryID == "" {
		return fmt.Errorf("%s", a.tr("err.notRunning"))
	}
	if err := validateOperation(&op); err != nil {
		return err
	}
	for _, id := range a.order {
		if id == a.primaryID {
			continue
		}
		if p := a.pages[id]; p != nil {
			a.enqueueOp(p, op)
		}
	}
	return nil
}

// setSync flips a page's sync flag and its panel Switch binding together so
// the two never disagree. Both writes happen on the UI thread: the binding is
// owned by the Switch widget, and applySyncToggles copies it back into syncOn
// each frame, so writing either off-thread would race the widget or be reverted
// on the next render.
func (a *app) setSync(id string, on bool) error {
	a.mu.Lock()
	p := a.pages[id]
	alive := p != nil && !p.closed
	a.mu.Unlock()
	if !alive {
		return fmt.Errorf("unknown page %q", id)
	}
	mygo.RunOnMain(func() {
		a.mu.Lock()
		if p := a.pages[id]; p != nil && !p.closed {
			p.syncOn = on
		}
		if v, ok := a.syncUI[id]; ok {
			*v = on
		} else {
			b := on
			a.syncUI[id] = &b
		}
		a.mu.Unlock()
	})
	a.pushState()
	return nil
}

// navigatePage loads url on a page (default primary). A primary move then
// propagates to followers through navigation follow.
func (a *app) navigatePage(id, url string) error {
	if err := a.validateURL(url); err != nil {
		return err
	}
	a.mu.Lock()
	if id == "" {
		id = a.primaryID
	}
	p := a.pages[id]
	var view *mygo.WebView
	if p != nil && !p.closed {
		resetNavLocked(p)
		view = p.view
	}
	a.mu.Unlock()
	if view == nil {
		return fmt.Errorf("unknown page %q", id)
	}
	return view.Page().LoadURL(url)
}

// captureTimeout is the default budget for one CapturePage call. WebView2's
// CapturePreview callback never fires for a view that is not rendering — the
// window is minimized or hidden, or the page is clipped out of the stage — so
// the call must be bounded, or the MCP request hangs until the client times out.
const captureTimeout = 10 * time.Second

// captureTimeoutFromMs clamps a caller-supplied millisecond budget into the
// supported 1s–60s range, falling back to the default when unset.
func captureTimeoutFromMs(ms float64) time.Duration {
	if ms <= 0 {
		return captureTimeout
	}
	return time.Duration(min(max(ms, 1000), 60000)) * time.Millisecond
}

// errCaptureTimeout reports a capture that outlived its budget. The message is
// actionable for an agent: the page is not rendering, so restore the window or
// make the page active rather than blindly retrying.
type errCaptureTimeout struct{ timeout time.Duration }

func (e errCaptureTimeout) Error() string {
	return fmt.Sprintf("screenshot timed out after %s: the page is not rendering "+
		"(the Peapod window may be minimized or hidden, or this page is stacked behind the active one). "+
		"Restore the Peapod window and retry, or make this page active first.", e.timeout)
}

// captureWithTimeout runs capture in a goroutine and waits up to timeout. The
// result channel is buffered, so a CapturePreview callback that lands after the
// deadline is discarded rather than blocking or double-delivering.
func captureWithTimeout(capture func() ([]byte, error), timeout time.Duration) ([]byte, error) {
	if timeout <= 0 {
		timeout = captureTimeout
	}
	type result struct {
		png []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		png, err := capture()
		ch <- result{png, err}
	}()
	select {
	case r := <-ch:
		return r.png, r.err
	case <-time.After(timeout):
		return nil, errCaptureTimeout{timeout}
	}
}

// capturePNG grabs one page (default primary) as PNG bytes, bounded by timeout
// so a non-rendering page fails fast instead of hanging the caller.
func (a *app) capturePNG(id string, timeout time.Duration) (string, []byte, error) {
	a.mu.Lock()
	if id == "" {
		id = a.primaryID
	}
	p := a.pages[id]
	var view *mygo.WebView
	name := ""
	if p != nil && !p.closed {
		view = p.view
		name = p.name
	}
	a.mu.Unlock()
	if view == nil {
		return "", nil, fmt.Errorf("unknown page %q", id)
	}
	png, err := captureWithTimeout(view.CapturePage, timeout)
	return name, png, err
}

// diagTimeout bounds the Eval that reads a page's diagnostics. A failed page
// (DNS/TLS error) may show WebView2's error document with no bridge, so the read
// can hang; fail fast and still return the Go-side status and navError.
const diagTimeout = 5 * time.Second

// pageDiagnostics collects one page's acceptance diagnostics: the Go-side
// status/navError plus, via a bounded Eval, the page's URL/title/readyState, the
// main document's HTTP status, bounded body text, and the console/network/error
// ring buffers. The collector is opt-in: diagOnDemand installs it lazily on the
// first call (idempotent), or it was already injected at document start when the
// task began with diagnostics:true. Page content is untrusted data.
func (a *app) pageDiagnostics(id string) (map[string]any, error) {
	a.mu.Lock()
	if id == "" {
		id = a.primaryID
	}
	p := a.pages[id]
	var view *mygo.WebView
	out := map[string]any{}
	if p != nil && !p.closed {
		view = p.view
		out["pageId"] = p.id
		out["name"] = p.name
		out["url"] = p.url
		out["status"] = p.status
		if p.navError != nil {
			out["navError"] = p.navError
		}
	}
	a.mu.Unlock()
	if view == nil {
		return nil, fmt.Errorf("unknown page %q", id)
	}
	ctx, cancel := context.WithTimeout(context.Background(), diagTimeout)
	defer cancel()
	res, err := view.Page().EvalContext(ctx, diagOnDemand())
	if err != nil {
		// The in-page read is unavailable (e.g. an error page with no bridge);
		// the Go-side state above is still useful, so report it with the reason.
		out["evalError"] = err.Error()
		return out, nil
	}
	out["eval"] = res
	return out, nil
}

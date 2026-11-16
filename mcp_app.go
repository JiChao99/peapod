package main

import (
	"fmt"

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
		view = p.view
	}
	a.mu.Unlock()
	if view == nil {
		return fmt.Errorf("unknown page %q", id)
	}
	return view.Page().LoadURL(url)
}

// capturePNG grabs one page (default primary) as PNG bytes.
func (a *app) capturePNG(id string) (string, []byte, error) {
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
	png, err := view.CapturePage()
	return name, png, err
}
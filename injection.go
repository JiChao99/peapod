package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

const (
	maxInjectedJSBytes         = 64 << 10
	maxInjectedEvents          = 50
	maxInjectedNameBytes       = 64
	maxInjectedPayloadBytes    = 4000
	maxInjectedEventBurst      = 50
	maxInjectedEventsPerSecond = 20
)

type injectedEvent struct {
	At      time.Time `json:"at"`
	Name    string    `json:"name"`
	Payload string    `json:"payload,omitempty"`
}

type injectedEventPayload struct {
	PageID  string `json:"pageId"`
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

type injectedScriptPayload struct {
	PageID string `json:"pageId"`
	Source string `json:"source"`
}

func injectedPreload(pageID, script string) string {
	if strings.TrimSpace(script) == "" {
		return ""
	}
	return fmt.Sprintf(`(() => {
  const PAGE_ID = %q;
  const INJECT_SOURCE = %q;
  const MAX_NAME_BYTES = %d;
  const MAX_PAYLOAD_BYTES = %d;
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();
  const clipText = (value, maxBytes) => {
    if (value.length <= maxBytes) {
      const bytes = encoder.encode(value);
      return bytes.length <= maxBytes ? value : decoder.decode(bytes.slice(0, maxBytes));
    }
    return decoder.decode(encoder.encode(value.slice(0, maxBytes)).slice(0, maxBytes));
  };
  const stringifyPayload = (value) => {
    let budget = MAX_PAYLOAD_BYTES;
    let nodes = 0;
    try {
      const text = JSON.stringify(value === undefined ? null : value, function(key, item) {
        if (Array.isArray(item) && item.length > 1000) throw new Error('payload is too complex');
        if (++nodes > 1000) throw new Error('payload is too complex');
        if (key && key.length > budget) throw new Error('payload is too large');
        budget -= key.length;
        if (typeof item === 'string') {
          if (item.length > budget) throw new Error('payload is too large');
          budget -= item.length;
        }
        return item;
      });
      if (text.length > MAX_PAYLOAD_BYTES) throw new Error('payload is too large');
      return text;
    } catch (_) {
      return JSON.stringify({error: 'payload is too large or not serializable'});
    }
  };
  let eventTokens = %d;
  let eventAt = Date.now();
  const takeEventToken = () => {
    const now = Date.now();
    eventTokens = Math.min(%d, eventTokens + (now - eventAt) * %d / 1000);
    eventAt = now;
    if (eventTokens < 1) return false;
    eventTokens--;
    return true;
  };
  try {
    const peapod = window.peapod || (window.peapod = {});
    peapod.pageId = PAGE_ID;
    peapod.emit = function(name, payload) {
      if (!takeEventToken()) return Promise.resolve(false);
      const eventName = clipText(name === undefined || name === null || name === '' ? 'event' : String(name), MAX_NAME_BYTES);
      const eventPayload = clipText(stringifyPayload(payload), MAX_PAYLOAD_BYTES);
      try {
        return Promise.resolve(window.mygo.call('Sync.InjectEvent', {
          pageId: PAGE_ID,
          name: eventName,
          payload: eventPayload
        })).catch(() => false);
      } catch (_) {
        return Promise.resolve(false);
      }
    };
  } catch (err) {
    console.error('[Peapod inject bridge]', err);
  }
  try {
    const factory = new Function('return (async () => {\n' + INJECT_SOURCE + '\n})();\n//# sourceURL=peapod-inject.js');
    Promise.resolve(factory()).catch((err) => console.error('[Peapod inject]', err));
  } catch (_) {
    try {
      Promise.resolve(window.mygo.call('Sync.RunInjectedScript', {
        pageId: PAGE_ID,
        source: INJECT_SOURCE
      })).catch((err) => console.error('[Peapod inject]', err));
    } catch (err) {
      console.error('[Peapod inject]', err);
    }
  }
})();`, pageID, script, maxInjectedNameBytes, maxInjectedPayloadBytes, maxInjectedEventBurst, maxInjectedEventBurst, maxInjectedEventsPerSecond)
}

func (s *SyncService) RunInjectedScript(ctx context.Context, payload injectedScriptPayload) error {
	if len(payload.Source) > maxInjectedJSBytes {
		return fmt.Errorf("injected script exceeds %d bytes", maxInjectedJSBytes)
	}
	a := s.a
	caller := mygo.CallerPage(ctx)
	a.mu.Lock()
	p := a.pages[payload.PageID]
	if p == nil || p.closed || caller != p.view.Page() {
		a.mu.Unlock()
		return fmt.Errorf("forbidden")
	}
	view := p.view
	a.mu.Unlock()
	code := `(async () => {
` + payload.Source + `
})().catch((err) => console.error('[Peapod inject]', err));`
	_, err := view.Page().EvalContext(ctx, code)
	return err
}

func (s *SyncService) InjectEvent(ctx context.Context, payload injectedEventPayload) error {
	a := s.a
	caller := mygo.CallerPage(ctx)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.status != "starting" && a.status != "running" {
		return nil
	}
	p := a.pages[payload.PageID]
	if p == nil || p.closed || caller != p.view.Page() {
		return fmt.Errorf("forbidden")
	}
	if !a.allowInjectedEventLocked(p, time.Now()) {
		return fmt.Errorf("injected event rate limit exceeded")
	}
	a.recordInjectedEventLocked(p, injectedEvent{
		Name:    clipUTF8(strings.TrimSpace(payload.Name), maxInjectedNameBytes),
		Payload: clipUTF8(payload.Payload, maxInjectedPayloadBytes),
	})
	return nil
}

func (a *app) allowInjectedEventLocked(p *pageRecord, now time.Time) bool {
	if p.injectTokenAt.IsZero() {
		p.injectTokens = maxInjectedEventBurst
	} else {
		elapsed := now.Sub(p.injectTokenAt).Seconds()
		p.injectTokens = min(maxInjectedEventBurst, p.injectTokens+elapsed*maxInjectedEventsPerSecond)
	}
	p.injectTokenAt = now
	if p.injectTokens < 1 {
		return false
	}
	p.injectTokens--
	return true
}

func (a *app) recordInjectedEventLocked(p *pageRecord, event injectedEvent) {
	if event.At.IsZero() {
		event.At = time.Now()
	}
	p.injectEvents = append(p.injectEvents, event)
	if len(p.injectEvents) > maxInjectedEvents {
		p.injectEvents = append([]injectedEvent(nil), p.injectEvents[len(p.injectEvents)-maxInjectedEvents:]...)
	}
}

func (a *app) injectedEventsSnapshotLocked(p *pageRecord) []injectedEvent {
	if len(p.injectEvents) == 0 {
		return nil
	}
	return append([]injectedEvent(nil), p.injectEvents...)
}

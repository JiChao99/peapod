package main

import "fmt"

// capturePreload is injected into every guest page before its own scripts.
// It captures trusted user operations on the primary page and sends them to
// Go through window.mygo.call. Replayed operations set window.__syncReplaying
// so the capture listener ignores them.
func capturePreload(pageID string) string {
	return fmt.Sprintf(`(() => {
  const PAGE_ID = %q;
  const TYPES = ['click', 'input', 'change', 'select', 'scroll', 'submit'];
  const VALUE_TYPES = new Set(['text','search','email','url','tel','number','range','date','month','week','time','datetime-local','color']);
  const MAX_SEL = 2048;
  const MAX_VAL = 10000;

  function escapeIdentifier(value) {
    if (typeof CSS !== 'undefined' && typeof CSS.escape === 'function') return CSS.escape(value);
    let escaped = '';
    const input = String(value);
    for (let i = 0; i < input.length;) {
      const cp = input.codePointAt(i);
      const ch = String.fromCodePoint(cp);
      const pos = i;
      i += cp > 0xffff ? 2 : 1;
      if (cp === 0) escaped += '\\fffd ';
      else if ((cp >= 1 && cp <= 31) || cp === 127 || (pos === 0 && cp >= 48 && cp <= 57) || (pos === 1 && cp >= 48 && cp <= 57 && input[0] === '-')) escaped += '\\' + cp.toString(16) + ' ';
      else if (pos === 0 && ch === '-' && input.length === 1) escaped += '\\-';
      else if (cp >= 128 || ch === '-' || ch === '_' || /[a-zA-Z0-9]/.test(ch)) escaped += ch;
      else escaped += '\\' + ch;
    }
    return escaped;
  }

  function escapeString(value) {
    return String(value).replace(/[\\"\0\n\r\f]/g, (c) => {
      switch (c) {
        case '\\': return '\\\\';
        case '"': return '\\"';
        case '\0': return '\\fffd ';
        case '\n': return '\\a ';
        case '\r': return '\\d ';
        default: return '\\c ';
      }
    });
  }

  function uniqueAnchor(selector, target, type) {
    if (!selector || selector.length > MAX_SEL) return null;
    try {
      const matches = document.querySelectorAll(selector);
      return matches.length === 1 && matches[0] === target ? { type, selector } : null;
    } catch { return null; }
  }

  function buildAnchor(target) {
    if (!target || target.nodeType !== 1) return null;
    if (target.id) {
      const a = uniqueAnchor('#' + escapeIdentifier(target.id), target, 'id');
      if (a) return a;
    }
    const name = target.getAttribute && target.getAttribute('name');
    if (name) {
      const a = uniqueAnchor('[name="' + escapeString(name) + '"]', target, 'name');
      if (a) return a;
    }
    const segments = [];
    let current = target;
    while (current && current.nodeType === 1) {
      const tag = String(current.localName || current.tagName || '').toLowerCase();
      if (!/^[a-z][a-z0-9-]*$/.test(tag)) return null;
      let segment = tag;
      const parent = current.parentElement;
      if (parent) {
        const siblings = Array.from(parent.children || []).filter((s) => String(s.localName || s.tagName || '').toLowerCase() === tag);
        const index = siblings.indexOf(current);
        if (index < 0) return null;
        if (siblings.length > 1) segment += ':nth-of-type(' + (index + 1) + ')';
      }
      segments.unshift(segment);
      current = parent;
    }
    return uniqueAnchor(segments.join(' > '), target, 'css');
  }

  function captureOperation(event) {
    if (!event || !event.isTrusted || TYPES.indexOf(event.type) < 0) return null;
    let target = event.target;
    if (event.type === 'scroll' && (target === document || target === window)) {
      target = document.scrollingElement || document.documentElement;
    }
    const anchor = buildAnchor(target);
    if (!anchor) return null;
    const tag = String(target.tagName || '').toLowerCase();
    const inputType = String(target.type || '').toLowerCase();
    if (tag === 'input' && (inputType === 'password' || inputType === 'file')) return null;

    const operation = {
      type: event.type,
      anchor,
      timestamp: Number.isFinite(event.timeStamp) ? event.timeStamp : Date.now(),
    };
    if (event.type === 'input' || event.type === 'change' || event.type === 'select') {
      if (tag === 'input' && (inputType === 'checkbox' || inputType === 'radio') && typeof target.checked === 'boolean') {
        operation.checked = target.checked;
      } else if (tag === 'select' || tag === 'textarea' || (tag === 'input' && VALUE_TYPES.has(inputType))) {
        const value = target.value;
        if (typeof value === 'string' && value.length <= MAX_VAL) operation.value = value;
        else if (typeof value === 'number' && Number.isFinite(value)) operation.value = value;
      }
    }
    if (event.type === 'scroll') {
      const scrollX = Number.isFinite(target.scrollLeft) ? target.scrollLeft : window.scrollX;
      const scrollY = Number.isFinite(target.scrollTop) ? target.scrollTop : window.scrollY;
      if (Number.isFinite(scrollX)) operation.scrollX = scrollX;
      if (Number.isFinite(scrollY)) operation.scrollY = scrollY;
    }
    return operation;
  }

  // The same scroll event reaches both the document and the window capture
  // listener (scroll is registered on each). Remember handled events so one
  // user scroll is captured — and mirrored — exactly once.
  const seenEvents = new WeakSet();
  const capture = (event) => {
    if (seenEvents.has(event)) return;
    seenEvents.add(event);
    if (window.__syncReplaying) return;
    // A replayed scroll fires its scroll event in a queued task AFTER the
    // replay eval returned (and __syncReplaying was reset), and the event is
    // isTrusted even though the scroll was programmatic. Suppress captures
    // until that event has landed, or the replay echoes back as a guest op.
    if (event.type === 'scroll' && window.__syncReplayScrollUntil && Date.now() < window.__syncReplayScrollUntil) return;
    const operation = captureOperation(event);
    if (operation && window.mygo && typeof window.mygo.call === 'function') {
      window.mygo.call('Sync.Capture', { pageId: PAGE_ID, operation }).catch(() => {});
    }
  };
  for (const type of TYPES) document.addEventListener(type, capture, true);
  window.addEventListener('scroll', capture, true);
})();`, pageID)
}

// dialogPreload reports popups that appear while a task is recording. It is
// injected only for recording tasks, so ordinary mirroring pays nothing.
//
// Two kinds of popups are reported through Sync.Dialog:
//   - DOM popups: <dialog open>, [role=dialog], [role=alertdialog] and
//     [aria-modal=true]; floating live regions ([role=alert]/[role=status]
//     inside a fixed/sticky container) are reported as toasts. The observer
//     waits for a short quiet window so async content can render, and reports
//     an element again only when its text changes or it reopens.
//   - native alert/confirm/prompt: the message is reported before the native
//     dialog blocks the page. The dialog itself is not page pixels, so Go
//     records the text without a screenshot.
//
// Shadow roots and iframes are not observed: Peapod's preload runs in the main
// frame only, and a closed shadow root is not reachable.
func dialogPreload(pageID string) string {
	return fmt.Sprintf(`(() => {
  try {
    if (window.__peapodDialog) return;
    window.__peapodDialog = true;
    const PAGE_ID = %q;
    const MAX_TEXT = 4000;
    const MAX_SEL = 512;
    const QUIET = 220;
    const MAX_DELAY = 900;
    const MIN_GAP = 800;
    const DIALOGS = 'dialog[open],[role="dialog"],[role="alertdialog"],[aria-modal="true"],[role="alert"],[role="status"]';
    const FLOAT_ROLES = new Set(['alert', 'status']);
    const states = new WeakMap();
    // Elements actually reported. A tracked element that closes or stops
    // matching has its state cleared on the next scan, so reopening it with the
    // same text (e.g. a repeated confirm) is reported again.
    const tracked = new Set();
    let timer = 0;
    let firstAt = 0;

    const clip = (s, n) => {
      const v = String(s == null ? '' : s);
      return v.length > n ? v.slice(0, n) : v;
    };

    const visible = (el) => {
      try {
        if (!el || !el.isConnected) return false;
        const r = el.getBoundingClientRect();
        if (r.width < 2 || r.height < 2) return false;
        const s = getComputedStyle(el);
        if (s.display === 'none' || s.visibility === 'hidden' || s.visibility === 'collapse') return false;
        if (parseFloat(s.opacity || '1') < 0.05) return false;
        if (el.hidden || el.getAttribute('aria-hidden') === 'true') return false;
        return true;
      } catch { return false; }
    };

    // A live region only counts when it floats over the page: inline role=alert
    // validation text must not spawn popup evidence on every keystroke.
    const floating = (el) => {
      try {
        let n = el;
        for (let i = 0; n && n.nodeType === 1 && i < 6; i++, n = n.parentElement) {
          const p = getComputedStyle(n).position;
          if (p === 'fixed' || p === 'sticky') return true;
        }
      } catch {}
      return false;
    };

    // A live region nested in a modal belongs to that popup (e.g. inline
    // validation text); reporting it separately would duplicate the evidence.
    const inModal = (el) => {
      try {
        return !!el.closest('dialog[open],[role="dialog"],[role="alertdialog"],[aria-modal="true"]');
      } catch { return false; }
    };

    const isCandidate = (el) => {
      if (!el || el.nodeType !== 1) return false;
      const tag = String(el.localName || '').toLowerCase();
      const role = String(el.getAttribute('role') || '').toLowerCase();
      if (tag === 'dialog' && el.hasAttribute('open')) return true;
      if (role === 'dialog' || role === 'alertdialog' || el.getAttribute('aria-modal') === 'true') return true;
      if (FLOAT_ROLES.has(role)) return floating(el) && !inModal(el);
      return false;
    };

    const describe = (el) => {
      const tag = String(el.localName || el.tagName || '').toLowerCase();
      const role = String(el.getAttribute('role') || '').toLowerCase();
      let kind = 'modal';
      if (tag === 'dialog' || role === 'dialog' || role === 'alertdialog') kind = 'dialog';
      else if (FLOAT_ROLES.has(role)) kind = 'toast';
      const id = el.id ? '#' + el.id : '';
      const cls = typeof el.className === 'string'
        ? el.className.trim().split(/\s+/).filter(Boolean).slice(0, 3).join('.')
        : '';
      let text = '';
      try { text = (el.innerText || el.textContent || '').trim(); } catch {}
      return {
        kind,
        role,
        text: clip(text, MAX_TEXT),
        selector: clip(tag + id + (cls ? '.' + cls : ''), MAX_SEL),
      };
    };

    const send = (kind, role, text, selector, value) => {
      try {
        if (window.mygo && typeof window.mygo.call === 'function') {
          window.mygo.call('Sync.Dialog', {
            pageId: PAGE_ID, kind, role: role || '', text: text || '',
            selector: selector || '', value: value || '',
          }).catch(() => {});
        }
      } catch {}
    };

    const consider = (el) => {
      if (!isCandidate(el)) return;
      if (!visible(el)) {
        const prev = states.get(el);
        if (prev && prev.text !== '') states.set(el, { text: '', at: 0 });
        return;
      }
      const d = describe(el);
      const prev = states.get(el);
      if (prev && prev.text === d.text) return;
      const now = Date.now();
      if (prev && now - prev.at < MIN_GAP) {
        setTimeout(schedule, MIN_GAP - (now - prev.at));
        return;
      }
      states.set(el, { text: d.text, at: now });
      if (!d.text && d.kind !== 'dialog') return;
      tracked.add(el);
      send(d.kind, d.role, d.text, d.selector, '');
    };

    const scan = () => {
      // Forget popups that closed, detached or stopped matching, so a reopen
      // with the same copy is a new report.
      for (const el of tracked) {
        if (!el.isConnected || !isCandidate(el) || !visible(el)) {
          tracked.delete(el);
          states.delete(el);
        }
      }
      let nodes;
      try { nodes = document.querySelectorAll(DIALOGS); } catch { return; }
      for (const el of nodes) consider(el);
    };

    function schedule() {
      const now = Date.now();
      if (!firstAt) firstAt = now;
      clearTimeout(timer);
      const wait = Math.max(0, Math.min(QUIET, MAX_DELAY - (now - firstAt)));
      timer = setTimeout(() => { firstAt = 0; scan(); }, wait);
    }

    try {
      // Observe the document, not documentElement: it exists even at document
      // start, and it survives a document.write that replaces the root.
      new MutationObserver(schedule).observe(document, {
        childList: true,
        subtree: true,
        characterData: true,
        attributes: true,
        attributeFilter: ['class', 'style', 'open', 'role', 'aria-modal', 'aria-hidden', 'hidden'],
      });
    } catch {}
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', schedule, { once: true });
    } else {
      schedule();
    }

    const wrapNative = (name, kind) => {
      const orig = window[name];
      if (typeof orig !== 'function') return;
      try {
        window[name] = function (message) {
          send(kind, '', clip(message, MAX_TEXT), '', arguments.length > 1 ? clip(arguments[1], 200) : '');
          return orig.apply(window, arguments);
        };
      } catch {}
    };
    wrapNative('alert', 'alert');
    wrapNative('confirm', 'confirm');
    wrapNative('prompt', 'prompt');
  } catch {}
})();`, pageID)
}

// resolveScript waits until the anchor matches exactly one element, returning
// {found:true} or {errorCode}. The wait is short (3s): a missing static target
// must pause the follower quickly enough for the user to see it, while a
// re-rendering SPA still gets a brief window to attach the element.
func resolveScript(anchorJSON string) string {
	return `(async () => {
    const anchor = ` + anchorJSON + `;
    const deadline = Date.now() + 3000;
    while (Date.now() <= deadline) {
      let matches;
      try { matches = document.querySelectorAll(anchor.selector); }
      catch { return { errorCode: 'INVALID_LOCATOR' }; }
      if (matches.length === 1) return { found: true };
      if (matches.length > 1) return { errorCode: 'AMBIGUOUS' };
      await new Promise((resolve) => setTimeout(resolve, 100));
    }
    return { errorCode: 'NOT_FOUND' };
  })();`
}

// replayScript performs the operation on the unique anchored element. It sets
// window.__syncReplaying so the page's own capture listener ignores the
// synthetic events it dispatches.
func replayScript(operationJSON string) string {
	return `(() => {
    const operation = ` + operationJSON + `;
    window.__syncReplaying = true;
    try {
      const matches = document.querySelectorAll(operation.anchor.selector);
      if (matches.length !== 1) throw new Error(matches.length ? 'AMBIGUOUS' : 'NOT_FOUND');
      const target = matches[0];
      // Mirror the capture filter on the resolved element: a host-injected
      // operation (MCP) can name a password/file input the guest filter never
      // saw, so refuse to replay onto it here too.
      const tag = String(target.tagName || '').toLowerCase();
      const inputType = String(target.type || '').toLowerCase();
      if (tag === 'input' && (inputType === 'password' || inputType === 'file')) {
        return { completed: false, errorCode: 'SENSITIVE' };
      }
      const dispatch = (type) => target.dispatchEvent(new Event(type, { bubbles: true }));
      if (operation.type === 'click') {
        target.click();
      } else if (operation.type === 'input' || operation.type === 'change' || operation.type === 'select') {
        if (typeof operation.checked === 'boolean') target.checked = operation.checked;
        else if (Object.prototype.hasOwnProperty.call(operation, 'value')) target.value = operation.value;
        dispatch('input');
        dispatch('change');
        if (operation.type === 'select') dispatch('select');
      } else if (operation.type === 'scroll') {
        // Keep suppressing scroll capture past this eval: the scroll event is
        // dispatched by the UA in a later task, still isTrusted (see capture).
        window.__syncReplayScrollUntil = Date.now() + 500;
        target.scrollTo({
          left: typeof operation.scrollX === 'number' ? operation.scrollX : target.scrollLeft,
          top: typeof operation.scrollY === 'number' ? operation.scrollY : target.scrollTop,
          behavior: 'instant',
        });
      } else if (operation.type === 'submit') {
        if (target.tagName && target.tagName.toLowerCase() === 'form' && typeof target.requestSubmit === 'function') {
          target.requestSubmit();
        } else if (typeof target.click === 'function') {
          target.click();
        }
      }
      return { completed: true };
    } finally {
      window.__syncReplaying = false;
    }
  })();`
}

// diagPreload installs a lightweight, on-demand diagnostics collector in the
// page's main world. It keeps bounded ring buffers of console output, uncaught
// errors and network activity (fetch/XHR pass-through hooks plus resource
// timing), readable later through window.__peapodDiag. It sends nothing to Go on
// its own — peapod_diagnostics reads it via Eval — so the capture/replay path is
// untouched. Every hook is defensive: it passes the original call through
// unchanged and never throws into the page.
//
// The hooks are opt-in: they are injected at document start only when a task is
// started with diagnostics=true, or lazily by the first peapod_diagnostics call.
// The guard makes re-injection idempotent — it never double-wraps a page's
// console/fetch/XHR or resets the buffers already collected.
func diagPreload() string {
	return `(() => {
  try {
    if (window.__peapodDiag) return;
    const MAX = 200;
    const diag = { console: [], network: [], errors: [] };
    // Expose snapshots so page scripts cannot clear the evidence buffers.
    Object.defineProperty(window, '__peapodDiag', {
      configurable: false,
      enumerable: false,
      get: () => ({
        console: diag.console.map(item => ({ ...item })),
        network: diag.network.map(item => ({ ...item })),
        errors: diag.errors.map(item => ({ ...item })),
      }),
    });
    const now = () => Date.now();
    const push = (arr, item) => { arr.push(item); if (arr.length > MAX) arr.shift(); };
    const clip = (s) => String(s).slice(0, 2000);

    for (const level of ['log', 'info', 'warn', 'error', 'debug']) {
      const orig = typeof console[level] === 'function' ? console[level].bind(console) : null;
      if (!orig) continue;
      console[level] = (...args) => {
        try {
          const text = args.map((a) => {
            if (typeof a === 'string') return a;
            // Error props are non-enumerable, so JSON.stringify(err) is '{}'.
            // Keep the message/stack a caught console.error(err) would lose.
            if (a instanceof Error || (a && typeof a.message === 'string' && typeof a.stack === 'string')) {
              return a.stack || String(a);
            }
            try { return JSON.stringify(a); } catch { return String(a); }
          }).join(' ');
          push(diag.console, { level, text: clip(text), ts: now() });
        } catch {}
        return orig(...args);
      };
    }

    window.addEventListener('error', (e) => {
      try { push(diag.errors, { message: clip(e.message || (e.error && e.error.message) || 'error'), source: e.filename || '', line: e.lineno || 0, ts: now() }); } catch {}
    });
    window.addEventListener('unhandledrejection', (e) => {
      try { const r = e.reason; push(diag.errors, { message: clip(r && r.message ? r.message : r), rejection: true, ts: now() }); } catch {}
    });

    if (typeof window.fetch === 'function') {
      const origFetch = window.fetch;
      // Best-effort metadata. A fetch target may be a string, a Request (.url) or
      // a URL (.href); init.method may be a caller accessor. Read defensively so
      // an inspection failure never throws into the page.
      const fetchMeta = (args) => {
        try {
          const req = args[0];
          const url = clip(typeof req === 'string' ? req : (req && (req.url || req.href)) || '');
          const method = String((args[1] && args[1].method) || (req && req.method) || 'GET');
          return { url, method };
        } catch { return { url: '', method: 'GET' }; }
      };
      window.fetch = function (...args) {
        const start = now();
        let p;
        try { p = origFetch.apply(this, args); }
        catch (err) { const m = fetchMeta(args); push(diag.network, { method: m.method, url: m.url, status: 0, error: clip(err), ts: start }); throw err; }
        // Call native fetch FIRST, then inspect args: a stateful .url/.method
        // getter is consumed by fetch before the collector reads it, so enabling
        // diagnostics cannot alter the request (reading before could).
        const m = fetchMeta(args);
        return p.then((resp) => {
          try { push(diag.network, { method: m.method, url: m.url, status: resp.status, ok: resp.ok, duration: now() - start, ts: start }); } catch {}
          return resp;
        }, (err) => {
          try { push(diag.network, { method: m.method, url: m.url, status: 0, error: clip(err), duration: now() - start, ts: start }); } catch {}
          throw err;
        });
      };
    }

    if (typeof window.XMLHttpRequest === 'function') {
      const proto = window.XMLHttpRequest.prototype;
      const origOpen = proto.open;
      const origSend = proto.send;
      proto.open = function (method, url, ...rest) {
        // Call native open() FIRST so it converts method/url once. Record them
        // only when they are plain strings: re-converting a caller-owned object
        // with a stateful toString() here could alter the endpoint or throw.
        const ret = origOpen.call(this, method, url, ...rest);
        try {
          this.__peapod = {
            method: typeof method === 'string' ? method : 'GET',
            url: typeof url === 'string' ? clip(url) : '',
            ts: now(),
          };
        } catch {}
        return ret;
      };
      proto.send = function (...args) {
        const meta = this.__peapod;
        if (meta) {
          // One-shot: a reused XMLHttpRequest must not re-record an earlier
          // request with the latest response's status, and listeners must not
          // accumulate outside the bounded ring buffer.
          this.addEventListener('loadend', () => {
            try { push(diag.network, { method: meta.method, url: meta.url, status: this.status || 0, ok: this.status >= 200 && this.status < 400, duration: now() - meta.ts, ts: meta.ts }); } catch {}
          }, { once: true });
        }
        return origSend.apply(this, args);
      };
    }

    try {
      if (typeof PerformanceObserver === 'function') {
        const seen = new Set();
        const po = new PerformanceObserver((list) => {
          for (const e of list.getEntries()) {
            try {
              // fetch/XHR are already recorded by the hooks above.
              if (e.initiatorType === 'fetch' || e.initiatorType === 'xmlhttprequest') continue;
              // Dedup by entry identity, not URL: distinct requests to the same
              // asset (an image that reloads with a 404) must each be retained.
              if (seen.has(e)) continue;
              if (seen.size > 1000) seen.clear();
              seen.add(e);
              push(diag.network, { method: 'GET', url: clip(e.name), status: e.responseStatus || 0, resource: e.initiatorType || '', duration: Math.round(e.duration), ts: now() });
            } catch {}
          }
        });
        po.observe({ type: 'resource', buffered: true });
      }
    } catch {}
  } catch {}
})();`
}

// diagScript reads the collector plus the page's own state in one Eval. The
// navigation entry's responseStatus gives the main document's HTTP status without
// a DevTools protocol round-trip; body text is bounded for acceptance assertions.
func diagScript() string {
	return `(() => {
    const d = window.__peapodDiag || { console: [], network: [], errors: [] };
    let httpStatus = 0;
    try {
      const nav = performance.getEntriesByType('navigation')[0];
      if (nav && typeof nav.responseStatus === 'number') httpStatus = nav.responseStatus;
    } catch {}
    let text = '';
    try { text = ((document.body && document.body.innerText) || '').slice(0, 4000); } catch {}
    return {
      href: location.href,
      title: document.title,
      readyState: document.readyState,
      httpStatus,
      text,
      console: d.console.slice(-100),
      network: d.network.slice(-100),
      errors: d.errors.slice(-50),
    };
  })();`
}

// diagOnDemand installs the collector if it is absent (the diagPreload guard makes
// re-injection idempotent) and then reads it, all in one Eval. Both helpers are
// self-invoking expressions; nesting them in a single outer IIFE gives the Eval
// one completion value (the read result), matching diagScript/replayScript. This
// is the lazy path used by peapod_diagnostics when a task was not started with
// diagnostics:true — it captures activity from the moment of the first call.
func diagOnDemand() string {
	return `(() => {
  ` + diagPreload() + `
  return ` + diagScript() + `
})();`
}

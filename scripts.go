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

  const capture = (event) => {
    if (window.__syncReplaying) return;
    const operation = captureOperation(event);
    if (operation && window.mygo && typeof window.mygo.call === 'function') {
      window.mygo.call('Sync.Capture', { pageId: PAGE_ID, operation }).catch(() => {});
    }
  };
  for (const type of TYPES) document.addEventListener(type, capture, true);
  window.addEventListener('scroll', capture, true);
})();`, pageID)
}

// resolveScript waits until the anchor matches exactly one element, returning
// {found:true} or {errorCode}.
func resolveScript(anchorJSON string) string {
	return `(async () => {
    const anchor = ` + anchorJSON + `;
    const deadline = Date.now() + 10000;
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

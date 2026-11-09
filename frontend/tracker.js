// Clickstream tracker: collects user actions on the page and sends them to tracker-api.
//
// Request body (POST /events):
//   {"session_id": "<uuid>", "events": [{"event_id", "type", "element_id", "value"?, "client_ts"}]}
//
// Delivery is at-least-once:
//   - every event gets a UUID event_id here, so the backend can find duplicates;
//   - unconfirmed events are kept in localStorage until the server answers 2xx;
//   - network errors, 5xx and 429 are retried with exponential backoff;
//   - events left unsent by a closed or reloaded page are sent by the next page load.
// A retry after a lost response produces a duplicate with the same event_id.
//
// The endpoint is relative by default: nginx serves the page and proxies /events
// to tracker-api, so the page and the API share one origin.
// To send events to another origin, set the full URL on the script tag:
//   <script src="tracker.js" data-endpoint="http://localhost:8080/events"></script>
// The server then has to allow CORS for this page.
(function () {
  'use strict';

  const DEFAULT_ENDPOINT = '/events';
  const FLUSH_INTERVAL_MS = 5000;
  const MAX_BATCH_SIZE = 100;
  const MAX_STORED_EVENTS = 1000; // older events are dropped when the backend is down for long
  const RETRY_MIN_MS = 5000;
  const RETRY_MAX_MS = 60000;
  const ORPHAN_AFTER_MS = 10 * 60 * 1000; // outbox of a tab that has not updated it for this long
  const INPUT_DEBOUNCE_MS = 500;
  const SESSION_KEY = 'clickstream_session_id';
  const OUTBOX_PREFIX = 'clickstream_outbox:';

  // Elements whose actions are tracked; each of them must have a stable id.
  const TRACKED_SELECTOR = 'button[id], a[id], input[id], select[id], textarea[id], form[id]';
  const TEXT_INPUT_TYPES = ['text', 'search', 'email', 'tel', 'url', 'password', 'number'];

  const script = document.currentScript;
  const endpoint = (script && script.dataset.endpoint) || DEFAULT_ENDPOINT;

  const sessionId = getSessionId();
  const pendingInputs = new Map(); // element -> debounce timer id

  // Outbox: events not confirmed by the server yet, oldest first, as {session_id, event}.
  // Each page load owns its own localStorage key, so tabs never overwrite each other
  // (a duplicated tab shares sessionStorage and therefore the session id).
  const store = getLocalStorage();
  const outboxKey = OUTBOX_PREFIX + newUUID();
  let outbox = [];
  let left = false;   // pagehide fired: the next page load may take this outbox over
  let sending = false; // one request at a time keeps the keepalive body quota (64 KB) safe
  let retryDelay = 0;
  let retryAt = 0;

  // --- ids ---

  function getSessionId() {
    try {
      let id = sessionStorage.getItem(SESSION_KEY);
      if (!id) {
        id = newUUID();
        sessionStorage.setItem(SESSION_KEY, id);
      }
      return id;
    } catch (e) {
      // sessionStorage is unavailable (privacy mode, blocked storage): one id per page load
      return newUUID();
    }
  }

  function newUUID() {
    if (crypto.randomUUID) {
      return crypto.randomUUID();
    }
    // randomUUID needs a secure context (https or localhost); a phone opening
    // the page by LAN IP over http does not have it. getRandomValues works there.
    const b = crypto.getRandomValues(new Uint8Array(16));
    b[6] = (b[6] & 0x0f) | 0x40; // version 4
    b[8] = (b[8] & 0x3f) | 0x80; // variant 10
    const h = Array.from(b, (x) => x.toString(16).padStart(2, '0')).join('');
    return `${h.slice(0, 8)}-${h.slice(8, 12)}-${h.slice(12, 16)}-${h.slice(16, 20)}-${h.slice(20)}`;
  }

  // --- outbox storage ---

  function getLocalStorage() {
    try {
      return window.localStorage;
    } catch (e) {
      return null; // blocked storage: the outbox lives in memory only
    }
  }

  function saveOutbox() {
    if (!store) {
      return;
    }
    try {
      if (outbox.length === 0) {
        store.removeItem(outboxKey);
      } else {
        store.setItem(outboxKey, JSON.stringify({ updated: Date.now(), left: left, items: outbox }));
      }
    } catch (e) {
      // quota exceeded: the events are still in memory and will be sent from there
    }
  }

  // Takes over outboxes of closed pages (left) and of tabs silent for ORPHAN_AFTER_MS
  // (a mobile browser may kill a hidden tab without pagehide). If such a tab is
  // still alive, both pages send the same events: a duplicate, not a loss.
  function adoptOrphans() {
    if (!store) {
      return;
    }
    try {
      const keys = [];
      for (let i = 0; i < store.length; i++) {
        const key = store.key(i);
        if (key && key.startsWith(OUTBOX_PREFIX) && key !== outboxKey) {
          keys.push(key);
        }
      }
      const now = Date.now();
      for (const key of keys) {
        let record = null;
        try {
          record = JSON.parse(store.getItem(key));
        } catch (e) {
          // broken record: removed below
        }
        if (record && !record.left && now - record.updated < ORPHAN_AFTER_MS) {
          continue; // the owner tab may still be open
        }
        store.removeItem(key);
        if (record && Array.isArray(record.items)) {
          outbox = record.items.concat(outbox);
        }
      }
    } catch (e) {
      // storage became unavailable: nothing to adopt
    }
    trimOutbox();
    saveOutbox();
  }

  function trimOutbox() {
    const extra = outbox.length - MAX_STORED_EVENTS;
    if (extra > 0) {
      outbox.splice(0, extra);
      console.warn(`clickstream: outbox is full, ${extra} oldest events dropped`);
    }
  }

  // --- elements ---

  function trackedElement(target) {
    return target instanceof Element ? target.closest(TRACKED_SELECTOR) : null;
  }

  function isTextField(el) {
    return el.tagName === 'TEXTAREA' || (el.tagName === 'INPUT' && TEXT_INPUT_TYPES.includes(el.type));
  }

  // value for slider, select, checkbox and radio; only the input length for text fields
  function valueOf(el) {
    if (isTextField(el)) {
      return String(el.value.length);
    }
    if (el.tagName === 'INPUT') {
      switch (el.type) {
        case 'checkbox':
          return String(el.checked);
        case 'radio':
        case 'range':
          return el.value;
      }
      return undefined;
    }
    if (el.tagName === 'SELECT') {
      return el.value;
    }
    return undefined;
  }

  // --- tracking ---

  function track(type, elementId, value) {
    const event = {
      event_id: newUUID(),
      type: type,
      element_id: elementId,
      client_ts: new Date().toISOString(),
    };
    if (value !== undefined) {
      event.value = value;
    }
    outbox.push({ session_id: sessionId, event: event });
    trimOutbox();
    saveOutbox();
    if (outbox.length >= MAX_BATCH_SIZE) {
      flush(false);
    }
  }

  function trackInputNow(el) {
    clearTimeout(pendingInputs.get(el));
    pendingInputs.delete(el);
    track('input', el.id, valueOf(el));
  }

  // Debounced input events must not be lost on change, blur or page leave.
  function flushPendingInputs() {
    for (const el of Array.from(pendingInputs.keys())) {
      trackInputNow(el);
    }
  }

  // --- sending ---

  // Sends the oldest batch: up to MAX_BATCH_SIZE events of one session
  // (adopted outboxes may hold events of older sessions). force skips the backoff.
  function flush(force) {
    if (sending || outbox.length === 0 || (!force && Date.now() < retryAt)) {
      return;
    }
    const sid = outbox[0].session_id;
    const events = [];
    for (const item of outbox) {
      if (item.session_id === sid) {
        events.push(item.event);
        if (events.length === MAX_BATCH_SIZE) {
          break;
        }
      }
    }
    send(sid, events);
  }

  // fetch with keepalive instead of sendBeacon: the request survives page unload
  // like a beacon, and when the page lives on, the response tells whether to retry.
  function send(sid, events) {
    sending = true;
    let request;
    try {
      request = fetch(endpoint, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ session_id: sid, events: events }),
        keepalive: true,
      });
    } catch (e) {
      request = Promise.reject(e);
    }

    request.then(function (response) {
      if (response.ok) {
        confirm(events);
        retryDelay = 0;
        retryAt = 0;
      } else if (response.status === 429 || response.status >= 500) {
        scheduleRetry();
      } else {
        // other 4xx: the server will never accept this batch, a retry does not help
        console.warn(`clickstream: batch rejected with ${response.status}, ${events.length} events dropped`);
        confirm(events);
      }
    }, scheduleRetry).finally(function () {
      sending = false;
      if (retryAt === 0 && outbox.length > 0) {
        flush(false); // more events are waiting: send the next batch right away
      }
    });
  }

  function confirm(events) {
    const done = new Set(events.map((e) => e.event_id));
    outbox = outbox.filter((item) => !done.has(item.event.event_id));
    saveOutbox();
  }

  // network error, 5xx or 429: the events stay in the outbox
  function scheduleRetry() {
    retryDelay = retryDelay === 0 ? RETRY_MIN_MS : Math.min(retryDelay * 2, RETRY_MAX_MS);
    retryAt = Date.now() + retryDelay;
  }

  // --- listeners ---

  document.addEventListener('click', function (e) {
    const el = trackedElement(e.target);
    if (el && el.tagName !== 'FORM') {
      track('click', el.id);
    }
  }, true);

  document.addEventListener('input', function (e) {
    const el = trackedElement(e.target);
    if (!el || !(isTextField(el) || el.type === 'range')) {
      return; // select, checkbox and radio are reported by change
    }
    clearTimeout(pendingInputs.get(el));
    pendingInputs.set(el, setTimeout(trackInputNow, INPUT_DEBOUNCE_MS, el));
  }, true);

  document.addEventListener('change', function (e) {
    const el = trackedElement(e.target);
    if (el) {
      if (pendingInputs.has(el)) {
        trackInputNow(el);
      }
      track('change', el.id, valueOf(el));
    }
  }, true);

  document.addEventListener('focusin', function (e) {
    const el = trackedElement(e.target);
    if (el) {
      track('focus', el.id);
    }
  });

  document.addEventListener('focusout', function (e) {
    const el = trackedElement(e.target);
    if (el) {
      if (pendingInputs.has(el)) {
        trackInputNow(el);
      }
      track('blur', el.id);
    }
  });

  document.addEventListener('submit', function (e) {
    const el = trackedElement(e.target);
    if (el) {
      flushPendingInputs();
      track('submit', el.id);
    }
  }, true);

  // Page leave: pagehide covers closing and navigation (including bfcache),
  // visibilitychange covers mobile browsers that kill a hidden tab without pagehide.
  // Whatever is not confirmed stays in localStorage for the next page load.
  window.addEventListener('pagehide', function () {
    flushPendingInputs();
    left = true;
    saveOutbox();
    flush(true);
  });

  window.addEventListener('pageshow', function (e) {
    if (e.persisted) {
      left = false; // restored from bfcache: this page owns its outbox again
      saveOutbox();
    }
  });

  document.addEventListener('visibilitychange', function () {
    if (document.visibilityState === 'hidden') {
      flushPendingInputs();
      flush(true);
    }
  });

  setInterval(function () {
    saveOutbox(); // also refreshes "updated", so other tabs do not adopt a live outbox
    flush(false);
  }, FLUSH_INTERVAL_MS);

  adoptOrphans();
  track('page_view', 'page');
})();

/* Nurtur client behaviour.
 *
 * The server is authoritative. This file only:
 *   - ticks the running clock from server timestamps,
 *   - mirrors the active timer in localStorage for fast recovery,
 *   - reconciles with /partials/feed/timer on resume,
 *   - handles the theme toggle, <dialog>, toasts, and relative labels.
 *
 * No inline handlers: the CSP allows only this file plus the tiny theme
 * bootstrap nonce in <head>.
 */
(function () {
  "use strict";

  var CACHE_KEY = "nurtur.timer.v1";
  var THEME_KEY = "nurtur.theme";

  /* ---------- HTMX configuration ---------- */

  if (window.htmx) {
    // htmx injects a <style> block for indicators by default, which a strict
    // style-src 'self' CSP correctly blocks. The rules live in app.css instead.
    htmx.config.includeIndicatorStyles = false;
    // Swap validation errors (422) into the dialog instead of treating them as
    // transport failures. The server supplies HX-Retarget for those responses.
    htmx.config.responseHandling = [
      { code: "204", swap: false },
      { code: "[23]..", swap: true },
      { code: "422", swap: true, error: false },
      { code: "[45]..", swap: false, error: true }
    ];
  }

  /* ---------- Theme ---------- */

  var THEME_MODES = ["auto", "day", "night"];

  function resolveTheme(mode) {
    if (mode === "day" || mode === "night") return mode;
    var hour = new Date().getHours();
    if (hour >= 21 || hour < 7) return "night";
    var stored = null;
    try { stored = localStorage.getItem(THEME_KEY); } catch (e) {}
    var prefersDark = window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
    if (!stored && prefersDark) return "night";
    return "day";
  }

  function applyTheme(mode, persist) {
    var theme = resolveTheme(mode);
    var root = document.documentElement;
    root.dataset.theme = theme;
    root.dataset.themeMode = mode;

    var meta = document.querySelector('meta[name="theme-color"]');
    if (meta) meta.setAttribute("content", theme === "night" ? "#08080c" : "#fff8f0");

    var index = THEME_MODES.indexOf(mode);
    var next = THEME_MODES[(index + 1) % THEME_MODES.length];
    document.querySelectorAll("[data-theme-toggle]").forEach(function (button) {
      button.setAttribute("aria-label", "Theme: " + mode + ". Activate for " + next + ".");
      button.setAttribute("title", "Theme: " + mode);
    });

    if (persist) {
      try { localStorage.setItem(THEME_KEY, mode); } catch (e) {}
    }
  }

  function currentThemeMode() {
    var mode = document.documentElement.dataset.themeMode;
    return THEME_MODES.indexOf(mode) >= 0 ? mode : "auto";
  }

  document.querySelectorAll("[data-theme-toggle]").forEach(function (button) {
    button.addEventListener("click", function () {
      var index = THEME_MODES.indexOf(currentThemeMode());
      applyTheme(THEME_MODES[(index + 1) % THEME_MODES.length], true);
    });
  });
  applyTheme(currentThemeMode(), false);
  window.setInterval(function () {
    if (currentThemeMode() === "auto") applyTheme("auto", false);
  }, 60 * 1000);

  /* ---------- Time formatting (mirrors internal/view) ---------- */

  function pad2(n) {
    return n < 10 ? "0" + n : "" + n;
  }

  function formatTimer(elapsedMs) {
    var total = Math.floor(Math.max(0, elapsedMs) / 1000);
    var hours = Math.floor(total / 3600);
    var mins = Math.floor((total % 3600) / 60);
    var secs = total % 60;
    if (hours > 0) return hours + ":" + pad2(mins) + ":" + pad2(secs);
    return pad2(mins) + ":" + pad2(secs);
  }

  function formatDurationShort(elapsedMs) {
    var minutes = Math.floor(Math.max(0, elapsedMs) / 60000);
    var hours = Math.floor(minutes / 60);
    var mins = minutes % 60;
    if (hours > 0) return hours + "h " + pad2(mins) + "m";
    return mins + "m";
  }

  function formatAgo(elapsedMs) {
    var ms = Math.max(0, elapsedMs);
    if (ms < 60000) return "just now";
    var minutes = Math.floor(ms / 60000);
    if (minutes < 60) return minutes + "m ago";
    var hours = Math.floor(minutes / 60);
    var rem = minutes % 60;
    if (hours < 24) return rem === 0 ? hours + "h ago" : hours + "h " + pad2(rem) + "m ago";
    var days = Math.floor(hours / 24);
    var remHours = hours % 24;
    return remHours === 0 ? days + "d ago" : days + "d " + remHours + "h ago";
  }

  /* ---------- Cached timer ---------- */

  var timerState = {
    key: null,
    feedId: null,
    side: null,
    startedAt: 0,
    offset: 0
  };
  var cachedTimer = loadCache();
  var isOffline = false;
  var labelOffset = serverOffsetFromBody();

  function serverOffsetFromBody() {
    var rendered = Date.parse(document.body.dataset.serverNow || "");
    return isFinite(rendered) ? Date.now() - rendered : 0;
  }

  function loadCache() {
    try {
      var raw = localStorage.getItem(CACHE_KEY);
      if (!raw) return null;
      var value = JSON.parse(raw);
      if (!value || !value.feedId || !value.startedAt) return null;
      if (!isFinite(Date.parse(value.startedAt))) return null;
      if (!isFinite(Number(value.serverOffset))) return null;
      return {
        feedId: String(value.feedId),
        side: value.side === "right" ? "right" : "left",
        startedAt: value.startedAt,
        serverOffset: Number(value.serverOffset)
      };
    } catch (e) {
      return null;
    }
  }

  function saveCache(state) {
    cachedTimer = {
      feedId: String(state.feedId),
      side: state.side === "right" ? "right" : "left",
      startedAt: new Date(state.startedAt).toISOString(),
      serverOffset: state.offset
    };
    try { localStorage.setItem(CACHE_KEY, JSON.stringify(cachedTimer)); } catch (e) {}
  }

  function clearCache() {
    cachedTimer = null;
    try { localStorage.removeItem(CACHE_KEY); } catch (e) {}
  }

  /* ---------- Server-authoritative ticking ---------- */

  var lastAnnounceAt = 0;
  var lastAnnouncedMinute = -1;
  var lastOffsetKey = null;

  function timerCard() {
    return document.getElementById("timer-card");
  }

  function syncFromCard(card) {
    var feedId = card.dataset.feedId;
    if (!feedId) return null;

    var startedAt = Date.parse(card.dataset.startedAt || "");
    if (!isFinite(startedAt)) return null;

    var serverNow = Date.parse(card.dataset.serverNow || "");
    var key = [feedId, card.dataset.side, card.dataset.startedAt, card.dataset.serverNow].join("|");

    if (key !== timerState.key) {
      timerState.key = key;
      timerState.feedId = feedId;
      timerState.side = card.dataset.side === "right" ? "right" : "left";
      timerState.startedAt = startedAt;
      timerState.offset = Date.now() - (isFinite(serverNow) ? serverNow : Date.now());
      saveCache(timerState);
      if (lastOffsetKey !== key) {
        lastAnnouncedMinute = -1;
        lastOffsetKey = key;
      }
    }
    return timerState;
  }

  function announce(elapsedMs) {
    var now = Date.now();
    if (now - lastAnnounceAt < 30000) return;

    var minutes = Math.floor(elapsedMs / 60000);
    if (minutes === lastAnnouncedMinute) return;

    var live = document.getElementById("timer-announcer");
    if (!live) return;

    lastAnnounceAt = now;
    lastAnnouncedMinute = minutes;
    var side = timerState.side === "right" ? "right" : "left";
    var spoken = minutes < 1 ? "less than a minute" : minutes + (minutes === 1 ? " minute" : " minutes");
    live.textContent = "Nursing " + side + ", " + spoken + ".";
  }

  function activeTimerElapsed() {
    return Math.max(0, Date.now() - timerState.offset - timerState.startedAt);
  }

  // Any element with data-live-since ticks using the server's clock offset.
  // The sleep pill and the sheets use this; the feed card has its own cache.
  function tickLiveClocks() {
    document.querySelectorAll("[data-live-since]").forEach(function (el) {
      var since = Date.parse(el.dataset.liveSince || "");
      if (!isFinite(since)) return;
      var digits = el.querySelector("[data-live-digits]");
      if (!digits) return;
      if (!isFinite(el.__nurturServerOffset)) {
        var serverNow = Date.parse(el.dataset.liveServerNow || "");
        el.__nurturServerOffset = isFinite(serverNow) ? Date.now() - serverNow : labelOffset;
      }
      var text = formatTimer(Date.now() - el.__nurturServerOffset - since);
      if (digits.textContent !== text) digits.textContent = text;
    });
  }

  function tick() {
    tickLiveClocks();
    var card = timerCard();
    if (!card) return;

    if (card.dataset.feedId) {
      var state = syncFromCard(card);
      if (!state) return;
      var digits = card.querySelector("[data-timer-digits]");
      if (digits) {
        var elapsed = activeTimerElapsed();
        var text = formatTimer(elapsed);
        if (digits.textContent !== text) digits.textContent = text;
        announce(elapsed);
      }
      hideCachedHint(card);
      updateOfflineHint();
      return;
    }

    showCachedHint(card);
    updateOfflineHint();
  }

  function showCachedHint(card) {
    var hint = card.querySelector("[data-cached-timer]");
    if (!hint) return;
    if (!cachedTimer) {
      hint.hidden = true;
      return;
    }
    var elapsed = Date.now() - cachedTimer.serverOffset - Date.parse(cachedTimer.startedAt);
    hint.textContent = "Nursing " + cachedTimer.side + " · " + formatTimer(elapsed) + " · checking…";
    hint.hidden = false;
  }

  function hideCachedHint(card) {
    var hint = card.querySelector("[data-cached-timer]");
    if (hint) hint.hidden = true;
  }

  function updateOfflineHint() {
    var card = timerCard();
    if (!card) return;
    var hint = card.querySelector("[data-offline-hint]");
    if (!hint) return;
    var show = isOffline && (!!card.dataset.feedId || !!cachedTimer);
    hint.hidden = !show;
  }

  /* ---------- Reconciliation with the server ---------- */

  var reconciling = false;
  var lastReconcileAt = 0;

  function reconcile(force) {
    if (reconciling || !window.htmx || !timerCard()) return;
    var now = Date.now();
    if (!force && now - lastReconcileAt < 8000) return;

    reconciling = true;
    lastReconcileAt = now;

    htmx.ajax("GET", "/partials/feed/timer", {
      target: "#timer-card",
      swap: "outerHTML"
    }).then(function () {
      reconciling = false;
      isOffline = false;
      onTimerSwap();
      updateRelativeLabels();
    }).catch(function () {
      reconciling = false;
      if (navigator.onLine === false) isOffline = true;
      updateOfflineHint();
    });
  }

  function onTimerSwap() {
    var card = timerCard();
    timerState.key = null;
    if (card && card.dataset.feedId) {
      syncFromCard(card);
      hideCachedHint(card);
    } else {
      clearCache();
    }
    updateOfflineHint();
  }

  /* ---------- Relative labels ---------- */

  function updateRelativeLabels() {
    // Use the server's clock as the reference for "ago" and active-row totals,
    // not the phone's potentially skewed clock.
    var now = Date.now() - labelOffset;

    document.querySelectorAll("[data-relative-since]").forEach(function (el) {
      var then = Date.parse(el.dataset.relativeSince || "");
      if (!isFinite(then)) return;
      var elapsed = Math.max(0, now - then);
      var text = el.dataset.relativeMode === "duration"
        ? formatDurationShort(elapsed)
        : formatAgo(elapsed);
      if (el.textContent !== text) el.textContent = text;
    });

    document.querySelectorAll("[data-elapsed-since]").forEach(function (el) {
      var then = Date.parse(el.dataset.elapsedSince || "");
      if (!isFinite(then)) return;
      var text = formatDurationShort(Math.max(0, now - then));
      if (el.textContent !== text) el.textContent = text;
    });
  }

  /* ---------- Dialog ---------- */

  function openDialog(dialog) {
    if (!dialog || !dialog.querySelector(".sheet-inner")) return;
    if (!dialog.open) {
      if (typeof dialog.showModal === "function") {
        dialog.showModal();
      } else {
        dialog.setAttribute("open", "");
      }
    }
  }

  function closeDialog(dialog) {
    if (!dialog) dialog = document.querySelector("dialog.sheet[open]");
    if (!dialog) return;
    if (dialog.open && typeof dialog.close === "function") {
      dialog.close();
    } else {
      dialog.removeAttribute("open");
    }
    dialog.innerHTML = "";
    if (!document.activeElement || document.activeElement === document.body) {
      var fallback = document.querySelector(".timer-adjust") || document.querySelector("[data-theme-toggle]");
      if (fallback) fallback.focus({ preventScroll: true });
    }
  }

  function closeOpenDialogs() {
    document.querySelectorAll("dialog.sheet[open]").forEach(function (item) {
      closeDialog(item);
    });
  }

  document.querySelectorAll("dialog.sheet").forEach(function (dialog) {
    var dragStart = null;
    dialog.addEventListener("close", function () { dialog.innerHTML = ""; });
    dialog.addEventListener("click", function (event) {
      if (event.target === dialog) closeDialog(dialog);
    });
    dialog.addEventListener("touchstart", function (event) {
      if (event.target.closest && event.target.closest("[data-sheet-drag]")) {
        dragStart = event.touches[0].clientY;
      }
    }, { passive: true });
    dialog.addEventListener("touchmove", function (event) {
      if (dragStart === null) return;
      if (event.touches[0].clientY - dragStart > 64) {
        dragStart = null;
        closeDialog(dialog);
      }
    }, { passive: true });
    dialog.addEventListener("touchend", function () { dragStart = null; }, { passive: true });

    // A no-JS sheet ships with the open attribute. Upgrade it to a real modal
    // and keep focus trapped.
    if (dialog.hasAttribute("open") && dialog.querySelector(".sheet-inner")) {
      dialog.removeAttribute("open");
      openDialog(dialog);
    }
  });

  document.addEventListener("click", function (event) {
    var trigger = event.target.closest ? event.target.closest("[data-close-dialog]") : null;
    if (trigger) {
      event.preventDefault();
      closeDialog(trigger.closest("dialog.sheet"));
    }
  });

  document.body.addEventListener("feed-saved", closeOpenDialogs);
  document.body.addEventListener("sheet-saved", closeOpenDialogs);
  document.body.addEventListener("timer-cleared", function () {
    timerState.key = null;
    timerState.feedId = null;
    timerState.side = null;
    clearCache();
    updateOfflineHint();
  });

  /* ---------- Toasts ---------- */

  var toastTimer = null;

  function armToast() {
    var toast = document.querySelector("[data-toast]");
    if (!toast) return;
    if (toastTimer) window.clearTimeout(toastTimer);
    toast.classList.remove("is-leaving");

    var duration = parseInt(toast.getAttribute("data-toast-duration") || "5000", 10);
    if (!isFinite(duration) || duration <= 0) duration = 5000;

    var dismiss = function () {
      toast.classList.add("is-leaving");
      window.setTimeout(function () {
        if (toast.parentNode) toast.parentNode.removeChild(toast);
      }, 250);
    };
    var schedule = function () {
      if (toastTimer) window.clearTimeout(toastTimer);
      toastTimer = window.setTimeout(dismiss, duration);
    };
    var pause = function () {
      if (toastTimer) {
        window.clearTimeout(toastTimer);
        toastTimer = null;
      }
    };

    if (toast.dataset.armed !== "1") {
      toast.dataset.armed = "1";
      toast.addEventListener("mouseenter", pause);
      toast.addEventListener("mouseleave", function () {
        if (toast.parentNode) schedule();
      });
      toast.addEventListener("focusin", pause);
      toast.addEventListener("focusout", function () {
        if (toast.parentNode) schedule();
      });
    }
    schedule();
  }

  var ALERT_ICON = '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.25" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"><circle cx="12" cy="12" r="8.5"/><path d="M12 7.5v5"/><circle cx="12" cy="16.4" r=".6" fill="currentColor" stroke="none"/></svg>';

  function localToast(message, kind) {
    var region = document.getElementById("toast-region");
    if (!region) return;
    region.innerHTML = "";
    var toast = document.createElement("div");
    toast.className = "toast toast-" + (kind || "error");
    toast.setAttribute("data-toast", "");
    toast.innerHTML = '<span class="toast-icon" aria-hidden="true">' + ALERT_ICON + "</span>";
    var text = document.createElement("p");
    text.className = "toast-text";
    text.textContent = message;
    toast.appendChild(text);
    region.appendChild(toast);
    armToast();
  }

  /* ---------- HTMX lifecycle ---------- */

  function handleSwap(target) {
    if (!target) return;

    if (target.tagName === "DIALOG" && target.classList.contains("sheet")) {
      document.querySelectorAll("dialog.sheet[open]").forEach(function (other) {
        if (other !== target) closeDialog(other);
      });
      openDialog(target);
    }
    if (target.id === "timer-card") {
      onTimerSwap();
    }
    if (target.id === "toast-region") {
      armToast();
    }
    updateRelativeLabels();
    tickLiveClocks();
  }

  document.body.addEventListener("htmx:afterSwap", function (event) {
    handleSwap(event.detail ? event.detail.target : null);
  });

  // Out-of-band fragments (the toast, status strip, timeline, sleep pill) are
  // swapped with htmx:oobAfterSwap, not htmx:afterSwap. The toast must be armed
  // here or it would stay on screen forever.
  document.body.addEventListener("htmx:oobAfterSwap", function (event) {
    handleSwap(event.detail ? event.detail.target : null);
  });

  document.body.addEventListener("htmx:responseError", function (event) {
    var status = event.detail && event.detail.xhr ? event.detail.xhr.status : 0;
    if (status === 422) return;
    localToast("Couldn't save that. Check the connection and try again.", "error");
  });

  document.body.addEventListener("htmx:sendError", function () {
    isOffline = true;
    updateOfflineHint();
    localToast("Can't reach the server. Your timer is still running.", "warn");
  });

  document.body.addEventListener("htmx:afterRequest", function (event) {
    var detail = event.detail || {};
    if (!detail.successful || !detail.requestConfig) return;
    if (String(detail.requestConfig.verb).toLowerCase() === "get") return;
    try {
      if (navigator.vibrate) navigator.vibrate(8);
    } catch (e) {}
  });

  /* ---------- Resume and connectivity ---------- */

  document.addEventListener("visibilitychange", function () {
    if (document.visibilityState === "visible") {
      tick();
      updateRelativeLabels();
      reconcile(false);
    }
  });

  window.addEventListener("focus", function () {
    tick();
    reconcile(false);
  });

  window.addEventListener("pageshow", function (event) {
    tick();
    updateRelativeLabels();
    reconcile(!!event.persisted || !!cachedTimer);
  });

  window.addEventListener("online", function () {
    isOffline = false;
    updateOfflineHint();
    updateConnectivity();
    reconcile(true);
  });

  window.addEventListener("offline", function () {
    isOffline = true;
    updateOfflineHint();
    updateConnectivity();
  });

  function updateConnectivity() {
    var banner = document.getElementById("offline-banner");
    if (banner) banner.hidden = navigator.onLine !== false;
  }

  if ("serviceWorker" in navigator) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register("/sw.js").catch(function () {});
    });
  }

  /* ---------- Boot ---------- */

  var card = timerCard();
  if (card && card.dataset.feedId) {
    syncFromCard(card);
  } else if (cachedTimer) {
    // Paint the cached session immediately, then let the server win.
    reconcile(true);
  }

  updateRelativeLabels();
  armToast();
  updateConnectivity();
  tick();
  window.setInterval(tick, 250);
  window.setInterval(updateRelativeLabels, 30000);
})();

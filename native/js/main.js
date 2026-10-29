/* Nurtur Android promo — vanilla JS, no build step.
   The timer is a real state machine; the sheet is dragged with Pointer Events,
   projects momentum, hands its velocity to a spring, and can be interrupted. */

(function () {
  "use strict";

  // Set this to a real form endpoint (Formspree, Buttondown, your API) before
  // launch. When empty, the demo stores the address locally and shows success.
  var WAITLIST_ENDPOINT = window.NURTUR_WAITLIST_ENDPOINT || "";

  var root = document.documentElement;
  var reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

  function prefersReduced() {
    return reduceMotion.matches;
  }

  /* --------------------------------------------------------------------- *
   * Theme
   * --------------------------------------------------------------------- */

  var themeMeta = document.getElementById("theme-color");
  var themeButtons = document.querySelectorAll("[data-theme-toggle]");

  function updateThemeUI() {
    var night = root.dataset.theme === "night";
    themeButtons.forEach(function (button) {
      button.setAttribute("aria-pressed", String(night));
      button.setAttribute("aria-label", night ? "Switch to day mode" : "Switch to night mode");
    });
    if (themeMeta) {
      themeMeta.setAttribute("content", night ? "#1a1621" : "#fff8f0");
    }
  }

  function applyTheme(theme) {
    root.dataset.theme = theme;
    try {
      localStorage.setItem("nurtur.promo.theme", theme);
    } catch (e) {}
    updateThemeUI();
  }

  function setTheme(theme, origin) {
    if (prefersReduced() || !document.startViewTransition || !origin) {
      applyTheme(theme);
      return;
    }

    var rect = origin.getBoundingClientRect();
    var x = rect.left + rect.width / 2;
    var y = rect.top + rect.height / 2;
    var radius = Math.hypot(
      Math.max(x, window.innerWidth - x),
      Math.max(y, window.innerHeight - y)
    );

    try {
      var transition = document.startViewTransition(function () {
        applyTheme(theme);
      });
      transition.ready
        .then(function () {
          root.animate(
            {
              clipPath: [
                "circle(0px at " + x + "px " + y + "px)",
                "circle(" + radius + "px at " + x + "px " + y + "px)"
              ]
            },
            {
              duration: 520,
              easing: "cubic-bezier(.2,.8,.2,1)",
              pseudoElement: "::view-transition-new(root)"
            }
          );
        })
        .catch(function () {});
    } catch (e) {
      applyTheme(theme);
    }
  }

  themeButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      var next = root.dataset.theme === "night" ? "day" : "night";
      setTheme(next, button);
    });
  });

  updateThemeUI();

  /* --------------------------------------------------------------------- *
   * Header shadow
   * --------------------------------------------------------------------- */

  var header = document.getElementById("site-header");
  var scrollQueued = false;

  function updateHeader() {
    scrollQueued = false;
    if (header) header.classList.toggle("is-scrolled", window.scrollY > 8);
  }

  window.addEventListener(
    "scroll",
    function () {
      if (scrollQueued) return;
      scrollQueued = true;
      requestAnimationFrame(updateHeader);
    },
    { passive: true }
  );
  updateHeader();

  /* --------------------------------------------------------------------- *
   * Reveal on scroll
   * --------------------------------------------------------------------- */

  var revealItems = document.querySelectorAll("[data-reveal]");
  if ("IntersectionObserver" in window && !prefersReduced()) {
    revealItems.forEach(function (element, index) {
      element.style.setProperty("--reveal-i", String(index % 5));
    });
    var observer = new IntersectionObserver(
      function (entries) {
        entries.forEach(function (entry) {
          if (!entry.isIntersecting) return;
          entry.target.classList.add("is-visible");
          observer.unobserve(entry.target);
        });
      },
      { threshold: 0.12, rootMargin: "0px 0px -6% 0px" }
    );
    revealItems.forEach(function (element) {
      observer.observe(element);
    });
  } else {
    revealItems.forEach(function (element) {
      element.classList.add("is-visible");
    });
  }

  /* --------------------------------------------------------------------- *
   * Hero pointer glow
   * --------------------------------------------------------------------- */

  var hero = document.querySelector(".hero");
  var finePointer = window.matchMedia("(hover: hover) and (pointer: fine)");
  var glowQueued = false;

  if (hero && finePointer.matches) {
    hero.addEventListener(
      "pointermove",
      function (event) {
        if (glowQueued) return;
        glowQueued = true;
        requestAnimationFrame(function () {
          glowQueued = false;
          var rect = hero.getBoundingClientRect();
          var x = ((event.clientX - rect.left) / rect.width) * 100;
          var y = ((event.clientY - rect.top) / rect.height) * 100;
          hero.style.setProperty("--mx", x + "%");
          hero.style.setProperty("--my", y + "%");
        });
      },
      { passive: true }
    );
  }

  /* --------------------------------------------------------------------- *
   * Timer demo
   * --------------------------------------------------------------------- */

  var timerDisplay = document.querySelector("[data-timer-display]");
  var timerCard = document.querySelector("[data-timer-card]");
  var timerState = document.querySelector("[data-timer-state]");
  var timerSide = document.querySelector("[data-timer-side]");
  var sideButtons = document.querySelectorAll("[data-side]");
  var pauseButton = document.querySelector('[data-action="pause"]');
  var pauseLabel = document.querySelector("[data-pause-label]");
  var pauseIconUse = pauseButton ? pauseButton.querySelector("use") : null;
  var doneButton = document.querySelector('[data-action="done"]');
  var lastSideEl = document.querySelector("[data-last-side]");
  var lastFeedEl = document.querySelector("[data-last-feed]");
  var lastDiaperEl = document.querySelector("[data-last-diaper]");
  var lastDiaperTimeEl = document.querySelector("[data-last-diaper-time]");
  var todayList = document.querySelector("[data-today-list]");
  var todayCount = document.querySelector("[data-today-count]");
  var toastEl = document.querySelector("[data-phone-toast]");
  var diaperButtons = document.querySelectorAll("[data-diaper]");

  var timer = {
    status: "idle", // idle | running | paused
    side: "left",
    startedAt: 0,
    accumulated: 0,
    lastFeedAt: 0,
    frame: 0,
    toastTimer: 0
  };

  function now() {
    return performance.now();
  }

  function elapsed() {
    if (timer.status === "running") {
      return timer.accumulated + (now() - timer.startedAt);
    }
    return timer.accumulated;
  }

  function formatTimer(ms) {
    var total = Math.max(0, Math.floor(ms / 1000));
    var minutes = Math.floor(total / 60);
    var seconds = total % 60;
    if (minutes >= 60) {
      var hours = Math.floor(minutes / 60);
      var rest = minutes % 60;
      return hours + ":" + String(rest).padStart(2, "0") + ":" + String(seconds).padStart(2, "0");
    }
    return String(minutes).padStart(2, "0") + ":" + String(seconds).padStart(2, "0");
  }

  function humanDuration(ms) {
    var minutes = Math.max(0, Math.round(ms / 60000));
    if (minutes < 1) return "under a minute";
    if (minutes === 1) return "1 min";
    if (minutes < 60) return minutes + " min";
    var hours = Math.floor(minutes / 60);
    var rest = minutes % 60;
    return rest ? hours + "h " + rest + "m" : hours + "h";
  }

  function relativeTime(timestamp) {
    if (!timestamp) return "";
    var seconds = Math.max(0, Math.floor((Date.now() - timestamp) / 1000));
    if (seconds < 60) return "now";
    var minutes = Math.floor(seconds / 60);
    if (minutes < 60) return minutes + "m ago";
    var hours = Math.floor(minutes / 60);
    return hours + "h ago";
  }

  function showToast(message) {
    if (!toastEl) return;
    toastEl.textContent = message;
    toastEl.classList.add("is-visible");
    window.clearTimeout(timer.toastTimer);
    timer.toastTimer = window.setTimeout(function () {
      toastEl.classList.remove("is-visible");
    }, 2200);
  }

  function renderTimer() {
    if (!timerDisplay) return;

    var running = timer.status === "running";
    var paused = timer.status === "paused";
    var active = running || paused;

    timerDisplay.textContent = formatTimer(elapsed());

    if (timerCard) timerCard.classList.toggle("is-running", running);
    if (timerState) {
      timerState.textContent = running ? "Running" : paused ? "Paused" : "Ready";
    }

    if (timerSide) {
      if (running) {
        timerSide.textContent =
          (timer.side === "left" ? "Left" : "Right") + " side · tap the other to switch";
      } else if (paused) {
        timerSide.textContent =
          (timer.side === "left" ? "Left" : "Right") + " side · paused";
      } else {
        timerSide.textContent = "Tap a side to start";
      }
    }

    sideButtons.forEach(function (button) {
      var isActive = active && button.dataset.side === timer.side;
      button.classList.toggle("is-active", isActive);
      button.setAttribute("aria-pressed", String(isActive));
    });

    if (pauseButton) pauseButton.hidden = !active;
    if (doneButton) doneButton.hidden = !active;

    if (pauseButton && pauseLabel) {
      pauseLabel.textContent = paused ? "Resume" : "Pause";
      pauseButton.setAttribute("aria-label", paused ? "Resume nursing timer" : "Pause nursing timer");
    }
    if (pauseIconUse) {
      pauseIconUse.setAttribute("href", paused ? "#i-play" : "#i-pause");
    }
  }

  function tick() {
    if (timer.status !== "running") return;
    timerDisplay.textContent = formatTimer(elapsed());
    timer.frame = requestAnimationFrame(tick);
  }

  function startTicking() {
    cancelAnimationFrame(timer.frame);
    timer.frame = requestAnimationFrame(tick);
  }

  function startOrSwitch(side) {
    if (timer.status === "running") {
      if (side === timer.side) return;
      timer.accumulated += now() - timer.startedAt;
      timer.side = side;
      timer.startedAt = now();
      showToast("Switched to the " + side + " side");
    } else if (timer.status === "paused") {
      timer.side = side;
      showToast("Ready on the " + side + " side");
    } else {
      timer.status = "running";
      timer.side = side;
      timer.startedAt = now();
      timer.accumulated = 0;
      showToast("Nursing started · " + side);
      startTicking();
    }
    renderTimer();
  }

  function togglePause() {
    if (timer.status === "running") {
      timer.accumulated += now() - timer.startedAt;
      timer.status = "paused";
      cancelAnimationFrame(timer.frame);
      showToast("Paused · " + humanDuration(timer.accumulated) + " saved");
    } else if (timer.status === "paused") {
      timer.status = "running";
      timer.startedAt = now();
      startTicking();
      showToast("Resumed · " + (timer.side === "left" ? "left" : "right") + " side");
    }
    renderTimer();
  }

  function addTodayRow(title, detail, kind) {
    if (!todayList) return;
    var item = document.createElement("li");
    var dot = document.createElement("span");
    dot.className = "dot dot-" + kind;
    dot.setAttribute("aria-hidden", "true");

    var copy = document.createElement("div");
    var strong = document.createElement("strong");
    strong.textContent = title;
    var small = document.createElement("small");
    small.textContent = detail;
    copy.appendChild(strong);
    copy.appendChild(small);

    item.appendChild(dot);
    item.appendChild(copy);
    todayList.prepend(item);

    while (todayList.children.length > 3) {
      todayList.removeChild(todayList.lastElementChild);
    }

    var count = todayList.children.length;
    if (todayCount) {
      todayCount.textContent = count + (count === 1 ? " entry" : " entries");
    }
  }

  function finishFeed() {
    if (timer.status === "idle") return;

    var total = elapsed();
    var side = timer.side;
    timer.status = "idle";
    timer.accumulated = 0;
    timer.startedAt = 0;
    cancelAnimationFrame(timer.frame);

    addTodayRow(
      "Nursed · " + (side === "left" ? "Left" : "Right"),
      humanDuration(total) + " · just now",
      "feed"
    );

    if (lastSideEl) lastSideEl.textContent = side === "left" ? "Left" : "Right";
    timer.lastFeedAt = Date.now();
    if (lastFeedEl) lastFeedEl.textContent = "now";

    showToast("Feed saved locally · no network needed");
    renderTimer();
  }

  sideButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      startOrSwitch(button.dataset.side);
    });
  });

  if (pauseButton) pauseButton.addEventListener("click", togglePause);
  if (doneButton) doneButton.addEventListener("click", finishFeed);

  var diaperNames = {
    wet: "Wet diaper",
    dirty: "Dirty diaper",
    both: "Wet + dirty diaper"
  };

  var diaperShort = {
    wet: "Wet",
    dirty: "Dirty",
    both: "Both"
  };

  diaperButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      var kind = button.dataset.diaper;
      addTodayRow(diaperNames[kind], "just now", "diaper");
      if (lastDiaperEl) lastDiaperEl.textContent = diaperShort[kind];
      if (lastDiaperTimeEl) lastDiaperTimeEl.textContent = "now";
      showToast(diaperNames[kind] + " saved locally");
    });
  });

  // Keep the relative labels honest without a per-second timer.
  window.setInterval(function () {
    if (lastFeedEl && timer.lastFeedAt) {
      lastFeedEl.textContent = relativeTime(timer.lastFeedAt);
    }
  }, 30000);

  renderTimer();

  /* --------------------------------------------------------------------- *
   * Waitlist sheet
   * --------------------------------------------------------------------- */

  var sheet = document.getElementById("beta-sheet");
  var scrim = document.querySelector(".scrim");
  var page = document.getElementById("page");
  var openButtons = document.querySelectorAll("[data-open-beta]");
  var closeButtons = document.querySelectorAll("[data-sheet-close]");
  var grab = document.querySelector("[data-sheet-grab]");
  var form = document.querySelector("[data-waitlist]");
  var emailInput = document.getElementById("beta-email");
  var formMessage = document.querySelector("[data-form-msg]");
  var success = document.querySelector("[data-sheet-success]");

  var sheetOpen = false;
  var lastFocused = null;
  var currentY = 0;
  var sheetHeight = 0;
  var sheetSpringCancel = null;
  var drag = null;

  function springTo(from, to, velocity, onUpdate, onComplete, config) {
    config = config || {};
    var stiffness = config.stiffness || 190;
    var damping = config.damping || 24;
    var mass = config.mass || 1;

    if (sheetSpringCancel) sheetSpringCancel();

    var x = from;
    var v = velocity || 0;
    var last = performance.now();
    var raf = 0;

    function frame(time) {
      var dt = Math.min((time - last) / 1000, 0.032);
      last = time;

      // Fixed substeps keep the spring stable on slow frames.
      var steps = 4;
      var h = dt / steps;
      for (var i = 0; i < steps; i++) {
        var acceleration = (-stiffness * (x - to) - damping * v) / mass;
        v += acceleration * h;
        x += v * h;
      }

      onUpdate(x);

      // Snap the last few pixels instead of waiting for an exponential tail.
      // A sheet that takes 1.8s to dismiss feels broken even when the visible
      // motion finished half a second ago.
      if (Math.abs(v) < 24 && Math.abs(x - to) < 3) {
        onUpdate(to);
        if (onComplete) onComplete();
        return;
      }
      raf = requestAnimationFrame(frame);
    }

    raf = requestAnimationFrame(frame);
    sheetSpringCancel = function () {
      cancelAnimationFrame(raf);
    };
  }

  function setSheetY(y) {
    currentY = Math.max(0, y);
    if (sheet) sheet.style.setProperty("--sheet-y", currentY + "px");
    if (scrim) {
      var progress = sheetHeight ? 1 - Math.min(1, currentY / sheetHeight) : 0;
      scrim.style.opacity = String(progress);
    }
  }

  function focusableInSheet() {
    if (!sheet) return [];
    return Array.prototype.slice
      .call(
        sheet.querySelectorAll(
          'a[href], button:not([disabled]), input:not([disabled]), select, textarea, [tabindex]:not([tabindex="-1"])'
        )
      )
      .filter(function (element) {
        return element.offsetParent !== null && !element.hidden;
      });
  }

  function openSheet() {
    if (!sheet || sheetOpen) return;
    sheetOpen = true;
    lastFocused = document.activeElement;

    sheet.hidden = false;
    sheet.classList.remove("is-closing");
    if (scrim) {
      scrim.hidden = false;
      scrim.classList.remove("is-closing");
    }
    sheet.setAttribute("aria-hidden", "false");
    if (page) page.inert = true;
    document.body.classList.add("is-locked");

    // Force layout so the sheet measures and starts below the viewport.
    sheetHeight = sheet.offsetHeight;
    setSheetY(sheetHeight);
    if (scrim) {
      scrim.classList.add("is-visible");
      scrim.style.opacity = "0";
    }

    if (prefersReduced()) {
      setSheetY(0);
      if (scrim) scrim.style.opacity = "1";
      focusSheetStart();
      return;
    }

    // Focus immediately; the sheet is already interactive while it settles.
    window.setTimeout(function () {
      if (sheetOpen) focusSheetStart();
    }, 60);

    springTo(
      sheetHeight,
      0,
      0,
      setSheetY,
      null,
      { stiffness: 320, damping: 34 }
    );
  }

  function successHidden() {
    return !success || success.hidden;
  }

  function focusSheetStart() {
    var target = null;
    if (successHidden()) {
      target = emailInput;
    } else {
      target = success.querySelector("button");
    }
    if (target) target.focus({ preventScroll: true });
  }

  function finalizeClose() {
    if (!sheet) return;
    sheetOpen = false;
    sheet.hidden = true;
    sheet.classList.remove("is-closing");
    sheet.setAttribute("aria-hidden", "true");
    if (scrim) {
      scrim.hidden = true;
      scrim.classList.remove("is-visible", "is-closing");
      scrim.style.opacity = "";
    }
    if (page) page.inert = false;
    document.body.classList.remove("is-locked");
  }

  // The sheet visually leaves in a few hundred milliseconds; releasing the
  // page and returning focus should not wait for the spring's last invisible
  // millimetre.
  function beginCloseUI() {
    if (sheet) {
      sheet.classList.add("is-closing");
      sheet.setAttribute("aria-hidden", "true");
    }
    if (scrim) scrim.classList.add("is-closing");
    if (page) page.inert = false;
    document.body.classList.remove("is-locked");
    if (lastFocused && typeof lastFocused.focus === "function") {
      lastFocused.focus({ preventScroll: true });
    }
    lastFocused = null;
  }

  function closeSheet(velocity) {
    if (!sheet || !sheetOpen) return;
    sheetOpen = false;
    beginCloseUI();
    if (sheetSpringCancel) sheetSpringCancel();

    if (prefersReduced()) {
      setSheetY(sheetHeight);
      finalizeClose();
      return;
    }

    springTo(
      currentY,
      sheetHeight,
      velocity || 0,
      setSheetY,
      finalizeClose,
      { stiffness: 360, damping: 40 }
    );
  }

  function rubberband(overshoot, dimension, constant) {
    constant = constant || 0.55;
    return (overshoot * dimension * constant) / (dimension + constant * Math.abs(overshoot));
  }

  function project(velocity, deceleration) {
    deceleration = deceleration || 0.998;
    return ((velocity / 1000) * deceleration) / (1 - deceleration);
  }

  openButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      openSheet();
    });
  });

  closeButtons.forEach(function (button) {
    button.addEventListener("click", function () {
      closeSheet(0);
    });
  });

  if (grab) {
    grab.addEventListener("pointerdown", function (event) {
      if (event.button !== 0) return;
      if (sheetSpringCancel) sheetSpringCancel();
      drag = {
        startY: event.clientY,
        startOffset: currentY,
        lastY: event.clientY,
        lastTime: performance.now(),
        velocity: 0
      };
      grab.setPointerCapture(event.pointerId);
      sheet.classList.add("is-dragging");
      event.preventDefault();
    });

    grab.addEventListener("pointermove", function (event) {
      if (!drag) return;

      var time = performance.now();
      var dt = time - drag.lastTime;
      if (dt > 0) {
        drag.velocity = ((event.clientY - drag.lastY) / dt) * 1000;
      }
      drag.lastY = event.clientY;
      drag.lastTime = time;

      var delta = event.clientY - drag.startY;
      var next;
      if (delta >= 0) {
        next = drag.startOffset + delta;
      } else {
        next = drag.startOffset + rubberband(delta, 520);
      }
      setSheetY(next);
    });

    function endDrag() {
      if (!drag) return;
      var velocity = drag.velocity;
      drag = null;
      if (sheet) sheet.classList.remove("is-dragging");

      // A small upward tug at the top should settle, not bounce.
      if (currentY <= 0.5 && velocity < 0) velocity = 0;

      var projected = currentY + project(velocity);
      var shouldClose = projected > sheetHeight * 0.32 || velocity > 700;

      if (shouldClose) {
        closeSheet(velocity);
      } else {
        springTo(
          currentY,
          0,
          velocity,
          setSheetY,
          null,
          { stiffness: 340, damping: 34 }
        );
      }
    }

    grab.addEventListener("pointerup", endDrag);
    grab.addEventListener("pointercancel", endDrag);
  }

  if (scrim) {
    scrim.addEventListener("click", function () {
      closeSheet(0);
    });
  }

  document.addEventListener("keydown", function (event) {
    if (!sheetOpen) return;

    if (event.key === "Escape") {
      event.preventDefault();
      closeSheet(0);
      return;
    }

    if (event.key !== "Tab") return;

    var focusables = focusableInSheet();
    if (!focusables.length) return;

    var first = focusables[0];
    var last = focusables[focusables.length - 1];

    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  });

  function validEmail(value) {
    return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
  }

  function rememberLocally(email) {
    try {
      var key = "nurtur.promo.beta";
      var existing = JSON.parse(localStorage.getItem(key) || "[]");
      if (!Array.isArray(existing)) existing = [];
      if (existing.indexOf(email) === -1) existing.push(email);
      localStorage.setItem(key, JSON.stringify(existing));
    } catch (e) {}
  }

  function showSuccess() {
    if (form) form.hidden = true;
    if (formMessage) formMessage.textContent = "";
    if (success) {
      success.hidden = false;
      var close = success.querySelector("button");
      if (close) close.focus({ preventScroll: true });
    }
  }

  if (form) {
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      if (!emailInput) return;

      var value = emailInput.value.trim();
      if (!validEmail(value)) {
        if (formMessage) formMessage.textContent = "That email doesn't look right. Try again?";
        emailInput.focus();
        return;
      }

      if (formMessage) formMessage.textContent = "";
      var submit = form.querySelector('button[type="submit"]');
      if (submit) {
        submit.disabled = true;
        submit.textContent = "Joining…";
      }

      function done() {
        rememberLocally(value);
        showSuccess();
      }

      if (!WAITLIST_ENDPOINT) {
        window.setTimeout(done, 420);
        return;
      }

      fetch(WAITLIST_ENDPOINT, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email: value, source: "android-promo" })
      })
        .then(function (response) {
          if (!response.ok) throw new Error("Request failed");
          done();
        })
        .catch(function () {
          if (submit) {
            submit.disabled = false;
            submit.textContent = "Join the beta";
          }
          if (formMessage) {
            formMessage.textContent =
              "Something went wrong. Try again, or say hello on GitHub.";
          }
        });
    });
  }

  // Leave a little breathing room for the theme transition on slower devices.
  window.addEventListener("pageshow", updateThemeUI);
})();

(function () {
  var deferredPrompt = null;

  function isStandalone() {
    return (window.matchMedia && window.matchMedia("(display-mode: standalone)").matches) ||
      window.navigator.standalone === true;
  }

  function isIOS() {
    var ua = navigator.userAgent || "";
    return /iPad|iPhone|iPod/.test(ua) ||
      (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
  }

  function isFirefox() {
    return /Firefox|FxiOS/i.test(navigator.userAgent || "");
  }

  function syncInstallUI() {
    var show = !isStandalone() && (!!deferredPrompt || isIOS() || isFirefox());
    document.querySelectorAll("[data-pwa-install]").forEach(function (el) {
      el.hidden = !show;
    });
  }

  function openGuide(kind) {
    var dialog = document.getElementById("pwa-ios-dialog");
    if (!dialog || typeof dialog.showModal !== "function") return;
    var title = dialog.querySelector("[data-pwa-guide-title]");
    var copy = dialog.querySelector("[data-pwa-guide-copy]");
    var iosSteps = dialog.querySelector('[data-pwa-guide-steps="ios"]');
    var ffSteps = dialog.querySelector('[data-pwa-guide-steps="firefox"]');
    if (kind === "firefox") {
      if (title) title.textContent = "Install in Firefox";
      if (copy) copy.textContent = "Firefox doesn't allow one-tap install buttons, so it takes two quick taps:";
      if (iosSteps) iosSteps.hidden = true;
      if (ffSteps) ffSteps.hidden = false;
    } else {
      if (title) title.textContent = "Install on iPhone or iPad";
      if (copy) copy.textContent = "Safari doesn't allow one-tap install buttons, so it takes two quick taps:";
      if (iosSteps) iosSteps.hidden = false;
      if (ffSteps) ffSteps.hidden = true;
    }
    dialog.showModal();
  }

  window.addEventListener("beforeinstallprompt", function (event) {
    event.preventDefault();
    deferredPrompt = event;
    syncInstallUI();
  });

  window.addEventListener("appinstalled", function () {
    deferredPrompt = null;
    syncInstallUI();
  });

  document.addEventListener("click", function (event) {
    var closeBtn = event.target.closest ? event.target.closest("[data-pwa-ios-close]") : null;
    if (closeBtn) {
      var openDialog = document.getElementById("pwa-ios-dialog");
      if (openDialog && openDialog.open) openDialog.close();
      return;
    }

    var installBtn = event.target.closest ? event.target.closest("[data-pwa-install-btn]") : null;
    if (!installBtn) return;

    if (deferredPrompt) {
      var promptEvent = deferredPrompt;
      deferredPrompt = null;
      promptEvent.prompt();
      if (promptEvent.userChoice && typeof promptEvent.userChoice.then === "function") {
        promptEvent.userChoice.then(function (choice) {
          if (choice && choice.outcome === "accepted") {
            syncInstallUI();
          }
        }).catch(function () {});
      }
      return;
    }

    if (isIOS()) {
      openGuide("ios");
    } else if (isFirefox()) {
      openGuide("firefox");
    }
  });

  document.addEventListener("DOMContentLoaded", syncInstallUI);
  document.addEventListener("htmx:afterSettle", syncInstallUI);

  if ("serviceWorker" in navigator) {
    window.addEventListener("load", function () {
      navigator.serviceWorker.register("/sw.js").catch(function () {});
    });
  }
})();

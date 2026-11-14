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

  function syncInstallUI() {
    var show = !isStandalone() && (!!deferredPrompt || isIOS());
    document.querySelectorAll("[data-pwa-install]").forEach(function (el) {
      el.hidden = !show;
    });
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
      var iosDialog = document.getElementById("pwa-ios-dialog");
      if (iosDialog && typeof iosDialog.showModal === "function") {
        iosDialog.showModal();
      }
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

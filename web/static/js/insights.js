/* Nurtur insights: tap-to-inspect chart tooltips and copyable checkup summary. */
(function () {
  "use strict";

  function hideAllTooltips(except) {
    document.querySelectorAll("[data-insights-tooltip]").forEach(function (tip) {
      if (tip !== except) tip.hidden = true;
    });
  }

  function showTooltip(target) {
    var wrap = target.closest("[data-insights-chart]");
    if (!wrap) return;
    var tip = wrap.querySelector("[data-insights-tooltip]");
    if (!tip) return;
    var title = target.getAttribute("data-tip-title") || "";
    var value = target.getAttribute("data-tip-value") || "";
    var sub = target.getAttribute("data-tip-sub") || "";
    if (!title && !value) return;

    hideAllTooltips(tip);
    var text = title;
    if (value) text += (text ? " · " : "") + value;
    if (sub) text += " (" + sub + ")";
    tip.textContent = text;
    tip.hidden = false;
  }

  document.addEventListener("click", function (event) {
    var copyBtn = event.target.closest ? event.target.closest("[data-copy-summary]") : null;
    if (copyBtn) {
      event.preventDefault();
      var text = copyBtn.getAttribute("data-copy-text") || "";
      if (!text) return;
      var finish = function () {
        var span = copyBtn.querySelector("span");
        if (span) {
          var prev = span.textContent;
          span.textContent = "Copied";
          window.setTimeout(function () {
            if (span.isConnected) span.textContent = prev;
          }, 1800);
        }
      };
      if (navigator.clipboard && typeof navigator.clipboard.writeText === "function") {
        navigator.clipboard.writeText(text).then(finish).catch(function () {});
      }
      return;
    }

    var target = event.target.closest ? event.target.closest("[data-tip-title]") : null;
    if (target) {
      showTooltip(target);
      return;
    }
    hideAllTooltips(null);
  });

  document.addEventListener("focusin", function (event) {
    var target = event.target && event.target.matches && event.target.matches("[data-tip-title]")
      ? event.target
      : null;
    if (target) showTooltip(target);
  });
})();

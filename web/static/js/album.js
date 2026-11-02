/*
 * Nurtur album capture.
 *
 * Photos are decoded, oriented, resized, and re-encoded in the browser before
 * they leave the phone. The server sees a WebP display copy and a WebP
 * thumbnail; if this browser cannot encode WebP, both are JPEG instead. No
 * camera original, GPS data, or third-party library is uploaded or fetched.
 */
(function () {
  "use strict";

  var FULL_EDGE = 2560;
  var THUMB_EDGE = 480;
  var FULL_QUALITY = 0.82;
  var THUMB_QUALITY = 0.78;
  var MAX_ACTIVE = 2;
  var MAX_PENDING = 60;
  var TEN_YEARS_MS = 10 * 365 * 24 * 60 * 60 * 1000;
  var DAY_MS = 24 * 60 * 60 * 1000;

  var active = 0;
  var queue = [];
  var viewerItems = [];
  var viewerIndex = -1;
  var viewerReturnFocus = null;

  function find(id) {
    return document.getElementById(id);
  }

  function isAlbumPage() {
    return !!find("album-days");
  }

  /* ---------- Chooser ---------- */

  function openChooser() {
    var dialog = find("album-chooser");
    if (!dialog) return;
    if (dialog.open) return;
    if (typeof dialog.showModal === "function") {
      try {
        dialog.showModal();
        return;
      } catch (e) {}
    }
    dialog.setAttribute("open", "");
  }

  function chooseFrom(source) {
    var input = find(source === "camera" ? "album-camera-input" : "album-library-input");
    var dialog = find("album-chooser");
    if (dialog && dialog.open && typeof dialog.close === "function") {
      dialog.close();
    } else if (dialog) {
      dialog.removeAttribute("open");
    }
    if (input) input.click();
  }

  document.addEventListener("click", function (event) {
    var add = event.target.closest ? event.target.closest("[data-album-add]") : null;
    if (add) {
      event.preventDefault();
      openChooser();
      return;
    }
    var source = event.target.closest ? event.target.closest("[data-album-source]") : null;
    if (source) {
      event.preventDefault();
      chooseFrom(source.dataset.albumSource);
    }
  });

  document.addEventListener("change", function (event) {
    var target = event.target;
    if (!target || !target.files) return;
    if (target.id !== "album-camera-input" && target.id !== "album-library-input") return;
    var files = Array.prototype.slice.call(target.files);
    target.value = "";
    files.forEach(enqueue);
  });

  document.addEventListener("click", function (event) {
    var dialog = event.target;
    if (dialog && dialog.id === "album-chooser" && dialog.open) {
      dialog.close();
    }
  });

  /* ---------- Viewer ---------- */

  document.addEventListener("click", function (event) {
    if (!isAlbumPage() || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey) return;

    if (event.target.closest("[data-album-viewer-close]")) {
      event.preventDefault();
      closeViewer();
      return;
    }
    if (event.target.closest("[data-album-viewer-prev]")) {
      event.preventDefault();
      stepViewer(-1);
      return;
    }
    if (event.target.closest("[data-album-viewer-next]")) {
      event.preventDefault();
      stepViewer(1);
      return;
    }
    var edit = event.target.closest("[data-album-viewer-edit]");
    if (edit) {
      event.preventDefault();
      openEditSheet(edit.getAttribute("href"));
      return;
    }
    if (event.target === find("album-viewer")) {
      closeViewer();
      return;
    }
    var tile = event.target.closest(".album-tile");
    if (tile) {
      event.preventDefault();
      openViewer(tile);
    }
  });

  document.addEventListener("keydown", function (event) {
    var dialog = find("album-viewer");
    if (!dialog || !dialog.open) return;
    var sheet = find("action-dialog");
    if (sheet && sheet.open) return;
    if (event.key === "ArrowLeft") {
      event.preventDefault();
      stepViewer(-1);
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      stepViewer(1);
    }
  });

  document.addEventListener("submit", function (event) {
    var form = event.target;
    if (form.matches && form.matches("[data-album-edit-form]")) {
      event.preventDefault();
      saveAlbumEdit(form);
      return;
    }
    if (form.matches && form.matches("[data-album-viewer-delete-form]")) {
      event.preventDefault();
      deleteViewerPhoto(form);
    }
  });

  function viewerItem(tile) {
    var img = tile.querySelector("img");
    return {
      id: tile.getAttribute("data-photo-id"),
      fullURL: tile.getAttribute("href"),
      fullLabel: tile.getAttribute("data-full-label") || "",
      caption: tile.getAttribute("data-caption") || "",
      alt: (img && img.alt) || "",
      editURL: tile.getAttribute("data-edit-url") || "",
      downloadURL: tile.getAttribute("data-download-url") || tile.getAttribute("href")
    };
  }

  function collectViewerItems() {
    return Array.prototype.slice.call(document.querySelectorAll("#album-days .album-tile[data-photo-id]"));
  }

  function openEditSheet(url) {
    if (!url || url === "#") return;
    if (window.htmx && typeof window.htmx.ajax === "function") {
      htmx.ajax("GET", url, { target: "#action-dialog", swap: "innerHTML" });
      return;
    }
    window.location.assign(url);
  }

  function openViewer(tile) {
    var dialog = find("album-viewer");
    if (!dialog) {
      window.location.assign(tile.href);
      return;
    }
    viewerItems = collectViewerItems();
    viewerIndex = viewerItems.indexOf(tile);
    if (viewerIndex < 0) viewerIndex = 0;
    viewerReturnFocus = tile;
    if (!dialog.open) {
      if (typeof dialog.showModal === "function") {
        dialog.showModal();
      } else {
        dialog.setAttribute("open", "");
      }
    }
    renderViewer();
  }

  function closeViewer() {
    var dialog = find("album-viewer");
    if (!dialog || !dialog.open) return;
    if (typeof dialog.close === "function") {
      dialog.close();
    } else {
      dialog.removeAttribute("open");
      afterViewerClosed();
    }
  }

  function afterViewerClosed() {
    document.body.classList.remove("album-viewer-open");
    var focus = viewerReturnFocus;
    viewerReturnFocus = null;
    if (focus && focus.isConnected) {
      focus.focus({ preventScroll: true });
      return;
    }
    var main = find("main");
    if (main) main.focus({ preventScroll: true });
  }

  function stepViewer(delta) {
    if (viewerItems.length < 2) return;
    viewerIndex = (viewerIndex + delta + viewerItems.length) % viewerItems.length;
    renderViewer();
  }

  function renderViewer() {
    var dialog = find("album-viewer");
    if (!dialog || !dialog.open) return;
    var tile = viewerItems[viewerIndex];
    if (!tile) {
      closeViewer();
      return;
    }
    var item = viewerItem(tile);
    var image = dialog.querySelector("[data-album-viewer-image]");
    var date = dialog.querySelector("[data-album-viewer-date]");
    var caption = dialog.querySelector("[data-album-viewer-caption]");
    var download = dialog.querySelector("[data-album-viewer-download]");
    var edit = dialog.querySelector("[data-album-viewer-edit]");
    var form = dialog.querySelector("[data-album-viewer-delete-form]");
    var prev = dialog.querySelector("[data-album-viewer-prev]");
    var next = dialog.querySelector("[data-album-viewer-next]");

    if (image) {
      image.src = item.fullURL;
      image.alt = item.alt;
    }
    if (date) date.textContent = item.fullLabel;
    if (caption) {
      caption.textContent = item.caption;
      caption.hidden = !item.caption;
    }
    if (download) download.href = item.downloadURL;
    if (edit) edit.href = item.editURL;
    if (form) {
      form.action = "/album/photos/" + item.id + "/delete";
      form.setAttribute("data-photo-id", item.id);
    }
    if (prev) prev.hidden = viewerItems.length < 2;
    if (next) next.hidden = viewerItems.length < 2;

    document.body.classList.add("album-viewer-open");
    prefetchViewerNeighbours();
  }

  function prefetchViewerNeighbours() {
    if (viewerItems.length < 2) return;
    [viewerIndex - 1, viewerIndex + 1].forEach(function (index) {
      var tile = viewerItems[(index + viewerItems.length) % viewerItems.length];
      if (!tile) return;
      var item = viewerItem(tile);
      if (!item.fullURL) return;
      var image = new Image();
      image.src = item.fullURL;
    });
  }

  function syncViewerAfterEdit(id) {
    var dialog = find("album-viewer");
    if (!dialog || !dialog.open) return;
    viewerItems = collectViewerItems();
    var found = -1;
    for (var i = 0; i < viewerItems.length; i++) {
      if (viewerItems[i].getAttribute("data-photo-id") === String(id)) {
        found = i;
        break;
      }
    }
    if (found < 0) {
      closeViewer();
      return;
    }
    viewerIndex = found;
    renderViewer();
  }

  function initViewerDialog() {
    var dialog = find("album-viewer");
    if (!dialog || dialog.getAttribute("data-viewer-ready") === "1") return;
    dialog.setAttribute("data-viewer-ready", "1");
    dialog.addEventListener("close", afterViewerClosed);

    var stage = dialog.querySelector("[data-album-viewer-stage]");
    if (!stage) return;
    var swipe = null;
    stage.addEventListener("pointerdown", function (event) {
      if (event.button !== 0 || reducedMotion()) return;
      if (event.target.closest("[data-album-viewer-prev], [data-album-viewer-next]")) return;
      swipe = { x: event.clientX, y: event.clientY, id: event.pointerId };
      if (stage.setPointerCapture) {
        try {
          stage.setPointerCapture(event.pointerId);
        } catch (e) {}
      }
    });
    stage.addEventListener("pointerup", function (event) {
      if (!swipe || event.pointerId !== swipe.id) return;
      var dx = event.clientX - swipe.x;
      var dy = event.clientY - swipe.y;
      swipe = null;
      if (Math.abs(dx) < 48 || Math.abs(dx) < Math.abs(dy) * 1.4) return;
      stepViewer(dx > 0 ? -1 : 1);
    });
    stage.addEventListener("pointercancel", function () {
      swipe = null;
    });
  }

  document.addEventListener("DOMContentLoaded", initViewerDialog);

  function reducedMotion() {
    return !!(window.matchMedia && window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  }

  /* ---------- Edit and delete ---------- */

  function saveAlbumEdit(form) {
    var button = form.querySelector("button[type='submit']");
    var general = form.querySelector('[data-album-edit-error="general"]');
    clearEditErrors(form);
    if (general) {
      general.hidden = true;
      general.textContent = "";
    }
    if (button) button.disabled = true;

    var params = new URLSearchParams(new FormData(form));
    params.set("loaded_before", oldestLoadedStart());

    fetch(form.getAttribute("action"), {
      method: "PATCH",
      credentials: "same-origin",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/x-www-form-urlencoded"
      },
      body: params.toString()
    })
      .then(function (response) {
        return response.json().catch(function () {
          return { ok: false, error: "Couldn't save that. Try again." };
        });
      })
      .then(function (data) {
        if (!data || !data.ok) {
          showEditErrors(form, data || {});
          return;
        }
        applyAlbumEdit(data);
        closeEditSheet();
      })
      .catch(function () {
        showEditErrors(form, { error: "Couldn't save that. Check the connection and try again." });
      })
      .then(function () {
        if (button) button.disabled = false;
      });
  }

  function clearEditErrors(form) {
    form.querySelectorAll("[data-album-edit-error]").forEach(function (node) {
      node.textContent = "";
      if (node.getAttribute("data-album-edit-error") === "general") node.hidden = true;
    });
  }

  function showEditErrors(form, data) {
    var errors = data.errors || {};
    setEditError(form, "taken_at", errors.taken_at);
    setEditError(form, "caption", errors.caption);
    var general = form.querySelector('[data-album-edit-error="general"]');
    var message = data.error || "";
    if (!message && (errors.taken_at || errors.caption)) {
      message = "Check the highlighted fields and try again.";
    }
    if (!message) message = "Couldn't save that. Try again.";
    if (general) {
      general.textContent = message;
      general.hidden = false;
    }
    var focus = form.querySelector("[aria-invalid='true']");
    if (focus) focus.focus();
  }

  function setEditError(form, field, message) {
    var node = form.querySelector('[data-album-edit-error="' + field + '"]');
    if (node) node.textContent = message || "";
    var input = form.querySelector('[name="' + field + '"]');
    if (input) {
      if (message) input.setAttribute("aria-invalid", "true");
      else input.removeAttribute("aria-invalid");
    }
  }

  function closeEditSheet() {
    document.body.dispatchEvent(new Event("sheet-saved", { bubbles: true }));
  }

  function applyAlbumEdit(data) {
    var tile = document.querySelector('.album-tile[data-photo-id="' + String(data.id) + '"]');
    if (!tile) return;
    var day = tile.closest(".album-day");
    var oldKey = day ? day.getAttribute("data-day") : "";
    var sameDay = oldKey === data.day_key;

    if (sameDay) {
      tile.outerHTML = data.tile_html;
      var updated = document.querySelector('.album-tile[data-photo-id="' + String(data.id) + '"]');
      if (viewerReturnFocus === tile && updated) viewerReturnFocus = updated;
      syncViewerAfterEdit(data.id);
      dispatchToast("Photo saved.", "success");
      return;
    }

    viewerReturnFocus = null;
    closeViewer();
    removeAlbumTile(tile);
    if (data.out_of_range) {
      dispatchToast("Moved to " + (data.day_label || "another day") + ".", "success", "View that day", data.day_url);
      return;
    }
    insertTile(data);
    dispatchToast("Photo moved to " + (data.day_label || "another day") + ".", "success", "View that day", data.day_url);
  }

  function deleteViewerPhoto(form) {
    var tile = viewerItems[viewerIndex];
    var id = form.getAttribute("data-photo-id") || (tile && tile.getAttribute("data-photo-id"));
    if (!id) return;
    var button = form.querySelector("button[type='submit']");
    if (button) button.disabled = true;

    fetch("/album/photos/" + encodeURIComponent(id), {
      method: "DELETE",
      credentials: "same-origin",
      headers: { Accept: "application/json" }
    })
      .then(function (response) {
        return response.json().catch(function () {
          return { ok: false, error: "Couldn't delete that photo. Try again." };
        });
      })
      .then(function (data) {
        if (!data || !data.ok) {
          dispatchToast((data && data.error) || "Couldn't delete that photo. Try again.", "error");
          return;
        }
        var dayKey = data.day_key || "";
        viewerReturnFocus = null;
        closeViewer();
        var current = document.querySelector('.album-tile[data-photo-id="' + String(id) + '"]');
        if (current) removeAlbumTile(current);
        dispatchToast("Photo deleted.", "success", "Undo", data.undo_url, "post", "#album-day-" + dayKey, 8000);
      })
      .catch(function () {
        dispatchToast("Couldn't delete that photo. Check the connection and try again.", "error");
      })
      .then(function () {
        if (button && button.isConnected) button.disabled = false;
      });
  }

  function removeAlbumTile(tile) {
    var day = tile.closest(".album-day");
    tile.remove();
    if (day && !day.querySelector(".album-tile")) {
      day.hidden = true;
      window.setTimeout(function () {
        if (day.hidden && day.isConnected) day.remove();
      }, 20000);
    }
    updateCount();
  }

  function dispatchToast(message, kind, actionLabel, actionURL, actionMethod, actionTarget, duration) {
    document.body.dispatchEvent(
      new CustomEvent("nurtur:toast", {
        bubbles: true,
        detail: {
          message: message,
          kind: kind,
          actionLabel: actionLabel || "",
          actionURL: actionURL || "",
          actionMethod: actionMethod || "",
          actionTarget: actionTarget || "",
          duration: duration || 0
        }
      })
    );
  }

  document.body.addEventListener("htmx:afterSwap", function () {
    if (isAlbumPage()) {
      initViewerDialog();
      updateCount();
    } else {
      document.body.classList.remove("album-viewer-open");
      viewerReturnFocus = null;
    }
  });

  document.body.addEventListener("htmx:beforeRequest", function (event) {
    if (isPagingRequest(event)) setPagingError(false);
  });

  document.body.addEventListener("htmx:responseError", function (event) {
    if (isPagingRequest(event)) setPagingError(true);
  });

  document.body.addEventListener("htmx:sendError", function (event) {
    if (isPagingRequest(event)) setPagingError(true);
  });

  function isPagingRequest(event) {
    var detail = event && event.detail;
    var path = detail && detail.pathInfo && detail.pathInfo.requestPath;
    return typeof path === "string" && path.indexOf("/album/partial") === 0;
  }

  function setPagingError(show) {
    var error = document.querySelector("[data-album-more-error]");
    if (error) error.hidden = !show;
  }

  /* ---------- Queue ---------- */

  function enqueue(file) {
    if (!file) return;
    if (!isAlbumPage()) return;
    if (pendingUploads() >= MAX_PENDING) {
      dispatchToast("Add up to " + MAX_PENDING + " photos at a time.", "warn");
      return;
    }
    var job = {
      file: file,
      uploadID: newUploadID(),
      state: "queued",
      prepared: false,
      cancelled: false,
      full: null,
      thumb: null,
      meta: null,
      xhr: null,
      preview: null
    };
    job.preview = createPreview(job);
    queue.push(job);
    pump();
  }

  function pendingUploads() {
    var count = 0;
    for (var i = 0; i < queue.length; i++) {
      var state = queue[i].state;
      if (state === "queued" || state === "preparing" || state === "uploading") count++;
    }
    return count;
  }

  function pump() {
    while (active < MAX_ACTIVE) {
      var next = null;
      for (var i = 0; i < queue.length; i++) {
        if (queue[i].state === "queued") {
          next = queue[i];
          break;
        }
      }
      if (!next) return;
      start(next);
    }
  }

  function start(job) {
    active++;
    job.state = "preparing";

    if (job.prepared) {
      job.state = "uploading";
      setPreview(job, "Retrying…", 4, false);
      send(job)
        .catch(function (err) {
          if (job.cancelled) return;
          job.state = "error";
          setPreview(job, errorMessage(err), 0, true);
        })
        .then(function () {
          active = Math.max(0, active - 1);
          pump();
        });
      return;
    }

    setPreview(job, "Preparing…", 0, false);

    prepare(job.file)
      .then(function (prepared) {
        if (job.cancelled) return;
        job.full = prepared.full;
        job.thumb = prepared.thumb;
        job.meta = {
          takenAt: prepared.takenAt,
          source: prepared.source
        };
        job.prepared = true;
        job.state = "uploading";
        setPreview(job, "Uploading…", 4, false);
        return send(job);
      })
      .catch(function (err) {
        if (job.cancelled) return;
        job.state = "error";
        setPreview(job, errorMessage(err), 0, true);
      })
      .then(function () {
        active = Math.max(0, active - 1);
        pump();
      });
  }

  function retry(job) {
    if (!job) return;
    job.cancelled = false;
    job.state = "queued";
    setPreview(job, job.prepared ? "Retrying…" : "Preparing…", 0, false);
    pump();
  }

  function cancel(job) {
    if (!job) return;
    job.cancelled = true;
    if (job.xhr) {
      try {
        job.xhr.abort();
      } catch (e) {}
    }
    removePreview(job);
    job.state = "done";
  }

  /* ---------- Preparation ---------- */

  function prepare(file) {
    return readExif(file)
      .then(function (exif) {
        return decodeImage(file).then(function (source) {
          return {
            source: source,
            date: chooseDate(exif, file)
          };
        });
      })
      .then(function (decoded) {
        var width = decoded.source.width || decoded.source.naturalWidth;
        var height = decoded.source.height || decoded.source.naturalHeight;
        if (!width || !height) throw new Error("That photo couldn't be read.");

        var fullCanvas = drawScaled(decoded.source, width, height, FULL_EDGE);
        var thumbCanvas = drawScaled(decoded.source, width, height, THUMB_EDGE);
        if (decoded.source.close) decoded.source.close();

        return encodeCanvas(fullCanvas, FULL_QUALITY).then(function (full) {
          return encodeCanvas(thumbCanvas, THUMB_QUALITY).then(function (thumb) {
            return {
              full: full,
              thumb: thumb,
              takenAt: decoded.date.takenAt,
              source: decoded.date.source
            };
          });
        });
      });
  }

  function decodeImage(file) {
    if (window.createImageBitmap) {
      return window
        .createImageBitmap(file, { imageOrientation: "from-image" })
        .catch(function () {
          return window.createImageBitmap(file);
        })
        .catch(function () {
          return decodeWithImage(file);
        });
    }
    return decodeWithImage(file);
  }

  function decodeWithImage(file) {
    return new Promise(function (resolve, reject) {
      var img = new Image();
      var url = URL.createObjectURL(file);
      img.onload = function () {
        URL.revokeObjectURL(url);
        resolve(img);
      };
      img.onerror = function () {
        URL.revokeObjectURL(url);
        reject(new Error("This photo can't be opened in this browser."));
      };
      img.src = url;
    });
  }

  function drawScaled(source, width, height, longEdge) {
    var scale = Math.min(1, longEdge / Math.max(width, height));
    var outWidth = Math.max(1, Math.round(width * scale));
    var outHeight = Math.max(1, Math.round(height * scale));
    var canvas = document.createElement("canvas");
    canvas.width = outWidth;
    canvas.height = outHeight;
    var ctx = canvas.getContext("2d");
    if (!ctx) throw new Error("This browser can't prepare photos.");
    ctx.imageSmoothingEnabled = true;
    ctx.imageSmoothingQuality = "high";
    ctx.drawImage(source, 0, 0, outWidth, outHeight);
    return canvas;
  }

  function encodeCanvas(canvas, quality) {
    if (!canvas.toBlob) {
      return Promise.reject(new Error("This browser can't prepare photos."));
    }
    return toBlob(canvas, "image/webp", quality).then(function (webp) {
      if (webp && webp.type === "image/webp") return webp;
      return toBlob(canvas, "image/jpeg", quality).then(function (jpeg) {
        if (jpeg && jpeg.type === "image/jpeg") return jpeg;
        throw new Error("This photo couldn't be prepared. Try another browser or photo.");
      });
    });
  }

  function toBlob(canvas, type, quality) {
    return new Promise(function (resolve) {
      try {
        canvas.toBlob(function (blob) {
          resolve(blob);
        }, type, quality);
      } catch (e) {
        resolve(null);
      }
    });
  }

  function chooseDate(exif, file) {
    var now = new Date();
    if (exif && exif.takenAt && plausible(exif.takenAt, now)) {
      return { takenAt: exif.takenAt, source: "exif" };
    }
    var modified = new Date(file.lastModified || 0);
    if (isFinite(modified.getTime()) && plausible(modified, now)) {
      return { takenAt: modified, source: "file" };
    }
    return { takenAt: now, source: "upload" };
  }

  function plausible(date, now) {
    var time = date.getTime();
    if (!isFinite(time)) return false;
    if (time > now.getTime() + DAY_MS) return false;
    return time >= now.getTime() - TEN_YEARS_MS;
  }

  /* ---------- EXIF ---------- */

  // Read only the JPEG APP1 header. Returns the capture date and orientation,
  // or nulls when there is no readable EXIF block.
  function readExif(file) {
    if (!file || !/^image\/jpe?g$/i.test(file.type || "")) {
      return Promise.resolve(null);
    }
    var slice = file.slice(0, 256 * 1024);
    if (!slice.arrayBuffer) return Promise.resolve(null);
    return slice
      .arrayBuffer()
      .then(function (buffer) {
        return parseExif(new DataView(buffer));
      })
      .catch(function () {
        return null;
      });
  }

  function parseExif(view) {
    if (view.byteLength < 4 || view.getUint8(0) !== 0xff || view.getUint8(1) !== 0xd8) {
      return null;
    }
    var offset = 2;
    while (offset + 4 <= view.byteLength) {
      if (view.getUint8(offset) !== 0xff) {
        offset++;
        continue;
      }
      var marker = view.getUint8(offset + 1);
      if (marker === 0xd8 || marker === 0x01 || (marker >= 0xd0 && marker <= 0xd7)) {
        offset += 2;
        continue;
      }
      if (marker === 0xda || marker === 0xd9) break;
      if (offset + 4 > view.byteLength) break;
      var length = view.getUint16(offset + 2, false);
      if (length < 2) break;
      if (marker === 0xe1 && length >= 8) {
        var start = offset + 4;
        if (
          view.getUint8(start) === 0x45 &&
          view.getUint8(start + 1) === 0x78 &&
          view.getUint8(start + 2) === 0x69 &&
          view.getUint8(start + 3) === 0x66 &&
          view.getUint8(start + 4) === 0x00 &&
          view.getUint8(start + 5) === 0x00
        ) {
          var parsed = parseTiff(view, start + 6, length - 8);
          if (parsed) return parsed;
        }
      }
      offset += 2 + length;
    }
    return null;
  }

  function parseTiff(view, start, length) {
    if (length < 8 || start + length > view.byteLength) return null;
    var little;
    if (view.getUint8(start) === 0x49 && view.getUint8(start + 1) === 0x49) {
      little = true;
    } else if (view.getUint8(start) === 0x4d && view.getUint8(start + 1) === 0x4d) {
      little = false;
    } else {
      return null;
    }
    if (view.getUint16(start + 2, little) !== 42) return null;

    var exifOffset = null;
    var orientation = 1;
    var dateRaw = null;
    var offsetRaw = null;
    var visited = {};

    function readEntry(entry) {
      var tag = view.getUint16(entry, little);
      var type = view.getUint16(entry + 2, little);
      var count = view.getUint32(entry + 4, little);
      var valueOffset = entry + 8;
      if (tag === 0x0112 && type === 3 && count >= 1) {
        orientation = view.getUint16(entry + 8, little);
      } else if (tag === 0x8769 && type === 4 && count >= 1) {
        exifOffset = view.getUint32(entry + 8, little);
      } else if (tag === 0x9003 && type === 2) {
        dateRaw = readAscii(view, start, length, valueOffset, count, little);
      } else if (tag === 0x9011 && type === 2) {
        offsetRaw = readAscii(view, start, length, valueOffset, count, little);
      }
    }

    function readIFD(relative) {
      if (relative == null || visited[relative]) return;
      visited[relative] = true;
      var ifd = start + relative;
      if (ifd + 2 > start + length) return;
      var count = view.getUint16(ifd, little);
      if (count > 512 || ifd + 2 + count * 12 > start + length) return;
      for (var i = 0; i < count; i++) {
        readEntry(ifd + 2 + i * 12);
      }
    }

    var ifd0 = view.getUint32(start + 4, little);
    readIFD(ifd0);
    if (exifOffset != null) readIFD(exifOffset);

    var takenAt = parseExifDate(dateRaw, offsetRaw);
    if (!takenAt && !dateRaw) return null;
    return { takenAt: takenAt, orientation: orientation };
  }

  function readAscii(view, start, length, entryValueOffset, count, little) {
    var value = view.getUint32(entryValueOffset, little);
    var at = start + value;
    if (value + count > length || at + count > view.byteLength) return null;
    var text = "";
    for (var i = 0; i < count; i++) {
      var code = view.getUint8(at + i);
      if (code === 0) break;
      text += String.fromCharCode(code);
    }
    return text;
  }

  function parseExifDate(raw, offsetRaw) {
    if (!raw) return null;
    var match = /^(\d{4}):(\d{2}):(\d{2})[ T](\d{2}):(\d{2}):(\d{2})/.exec(raw.trim());
    if (!match) return null;
    var year = parseInt(match[1], 10);
    var month = parseInt(match[2], 10);
    var day = parseInt(match[3], 10);
    var hour = parseInt(match[4], 10);
    var minute = parseInt(match[5], 10);
    var second = parseInt(match[6], 10);
    if (month < 1 || month > 12 || day < 1 || day > 31 || hour > 23 || minute > 59 || second > 60) {
      return null;
    }
    if (offsetRaw && /^[+-]\d{2}:\d{2}$/.test(offsetRaw.trim())) {
      var parsed = new Date(
        match[1] + "-" + match[2] + "-" + match[3] + "T" + match[4] + ":" + match[5] + ":" + match[6] + offsetRaw.trim()
      );
      if (isFinite(parsed.getTime())) return parsed;
    }
    var local = new Date(year, month - 1, day, hour, minute, Math.min(second, 59), 0);
    return isFinite(local.getTime()) ? local : null;
  }

  /* ---------- Upload ---------- */

  function send(job) {
    return new Promise(function (resolve, reject) {
      if (!job.full || !job.thumb || !job.meta) {
        reject(new Error("That photo couldn't be prepared."));
        return;
      }
      var xhr = new XMLHttpRequest();
      job.xhr = xhr;
      var form = new FormData();
      form.append("upload_id", job.uploadID);
      form.append("taken_at", job.meta.takenAt.toISOString());
      form.append("taken_at_source", job.meta.source);
      form.append("full", job.full, uploadName("full", job.full.type));
      form.append("thumb", job.thumb, uploadName("thumb", job.thumb.type));
      var oldest = oldestLoadedStart();
      if (oldest) form.append("loaded_before", oldest);

      xhr.open("POST", "/album/photos", true);
      xhr.setRequestHeader("Accept", "application/json");
      xhr.upload.onprogress = function (event) {
        if (!event.lengthComputable) return;
        var percent = Math.max(4, Math.min(99, Math.round((event.loaded / event.total) * 100)));
        setPreview(job, "Uploading " + percent + "%", percent, false);
      };
      xhr.onload = function () {
        job.xhr = null;
        var data = null;
        try {
          data = JSON.parse(xhr.responseText);
        } catch (e) {}
        if (xhr.status >= 200 && xhr.status < 300 && data && data.ok) {
          job.state = "done";
          removePreview(job);
          if (data.out_of_range) {
            showOutOfRange(data);
          } else {
            insertTile(data);
          }
          resolve();
          return;
        }
        reject(new Error((data && data.error) || uploadErrorMessage(xhr.status)));
      };
      xhr.onerror = function () {
        job.xhr = null;
        reject(new Error("Connection lost. Retry when you're back online."));
      };
      xhr.onabort = function () {
        job.xhr = null;
        if (job.cancelled) {
          resolve();
          return;
        }
        reject(new Error("Upload stopped."));
      };
      xhr.send(form);
    });
  }

  function uploadName(kind, type) {
    return kind + (type === "image/jpeg" ? ".jpg" : type === "image/png" ? ".png" : ".webp");
  }

  function uploadErrorMessage(status) {
    if (status === 413) return "That photo is too large. Try a smaller one.";
    if (status === 422) return "That photo format isn't supported.";
    if (status === 502 || status === 503) return "Couldn't save that photo. Check the connection and try again.";
    return "Couldn't save that photo. Try again.";
  }

  function errorMessage(err) {
    if (err && err.message) return err.message;
    return "Couldn't prepare that photo. Try another one.";
  }

  function oldestLoadedStart() {
    var days = document.querySelectorAll("#album-days .album-day[data-start]");
    if (!days.length) return "";
    return days[days.length - 1].getAttribute("data-start") || "";
  }

  /* ---------- Previews ---------- */

  function createPreview(job) {
    var container = find("album-upload-previews");
    if (!container) return null;
    var el = document.createElement("div");
    el.className = "album-preview";
    el.setAttribute("aria-busy", "true");

    var img = document.createElement("img");
    img.alt = "";
    img.decoding = "async";
    var objectURL = URL.createObjectURL(job.file);
    img.src = objectURL;

    var status = document.createElement("div");
    status.className = "album-preview-status";
    var text = document.createElement("span");
    text.className = "album-preview-text";
    text.textContent = "Preparing…";
    var bar = document.createElement("div");
    bar.className = "album-preview-progress";
    var fill = document.createElement("span");
    bar.appendChild(fill);
    var actions = document.createElement("div");
    actions.className = "album-preview-actions";
    var retryButton = document.createElement("button");
    retryButton.type = "button";
    retryButton.textContent = "Retry";
    retryButton.hidden = true;
    retryButton.addEventListener("click", function () {
      retry(job);
    });
    var cancelButton = document.createElement("button");
    cancelButton.type = "button";
    cancelButton.textContent = "Cancel";
    cancelButton.addEventListener("click", function () {
      cancel(job);
    });
    actions.appendChild(retryButton);
    actions.appendChild(cancelButton);

    status.appendChild(text);
    status.appendChild(bar);
    status.appendChild(actions);
    el.appendChild(img);
    el.appendChild(status);
    container.appendChild(el);
    container.hidden = false;
    return { el: el, img: img, text: text, fill: fill, retry: retryButton, cancel: cancelButton, url: objectURL };
  }

  function setPreview(job, message, percent, isError) {
    var preview = job.preview;
    if (!preview) return;
    preview.text.textContent = message;
    preview.fill.style.width = Math.max(0, Math.min(100, percent || 0)) + "%";
    preview.el.classList.toggle("is-error", !!isError);
    preview.el.setAttribute("aria-busy", isError ? "false" : "true");
    preview.retry.hidden = !isError;
    preview.cancel.textContent = isError ? "Remove" : "Cancel";
    preview.cancel.hidden = false;
  }

  function removePreview(job) {
    var preview = job.preview;
    if (!preview) return;
    if (preview.url) URL.revokeObjectURL(preview.url);
    if (preview.el && preview.el.parentNode) preview.el.parentNode.removeChild(preview.el);
    job.preview = null;
    var container = find("album-upload-previews");
    if (container && !container.children.length) container.hidden = true;
  }

  /* ---------- Tile insertion ---------- */

  function insertTile(data) {
    var root = find("album-days");
    if (!root || !data || !data.tile_html) return;
    if (root.querySelector('[data-photo-id="' + String(data.id) + '"]')) return;

    var day = null;
    var days = root.querySelectorAll(".album-day[data-day]");
    for (var i = 0; i < days.length; i++) {
      if (days[i].getAttribute("data-day") === data.day_key) {
        day = days[i];
        break;
      }
    }

    if (!day) {
      day = document.createElement("section");
      day.className = "album-day";
      day.id = "album-day-" + data.day_key;
      day.setAttribute("data-day", data.day_key);
      day.setAttribute("data-start", data.day_start || "");
      var heading = document.createElement("h3");
      heading.className = "album-day-title";
      var label = document.createElement("span");
      label.className = "album-day-label";
      label.textContent = data.day_label || "";
      heading.appendChild(label);
      if (data.day_date && data.day_date !== data.day_label) {
        var date = document.createElement("span");
        date.className = "album-day-date";
        date.textContent = " · " + data.day_date;
        heading.appendChild(date);
      }
      var grid = document.createElement("div");
      grid.className = "album-grid";
      day.appendChild(heading);
      day.appendChild(grid);
      insertDay(root, day, data.day_key);
    }

    var empty = root.querySelector(".album-empty");
    if (empty) empty.parentNode.removeChild(empty);

    var grid = day.querySelector(".album-grid");
    if (!grid) return;
    var temp = document.createElement("div");
    temp.innerHTML = data.tile_html;
    var tile = temp.firstElementChild;
    if (!tile) return;

    var taken = Date.parse(data.taken_at || "");
    var siblings = grid.querySelectorAll(".album-tile[data-taken-at]");
    var before = null;
    for (var j = 0; j < siblings.length; j++) {
      var sibling = Date.parse(siblings[j].getAttribute("data-taken-at") || "");
      if (isFinite(taken) && (!isFinite(sibling) || sibling < taken)) {
        before = siblings[j];
        break;
      }
    }
    grid.insertBefore(tile, before);
    updateCount();
  }

  function insertDay(root, day, key) {
    var existing = root.querySelectorAll(".album-day[data-day]");
    for (var i = 0; i < existing.length; i++) {
      if ((existing[i].getAttribute("data-day") || "") < key) {
        root.insertBefore(day, existing[i]);
        return;
      }
    }
    var more = find("album-more");
    if (more && more.parentNode === root) {
      root.insertBefore(day, more);
      return;
    }
    root.appendChild(day);
  }

  function updateCount() {
    var root = find("album-days");
    if (!root) return;
    var count = root.querySelectorAll(".album-tile").length;
    var heading = document.querySelector(".album-heading");
    if (!heading) return;
    var label = document.querySelector(".album-count");
    if (count === 0) {
      if (label) label.parentNode.removeChild(label);
      return;
    }
    if (!label) {
      label = document.createElement("p");
      label.className = "album-count";
      var sub = heading.querySelector(".page-sub");
      if (sub && sub.nextSibling) heading.insertBefore(label, sub.nextSibling);
      else heading.appendChild(label);
    }
    label.textContent = count + (count === 1 ? " photo" : " photos");
  }

  function showOutOfRange(data) {
    var message = "Saved to " + (data.day_label || "that day") + ".";
    document.body.dispatchEvent(
      new CustomEvent("nurtur:toast", {
        bubbles: true,
        detail: {
          message: message,
          kind: "success",
          actionLabel: "View that day",
          actionURL: data.day_url || "/album"
        }
      })
    );
  }

  function newUploadID() {
    try {
      if (window.crypto && typeof window.crypto.randomUUID === "function") {
        return window.crypto.randomUUID();
      }
      var bytes = new Uint8Array(16);
      window.crypto.getRandomValues(bytes);
      var out = "";
      for (var i = 0; i < bytes.length; i++) {
        out += bytes[i].toString(16).padStart(2, "0");
      }
      return out;
    } catch (e) {
      var fallback = "";
      for (var j = 0; j < 32; j++) fallback += Math.floor(Math.random() * 16).toString(16);
      return fallback;
    }
  }
})();

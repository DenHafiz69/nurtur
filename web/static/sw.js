/* Nurtur service worker.
 *
 * Static assets are cache-first. HTML is network-first and is never cached:
 * a bundled shell would show stale data and could outlive a passcode gate.
 * Mutation responses are never cached because only GET requests are handled.
 */
const VERSION = "nurtur-v7";
const SHELL = [
  "/offline",
  "/static/app.css",
  "/static/js/timer.js",
  "/static/htmx.min.js",
  "/static/fonts/fredoka-var.woff2",
  "/static/fonts/nunito-sans-var.woff2",
  "/static/img/pip-face.svg",
  "/static/img/icon-192.png",
  "/static/img/icon-512.png"
];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches.open(VERSION).then((cache) =>
      Promise.all(SHELL.map((url) => cache.add(url).catch(() => {})))
    ).then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((key) => key !== VERSION).map((key) => caches.delete(key)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (event) => {
  const request = event.request;
  if (request.method !== "GET") return;

  const url = new URL(request.url);
  if (url.origin !== self.location.origin) return;

  if (request.mode === "navigate") {
    event.respondWith(networkFirstNavigation(request));
    return;
  }

  if (url.pathname.startsWith("/static/")) {
    event.respondWith(cacheFirst(request));
  }
});

async function networkFirstNavigation(request) {
  try {
    return await fetch(request);
  } catch (err) {
    const cached = await caches.match("/offline");
    if (cached) return cached;
    throw err;
  }
}

async function cacheFirst(request) {
  const cached = await caches.match(request);
  if (cached) {
    // Refresh in the background without delaying the response.
    fetch(request)
      .then((response) => {
        if (response && response.ok) {
          caches.open(VERSION).then((cache) => cache.put(request, response));
        }
      })
      .catch(() => {});
    return cached;
  }
  const response = await fetch(request);
  if (response && response.ok) {
    const copy = response.clone();
    caches.open(VERSION).then((cache) => cache.put(request, copy));
  }
  return response;
}

// ComiNavi service worker: caches the app shell so an installed app opens
// fast. It is not an offline mode. It never touches /api (book data, page
// images, thumbnails, progress), anything cross-origin, or any non-GET.
//
//   /assets/*    hashed and immutable: cache first.
//   navigations  network first, falling back to the cached shell only when
//                the server cannot be reached. A cache-first index.html would
//                outlive an upgrade and point at chunks that no longer exist.
//   other files  (icons, manifest) network first, cache as fallback.
//
// Bump VERSION to drop every older cache on the next activation.
const VERSION = "v1";
const CACHE = `cominavi-shell-${VERSION}`;
const SHELL = ["/", "/manifest.webmanifest", "/favicon.svg", "/icon-192.png"];

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(CACHE)
      .then((c) => c.addAll(SHELL))
      .then(() => self.skipWaiting()),
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k.startsWith("cominavi-shell-") && k !== CACHE).map((k) => caches.delete(k))))
      .then(() => self.clients.claim()),
  );
});

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname === "/sw.js") return;

  if (req.mode === "navigate") {
    event.respondWith(fetch(req).catch(() => caches.match("/").then((r) => r ?? Response.error())));
    return;
  }
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(
      caches.match(req).then(
        (hit) =>
          hit ??
          fetch(req).then((res) => {
            if (res.ok) {
              const copy = res.clone();
              event.waitUntil(caches.open(CACHE).then((c) => c.put(req, copy)));
            }
            return res;
          }),
      ),
    );
    return;
  }
  if (SHELL.includes(url.pathname) || url.pathname.endsWith(".png")) {
    event.respondWith(
      fetch(req)
        .then((res) => {
          if (res.ok) {
            const copy = res.clone();
            event.waitUntil(caches.open(CACHE).then((c) => c.put(req, copy)));
          }
          return res;
        })
        .catch(() => caches.match(req).then((r) => r ?? Response.error())),
    );
  }
});

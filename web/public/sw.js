// MARS service worker: caches the app shell so an installed app opens
// fast. It is not an offline mode. It never touches /api (book data, page
// images, thumbnails, progress, the /api/v1/events stream), anything
// cross-origin, or any non-GET.
//
//   /assets/*    hashed and immutable: cache first.
//   navigations  network first; each successful load refreshes the cached
//                shell. Only when the server cannot be reached does the
//                cached shell answer, and its entry files are precached with
//                it, so it never points at files it lacks.
//   other files  (icons, manifest) network first, cache as fallback.
//
// `npm run build` stamps VERSION and ENTRY (vite.config.ts): every deploy
// changes this file, so the browser installs the new worker, which drops
// all but the previous generation of caches. The previous one is kept so a
// tab still running the old build can load the chunks it already fetched;
// a chunk it never fetched is gone from the server and the route's error
// screen offers a reload.
const VERSION = "dev";
const ENTRY = [];
const PREFIX = "cominavi-shell-";
const CACHE = PREFIX + VERSION;
const SHELL = ["/", "/manifest.webmanifest", "/favicon.svg", "/icon-192.png", ...ENTRY];

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
      .then((keys) => {
        // keys() lists caches in creation order: keep this one and the one before it.
        const old = keys.filter((k) => k.startsWith(PREFIX) && k !== CACHE);
        return Promise.all(old.slice(0, -1).map((k) => caches.delete(k)));
      })
      .then(() => self.clients.claim()),
  );
});

/** Stores a copy of a plain same-origin 200 (never a redirect or opaque response). */
function keep(event, key, res) {
  if (res.ok && res.type === "basic" && !res.redirected) {
    const copy = res.clone();
    event.waitUntil(caches.open(CACHE).then((c) => c.put(key, copy)));
  }
  return res;
}

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname === "/sw.js") return;

  if (req.mode === "navigate") {
    // Every client route is index.html: the latest one becomes the fallback.
    event.respondWith(
      fetch(req)
        .then((res) => (res.headers.get("Content-Type")?.startsWith("text/html") ? keep(event, "/", res) : res))
        .catch(() => caches.match("/", { cacheName: CACHE }).then((r) => r ?? caches.match("/")).then((r) => r ?? Response.error())),
    );
    return;
  }
  if (url.pathname.startsWith("/assets/")) {
    event.respondWith(caches.match(req).then((hit) => hit ?? fetch(req).then((res) => keep(event, req, res))));
    return;
  }
  if (SHELL.includes(url.pathname) || url.pathname.endsWith(".png")) {
    event.respondWith(
      fetch(req)
        .then((res) => keep(event, req, res))
        .catch(() => caches.match(req).then((r) => r ?? Response.error())),
    );
  }
});

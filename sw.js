// Offline-Cache: Seite immer zuerst frisch aus dem Netz holen, ohne Netz aus dem Cache
const C = "grimoire-1795386283";
const FILES = ["./", "index.html", "manifest.webmanifest", "icon-192.png", "icon-512.png"];
self.addEventListener("install", e => { e.waitUntil(caches.open(C).then(c => c.addAll(FILES.map(f => new Request(f, {cache: "reload"}))))); self.skipWaiting(); });
self.addEventListener("activate", e => { e.waitUntil(caches.keys().then(ks => Promise.all(ks.filter(k => k !== C).map(k => caches.delete(k))))); self.clients.claim(); });
self.addEventListener("fetch", e => {
  const r = e.request, page = r.mode === "navigate" || /\/(index\.html)?$/.test(new URL(r.url).pathname);
  if (r.method !== "GET") return;
  if (page){
    e.respondWith(fetch(r, {cache: "no-cache"}).then(res => { if (res.ok){ const cp = res.clone(); caches.open(C).then(c => c.put("index.html", cp)); } return res; })
      .catch(() => caches.match(r, {ignoreSearch: true}).then(x => x || caches.match("index.html"))));
    return;
  }
  e.respondWith(caches.match(r, {ignoreSearch: true}).then(x => x || fetch(r)));
});

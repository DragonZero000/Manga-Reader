// Встроенное расширение MangaReader.
// BRIDGE (config.js) — порт и токен моста приложения, записываются при запуске браузера.

// id загрузки → промис адреса страницы, открытой в активной вкладке в момент
// начала загрузки. Referer сайта не подходит: его обрезают до адреса сайта.
const pages = new Map();

browser.downloads.onCreated.addListener((item) => {
  pages.set(item.id, browser.tabs.query({ active: true, lastFocusedWindow: true })
    .then(([tab]) => (tab && /^https?:/i.test(tab.url) ? tab.url : ""))
    .catch(() => ""));
});

function post(path, body) {
  return fetch(`http://127.0.0.1:${BRIDGE.port}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-MangaReader-Token": BRIDGE.token },
    body: JSON.stringify(body),
  }).catch(() => {});
}

browser.downloads.onChanged.addListener(async (delta) => {
  if (!delta.state || delta.state.current === "in_progress") return;
  const page = await (pages.get(delta.id) || Promise.resolve(""));
  pages.delete(delta.id);
  if (delta.state.current !== "complete" || !page) return;
  const [item] = await browser.downloads.search({ id: delta.id });
  if (item) post("/download", { file: item.filename, page, url: item.url });
});

// «MangaReader»: свернуть браузер и показать приложение.
browser.browserAction.onClicked.addListener(async (tab) => {
  await browser.windows.update(tab.windowId, { state: "minimized" });
  post("/show", {});
});

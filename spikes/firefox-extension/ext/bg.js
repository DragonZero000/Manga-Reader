// страница, с которой начата загрузка: активная вкладка в момент начала
const pages = {};
browser.downloads.onCreated.addListener(async (item) => {
  const [tab] = await browser.tabs.query({ active: true, lastFocusedWindow: true });
  pages[item.id] = tab ? tab.url : "";
});
async function post(path, body) {
  const cfg = BRIDGE; // config.js: порт и токен, записанные приложением
  return fetch(`http://127.0.0.1:${cfg.port}${path}`, {
    method: "POST",
    headers: { "Content-Type": "application/json", "X-MangaReader-Token": cfg.token },
    body: JSON.stringify(body),
  });
}
browser.downloads.onChanged.addListener(async (d) => {
  if (!d.state || d.state.current !== "complete") return;
  const [item] = await browser.downloads.search({ id: d.id });
  await post("/download", { file: item.filename, page: pages[d.id] || "", url: item.url, referrer: item.referrer || "" });
  delete pages[d.id];
});
browser.browserAction.onClicked.addListener(async (tab) => {
  post("/click", { windowId: tab.windowId });
  try {
    await browser.windows.update(tab.windowId, { state: "minimized" });
  } catch (e) {
    post("/error", { message: String(e) });
  }
  await post("/show", {});
});
post("/hello", {});

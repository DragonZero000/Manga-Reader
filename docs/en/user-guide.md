**English** | [Русский](../ru/user-guide.md)

# User guide

MangaReader shows manga from zip archives in your library folder and helps you grow it: the built-in browser saves downloads straight into that folder. Only the browser needs the internet — the library, reader and search work offline.

> The interface is currently in Russian only. Buttons and tabs are quoted here the way you see them on screen, with an English translation at first mention.

- [Library folder](#library-folder)
- [Archive format](#archive-format)
- [Library](#library)
- [Gallery page](#gallery-page)
- [Reader](#reader)
- [Search](#search)
- [Built-in browser](#built-in-browser)
- [Errors](#errors)
- [Settings](#settings)

## Library folder

### Windows — portable mode

Everything is kept in the application folder and nowhere else (not in `%APPDATA%`, not in your user profile):

```text
MangaReader\
├── mangareader.exe
├── settings.json   ← settings, created after the first change
├── manga\          ← put your .zip archives here (created on first start)
└── browser\
    ├── firefox\    ← built-in browser
    └── profile\    ← browser profile: bookmarks, cookies, extensions
```

- You can move the whole application folder, for example to a USB stick.
- Don't put it where you can't write (for example, `Program Files`): the app will show an error with the path.
- Archives may be arranged in subfolders inside `manga` — they are picked up too.

### Android — a folder you choose

On first start the system folder picker opens. Pick or create a folder **inside** `Download`, for example `Download/manga`, and tap "Use this folder" → "Allow". The app remembers the choice and sees everything in that folder and its subfolders: files saved by a browser, moved by a file manager or copied from a PC over USB. No special permissions ("all files access") are needed.

- Android doesn't allow picking the `Download` root itself. If a browser saved an archive directly into `Download`, move it into your folder.
- To change the folder: **Настройки** (Settings) → **Изменить папку** (Change folder).
- If the folder was deleted or renamed, the app asks you to pick it again.
- Copying from a PC with adb: `adb push example.zip /sdcard/Download/manga/`.

### Automatic updates

While the app is open, the library updates itself: new, changed and removed files show up within a second or two, no need to press **Обновить** (Refresh). Unfinished downloads (`*.part`) don't get in the way — the file is checked once the download completes. If a file is briefly locked by another program (an antivirus, for example), the app waits.

## Archive format

A gallery is a `.zip` archive with images (`.jpg`, `.jpeg`, `.png`, `.gif`, `.webp`) and, optionally, a `meta.json` file:

```text
example.zip
└── example/
    ├── 1.jpg
    ├── 2.jpg
    └── meta.json
```

- Pages are ordered by name in natural order: `2.jpg` comes before `10.jpg`. Subfolders inside the archive are allowed.
- The first page is the cover.
- Without `meta.json` the file name becomes the title, and there are no tags or details.

`meta.json` fields the app understands (everything else is ignored):

| Field | Meaning | Where it shows |
|---|---|---|
| `title.english` | Main title | Card, gallery page, reader |
| `title.japanese` | Alternative title (or the main one if there is no English title) | Gallery page |
| `id` | ID on the site (a number or a string with a number) | Search `id:` |
| `tags` | List of `{"type": "...", "name": "..."}`; types `artist`, `group`, `parody`, `character`, `language`, `category`, `tag`; other types are shown too | Gallery page, search |
| `upload_date` | Upload date on the site (Unix time) | "Загружено" (Uploaded), search `uploaded:` |
| `num_pages` | Page count according to the site | "Страниц" (Pages), if it differs from the archive |
| `num_favorites` | Number of favorites on the site | "Избранное" (Favorites), search `favorites:` |
| `scanlator` | Scanlator | "Сканлейтор" (Scanlator), search `scanlator:` |
| `url`, `source`, `link`, `source_url`, `gallery_url` | Link to the work (the first `http(s)` one is used) | **Открыть в браузере** (Open in browser) button |

Example: [`testdata/example.zip`](../../testdata/example.zip).

## Library

The **Библиотека** (Library) tab shows galleries as a grid of cards: cover and title. Newest files come first (by file modification time). At the top are the gallery count and the **Обновить** (Refresh) button — usually not needed, since the folder is watched automatically.

![Library on Windows](../images/library.png)

Tapping a card opens its gallery page. If the library is empty, the app shows the path of the folder to put archives into; you can copy the path in **Настройки** (Settings).

<img src="../images/mobile-library.png" alt="Empty library on Android" width="280">

## Gallery page

Everything about one gallery:

![Gallery page](../images/manga-page.png)


- cover, title and alternative title;
- the **Читать** (Read) button;
- the **Открыть в браузере** (Open in browser) button — when a link to the work is known: from `meta.json`, or the page the file was downloaded from with the built-in browser;
- tags grouped by type: "Автор" (Artist), "Группа" (Group), "Пародия" (Parody), "Персонаж" (Character), "Язык" (Language), "Категория" (Category), "Теги" (Tags), then groups of other types. **Tapping a tag searches for it**;
- details: "Страниц" (Pages), "Загружено" (Uploaded), "Избранное" (Favorites), "Сканлейтор" (Scanlator), "Файл" (File), "Размер" (Size), "Изменён" (Modified). Empty fields are hidden.

## Reader

**Читать** (Read) opens the reader on top of all tabs. Close it with ✕ on the panel, Esc (PC) or Back (Android). Esc and Back close the reader first, then the gallery page.

![Reader with the panel shown](../images/reader.png)

Two modes, switched on the panel and remembered:

**Страницы** (Pages) — one page at a time, western order (left to right):

| Action | PC | Phone |
|---|---|---|
| Next page | →, ↓, PageDown, Space, wheel down, click the right third | Tap the right third, swipe right to left |
| Previous page | ←, ↑, PageUp, wheel up, click the left third | Tap the left third, swipe left to right |
| First / last | Home / End | Slider on the panel |
| Zoom ×2 / back | Double click | Double tap |
| Move a zoomed page | Drag, wheel | Drag |
| Show / hide the panel | Click the middle third | Tap the middle third |

**Лента** (Strip) — all pages one below another. Scroll with the wheel, ↓/↑, PageDown/PageUp, Space; → and ← jump to the next and previous page; Home/End go to the start and end. A single tap shows and hides the panel.

The panel has the title, page number "N / total", a slider to jump to a page, the **Страницы** / **Лента** switch and a close button. Pages load in the background; neighbouring pages are preloaded.

## Search

The **Поиск** (Search) tab searches the whole library. Conditions separated by spaces are combined with AND. When the query runs depends on the mode chosen in **Настройки** (Settings):

- **При вводе** (As you type, the default) — half a second after you stop typing; results appear even while the keyboard is open. There is no 🔍 button, and Enter only closes the keyboard.
- **По кнопке** (On button) — only when you press 🔍 or Enter.

The **✕** button clears the box and keeps the results on screen. The **Как искать** (How to search) cheat sheet is shown until the first query of the session; after that the screen always shows the last result, even with an empty box — the table below covers the same syntax.

![Search by tag](../images/search.png)

| Query | Finds |
|---|---|
| `school` | Part of a word in the title, tags, scanlator, file name, ID |
| `"english name"` | The whole phrase |
| `tag:"tag 1"` | An exact tag of any type |
| `artist:"artist 1"` | A tag of a given type: `artist`, `group`, `parody`, `character`, `language`, `category` |
| `-tag:yuri` | Excludes works with the tag (also `-artist:…` etc.) |
| `pages:>20` | Page count: `>`, `<`, `>=`, `<=`, range `10..50` |
| `uploaded:2024` | Upload date on the site: `2024`, `>2024-06`, `2024-01..2024-03` |
| `added:>=2025-01` | When the file appeared in the folder (file modification time) |
| `size:>10mb` | File size: `k`, `mb`, `gb` |
| `favorites:>100`, `id:535147` | Favorites on the site and ID |
| `scanlator:name` | Scanlator |
| `school pages:>20 -tag:yuri` | All together |

Dates: `2024` is the whole year, `2024-06` a month, `2024-06-15` a day; `>period` means after its end, `<period` before its start. If a query has a mistake, "Ошибка в запросе: …" (Query error) appears below the search box with an explanation. Results refresh on their own when the library changes: the last query that ran is repeated.

## Built-in browser

The **Браузер** (Browser) button on the library toolbar opens a Firefox-based browser. Everything you download there is saved straight into the library folder and shows up in the library within a second or two (or on the **Ошибки** (Errors) tab if it isn't an archive). Downloaded files remember the page they came from — for the **Открыть в браузере** (Open in browser) button.

### Windows

- Portable Firefox ESR from the `browser\firefox` folder; browsers installed on the system and their profiles are not used.
- The browser window is tied to the app window, like the browser in Telegram: always on top of it, minimized together with it, no separate taskbar button. Closing the app closes the browser.
- Tabs are switched with the "all tabs" button in the header (next to "new tab"). The "MangaReader" button (book icon) minimizes the browser and shows the app.
- Extensions are installed from addons.mozilla.org and kept in the profile. The browser theme is always dark.
- Firefox updates, telemetry, ads and welcome screens are disabled. The Firefox version is updated together with the app.
- The source page address is stored in the file itself (an NTFS stream) and survives renaming; it is lost when copying to a FAT/exFAT USB stick. For files downloaded by other browsers, the Windows download mark is used — sites often trim it to the home page.
- While open, Firefox keeps a few values in the registry (`HKCU\Software\Mozilla\Firefox`); after the browser closes, the app removes them. Values of other Firefox installations are left alone.

### Android

<img src="../images/mobile-browser.png" alt="Built-in browser on Android" width="280">

- A Firefox engine (GeckoView) inside the app — a screen on top of the reader, with no separate icon.
- Toolbar: address or search, back/forward, reload, **tabs** (the number shows how many are open), **MangaReader** (back to the reader), **☆** (bookmark), and a menu: new tab, bookmarks, downloads, extensions. The toolbar can be at the top or at the bottom.
- "MangaReader" and the system Back on the first page of a tab return to the reader; tabs are kept until the system unloads the app.
- Downloads go to the root of the library folder: `name.part` while downloading, `name (1).zip` if the name is taken. Progress is shown in the notification shade (Android asks for permission on the first download); the download continues if you minimize the app.
- Extensions come from addons.mozilla.org via "Add to Firefox" (for example, uBlock Origin); menu → "Extensions" to enable, disable or remove them.
- The browser needs write access to the library folder. If the folder was granted read-only, the app asks you to pick it again the first time you open the browser.

## Errors

The app checks **every** file in the library folder. Only a `.zip` archive with images becomes a gallery; everything else lands on the **Ошибки** (Errors) tab with a reason, for example:

- "Изображение — поддерживаются только zip-архивы" (An image — only zip archives are supported): pictures, video, audio, PDF, RAR/7z, text — the type is detected by content, not by extension;
- "Веб-страница (HTML) вместо архива" (A web page instead of an archive) — the site returned an error or check page instead of the file;
- "не zip-архив или архив повреждён" (not a zip or a damaged archive), "нет изображений" (no images), "Пустой файл" (empty file), "Файл занят другой программой" (file is locked by another program).

Broken files are **not deleted** — remove or replace them yourself; the entry disappears after the next scan. The number on the tab icon counts unseen errors; opening the tab marks them as seen.

Not treated as errors: hidden files and folders (name starts with `.`) — you can move unrelated files there; unfinished downloads (`*.part` and an empty file next to `<name>.part`).

## Settings

The **Настройки** (Settings) tab:

- **Папка библиотеки** (Library folder) — the folder path; on Windows the **Скопировать путь** (Copy path) button, on Android **Изменить папку** (Change folder).
- **Браузер** (Browser, Windows) — **Домашняя страница** (Home page), **Поисковик** (Search engine — opens Firefox search settings), **Расширения** (Extensions), **Очистить cookies и данные сайтов** (Clear cookies and site data), **Очистить историю** (Clear history). Clearing happens the next time the browser opens; bookmarks are kept.
- **Браузер** (Browser, Android) — **Домашняя страница** (Home page), **Поисковик** (Search engine), **Панель браузера** (Browser toolbar: **Снизу** / **Сверху**, bottom / top), clearing cookies and history (immediately; bookmarks and extensions are kept).
- **Поиск** (Search) — when a query runs: **При вводе** (As you type) or **По кнопке** (On button); see [Search](#search).
- **Экран** (Screen, Android) — **Ограничить 60 Гц** (Limit to 60 Hz, on by default): on 120 Hz screens the app asks the system for 60 Hz, which makes scrolling smoother. The built-in browser runs at whatever rate the system picks.
- **О приложении** (About) — the version.

The reader mode is remembered automatically. On Windows, settings are stored in `settings.json` next to the app.

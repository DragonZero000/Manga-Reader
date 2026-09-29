**English** | [Русский](../ru/user-guide.md)

# User guide

MangaReader shows manga from zip archives in your library folder and helps you grow it: the built-in browser saves downloads straight into that folder. Only the browser needs the internet — the library, reader and search work offline.

> The interface is available in English and Russian. By default it follows the system language (English if the app has no translation for it); you can pick the language in **Settings** → **Язык / Language**.

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
├── library.db      ← library catalog (created automatically, safe to delete)
├── user.db         ← your data about works (don't delete)
├── manga\          ← put your .zip archives here (created on first start)
└── browser\
    ├── firefox\    ← built-in browser
    └── profile\    ← browser profile: bookmarks, cookies, extensions
```

- You can move the whole application folder, for example to a USB stick. Move it while the app is closed.
- `library.db` is the library catalog: parsed archives, errors, covers, search data and a copy of the page links of downloaded files. With it the app shows the library right away at startup and opens only new and changed archives. It is a cache: if you delete it, it is created again and the library is scanned from scratch. On Android the catalog lives in the app's private folder.
- `user.db` is your own data about works that can't be restored from the archives. Unlike the catalog, don't delete it: keep it when updating or moving the app. Records follow an archive when it is renamed or moved to another subfolder of `manga` (the app recognizes the archive by its pages). If the file gets damaged, the app renames it to `user.db.broken-<date-time>`, creates a new one and tells you at startup; the damaged copy stays next to it. On Android `user.db` lives in the app's private folder.
- Don't put it where you can't write (for example, `Program Files`): the app will show an error with the path.
- Archives may be arranged in subfolders inside `manga` — they are picked up too.

### Android — a folder you choose

On first start the system folder picker opens. Pick or create a folder **inside** `Download`, for example `Download/manga`, and tap "Use this folder" → "Allow". The app remembers the choice and sees everything in that folder and its subfolders: files saved by a browser, moved by a file manager or copied from a PC over USB. No special permissions ("all files access") are needed.

- Android doesn't allow picking the `Download` root itself. If a browser saved an archive directly into `Download`, move it into your folder.
- To change the folder: **Settings** → **Change folder**.
- If the folder was deleted or renamed, the app asks you to pick it again.
- Copying from a PC with adb: `adb push example.zip /sdcard/Download/manga/`.

### Automatic updates

While the app is open, the library updates itself: new, changed and removed files show up within a second or two, no need to press **Refresh**. Unfinished downloads (`*.part`) don't get in the way — the file is checked once the download completes. If a file is briefly locked by another program (an antivirus, for example), the app waits.

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
| `upload_date` | Upload date on the site (Unix time) | "Uploaded", search `uploaded:` |
| `num_pages` | Page count according to the site | "Pages", if it differs from the archive |
| `num_favorites` | Number of favorites on the site | "Favorites", search `favorites:` |
| `scanlator` | Scanlator | "Scanlator", search `scanlator:` |
| `url`, `source`, `link`, `source_url`, `gallery_url` | Link to the work (the first `http(s)` one is used) | **Open in browser** button |

Example: [`testdata/example.zip`](../../testdata/example.zip).

## Library

The **Library** tab shows galleries as a grid of cards: cover and title. Card size is set in **Settings**, in the **Grid** section. Newest files come first (by file modification time). At the top are the gallery count and the **Refresh** button — usually not needed, since the folder is watched automatically.

![Library on Windows](../images/library.png)

Tapping a card opens its gallery page. The 🎲 button on the panel opens the page of a random gallery from the library; it is inactive while the library is empty. If the library is empty, the app shows the path of the folder to put archives into; you can copy the path in **Settings**.

<img src="../images/mobile-library.png" alt="Empty library on Android" width="280">

### Card menu

Every card in the library and in search results has a **⋮** button in the top-right corner of the cover. Right-clicking a card (PC) or pressing and holding it (phone) opens the same menu:

- **Open** — the gallery page;
- **Read** — the reader from page 1; closing it returns you to the grid;
- **Open in browser** — only when a link to the work is known; works like the button on the gallery page;
- **Copy title** — copies the main title to the clipboard;
- **Show in folder** (Windows only) — opens File Explorer in the archive's folder with the file selected;
- **Delete…** — deletes the archive after confirmation (see below).

### Deleting an archive

**Delete…** always asks first and shows the title and the file's path inside the library folder:

- **Windows**: the file is moved to the **Recycle Bin** and can be restored from there. If the drive has no Recycle Bin (a USB stick, a network drive), the app says so and offers to delete the file permanently.
- **Android**: the file is deleted **permanently** — Android folders have no Recycle Bin, and the confirmation says so. Deleting needs write access to the library folder: if the folder was granted read-only, the app asks you to pick it again.

The card disappears from the library and the search results right away; then the library is rescanned. Your data about the work is kept for the period set in **Settings → Data retention**: if the file is restored (for example, from the Recycle Bin), the data comes back. If another program is using the file, the app says so and the file stays. A gallery that is open in the reader can't be deleted — close it first.

## Gallery page

Everything about one gallery:

![Gallery page](../images/manga-page.png)


- cover, title and alternative title;
- the **Read** button;
- the **Open in browser** button — when a link to the work is known: from `meta.json`, or the page the file was downloaded from with the built-in browser;
- tags grouped by type: "Artist", "Group", "Parody", "Character", "Language", "Category", "Tags", then groups of other types. Your own tags come after the original ones in their group and have a yellow background. **Tapping a tag searches for it**;
- details: "Pages", "Uploaded", "Favorites", "Scanlator", "File", "Size", "Modified". Empty fields are hidden. Dates, numbers and sizes follow the interface language.

The **⋮** button on the right of the top bar holds the actions that are not on the page itself: **Copy title**, **Show in folder** (Windows) and **Delete…**. After deleting, the page closes and you return to the grid.

### Editing tags

Tags from `meta.json` are set by the site. You can add your own tags of any type and hide the ones you don't need; the archive and `meta.json` are not changed. Press **Edit tags** under the details; **Done** leaves the edit mode (so does closing the page). Every change is saved right away. In the edit mode:

- **+** in a group adds a tag of that type. Type the name and press Enter or **Add**; Esc or **Cancel** closes the box. While you type, up to 10 suggestions show up below: names of this type from the library and your own tags, the most used first. Tap a suggestion to add it.
- **Add to another group** adds a tag of a type the work doesn't have yet (for example, "Group").
- **✕** on your own tag removes it. **✕** on an original tag hides it: in the edit mode it stays in place in red strikethrough text with **↺** to bring it back; outside the edit mode it is not shown, and a group with only hidden tags disappears.
- **Reset to original…** removes all your tags and shows all hidden ones again, after a confirmation.

Names are stored in lower case with single spaces, like the original tags. A name can't be empty, longer than 100 characters or contain `"`. If the work already has a tag of the same type and name (original, hidden or your own), the app says "This tag is already there" and changes nothing.

Your tags and hidden tags are kept in `user.db` (see [Windows — portable mode](#windows--portable-mode)). They survive restarts and follow the archive when it is renamed or moved inside the library folder. If `user.db` can't be opened, **Edit tags** is inactive and the tags from `meta.json` are shown as usual.

## Reader

**Read** opens the reader on top of all tabs. Close it with ✕ on the panel, Esc (PC) or Back (Android). Esc and Back close the reader first, then the gallery page.

![Reader with the panel shown](../images/reader.png)

Two modes, switched on the panel and remembered:

**Pages** — one page at a time, western order (left to right):

| Action | PC | Phone |
|---|---|---|
| Next page | →, ↓, PageDown, Space, wheel down, click the right third | Tap the right third, swipe right to left |
| Previous page | ←, ↑, PageUp, wheel up, click the left third | Tap the left third, swipe left to right |
| First / last | Home / End | Slider on the panel |
| Zoom ×2 / back | Double click | Double tap |
| Move a zoomed page | Drag, wheel | Drag |
| Show / hide the panel | Click the middle third | Tap the middle third |

**Strip** — all pages one below another. Scroll with the wheel, ↓/↑, PageDown/PageUp, Space; → and ← jump to the next and previous page; Home/End go to the start and end. A single tap shows and hides the panel.

The panel has the title, page number "N / total", a slider to jump to a page, the **Pages** / **Strip** switch and a close button. Pages load in the background; neighbouring pages are preloaded.

## Search

The **Search** tab searches the whole library. Conditions separated by spaces are combined with AND. Letter case, "ё"/"е" and character width don't matter: `елка` finds "Ёлка", `ＡＢＣ` finds "ABC". When the query runs depends on the mode chosen in **Settings**:

- **While typing** (the default) — half a second after you stop typing; results appear even while the keyboard is open. There is no 🔍 button, and Enter only closes the keyboard.
- **On button** — only when you press 🔍 or Enter.

The **✕** button clears the box and keeps the results on screen. The 🎲 button next to it opens the page of a random gallery from the current results; it is always visible but inactive until a query has found something. A query with a mistake keeps the previous results, so 🎲 keeps picking from them. The **How to search** cheat sheet is shown until the first query of the session; after that the screen always shows the last result, even with an empty box — the table below covers the same syntax.

![Search by tag](../images/search.png)

| Query | Finds |
|---|---|
| `school` | Part of a word in the title, tags, scanlator, file name, ID |
| `"english name"` | The whole phrase |
| `tag:"tag 1"` | An exact tag of any type |
| `artist:"artist 1"` | A tag of a given type: `artist`, `group`, `parody`, `character`, `language`, `category` |
| `-tag:yuri` | Excludes works with the tag (also `-artist:…` etc.) |
| `custom-tag:"my fav"` | Your own tag of any type, see [Editing tags](#editing-tags) |
| `hidden-tag:yuri` | An original tag you hid |
| `pages:>20` | Page count: `>`, `<`, `>=`, `<=`, range `10..50` |
| `uploaded:2024` | Upload date on the site: `2024`, `>2024-06`, `2024-01..2024-03` |
| `added:>=2025-01` | When the file appeared in the folder (file modification time) |
| `size:>10mb` | File size: `k`, `mb`, `gb` |
| `favorites:>100`, `id:535147` | Favorites on the site and ID |
| `scanlator:name` | Scanlator |
| `school pages:>20 -tag:yuri` | All together |

Tags are searched as you see them on the gallery page: `tag:`, `artist:` and the other types find the original tags that aren't hidden plus your own ones, and a word in the query doesn't match hidden tags. `custom-tag:` finds only your own tags, `hidden-tag:` only hidden ones; both work with `-`.

Dates: `2024` is the whole year, `2024-06` a month, `2024-06-15` a day; `>period` means after its end, `<period` before its start. If a query has a mistake, "Query error: …" appears below the search box with an explanation. Results refresh on their own when the library changes: the last query that ran is repeated.

## Built-in browser

The **Browser** button on the library toolbar opens a Firefox-based browser. Everything you download there is saved straight into the library folder and shows up in the library within a second or two (or on the **Errors** tab if it isn't an archive). Downloaded files remember the page they came from — for the **Open in browser** button.

### Windows

- Portable Firefox ESR from the `browser\firefox` folder; browsers installed on the system and their profiles are not used.
- The browser window is tied to the app window, like the browser in Telegram: always on top of it, minimized together with it, no separate taskbar button. Closing the app closes the browser.
- Tabs are switched with the "all tabs" button in the header (next to "new tab"). The "MangaReader" button (book icon) minimizes the browser and shows the app.
- Extensions are installed from addons.mozilla.org and kept in the profile. The browser theme is always dark.
- Firefox updates, telemetry, ads and welcome screens are disabled. The Firefox version is updated together with the app.
- Firefox menus, settings and dialogs follow the app language (it applies the next time the browser opens after the app restarts); strings missing from a translation are shown in English. Sites are asked for pages in that language too, as in regular Firefox; if you change the page languages in Firefox settings, your choice is kept.
- The source page address is stored in the file itself (an NTFS stream) and survives renaming; it is lost when copying to a FAT/exFAT USB stick. For files downloaded by other browsers, the Windows download mark is used — sites often trim it to the home page.
- While open, Firefox keeps a few values in the registry (`HKCU\Software\Mozilla\Firefox`); after the browser closes, the app removes them. Values of other Firefox installations are left alone.

### Android

<img src="../images/mobile-browser.png" alt="Built-in browser on Android" width="280">

- A Firefox engine (GeckoView) inside the app — a screen on top of the reader, with no separate icon.
- Toolbar: address or search, back/forward, reload, **tabs** (the number shows how many are open), **MangaReader** (back to the reader), **☆** (bookmark), and a menu: new tab, bookmarks, downloads, extensions. The toolbar can be at the top or at the bottom.
- "MangaReader" and the system Back on the first page of a tab return to the reader; tabs are kept until the system unloads the app.
- To keep the browser's memory down, inactive tabs are unloaded: while you are in the browser the current and the previous tab stay loaded, after you return to the reader only the current one does. An unloaded tab stays in the list and reloads with the same history when you pick it (anything typed into forms is lost). A tab with a download in progress is not unloaded until the download finishes.
- Downloads go to the root of the library folder: `name.part` while downloading, `name (1).zip` if the name is taken. Progress is shown in the notification shade (Android asks for permission on the first download); the download continues if you minimize the app.
- Extensions come from addons.mozilla.org via "Add to Firefox" (for example, uBlock Origin); menu → "Extensions" to enable, disable or remove them.
- The browser's toolbar, menus, dialogs and download notifications follow the app language; sites are asked for pages in that language too (as in Firefox, where the page language follows the interface language).
- The browser needs write access to the library folder. If the folder was granted read-only, the app asks you to pick it again the first time you open the browser.

## Errors

The app checks **every** file in the library folder. Only a `.zip` archive with images becomes a gallery; everything else lands on the **Errors** tab with a reason, for example:

- "Image - only zip archives are supported": pictures, video, audio, PDF, RAR/7z, text — the type is detected by content, not by extension;
- "Web page (HTML) instead of an archive" — the site returned an error or check page instead of the file;
- "not a zip archive or the archive is damaged", "no images", "Empty file", "The file is in use by another program".

Reasons are shown in the current interface language, including for files checked before the language was changed.

Broken files are **not deleted** — remove or replace them yourself; the entry disappears after the next scan. The number on the tab icon counts unseen errors; opening the tab marks them as seen.

Not treated as errors: hidden files and folders (name starts with `.`) — you can move unrelated files there; unfinished downloads (`*.part` and an empty file next to `<name>.part`).

## Settings

The **Settings** tab:

- **Язык / Language** — **System** (the default: the system language, or English if the app has no translation for it), **English** or **Русский**. The new language applies after the app restarts; until then a hint in the chosen language is shown.
- **Library folder** — the folder path; on Windows the **Copy path** button, on Android **Change folder**.
- **Browser** (Windows) — **Home page**, **Search engine** (opens Firefox search settings), **Extensions**, **Clear cookies and site data**, **Clear history**. Clearing happens the next time the browser opens; bookmarks are kept.
- **Browser** (Android) — **Home page**, **Search engine**, **Browser toolbar** (**Bottom** / **Top**), clearing cookies and history (immediately; bookmarks and extensions are kept).
- **Search** — when a query runs: **While typing** or **On button**; see [Search](#search).
- **Random pick** — how the 🎲 button picks: **With repeats** (the default) — every press picks from the whole set, so the same gallery can come up again; **No repeats** — a gallery doesn't come up again until every gallery in the set has, and the next round never starts with the one shown last. The library and search keep separate rounds; a round starts over when the set changes (different search results, library contents changed after a rescan) and isn't kept between launches.
- **Grid** — how dense the card grid is in the library and search; applies immediately, no restart needed. On Android — **Cards per row**: **2**, **3** (the default) or **4**; in portrait the cards split the screen width evenly, in landscape they keep the same size and more fit in a row. On Windows — **Card size**: **Small**, **Medium** (the default) or **Large**; the number of columns follows the window width. After a change, covers are reloaded for the new size.
- **Display** (Android) — **Limit to 60 Hz** (on by default): on 120 Hz screens the app asks the system for 60 Hz, which makes scrolling smoother. The built-in browser runs at whatever rate the system picks.
- **Data retention** — **Keep data of deleted works**: **1 day**, **1 week**, **1 month** (the default), **3 months**, **1 year**, **Forever** or **Custom number of days…** (a whole number from 1 to 36500). When a work's file is no longer found in the library folder (deleted, or moved out of the folder), your data about it is kept for this period and comes back if the file returns — even under a different name or in another subfolder. After the period the data is deleted. The check runs after each successful scan: if the folder is unavailable (a USB stick is unplugged, no access) or contains no files at all, nothing is marked or deleted.
- **About** — the version.

The reader mode is remembered automatically. On Windows, settings are stored in `settings.json` next to the app.

**English** | [Русский](../ru/architecture.md)

# Architecture

MangaReader is a single Go application built with [Fyne](https://fyne.io) for Windows and Android. On Windows the built-in browser is a separate portable Firefox ESR process; on Android it is GeckoView inside the APK with Kotlin screens.

- [Repository map](#repository-map)
- [Packages](#packages)
- [Package boundaries](#package-boundaries)
- [Platforms and build tags](#platforms-and-build-tags)
- [Threads and UI](#threads-and-ui)
- [User data](#user-data)
- [Built-in browser](#built-in-browser)
- [Version, build and CI](#version-build-and-ci)
- [OpenSpec](#openspec)

## Repository map

```text
cmd/mangareader/     entry point; app metadata for Android
internal/            application code (see "Packages")
tools/               build tools: android-build, fetch-firefox, version
android/             Android Gradle project; browser screens in Kotlin
openspec/            specifications and change history (OpenSpec)
docs/                documentation (en, ru, images)
testdata/            example.zip — a sample archive
.github/workflows/   CI and releases
FyneApp.toml         app metadata and the single source of the version
```

## Packages

| Package | Purpose |
|---|---|
| `cmd/mangareader` | Entry point: creates the Fyne app, wires dependencies (`internal/app`), starts the UI shell |
| `internal/app` | Dependency wiring without widgets: library folder, storage, scanner, search index, browser, settings keys |
| `internal/paths` | Application folder on PC (portable mode): next to the exe, the working directory under `go run` |
| `internal/storage` | Access to library files and settings: the file system on PC, Storage Access Framework (JNI) on Android; download source marks (NTFS stream, `Zone.Identifier`) |
| `internal/library` | Folder scanning, zip and `meta.json` parsing, file type detection, change watching (fsnotify), page sizes |
| `internal/model` | Model: `Gallery`, `Tag`, `Key`, the user's tag overlay (`EffectiveTags`, `TagViews`), natural sorting |
| `internal/search` | Search fields, query parsing, `Query`, the `Index` interface and the in-memory index |
| `internal/problems` | The list of library errors and their seen status |
| `internal/thumbs` | Cover thumbnails: background decoding, downscaling, in-memory cache |
| `internal/pages` | Prioritized page loading for the reader and display geometry |
| `internal/browser` | Windows built-in browser: Firefox profile, bridge extension, launching, window attachment, registry cleanup |
| `internal/mobilebrowser` | Bridge to the Android browser (JNI): open, clear data, receive download messages |
| `internal/catalog` | Library catalog in SQLite (`library.db`, FTS5, `sqlite_fts5` tag): scan results across launches, the search index (`search.Index`), covers on disk, a copy of the page links of downloaded files; a recoverable cache |
| `internal/userdata` | User data about works in SQLite (`user.db`): work records with a stable `uid`, schema migrations, backup of a damaged file, reconciliation with scans and moving records by content fingerprint, the user's own and hidden tags, reading progress |
| `internal/i18n` | Interface language: translations from `locales/<lang>.json` (go-i18n), plural forms, date, number and size formats, language choice at startup, Fyne built-in texts in the app language |
| `internal/display` | Refresh rate of the app window on Android (JNI, `DisplayRate.kt`): the 60 Hz limit; a stub on other platforms |
| `internal/appversion` | The version from `FyneApp.toml` and the Android build number |
| `internal/ui` | Shell: window, tabs, toast notifications, wiring screens to services |
| `internal/ui/screens` | Screens: library, search, errors, settings |
| `internal/ui/details` | Gallery page |
| `internal/ui/reader` | Reader: paged mode and strip |
| `internal/archtest` | Tests for architectural boundaries and documentation structure |
| `tools/android-build` | APK build: `libmangareader.so` via the NDK, Fyne Java classes, licenses in `assets/`, Gradle |
| `tools/fetch-firefox` | Downloads and unpacks the pinned Firefox ESR version (SHA-256 checked) |
| `tools/version` | Prints the version and build number for the Makefile and CI |

## Package boundaries

Fyne widgets (`fyne.io/fyne/v2/widget`, `container`) are used **only** in `internal/ui/...`. Service packages (`app`, `library`, `search`, `storage`, `browser` and others) don't depend on widgets and are tested without a window. `internal/archtest` enforces this for `GOOS=windows` and `GOOS=android`.

A new package in `internal/` must be added to the list in [`internal/archtest/imports_test.go`](../../internal/archtest/imports_test.go) and to the table above — in both languages.

## Platforms and build tags

Platform code is split by build tags and file suffixes:

| Tag | Files | Example |
|---|---|---|
| `windows` / `!windows` | `*_windows.go`, `*_other.go` / `stub_other.go` | browser window and registry, NTFS streams |
| `android` / `!android` | `*_android.go`, `other.go`, `*_other.go` | SAF, JNI bridge to GeckoView, refresh rate, app metadata |
| `frameprobe` / `!frameprobe` | `frameprobe_on.go`, `frameprobe_off.go` | frame measurement for development (`make … EXTRA_TAGS=frameprobe`) |

Where a feature doesn't exist, a stub takes over: for example, `internal/browser` on Android reports that the browser is unsupported, and the UI doesn't show the Windows browser settings. A new platform feature always comes with a stub for the other platforms.

Linux is not supported yet.

## Threads and UI

The app is migrated to `fyne.Do` (`[Migrations] fyneDo = true` in `FyneApp.toml`): widgets are changed **only** from the main thread. Scanning, thumbnail and page decoding, search and browser events run in goroutines and hand results to the UI through `fyne.Do`. `make run` starts the app with Fyne thread checks; release builds use the `migrated_fynedo` tag, without checks.

## User data

The app keeps two SQLite files next to each other (next to the exe on PC, in the app's private folder on Android):

| File | Package | Nature |
|---|---|---|
| `library.db` | `internal/catalog` | Cache: rebuilt from the archives. Recreated on a schema version change or damage, cleared when the library folder changes |
| `user.db` | `internal/userdata` | User data (own and hidden tags, reading progress; later groups): cannot be restored from the archives. Never recreated: the schema changes by migrations (`PRAGMA user_version`); a damaged file is renamed to `user.db.broken-<YYYYMMDD-HHMMSS>` and a new one is created; a file from a newer app version is left untouched and user data is unavailable in that run |

A work record (`works`) stores a stable `uid`, the library folder key, the path relative to it, the content fingerprint and the "orphan" time (the file is not found). User data refers to `uid`, not to the path. Records are created only when data about a work is first saved. The folder key is `app:manga` on PC (moving the portable folder keeps the data) and the SAF tree URI on Android.

The fingerprint (`Gallery.Fingerprint`) is a SHA-256 of the page list from the zip directory (path, CRC32, size), computed while parsing the archive without unpacking; `meta.json` and the archive's file name don't affect it.

Reconciliation: `library.Source` notifies an `Observer` after every successful scan (before the list and index are published) and after a delete through the app; the adapter in `internal/app` calls `userdata.Store.Reconcile` in one transaction:

1. records whose file is found lose the orphan mark (a changed fingerprint is updated);
2. a new file takes the record of a missing one if exactly one missing record and exactly one new file share the fingerprint (rename or move);
3. other missing records become orphans — unless the scan found no files at all;
4. orphans older than the retention period (`userdata.retention` setting, 30 days by default, 0 — forever) are deleted along with their data.

A failed scan changes nothing; only records of the current folder are touched. Files that failed to parse or are busy count as found.

### Tag overlay

`Gallery.Tags` holds the tags from `meta.json` and is cached in `library.db` as before. The user's changes are an overlay kept separately: `Gallery.Custom` (own tags in the order they were added) and `Gallery.Hidden` (hidden original tags), both `json:"-"`, so they never reach the scanner cache. Their source is the `user_tags(uid, kind, type, name, seq)` table in `user.db`, removed together with the work record. `EffectiveTags()` is the original tags without hidden ones plus own tags; `TagViews()` returns every tag with its origin for the edit mode. A hidden record for a tag that is no longer in `meta.json` is ignored.

`library.Source` applies the overlay through the `library.Overlay` interface (the adapter in `internal/app` reads `userdata.Store.Overlays` for the current folder key):

- `Scan`: walk → `Observer.Scanned` (reconciliation, moving by fingerprint) → `Overlay.Apply` to all, added and changed galleries → publish and update the index. A file moved by fingerprint comes as added and is indexed with its own tags in the same scan;
- `LoadCatalog`: `Apply` to the galleries from the catalog; the index already stores the overlay;
- `Refresh(k)` after an edit rereads one gallery's overlay, replaces it in the list (copy on write) and upserts it into the index; `Reindex` does the same for all galleries. Both take the scan lock, so a stale scan result can't overwrite an edit; a scan that starts meanwhile waits for them instead of being skipped.

`Services.EditTags` (always off the UI thread) checks the name (empty, over 100 characters, `"` → `ErrTagInvalid`) and duplicates against original tags, hidden ones included, and own tags (`ErrTagExists`), then calls `userdata.Ensure`, the operation and `Source.Refresh`. The gallery page talks to it through the `details.TagEditor` interface implemented in `Shell`; after a successful edit the current search is repeated.

In the search index a tag has an origin: `tags.src` is 0 for a visible original, 1 for an own tag, 2 for a hidden one. Words (`docs_fts.hay`) and suggestions use only 0 and 1. `search.Value.Scope` picks the area of a tag filter: effective tags for `tag:` and the type fields, own ones for `custom-tag:`, hidden ones for `hidden-tag:`. `MemIndex` keeps the same three sets and passes the same `search/indextest` checks.

The index in `library.db` stores the result of the overlay, so it must match `user.db`. `user.db` gets a random epoch when it is created (`meta.epoch`); `library.db` remembers the epoch its index was built with (`meta.overlay_epoch`, `none` without user data). At startup, if they differ (`user.db` restored from a backup, recreated after damage, unavailable), the library is reindexed in the background after `LoadCatalog` and the epoch is saved. Rebuilding the index from the scanner cache and changing the library folder reset the saved epoch.

Reading progress is the `progress(uid, page, page_index, total, finished, updated_at)` table in `user.db`, one row per work: the page's file name in the archive, its number and the page count when it was saved, and the "finished" mark. `app.ProgressService` keeps the positions of the current folder in memory (the Continue button and the menu read them on the UI thread) and writes to the database from a single background goroutine in order, so "finished" is never overwritten by an earlier position; positions are reloaded after reconciling with a scan and after changing the folder. `app.Resume` picks the page for Continue: by name first, then by number (the position may be inaccurate); for a finished work — the first page after the last one read, if it has appeared. The shell saves the position a second after a page change, when the reader closes and when the app goes to the background.

## Localization

User-visible texts live in `internal/i18n/locales/<lang>.json` (English is the fallback) and are taken with `i18n.T` / `i18n.N` in `internal/ui` only; a test fails on a string literal with Cyrillic in UI code, another one on a key missing from any language. The language is chosen once at startup (`ui.language` setting, otherwise the system language, otherwise English) before the screens are built; a change applies after a restart. Service packages don't know the language: their errors carry a kind and parameters, `err.Error()` is English technical text for the log, and the UI translates known kinds (`screens.ErrorText`, `screens.ProblemText`). Logs are in English. Fyne's own texts are switched to the app language by loading its translations under the system locale tag (`i18n.ApplyToFyne`). The Android browser gets the language in its settings and takes its strings from `res/values*/strings.xml` in that language.

A new language: `internal/i18n/locales/<lang>.json`, a copy of Fyne's `base.<lang>.json` in `internal/i18n/fyne/`, `android/app/src/main/res/values-<lang>/strings.xml`.

## Built-in browser

### Windows

```text
MangaReader ──launch──▶ firefox.exe -profile browser\profile
     ▲                        │
     │  127.0.0.1 + token     │ bridge@mangareader.app extension
     └────────────────────────┘ (download page address, commands)
```

- `tools/fetch-firefox` unpacks the Firefox ESR installer (`/ExtractDir`) into `browser\firefox`; the version and SHA-256 are pinned in code.
- On every start the app rewrites `browser\profile\user.js` (downloads folder = library folder, updates and telemetry off, dark theme) and rebuilds the `bridge@mangareader.app.xpi` extension from `internal/browser/ext`.
- The extension tells the app the address of the page a download started from, over `127.0.0.1` with a one-time token; the app writes it to the file's `:mangareader.source` NTFS stream.
- The Firefox window is found by process and gets the app window as its owner (Win32): on top of the app, minimized with it, no taskbar button.
- After the browser closes, Firefox's service values under `HKCU\Software\Mozilla\Firefox` that belong to the built-in instance are removed.

### Android

- GeckoView is added in `android/app/build.gradle.kts`; browser screens are Kotlin in `android/app/src/main/kotlin/io/github/mangareader/app/browser/` (`BrowserActivity`, `BrowserEngine`, `DownloadManager`, `DownloadService` and others).
- Go calls Kotlin and receives events via JNI (`internal/mobilebrowser`, `GoBridge.kt`): open the browser, clear data, report a download (path and page address).
- Downloads are written to the chosen SAF folder by `DownloadService` (a foreground service with a notification), as `name.part` while downloading.

## Version, build and CI

- The version lives only in `FyneApp.toml`; `internal/appversion` validates the format and computes `versionCode`. Details: [building.md](building.md#version).
- The `Makefile` wraps `go build`, `tools/android-build` and `tools/fetch-firefox`; it works from PowerShell, cmd and Git Bash (on Windows, commands run in `cmd.exe`).
- CI and releases: [`.github/workflows/`](../../.github/workflows/); release steps and keys: [building.md](building.md#releasing).
- Third-party components shipped in the packages are listed in [`THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md).

## OpenSpec

Requirements and decision history live in [`openspec/`](../../openspec/) ([OpenSpec](https://github.com/Fission-AI/OpenSpec)):

- `openspec/specs/<capability>/spec.md` — current requirements (what the app must do), with scenarios;
- `openspec/changes/archive/` — completed changes: proposal, design with alternatives, tasks;
- `openspec/config.yaml` — project context and rules for new changes (read by people and AI assistants alike).

How to work on a change: see [CONTRIBUTING](../../CONTRIBUTING.md).

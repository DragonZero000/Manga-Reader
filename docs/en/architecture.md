**English** | [Русский](../ru/architecture.md)

# Architecture

MangaReader is a single Go application built with [Fyne](https://fyne.io) for Windows and Android. On Windows the built-in browser is a separate portable Firefox ESR process; on Android it is GeckoView inside the APK with Kotlin screens.

- [Repository map](#repository-map)
- [Packages](#packages)
- [Package boundaries](#package-boundaries)
- [Platforms and build tags](#platforms-and-build-tags)
- [Threads and UI](#threads-and-ui)
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
| `internal/model` | Model: `Gallery`, `Tag`, `Key`, natural sorting |
| `internal/search` | Search fields, query parsing, `Query`, the `Index` interface and the in-memory index |
| `internal/problems` | The list of library errors and their seen status |
| `internal/thumbs` | Cover thumbnails: background decoding, downscaling, in-memory cache |
| `internal/pages` | Prioritized page loading for the reader and display geometry |
| `internal/browser` | Windows built-in browser: Firefox profile, bridge extension, launching, window attachment, registry cleanup |
| `internal/mobilebrowser` | Bridge to the Android browser (JNI): open, clear data, receive download messages |
| `internal/catalog` | Library catalog in SQLite (`library.db`, FTS5, `sqlite_fts5` tag): scan results across launches, the search index (`search.Index`), covers on disk, a copy of the page links of downloaded files; a recoverable cache |
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

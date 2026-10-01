**English** | [Русский](README.ru.md)

# MangaReader

[![CI](https://github.com/DragonZero000/Manga-Reader/actions/workflows/ci.yml/badge.svg)](https://github.com/DragonZero000/Manga-Reader/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/DragonZero000/Manga-Reader)](https://github.com/DragonZero000/Manga-Reader/releases/latest)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

An offline manga reader for zip archives on **Windows** and **Android**, with a built-in Firefox-based browser: archives you download in it go straight into your library. Written in Go with [Fyne](https://fyne.io).

> **The interface is in English and Russian.** It follows the system language by default (English if there is no translation for it); pick the language in **Settings** → **Язык / Language**.

<p align="center">
  <img src="docs/images/library.png" alt="Library on Windows" height="240">
  <img src="docs/images/reader.png" alt="Reader on Windows" height="240">
  <img src="docs/images/mobile-library.png" alt="Library on Android" height="240">
</p>

## Features

- **Library** — a grid of covers from a folder of `.zip` archives; new files show up on their own, no restart needed. Pick the card size (Windows) or the number of cards per row (Android); the 🎲 button opens a random gallery.
- **Card menu** — **⋮** on a card, right-click (PC) or long press (phone): open, read, open in browser, copy title, show in folder (Windows), reset progress, delete — to the Recycle Bin on Windows, after a confirmation.
- **Metadata** — titles, tags grouped by type (artist, group, character, language…), upload date and more from a `meta.json` inside the archive; add your own tags and hide unwanted ones without touching the archive.
- **Reader** — paged mode (tap zones, swipe, keyboard, wheel) and a continuous strip; zoom up to 500% (Ctrl+wheel on PC, pinch on Android, double tap); jump to a page by number; right-to-left reading for manga.
- **Reading progress** — the app remembers where you stopped: **Continue · p. N** on the gallery page, a "finished" mark, and new pages added to the archive are picked up.
- **Search** — by words and phrases, tags (including your own and hidden ones), excluded tags, page count, dates, file size; runs while you type or on a button; tapping a tag searches for it; 🎲 picks a random result.
- **Built-in browser** — Firefox ESR on Windows, GeckoView on Android, in the app's language; downloads are saved right into the library, and the app remembers the page each file came from.
- **Errors** — anything that isn't an archive of images (HTML instead of an archive, a broken zip, a PDF…) is listed on a separate tab with the reason; files are never deleted.
- **Portable on Windows** — everything lives in one folder you can carry on a USB stick.

## Download

Ready-made builds are on the [Releases](https://github.com/DragonZero000/Manga-Reader/releases/latest) page:

| File | For |
|---|---|
| `MangaReader-X.Y.Z-windows-x64.zip` | Windows 10/11, 64-bit (~160 MB, the built-in browser is included) |
| `MangaReader-X.Y.Z-android-arm64.apk` | Android 11+ (~140 MB) |
| `SHA256SUMS.txt` | Checksums: `sha256sum -c SHA256SUMS.txt` (Linux, Git Bash) or `Get-FileHash <file>` (PowerShell) |

### Installing on Windows

1. Unpack the zip anywhere you have write access (not `Program Files`), e.g. `D:\MangaReader`.
2. Run `MangaReader\mangareader.exe`.
3. Put `.zip` archives into the `manga` folder next to the app, or download them with the **Browser** button.

To update, replace `mangareader.exe` and the `browser\firefox` folder with the ones from the new zip; `manga`, `settings.json`, `library.db`, `user.db` and `browser\profile` are kept.

### Installing on Android

1. Download the APK on your phone and open it; allow installing from that source when Android asks.
2. On first start, pick a library folder inside `Download`, e.g. `Download/manga`.

New releases install **over** old ones: settings and the chosen folder are kept. If you previously installed an APK you built yourself, its signature differs — uninstall it once before installing a release (your manga files are not affected).

## System requirements

| Platform | Requirements |
|---|---|
| Windows | Windows 10 or 11, x64; ~400 MB of disk space (including the built-in Firefox) |
| Android | Android 11 or newer, arm64; ~300 MB after install |
| Linux, macOS | Not supported yet |

## Documentation

- [User guide](docs/en/user-guide.md) — library folder, archive format and `meta.json`, card menu and deleting, tag editing, reader and reading progress, search syntax, browser, errors, settings.
- [Building and releases](docs/en/building.md) — requirements, commands, Android, releasing, the signing key.
- [Architecture](docs/en/architecture.md) — packages, platforms, threads, how the browser works.

## For developers

```sh
make run                    # run in development mode (needs Go 1.26+ and gcc)
make test                   # go vet + go test (including the documentation check)
make build-windows          # dist/mangareader.exe
make package-windows        # dist/MangaReader/ with the browser and licenses, and dist/MangaReader.zip
make build-android          # dist/mangareader.apk (debug, arm64; needs Android Studio and NDK 27+)
make build-android-release  # dist/mangareader-release.apk (signing key: see building.md)
make browser                # portable Firefox ESR in browser/firefox (for make run, Windows only)
make clean                  # delete dist/
```

Details are in [building.md](docs/en/building.md). How to propose a change: [CONTRIBUTING.md](CONTRIBUTING.md).

## License

The project's code is [MIT](LICENSE). Third-party components (Firefox ESR, GeckoView, Fyne and others) are distributed under their own licenses — see [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). Firefox is a trademark of the Mozilla Foundation; this project is not affiliated with Mozilla.

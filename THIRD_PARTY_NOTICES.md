# Third-party notices

MangaReader's own code is licensed under the [MIT License](LICENSE). The releases also contain third-party components, listed below with their licenses. Each component remains under its own license.

## Embedded browser engine (Mozilla)

MangaReader includes unmodified builds of Mozilla software. They are licensed under the [Mozilla Public License 2.0](https://www.mozilla.org/MPL/2.0/). The MPL-2.0 applies only to Mozilla's files, not to MangaReader's code. You can get the source code of the exact versions we distribute from the links below.

| Component | Distributed in | Version (pinned in) | License | Source code |
|---|---|---|---|---|
| Firefox ESR | Windows package (`browser\firefox`) | `153.3.0esr` ([`tools/fetch-firefox/main.go`](tools/fetch-firefox/main.go)) | MPL-2.0 | [archive.mozilla.org/pub/firefox/releases/153.3.0esr/source/](https://archive.mozilla.org/pub/firefox/releases/153.3.0esr/source/) |
| GeckoView | Android APK | `156.0.20260921121718` ([`android/app/build.gradle.kts`](android/app/build.gradle.kts)) | MPL-2.0 | [hg.mozilla.org/releases/mozilla-release/rev/6f2c158dfc7e](https://hg.mozilla.org/releases/mozilla-release/rev/6f2c158dfc7e9693f880fad2510ceb51a158c069) (revision from the Maven POM), release archive [archive.mozilla.org/pub/firefox/releases/156.0/source/](https://archive.mozilla.org/pub/firefox/releases/156.0/source/) |

The Firefox ESR and GeckoView builds may themselves include third-party code. Their licenses are listed in `about:license` inside the browser and in the source archives above.

**Trademarks.** Firefox and the Firefox logo are trademarks of the Mozilla Foundation in the U.S. and other countries. MangaReader is an independent project and is not affiliated with, sponsored by or endorsed by Mozilla.

## Go modules (compiled into the application)

| Module | Version | License |
|---|---|---|
| [Go standard library](https://go.dev/LICENSE) | toolchain from `go.mod` | BSD-3-Clause |
| [fyne.io/fyne/v2](https://github.com/fyne-io/fyne) | v2.8.0 | BSD-3-Clause |
| [fyne.io/systray](https://github.com/fyne-io/systray) | v1.12.2 | Apache-2.0 |
| [github.com/fsnotify/fsnotify](https://github.com/fsnotify/fsnotify) | v1.9.0 | BSD-3-Clause |
| [golang.org/x/image](https://cs.opensource.google/go/x/image) | v0.24.0 | BSD-3-Clause |
| [golang.org/x/net](https://cs.opensource.google/go/x/net) | v0.35.0 | BSD-3-Clause |
| [golang.org/x/sys](https://cs.opensource.google/go/x/sys) | v0.30.0 | BSD-3-Clause |
| [golang.org/x/text](https://cs.opensource.google/go/x/text) | v0.22.0 | BSD-3-Clause |
| [github.com/go-gl/glfw](https://github.com/go-gl/glfw) (with bundled GLFW) | v3.4 | BSD-3-Clause (bindings), zlib (GLFW) |
| [github.com/go-gl/gl](https://github.com/go-gl/gl) | 2026-03-31 | MIT |
| [github.com/go-text/render](https://github.com/go-text/render) | v0.2.1 | BSD-3-Clause |
| [github.com/go-text/typesetting](https://github.com/go-text/typesetting) | v0.3.4 | BSD-3-Clause |
| [github.com/fyne-io/image](https://github.com/fyne-io/image) | v0.1.1 | BSD-3-Clause |
| [github.com/fyne-io/oksvg](https://github.com/fyne-io/oksvg) | v0.2.0 | BSD-3-Clause |
| [github.com/srwiley/oksvg](https://github.com/srwiley/oksvg) | 2022-10-11 | BSD-3-Clause |
| [github.com/srwiley/rasterx](https://github.com/srwiley/rasterx) | 2022-07-30 | BSD-3-Clause |
| [github.com/FyshOS/fancyfs](https://github.com/FyshOS/fancyfs) | v0.0.1 | BSD-3-Clause |
| [github.com/godbus/dbus/v5](https://github.com/godbus/dbus) | v5.2.2 | BSD-2-Clause |
| [github.com/BurntSushi/toml](https://github.com/BurntSushi/toml) | v1.6.0 | MIT |
| [github.com/anthonynsimon/bild](https://github.com/anthonynsimon/bild) | v0.14.0 | MIT |
| [github.com/clipperhouse/uax29/v2](https://github.com/clipperhouse/uax29) | v2.2.0 | MIT |
| [github.com/fredbi/uri](https://github.com/fredbi/uri) | v1.1.1 | MIT |
| [github.com/jeandeaual/go-locale](https://github.com/jeandeaual/go-locale) | 2025-06-12 | MIT |
| [github.com/jsummers/gobmp](https://github.com/jsummers/gobmp) | 2023-06-14 | MIT |
| [github.com/mattn/go-runewidth](https://github.com/mattn/go-runewidth) | v0.0.24 | MIT |
| [github.com/nicksnyder/go-i18n/v2](https://github.com/nicksnyder/go-i18n) | v2.5.1 | MIT |
| [github.com/yuin/goldmark](https://github.com/yuin/goldmark) | v1.8.2 | MIT |
| [github.com/nfnt/resize](https://github.com/nfnt/resize) | 2018-02-21 | ISC |

Exact versions are in [`go.mod`](go.mod). The full license texts ship with each module and are available at the links above.

## Android libraries (APK)

| Component | License |
|---|---|
| [Kotlin standard library](https://github.com/JetBrains/kotlin) (Kotlin 2.4.10, [`android/build.gradle.kts`](android/build.gradle.kts)) | Apache-2.0 |
| [AndroidX](https://developer.android.com/jetpack/androidx) libraries (transitive dependencies of GeckoView) | Apache-2.0 |

Apache-2.0 text: <https://www.apache.org/licenses/LICENSE-2.0>.

## Keeping this file up to date

When you change a distributed component (for example, the Firefox ESR version in `tools/fetch-firefox`, `geckoviewVersion`, or modules in `go.mod`), update its version, license and source link here. The list of Go modules compiled into the app is produced by:

```sh
GOOS=windows CGO_ENABLED=1 go list -deps -f '{{with .Module}}{{.Path}} {{.Version}}{{end}}' ./cmd/mangareader | sort -u
GOOS=android CGO_ENABLED=1 go list -deps -f '{{with .Module}}{{.Path}} {{.Version}}{{end}}' ./cmd/mangareader | sort -u
```

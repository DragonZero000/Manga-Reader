**English** | [Русский](../ru/building.md)

# Building and releases

- [Requirements](#requirements)
- [Commands](#commands)
- [Android build](#android-build)
- [Version](#version)
- [Built-in browser during development](#built-in-browser-during-development)
- [CI](#ci)
- [Releasing](#releasing)
- [APK signing key and GitHub secrets](#apk-signing-key-and-github-secrets)

## Requirements

- **Go 1.26+**.
- **Windows:** gcc (for example, [MinGW-w64](https://www.mingw-w64.org/) or TDM-GCC) — Fyne uses cgo; GNU make (`winget install GnuWin32.Make` or `choco install make`).
- **Android:** [Android Studio](https://developer.android.com/studio) (its bundled JDK, JBR, is used), and in Android Studio → Settings → Android SDK:
  - *SDK Tools*: NDK (Side by side) **27** or newer, Android SDK Command-line Tools;
  - *SDK Platforms* don't have to be installed manually: Gradle downloads missing platforms and Build-Tools itself.

  Kotlin and Gradle don't need a separate install — the Gradle Wrapper downloads them. Phone: Android 11+ (arm64).

Linux and macOS are not supported as app platforms yet; the APK can be built on Linux (CI does that).

## Commands

Run from the project root; they work from PowerShell, cmd and Git Bash:

```sh
make run                    # run in development mode (Fyne thread checks enabled)
make test                   # go vet + go test (including the documentation check)
make build-windows          # dist/mangareader.exe
make package-windows        # dist/MangaReader/ with the browser and licenses, and dist/MangaReader.zip
make build-android          # dist/mangareader.apk (debug, arm64)
make build-android-release  # dist/mangareader-release.apk (key: see below)
make browser                # portable Firefox ESR in browser/firefox (for make run, Windows only)
make clean                  # delete dist/
```

With `make run` (`go run`) the working directory is treated as the application folder: `manga/`, `settings.json` and `browser/` appear in the project root (they are in `.gitignore`).

> `make package-windows` does not clean `dist/MangaReader/`. If you ran `mangareader.exe` from there, your `manga/` and `browser/profile/` (cookies, history) end up in the zip. For distribution use the zip from [Releases](https://github.com/DragonZero000/Manga-Reader/releases), or delete `dist/` before building (`make clean`).

## Android build

`make build-android` (same as `go run ./tools/android-build`) compiles the Go part into `libmangareader.so` with the NDK compiler, takes Fyne's Java classes from the module version in `go.mod`, puts `LICENSE` and `THIRD_PARTY_NOTICES.md` into `assets/`, and builds the APK with the Gradle project in `android/`.

- **SDK** — `ANDROID_HOME` (Android Studio sets it). **NDK** — `ANDROID_NDK_HOME`, or the newest version ≥ 27 in `%ANDROID_HOME%\ndk`. **JDK** — `JAVA_HOME` if it is JDK 17+, otherwise JBR from Android Studio.
- **Architectures** — `ANDROID_ABIS` (default `arm64-v8a`): `make build-android ANDROID_ABIS=arm64-v8a,armeabi-v7a`. Available: `arm64-v8a`, `armeabi-v7a`, `x86_64`, `x86`; each adds ~25 MB to the APK.
- **Signing.** A debug build is signed with this computer's Android SDK debug key — new builds install over old ones (`adb install -r dist/mangareader.apk`) keeping settings. A release build is signed with the key from `MANGAREADER_KEYSTORE` (path to the `.jks`), `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD`; without them the APK is unsigned. `go run ./tools/android-build -release -require-signing` refuses to build without a key (that's how CI builds).
- APKs signed with different keys (your debug key and the project's releases) can't be installed over each other: switch once by uninstalling the app. Manga files are not affected.
- The library is aligned to 16 KB memory pages (otherwise Android 15+ warns about incompatibility).

## Version

The version is set **only** in `FyneApp.toml`, field `Version`, as `MAJOR.MINOR.PATCH` (no suffixes, `MINOR` and `PATCH` ≤ 99). All builds use it; `make VERSION=…` has no effect. The Android build number is computed: `MAJOR*10000 + MINOR*100 + PATCH` (`0.2.0` → `200`), so a new version always installs over the previous one. `go run ./tools/version` prints the version, `-code` prints the build number.

## Built-in browser during development

`make browser` downloads Firefox ESR into `browser\firefox` once (the version and SHA-256 are pinned in `tools/fetch-firefox`); without it the **Браузер** (Browser) button shows "Браузер не найден" (Browser not found). The installer is cached in `browser\.cache`. Windows only.

The browser integration test runs when `MANGAREADER_BROWSER_IT` is set to the Firefox folder; otherwise it is skipped.

## CI

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) — on every push and pull request: `go vet` and `go test` on Windows, a debug APK build on Ubuntu. No secrets needed; works for forks too.
- [`.github/workflows/release.yml`](../../.github/workflows/release.yml) — release build for a `vX.Y.Z` tag.
- [`.github/dependabot.yml`](../../.github/dependabot.yml) — monthly GitHub Actions updates.

## Releasing

1. Bump `Version` in `FyneApp.toml`, commit, push.
2. Push a tag with the same version: `git tag v0.2.0` and `git push origin v0.2.0`.
3. `release.yml` checks that the tag matches `FyneApp.toml`, runs the tests, builds `MangaReader-X.Y.Z-windows-x64.zip` (with Firefox) and a signed `MangaReader-X.Y.Z-android-arm64.apk`, computes `SHA256SUMS.txt` and creates a **draft** release.
4. Review the draft and the notes (GitHub generates them from changes since the previous tag) and click *Publish release*.

If the build failed because of a code problem: fix it, commit, and move the tag to the new commit (`git tag -f vX.Y.Z` and `git push -f origin vX.Y.Z`) — safe as long as the release isn't published. Failed steps that need no code change (for example, after fixing secrets) can be rerun with *Re-run failed jobs*.

## APK signing key and GitHub secrets

All releases are signed with **one** key — only then do new versions install over installed ones. This is set up once. Commands are for PowerShell; run them in a folder **outside the repository**, for example `C:\Users\<you>\Keys\`.

### 1. Create the key

```powershell
keytool -genkeypair -v -keystore mangareader-release.jks -alias mangareader -keyalg RSA -keysize 4096 -validity 10000
```

`keytool` comes with the JDK and with Android Studio (`<Android Studio>\jbr\bin\keytool.exe`).

| Prompt | Answer |
|---|---|
| `Enter keystore password` | A password: **Latin letters and digits only, no spaces**, at least 6 characters |
| `Re-enter new password` | The same password |
| `What is your first and last name?` and the rest | Anything. **This is visible to everyone** who downloads the APK (`apksigner verify --print-certs`) — don't enter personal data, e.g. `CN=MangaReader` |
| `Is CN=... correct?` | `yes` |

`keytool` won't ask for a separate key password: in the PKCS12 format (the default) it equals the keystore password.

### 2. Check the key and get the fingerprint

```powershell
keytool -list -v -keystore mangareader-release.jks
```

You need two lines from the output:

```text
Alias name: mangareader        ← ANDROID_KEY_ALIAS
SHA256: 3A:7F:0C:…:9E          ← ANDROID_CERT_SHA256
```

If it says `password was incorrect`, the password is wrong; it's easier to create a new key while there are no releases.

### 3. What goes into each secret

| Name | Type | Value | How to check |
|---|---|---|---|
| `ANDROID_KEYSTORE_BASE64` | Secret | The `mangareader-release.jks` file in base64, **on one line** | ~3–4 thousand characters, starts with `MII` |
| `ANDROID_KEYSTORE_PASSWORD` | Secret | The password from step 1 | Exactly those characters, no surrounding spaces |
| `ANDROID_KEY_ALIAS` | Secret | `mangareader` | The `Alias name` line from step 2 |
| `ANDROID_KEY_PASSWORD` | Secret | The same password as `ANDROID_KEYSTORE_PASSWORD` | Equal for PKCS12 |
| `ANDROID_CERT_SHA256` | **Variable** | The `SHA256:` line from step 2 | Colons and case don't matter |

### 4. Set the secrets

**With GitHub CLI (recommended)** — values aren't copied by hand, so no stray spaces or line breaks. Run `gh auth login` once, then:

```powershell
$repo = "DragonZero000/Manga-Reader"
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$PWD\mangareader-release.jks")) | gh secret set ANDROID_KEYSTORE_BASE64 --repo $repo
gh secret set ANDROID_KEYSTORE_PASSWORD --repo $repo   # prompts for the value
gh secret set ANDROID_KEY_PASSWORD --repo $repo        # prompts for the value
gh secret set ANDROID_KEY_ALIAS --body mangareader --repo $repo
gh variable set ANDROID_CERT_SHA256 --body "3A:7F:0C:…:9E" --repo $repo
```

On Linux and in Git Bash, base64: `base64 -w0 mangareader-release.jks | gh secret set ANDROID_KEYSTORE_BASE64 --repo "$repo"`.

**On the website:** Settings → Secrets and variables → Actions → **Secrets** tab (*New repository secret*) for the four secrets, and the **Variables** tab for `ANDROID_CERT_SHA256`. Base64 can be put on the clipboard:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$PWD\mangareader-release.jks")) | Set-Clipboard
```

**Don't use** `certutil -encode` (it adds header lines and corrupts the file) or online converters.

If `ANDROID_CERT_SHA256` is not set, the first release build stops and prints the fingerprint in the log — save it to the variable and rerun. From then on CI won't release an APK signed with a different key.

### 5. Verify

The "Ключ подписи" (Signing key) step in the build log prints the key file's SHA-256. Compare it with `Get-FileHash mangareader-release.jks` (case doesn't matter):

- matches and the step is green — all good;
- matches, but "не удалось открыть ключ" (couldn't open the key) — `ANDROID_KEYSTORE_PASSWORD` is wrong;
- doesn't match — `ANDROID_KEYSTORE_BASE64` is corrupted; set it again with `gh`.

### 6. Backup — mandatory

Keep **the `mangareader-release.jks` file and its password** in two places off this computer (password manager, USB stick, cloud). They can't be read back from GitHub Secrets. **A lost key can't be recovered**: new versions will stop installing over existing ones for every user. `*.jks` and `*.keystore` are in `.gitignore`.

### Release APK locally

Set `MANGAREADER_KEYSTORE` (path to the `.jks`), `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD` and run `make build-android-release`.

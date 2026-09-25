[English](../en/building.md) | **Русский**

# Сборка и релизы

- [Требования](#требования)
- [Команды](#команды)
- [Сборка для Android](#сборка-для-android)
- [Версия](#версия)
- [Встроенный браузер при разработке](#встроенный-браузер-при-разработке)
- [CI](#ci)
- [Выпуск релиза](#выпуск-релиза)
- [Ключ подписи APK и секреты GitHub](#ключ-подписи-apk-и-секреты-github)

## Требования

- **Go 1.26+**.
- **Windows:** gcc (например, [MinGW-w64](https://www.mingw-w64.org/) или TDM-GCC) — Fyne использует cgo; GNU make (`winget install GnuWin32.Make` или `choco install make`).
- **Android:** [Android Studio](https://developer.android.com/studio) (из неё берётся JDK — JBR), а в Android Studio → Settings → Android SDK:
  - *SDK Tools*: NDK (Side by side) **27** или новее, Android SDK Command-line Tools;
  - *SDK Platforms* ставить вручную не обязательно: недостающие платформы и Build-Tools Gradle скачивает сам.

  Kotlin и Gradle отдельно ставить не нужно — их скачивает Gradle Wrapper. Телефон — Android 11+ (arm64).

Linux и macOS как платформы приложения пока не поддерживаются; собрать APK на Linux можно (так делает CI).

## Команды

Запускаются из корня проекта и работают из PowerShell, cmd и Git Bash:

```sh
make run                    # запуск в режиме разработки (проверки потоков Fyne включены)
make test                   # go vet + go test (включая проверку документации)
make build-windows          # dist/mangareader.exe
make package-windows        # dist/MangaReader/ с браузером и лицензиями и dist/MangaReader.zip
make build-android          # dist/mangareader.apk (debug, arm64)
make build-android-release  # dist/mangareader-release.apk (ключ — см. ниже)
make browser                # портативный Firefox ESR в browser/firefox (для make run, только Windows)
make clean                  # удалить dist/
```

При запуске через `make run` (`go run`) папкой приложения считается рабочая папка: `manga/`, `settings.json` и `browser/` появятся в корне проекта (они в `.gitignore`).

> `make package-windows` не очищает `dist/MangaReader/`. Если вы запускали `mangareader.exe` оттуда, в zip попадут ваши `manga/` и `browser/profile/` (cookies, история). Для раздачи используйте zip из [Releases](https://github.com/DragonZero000/Manga-Reader/releases) или удалите `dist/` перед сборкой (`make clean`).

## Сборка для Android

`make build-android` (он же `go run ./tools/android-build`) собирает Go-часть в `libmangareader.so` компилятором из NDK, берёт Java-классы Fyne из модуля той версии, что в `go.mod`, кладёт `LICENSE` и `THIRD_PARTY_NOTICES.md` в `assets/` и собирает APK Gradle-проектом из папки `android/`.

- **SDK** — `ANDROID_HOME` (Android Studio задаёт его сама). **NDK** — `ANDROID_NDK_HOME` или самая новая версия ≥ 27 из `%ANDROID_HOME%\ndk`. **JDK** — `JAVA_HOME`, если это JDK 17+, иначе JBR из Android Studio.
- **Архитектуры** — `ANDROID_ABIS` (по умолчанию `arm64-v8a`): `make build-android ANDROID_ABIS=arm64-v8a,armeabi-v7a`. Доступны `arm64-v8a`, `armeabi-v7a`, `x86_64`, `x86`; каждая добавляет к APK ~25 МБ.
- **Подпись.** Debug-сборка подписывается отладочным ключом Android SDK этого компьютера — новые сборки ставятся поверх (`adb install -r dist/mangareader.apk`) без потери настроек. Release-сборка подписывается ключом из переменных `MANGAREADER_KEYSTORE` (путь к `.jks`), `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD`; без них APK не подписан. Флаг `go run ./tools/android-build -release -require-signing` запрещает сборку без ключа (так собирает CI).
- APK, подписанные разными ключами (ваш debug и релизы проекта), поверх друг друга не ставятся: переход — один раз через удаление приложения. Файлы манги при этом не затрагиваются.
- Библиотека выравнивается по страницам памяти 16 КБ (иначе Android 15+ предупреждает о несовместимости).

## Версия

Версия задаётся **только** в `FyneApp.toml`, поле `Version`, в формате `MAJOR.MINOR.PATCH` (без суффиксов, `MINOR` и `PATCH` ≤ 99). Её берут все сборки; `make VERSION=…` не действует. Номер сборки Android вычисляется: `MAJOR*10000 + MINOR*100 + PATCH` (`0.2.0` → `200`), поэтому новая версия всегда ставится поверх предыдущей. `go run ./tools/version` печатает версию, `-code` — номер сборки.

## Встроенный браузер при разработке

`make browser` один раз скачивает Firefox ESR в `browser\firefox` (версия и SHA-256 закреплены в `tools/fetch-firefox`); без него кнопка **Браузер** показывает «Браузер не найден». Установщик кэшируется в `browser\.cache`. Работает только на Windows.

Интеграционный тест браузера запускается, если задать `MANGAREADER_BROWSER_IT` — путь к папке Firefox; без переменной он пропускается.

## CI

- [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) — на каждый push и pull request: `go vet` и `go test` на Windows, сборка debug-APK на Ubuntu. Секреты не нужны, работает и для форков.
- [`.github/workflows/release.yml`](../../.github/workflows/release.yml) — сборка релиза по тегу `vX.Y.Z`.
- [`.github/dependabot.yml`](../../.github/dependabot.yml) — ежемесячные обновления GitHub Actions.

## Выпуск релиза

1. Поднимите `Version` в `FyneApp.toml`, закоммитьте, запушьте.
2. Отправьте тег той же версии: `git tag v0.2.0` и `git push origin v0.2.0`.
3. `release.yml` проверит, что тег совпадает с `FyneApp.toml`, прогонит тесты, соберёт `MangaReader-X.Y.Z-windows-x64.zip` (с Firefox) и подписанный `MangaReader-X.Y.Z-android-arm64.apk`, посчитает `SHA256SUMS.txt` и создаст **черновик** релиза.
4. Проверьте черновик и заметки (GitHub генерирует их из изменений с прошлого тега) и нажмите *Publish release*.

Если сборка упала на проблеме в коде: исправьте, закоммитьте и перенесите тег на новый коммит (`git tag -f vX.Y.Z` и `git push -f origin vX.Y.Z`) — это безопасно, пока релиз не опубликован. Упавшие шаги без изменений в коде (например, после исправления секретов) можно перезапустить кнопкой *Re-run failed jobs*.

## Ключ подписи APK и секреты GitHub

Все релизы подписываются **одним** ключом — только тогда новые версии ставятся поверх установленных. Настраивается один раз. Команды — для PowerShell; выполняйте их в папке **вне репозитория**, например `C:\Users\<вы>\Keys\`.

### 1. Создать ключ

```powershell
keytool -genkeypair -v -keystore mangareader-release.jks -alias mangareader -keyalg RSA -keysize 4096 -validity 10000
```

`keytool` есть в JDK и в Android Studio (`<Android Studio>\jbr\bin\keytool.exe`).

| Вопрос | Ответ |
|---|---|
| `Enter keystore password` | Пароль: **только латиница и цифры, без пробелов**, от 6 символов |
| `Re-enter new password` | Тот же пароль |
| `What is your first and last name?` и остальные | Что угодно. **Эти данные видны всем**, кто скачает APK (`apksigner verify --print-certs`) — не пишите личное, например `CN=MangaReader` |
| `Is CN=... correct?` | `yes` |

Отдельный пароль ключа `keytool` не спросит: в формате PKCS12 (по умолчанию) он совпадает с паролем хранилища.

### 2. Проверить ключ и получить отпечаток

```powershell
keytool -list -v -keystore mangareader-release.jks
```

В выводе нужны две строки:

```text
Alias name: mangareader        ← ANDROID_KEY_ALIAS
SHA256: 3A:7F:0C:…:9E          ← ANDROID_CERT_SHA256
```

Если пишет `password was incorrect` — пароль не тот; проще создать ключ заново, пока нет релизов.

### 3. Что класть в секреты

| Имя | Тип | Значение | Как проверить |
|---|---|---|---|
| `ANDROID_KEYSTORE_BASE64` | Secret | Файл `mangareader-release.jks` в base64, **одной строкой** | ~3–4 тыс. символов, начинается с `MII` |
| `ANDROID_KEYSTORE_PASSWORD` | Secret | Пароль из шага 1 | Ровно те символы, без пробелов по краям |
| `ANDROID_KEY_ALIAS` | Secret | `mangareader` | Строка `Alias name` из шага 2 |
| `ANDROID_KEY_PASSWORD` | Secret | Тот же пароль, что `ANDROID_KEYSTORE_PASSWORD` | Для PKCS12 они совпадают |
| `ANDROID_CERT_SHA256` | **Variable** | Строка `SHA256:` из шага 2 | Двоеточия и регистр не важны |

### 4. Записать секреты

**Через GitHub CLI (рекомендуется)** — значения не копируются вручную, лишние пробелы и переводы строк не попадут. Один раз `gh auth login`, затем:

```powershell
$repo = "DragonZero000/Manga-Reader"
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$PWD\mangareader-release.jks")) | gh secret set ANDROID_KEYSTORE_BASE64 --repo $repo
gh secret set ANDROID_KEYSTORE_PASSWORD --repo $repo   # спросит значение
gh secret set ANDROID_KEY_PASSWORD --repo $repo        # спросит значение
gh secret set ANDROID_KEY_ALIAS --body mangareader --repo $repo
gh variable set ANDROID_CERT_SHA256 --body "3A:7F:0C:…:9E" --repo $repo
```

На Linux и в Git Bash base64: `base64 -w0 mangareader-release.jks | gh secret set ANDROID_KEYSTORE_BASE64 --repo "$repo"`.

**Через сайт:** Settings → Secrets and variables → Actions → вкладка **Secrets** (*New repository secret*) для четырёх секретов и вкладка **Variables** для `ANDROID_CERT_SHA256`. Base64 удобно положить в буфер обмена:

```powershell
[Convert]::ToBase64String([IO.File]::ReadAllBytes("$PWD\mangareader-release.jks")) | Set-Clipboard
```

**Не используйте** `certutil -encode` (добавляет строки-заголовки и портит файл) и онлайн-конвертеры.

Если `ANDROID_CERT_SHA256` не задан, первая сборка релиза остановится и выведет отпечаток в журнал — сохраните его в переменную и перезапустите. Дальше CI не выпустит APK, подписанный другим ключом.

### 5. Проверить

Шаг «Ключ подписи» в журнале сборки выводит `SHA-256 файла ключа`. Сравните с `Get-FileHash mangareader-release.jks` (регистр не важен):

- совпадает, шаг зелёный — всё верно;
- совпадает, но «не удалось открыть ключ» — неверен `ANDROID_KEYSTORE_PASSWORD`;
- не совпадает — испорчен `ANDROID_KEYSTORE_BASE64`, запишите его заново через `gh`.

### 6. Резервная копия — обязательно

Сохраните **файл `mangareader-release.jks` и пароль** в двух местах вне компьютера (менеджер паролей, флешка, облако). Из GitHub Secrets их не прочитать обратно. **Потерянный ключ не восстановить**: новые версии перестанут ставиться поверх у всех пользователей. Файлы `*.jks` и `*.keystore` в `.gitignore`.

### Release-APK локально

Задайте переменные `MANGAREADER_KEYSTORE` (путь к `.jks`), `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD` и выполните `make build-android-release`.

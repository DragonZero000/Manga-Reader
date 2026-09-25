## Context

После `repo-hygiene` версия берётся только из `FyneApp.toml` (`go run ./tools/version`), а `versionCode` из неё вычисляется. Сборка:
- **Windows:** `make build-windows` / `make package-windows`. Нужны cgo (gcc, mingw-w64), GNU make, Windows `tar.exe` для zip. Firefox ESR скачивает `tools/fetch-firefox`: это установщик `.exe`, который распаковывается ключом `/ExtractDir`, поэтому утилита работает только на Windows.
- **Android:** `go run ./tools/android-build`. Нужны Android SDK, NDK 27+ и JDK 17+. Утилита уже поддерживает Linux-хост (clang из `prebuilt/linux-x86_64`, `gradlew` без `.bat`). Подпись release — через `MANGAREADER_KEYSTORE`, `MANGAREADER_KEYSTORE_PASSWORD`, `MANGAREADER_KEY_ALIAS`, `MANGAREADER_KEY_PASSWORD`. Без ключа получается неподписанный APK с предупреждением.
- **Тесты:** часть тестов только для Windows (NTFS-потоки, реестр); интеграционный тест браузера без `MANGAREADER_BROWSER_IT` пропускается.

Репозиторий: `github.com/DragonZero000/Manga-Reader`, GitHub Actions ещё нет.

Платформы: Windows и Android. Linux добавится в `linux-support` отдельным job'ом в те же workflow.

## Goals / Non-Goals

**Goals:**
- Каждый push и PR проверяются тестами и Android-сборкой, без секретов.
- Тег `vX.Y.Z` → черновик релиза с zip для Windows, подписанным APK и контрольными суммами.
- Все релизные APK подписаны одним ключом. Случайная смена ключа обнаруживается до публикации.
- Лицензии лежат внутри пакетов.

**Non-Goals:**
- Linux, магазины приложений, автообновление, подпись exe, автоматическая публикация без участия автора.

## Decisions

### D1. Раннеры: тесты и Windows — `windows-latest`, Android — `ubuntu-latest`
- Тесты на Windows: основная ПК-платформа, только там работают `*_windows_test.go`. `internal/archtest` проверяет и `GOOS=android` через `go list`, компилятор для этого не нужен.
- Android на Ubuntu: в образе уже есть Android SDK, несколько NDK (≥27) и JDK 17. Linux-раннер быстрее и дешевле Windows. Заодно сборка проверяется на не-Windows хосте, это задел для `linux-support`.
- Windows-пакет на Windows: `fetch-firefox` и `-H windowsgui` требуют Windows.

*Альтернатива:* всё на Windows — медленнее, SDK пришлось бы ставить самим. Кросс-компиляция exe из Linux (mingw) — `fetch-firefox` всё равно работает только на Windows.

### D2. Инструменты на Windows-раннере
- Go — `actions/setup-go` с `go-version-file: go.mod` и кэшем модулей.
- gcc — MinGW-w64 из образа раннера (`C:\mingw64\bin`). Если его нет в `PATH` или версия не подходит Fyne — `choco install mingw` (проверяется задачей).
- make — `choco install make`. Сборка идёт через те же цели Makefile, что и локально, чтобы CI и разработчик собирали одинаково.

### D3. Проверка тега
Первый job `version` выполняет `go run ./tools/version` и сравнивает результат с `${GITHUB_REF_NAME#v}`. Остальные job'ы зависят от него (`needs`). Workflow запускается только для тегов `v[0-9]+.[0-9]+.[0-9]+` (фильтр `tags: ['v*.*.*']` плюс точная проверка формата в том же job'е). Версия передаётся дальше через `outputs`.

### D4. Структура `release.yml`
```
version ──▶ test (windows) ──┬──▶ windows (windows) ──┐
                             └──▶ android (ubuntu)  ──┴──▶ publish (ubuntu)
```
- `windows`: `make package-windows` → переименование `dist/MangaReader.zip` в `MangaReader-X.Y.Z-windows-x64.zip` → `upload-artifact`.
- `android`: восстановление ключа → `go run ./tools/android-build -release -require-signing -o dist/MangaReader-X.Y.Z-android-arm64.apk` → проверка подписи (D6) → `upload-artifact`.
- `publish`: `download-artifact`, затем `sha256sum * > SHA256SUMS.txt` и `gh release create vX.Y.Z --draft --generate-notes --title "MangaReader X.Y.Z" <файлы>`.
- Права: по умолчанию `contents: read`, `contents: write` только у `publish`. Секреты доступны только job'у `android`.
- `concurrency` по тегу: повторный push того же тега отменяет незавершённую сборку.

*Альтернатива:* сразу публиковать релиз — отклонено: автор хочет сам проверить заметки и файлы. Черновик можно удалить без следа.

### D5. Ключ подписи
Секреты: `ANDROID_KEYSTORE_BASE64` (keystore в base64), `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`.
- Первый шаг job'а `android` проверяет, что все четыре секрета заданы, и называет отсутствующий.
- Ключ декодируется в `$RUNNER_TEMP/release.jks`: вне рабочей копии, поэтому не попадает в артефакты. Раннер одноразовый.
- Переменные передаются в `tools/android-build` под их нынешними именами (`MANGAREADER_*`). Менять Gradle-скрипт не нужно.
- Флаг `-require-signing` в `tools/android-build`: при `-release` и пустом `MANGAREADER_KEYSTORE` ошибка до запуска Gradle. Локальная `make build-android-release` без флага ведёт себя как раньше.

Автор создаёт ключ один раз (`keytool -genkeypair -keyalg RSA -keysize 4096 -validity 10000 -alias mangareader`), кладёт его в секреты (`gh secret set ANDROID_KEYSTORE_BASE64 < release.jks.b64`) и хранит резервную копию keystore и паролей вне репозитория.

### D6. Защита от смены ключа
Переменная репозитория (не секрет) `ANDROID_CERT_SHA256` хранит SHA-256 отпечаток сертификата подписи. После сборки `apksigner verify --print-certs` сверяет отпечаток APK с этой переменной. При несовпадении сборка падает: APK с другим ключом нельзя поставить поверх, значит, публиковать его нельзя. Если переменная не задана, отпечаток выводится в журнал и сборка падает с подсказкой, какое значение задать. Так первая настройка сводится к одному копированию.

*Альтернатива:* не проверять — ошибку в секретах заметят только пользователи, у которых не встанет обновление.

### D7. `ci.yml`
Запуск на `push` (все ветки) и `pull_request`. Два job'а:
- `test` (windows-latest): `go vet ./...`, `go test ./...`. `make test` не используется, чтобы не ставить make ради двух команд;
- `android` (ubuntu-latest): `go run ./tools/android-build`. Debug-сборка подписывается отладочным ключом раннера, секретов не нужно; APK не публикуется.

Linux-тесты (`GOOS=linux`) добавятся в `linux-support`.

### D8. Кэши
- Go-модули и сборка — встроенный кэш `setup-go`.
- Gradle — `gradle/actions/setup-gradle`.
- Установщик Firefox — `actions/cache` для `browser/.cache` с ключом `hashFiles('tools/fetch-firefox/main.go')`. Версия и SHA-256 закреплены в этом файле, поэтому кэш сбрасывается при обновлении Firefox. Контрольную сумму `fetch-firefox` проверяет и для файла из кэша.

### D9. Лицензии в пакетах
- `Makefile` → `package-windows`: копирует `LICENSE` и `THIRD_PARTY_NOTICES.md` в `dist/MangaReader/` (через `copy`/`cp`, как `COPY_EXE`).
- `tools/android-build`: копирует оба файла в `android/app/src/main/assets/` так же, как `Icon.png` в `mipmap`. Папка добавляется в `.gitignore`.

### D10. Версии actions и их обновление
Actions закрепляются по мажорной версии (`actions/checkout@v5` и т. п.). `.github/dependabot.yml` (экосистема `github-actions`, раз в месяц) присылает PR с обновлениями. Go-модули в Dependabot не включаются: при их обновлении нужно обновлять `THIRD_PARTY_NOTICES.md` вручную.

*Альтернатива:* закрепление по SHA коммита — надёжнее против подмены тега, но хуже читается; для проекта такого размера избыточно.

## Risks / Trade-offs

- [MinGW в образе `windows-latest` может смениться или не подойти Fyne] → явная установка через choco, если проверка `gcc --version` падает; задача проверки на первом прогоне.
- [Образ Ubuntu содержит NDK другой версии, чем у автора] → `tools/android-build` берёт самый новый ≥27; различия в clang не влияют на Go-код. Если понадобится, закрепить `ANDROID_NDK_HOME` на `$ANDROID_HOME/ndk/<версия>` в workflow.
- [compileSdk 37.1 нет в образе] → AGP скачивает платформу сам (лицензии SDK в образе приняты).
- [Потеря ключа подписи] → обновления станут невозможны навсегда. Резервная копия у автора, напоминание в README (раздел для разработчиков).
- [Скачивание Firefox (~80 МБ) при каждом релизе] → кэш D8.
- [Релизный APK ставится только после удаления локальной debug-сборки] → однократно, описано в README.
- [Секреты недоступны в PR из форков] → в `ci.yml` подпись не используется (D7).

## Migration Plan

1. Автор создаёт keystore, задаёт четыре секрета, делает резервную копию.
2. Слить изменение, убедиться, что `ci.yml` зелёный.
3. Отправить тег текущей версии `v0.1.0`. Первый прогон упадёт на D6 и выведет отпечаток. Задать `ANDROID_CERT_SHA256`, перезапустить. Если черновик нужен лишь для проверки, удалить его.
4. Для следующих релизов: поднять `Version` в `FyneApp.toml`, закоммитить, отправить тег `vX.Y.Z`, проверить и опубликовать черновик.

Откат: удалить workflow-файлы. Секреты и переменная не мешают.

## Open Questions

- Нет.

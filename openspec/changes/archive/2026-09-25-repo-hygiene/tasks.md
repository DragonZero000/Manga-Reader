> Предусловие: автор сделал первый коммит текущего состояния (со спайками). Коммиты внутри изменения автор составляет сам; ниже отмечено, какие шаги удобно выделить в отдельный коммит.

## 1. Git: игнорирование и окончания строк

- [x] 1.1 Обновить `.gitignore`: добавить `.idea/`, `.vscode/`, `.claude/`, `.cline/`, `.clinerules/`, `*.jks`, `*.keystore`; удалить правила `spikes/**`; сгруппировать правила с комментариями (сборка, данные запуска, Android/Gradle, IDE и агенты, ключи)
- [x] 1.2 Убрать из индекса уже закоммиченные файлы IDE и агентов: `git rm -r --cached .idea .claude .cline .clinerules` (папки на диске остаются); проверить `git status`, что они больше не отслеживаются
- [x] 1.3 Создать `.gitattributes` по D2 (LF по умолчанию, CRLF для `*.bat`/`*.cmd`, LF для `gradlew`/`*.sh`, бинарные типы)
- [x] 1.4 Нормализовать окончания строк: `git add --renormalize .` (отдельный коммит «normalize line endings»); проверить `git ls-files --eol`, что ни у одного файла в индексе нет `i/crlf`
- [x] 1.5 Установить бит исполнения: `git update-index --chmod=+x android/gradlew`; проверить `git ls-files -s android/gradlew` → режим `100755`

## 2. Удаление лишнего

- [x] 2.1 Удалить пустую папку `-p/`
- [x] 2.2 Удалить `spikes/` (отдельный коммит «remove spikes»); убедиться, что ни `go.mod`, ни `Makefile`, ни `tools/`, ни `internal/` на них не ссылаются (`grep -r spikes` вне `openspec/changes/archive`)
- [x] 2.3 Убрать строку `spikes/` из раздела «Структура» в `README.md`

## 3. Единая версия

- [x] 3.1 Создать `internal/appversion`: `Parse`, `Read(path)`, `Version.String()`, `Version.Code()` по спецификации `app-versioning`; тесты — примеры `0.1.0→100`, `0.2.0→200`, `1.2.3→10203`, `0.9.99 < 0.10.0`, ошибки для `0.2.0-beta`, `1.100.0`, `01.2.3`, пустой строки и отсутствующего поля
- [x] 3.2 Добавить `mangareader/internal/appversion` в список пакетов `internal/archtest`
- [x] 3.3 Создать `tools/version`: без флагов печатает версию, `-code` — `versionCode`; при ошибке — сообщение в stderr и код 1
- [x] 3.4 Перевести `tools/android-build` на `internal/appversion`: удалить `readMetadata`, `reVersion`, `reBuild`; `-PversionCode` и `-X main.build` — из `Version.Code()`; обновить `main_test.go`
- [x] 3.5 Удалить поле `Build` из `FyneApp.toml`; `Website = "https://github.com/DragonZero000/Manga-Reader"`
- [x] 3.6 `Makefile`: `override VERSION = $(shell go run ./tools/version)`, проверка `$(if $(VERSION),,$(error …))` в `build-windows`; удалить цель `build-android-fyne` и её упоминание в `.PHONY`
- [x] 3.7 `README.md`: убрать `make build-android-fyne` и абзац о нём, указать, что версия задаётся в `FyneApp.toml`
- [x] 3.8 Проверить: `make run` показывает версию `dev` без ошибок чтения `FyneApp.toml`; при `Version = "0.2.0-beta"` `make build-windows` и `make build-android` останавливаются с понятной ошибкой; `make build-windows VERSION=9.9.9` всё равно берёт версию из `FyneApp.toml`

## 4. Лицензия и сторонние компоненты

- [x] 4.1 Создать `LICENSE` (MIT, `Copyright (c) 2026 DragonZero000`)
- [x] 4.2 Создать `THIRD_PARTY_NOTICES.md` по D6: таблица компонентов с версиями из `tools/fetch-firefox`, `android/app/build.gradle.kts`, `go.mod`; ссылки на тексты лицензий; ссылки на исходники Firefox ESR `153.3.0esr` и GeckoView `156.0.20260921121718` (проверить, что открываются); абзац о товарных знаках Mozilla
- [x] 4.3 Добавить комментарии рядом с `version` в `tools/fetch-firefox/main.go` и `geckoviewVersion` в `android/app/build.gradle.kts`: при обновлении — обновить `THIRD_PARTY_NOTICES.md`

## 5. OpenSpec

- [x] 5.1 Переписать `context` в `openspec/config.yaml` по D7: убрать SQLite, rate limit, онлайн-каталог и «known blockers»; описать реальный стек, платформы, build-теги, границы пакетов, портативный режим, язык UI, раскладку документации, `make test`, единую версию
- [x] 5.2 Обновить `rules`: сохранить действующие; убрать правило о rate limiting и API paths; добавить: задача `make test` в tasks, обновление документации на двух языках при изменении поведения для пользователя или сборки, платформы в design, новый пакет `internal/` → `internal/archtest` и описание архитектуры, изменение распространяемых компонентов → `THIRD_PARTY_NOTICES.md`
- [x] 5.3 Проверить: `openspec instructions proposal --change repo-hygiene --json` и `openspec instructions tasks --change repo-hygiene --json` выводят новый `context` и `rules`; `openspec validate repo-hygiene` без ошибок

## 6. Проверка

- [x] 6.1 `make test` проходит
- [x] 6.2 `make build-windows` и `make build-android` проходят; `aapt dump badging dist/mangareader.apk` показывает `versionName='0.1.0'` и `versionCode='100'`
- [x] 6.3 APK с `versionCode 100` устанавливается командой `adb install -r` поверх ранее установленной сборки (`versionCode 1`) без удаления; выбранная папка библиотеки сохраняется
- [x] 6.4 Чистый клон в отдельную папку: `git status` чист, `android/gradlew` имеет LF, `android/gradlew.bat` — CRLF, в `git ls-files` нет `spikes/`, `-p/`, `.idea/`, `.claude/`, `.cline/`, `.clinerules/`

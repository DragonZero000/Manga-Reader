## Context

В репозитории ещё нет коммитов. Автор сам делает первый коммит «как есть» (со спайками), а это изменение применяется поверх него. Затронуты только файлы репозитория и сборки; код приложения не меняется, кроме номера сборки Android.

Сейчас:
- версия записана дважды: `Makefile` (`VERSION ?= 0.1.0`, идёт в `-X main.version` для Windows и в `build-android-fyne`) и `FyneApp.toml` (`Version`, `Build`; их читает `tools/android-build` регулярными выражениями и передаёт в Gradle как `-PversionName`/`-PversionCode`);
- `versionCode` — ручной `Build = 1`, его легко забыть увеличить, и тогда APK не встанет поверх;
- `core.autocrlf=true`, `.gitattributes` нет;
- `openspec/config.yaml` скопирован из другого проекта.

Платформы: Windows (сборка, Makefile под `cmd.exe`) и Android (`tools/android-build`, Gradle). Linux не затрагивается, но всё новое (утилита версии, `.gitattributes`) должно работать и там — это задел для `linux-support`.

## Goals / Non-Goals

**Goals:**
- `git add .` в чистой рабочей копии добавляет только то, что нужно проекту.
- Клон на любой ОС даёт рабочие `gradlew` и `.bat`.
- Версия меняется в одном месте, `versionCode` растёт сам.
- Лицензионно корректное распространение Firefox ESR и GeckoView.
- `openspec/config.yaml` даёт будущим изменениям верный контекст и правила.

**Non-Goals:**
- CI, релизы, release-ключ в Secrets (`release-pipeline`).
- Переработка README и `docs/` (`docs`).
- Linux-сборка (`linux-support`).

## Decisions

### D1. `.gitignore`: папки агентов и IDE целиком
Игнорируются `.idea/`, `.vscode/`, `.claude/`, `.cline/`, `.clinerules/` полностью, а не выборочно (`workspace.xml`, `caches/`). Их содержимое (скиллы OpenSpec, настройки IDE) генерируется `openspec init` или самой IDE и у каждого разработчика своё. `openspec/` остаётся в репозитории: он и есть контекст проекта.
*Альтернатива:* коммитить `.claude/` и `.clinerules/` для воспроизводимого процесса — отклонено: три копии одних и тех же скиллов, привязка к конкретным агентам.

Правила для `spikes/**` удаляются вместе с папкой. `*.jks` и `*.keystore` добавляются заранее, до появления release-ключа в `release-pipeline`.

### D2. `.gitattributes`: `* text=auto eol=lf` с исключениями
```
* text=auto eol=lf
*.bat  text eol=crlf
*.cmd  text eol=crlf
gradlew text eol=lf
*.sh   text eol=lf
*.png *.jpg *.jar *.zip *.xpi *.apk *.aab *.so  binary
```
`eol=lf` для всех текстовых файлов: Go, Kotlin, Makefile и Gradle одинаково работают с LF на Windows, а на Linux CRLF ломает скрипты. Явное `eol` перекрывает `core.autocrlf` разработчика.
Бит исполнения `gradlew` задаётся `git update-index --chmod=+x android/gradlew` — `.gitattributes` его не хранит.
После добавления файла выполняется `git add --renormalize .`, чтобы файлы, уже попавшие в первый коммит с CRLF, были нормализованы.

### D3. Утилита версии `tools/version` и пакет `internal/appversion`
Makefile на Windows выполняется в `cmd.exe` — разбирать TOML средствами оболочки неудобно и непереносимо. Поэтому:
- `internal/appversion` — чистый Go без зависимостей: `Read(path) (Version, error)` читает `Version` из `FyneApp.toml`, `Parse(s)` проверяет формат `MAJOR.MINOR.PATCH` (ограничения из `app-versioning`), `Version.Code() int` = `MAJOR*10000 + MINOR*100 + PATCH`. Разбор — тем же регулярным выражением, что сейчас в `tools/android-build` (без зависимости от TOML-библиотеки в основном модуле).
- `tools/version` печатает версию (`go run ./tools/version`) или `versionCode` (`-code`); при ошибке — сообщение в stderr и код выхода 1.
- `tools/android-build` использует `internal/appversion` вместо своего `readMetadata` и передаёт в Gradle вычисленный код.
- Makefile: `override VERSION = $(shell go run ./tools/version)` — ленивое (`=`), чтобы `make test` и `make run` не запускали утилиту; `override` запрещает `make VERSION=…`. В рецептах, где нужна версия, — проверка `$(if $(VERSION),,$(error …))`, так как `$(shell)` не передаёт код выхода.

*Альтернативы:* разбор через `$(file <FyneApp.toml)` и текстовые функции make — хрупко к пробелам и порядку полей; версия в Makefile как источник, а `FyneApp.toml` генерируется — Fyne читает toml при `go run`, лишняя генерация; отдельный файл `VERSION` — третий источник вместо одного.

### D4. Убрать `Build` из `FyneApp.toml`
Поле `Build` удаляется: номер сборки теперь вычисляется. `-X main.build` в `tools/android-build` получает вычисленный код. Переход `1 → 100` для версии 0.1.0 — рост, обновление поверх установленной сборки работает.
*Альтернатива:* оставить `Build` и проверять, что он равен вычисленному — снова ручная работа и второй источник.

### D5. Удалить `make build-android-fyne`
Переход на Gradle завершён (изменение `android-build-gradle` в архиве), цель — единственный, кроме Windows, потребитель `VERSION` из Makefile и требует `fyne` CLI. Удаляется вместе с упоминанием в README.

### D6. `LICENSE` и `THIRD_PARTY_NOTICES.md`
MIT на код проекта: `Copyright (c) 2026 DragonZero000`. Код проекта не изменяет файлы Firefox/GeckoView, поэтому файловый копилефт MPL-2.0 на него не распространяется.
`THIRD_PARTY_NOTICES.md` — таблица распространяемых компонентов:

| Компонент | Где закреплена версия | Лицензия |
|---|---|---|
| Firefox ESR (Windows-дистрибутив) | `tools/fetch-firefox` (`153.3.0esr`) | MPL-2.0 |
| GeckoView (APK) | `android/app/build.gradle.kts` (`geckoviewVersion`) | MPL-2.0 |
| Fyne | `go.mod` | BSD-3-Clause |
| fsnotify | `go.mod` | BSD-3-Clause |
| `golang.org/x/image`, `golang.org/x/sys` | `go.mod` | BSD-3-Clause |
| AndroidX, Kotlin stdlib | `android/` Gradle | Apache-2.0 |

Для MPL-компонентов — ссылки на исходники конкретной версии (`archive.mozilla.org/pub/firefox/releases/<версия>/source/`, для GeckoView — тег в `mozilla-central`/`mozilla-firefox/firefox` по номеру сборки). Плюс абзац о товарных знаках Mozilla. Комментарии рядом с `version` в `tools/fetch-firefox` и `geckoviewVersion` напоминают обновить уведомления. Вложение файла в zip/APK — задача `release-pipeline`.

### D7. `openspec/config.yaml`
Новый `context` (кратко, фактами): назначение (офлайн-читалка zip-архивов манги со встроенным браузером для загрузки); Go 1.26, Fyne 2.8 с `fyneDo` — обновления UI из горутин только через `fyne.Do`, сборки с тегом `migrated_fynedo`; платформы Windows и Android, Linux — в планах; build-теги `windows`/`!windows`/`android`/`!android`; сервисные пакеты `internal/*` не импортируют виджеты Fyne (`internal/archtest`); портативный режим на ПК, SAF-папка на Android; строки UI и комментарии на русском, i18n пока нет; документация — `README.md` (en) + `README.ru.md`, `docs/en` + `docs/ru` с одинаковыми именами файлов; проверка — `make test`; версия — только `FyneApp.toml`.
`rules` сохраняют действующие (размер proposal, Non-goals, задачи до ~2 часов, `[x]` только по факту) и добавляют правила из спецификации `repository-layout`. Правило design про rate limiting и API paths удаляется — оно из чужого проекта; остаётся «явно указывать потокобезопасность UI».

## Risks / Trade-offs

- [`go run ./tools/version` добавляет ~1 с к `build-windows`] → только в целях сборки благодаря ленивому `=`; на фоне cgo-сборки незаметно.
- [`git add --renormalize` даст большой коммит без смысловых изменений] → отдельный коммит «normalize line endings», автор делает его сам.
- [Fyne при `go run` читает `FyneApp.toml` и может ожидать `Build`] → при отсутствии поля Fyne использует 0; `make run` показывает версию `dev`. Проверить задачей.
- [Ограничение `MINOR`, `PATCH` ≤ 99] → достаточно для проекта; при необходимости формула меняется до выхода версии с таким номером.
- [Ссылка на исходники GeckoView определяется неочевидно по номеру сборки] → в уведомлении указать и номер сборки Maven, и путь к исходникам; проверить, что ссылка открывается.
- [Спайки удаляются, ссылки на них в архивных design.md ведут в никуда] → они остаются в истории первого коммита; архив не правится.

## Migration Plan

1. Автор делает первый коммит текущего состояния (вне этого изменения).
2. Применить изменение; `.gitattributes` — затем `git add --renormalize .` и `git update-index --chmod=+x android/gradlew`.
3. `git rm -r --cached` для уже закоммиченных `.idea/`, `.claude/`, `.cline/`, `.clinerules/` (сами папки на диске остаются).
4. Удалить `spikes/` и `-p/`.
5. `make test`, `make build-windows`, `make build-android`; установить APK поверх предыдущей сборки.

Откат — `git revert` соответствующих коммитов.

## Open Questions

- Нет.

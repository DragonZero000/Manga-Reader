## Why

Репозиторий готовится к публикации на GitHub (`https://github.com/DragonZero000/Manga-Reader`), чтобы любой мог скачать релиз или доработать приложение сам. Сейчас в нём нет лицензии, нет `.gitattributes` (при `core.autocrlf=true` `gradlew` получит CRLF), в коммит попадут личные файлы IDE и агентов, версия записана в двух местах, а `openspec/config.yaml` описывает другой проект — и этот неверный контекст получает каждое новое изменение. Прежде чем строить релизы, документацию и Linux-сборку, нужен опрятный фундамент.

## What Changes

- `.gitignore`: папки IDE и AI-агентов (`.idea/`, `.vscode/`, `.claude/`, `.cline/`, `.clinerules/`), ключи подписи (`*.jks`, `*.keystore`); правила для `spikes/` убираются.
- Новый `.gitattributes`: LF для текстовых файлов, CRLF для `*.bat`/`*.cmd`, LF для `gradlew` и `*.sh`, бинарные типы (`png`, `jar`, `zip`, `xpi`, `apk`) помечены как binary; `gradlew` хранится с битом исполнения.
- Удаляются пустая папка `-p/` и `spikes/` (выводы спайков уже в `design.md` архивных изменений). Удаление — отдельным коммитом после первоначального, чтобы спайки остались в истории.
- Новый `LICENSE` (MIT, DragonZero000) и `THIRD_PARTY_NOTICES.md`: Firefox ESR и GeckoView (MPL-2.0, со ссылками на исходники закреплённых версий), Fyne, fsnotify, `golang.org/x/*` (BSD-3-Clause), AndroidX и Kotlin (Apache-2.0).
- `FyneApp.toml`: `Website = "https://github.com/DragonZero000/Manga-Reader"`.
- Единая версия: `FyneApp.toml` → `Version` — единственный источник. `Makefile` больше не задаёт `VERSION` сам, а читает её оттуда. Android `versionCode` вычисляется из версии (`major*10000 + minor*100 + patch`) вместо ручного `Build`, чтобы каждый новый релиз гарантированно ставился поверх предыдущего.
- **BREAKING (сборка):** удаляется `make build-android-fyne` — переход на Gradle завершён, а цель держала вторую копию версии и требовала `fyne` CLI.
- `openspec/config.yaml`: `context` переписан под реальный проект (стек, платформы, build-теги, границы пакетов, портативный режим, язык UI, раскладка документации); `rules` дополнены правилами для будущих изменений (проверка `make test`, обновление документации на двух языках, платформы в design, новый пакет → archtest и документация).

## Non-goals

- CI и релизы на GitHub (изменение `release-pipeline`).
- README, `docs/en`, `docs/ru`, CONTRIBUTING (изменение `docs`); здесь README правится только там, где он прямо противоречит изменению (команды сборки, версия).
- Сборка под Linux (изменение `linux-support`).
- Перевод интерфейса (будущее изменение `i18n`).
- Составление git-истории — её делает автор вручную.

## Capabilities

### New Capabilities
- `repository-layout`: что хранится в репозитории и в каком виде — игнорируемые файлы, окончания строк, лицензия и уведомления о сторонних компонентах, отсутствие временных экспериментов, актуальный контекст OpenSpec.
- `app-versioning`: единый источник версии приложения, правило вычисления Android `versionCode`, допустимый формат версии.

### Modified Capabilities
- `android-build`: требование «Состав APK» — `versionCode` вычисляется из версии, а не берётся из `Build`; требование «Подпись и обновление» — новая версия с большим номером всегда ставится поверх предыдущей.

## Impact

- Новое: `.gitattributes`, `LICENSE`, `THIRD_PARTY_NOTICES.md`, `internal/appversion` (разбор версии и `versionCode`, без зависимостей), `tools/version` (печать версии для Makefile).
- Изменено: `.gitignore`, `FyneApp.toml` (без `Build`), `Makefile` (версия из `FyneApp.toml`, нет `build-android-fyne`), `tools/android-build` (`versionCode` через `internal/appversion`), `internal/archtest` (новый пакет в списке), `openspec/config.yaml`, `README.md` (команды и раздел «Структура» — минимально).
- Удалено: `-p/`, `spikes/`.
- Установка: `versionCode` текущей версии 0.1.0 станет 100 (было 1) — обновление поверх уже установленной сборки продолжит работать.

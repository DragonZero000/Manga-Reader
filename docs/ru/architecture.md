[English](../en/architecture.md) | **Русский**

# Архитектура

MangaReader — одно Go-приложение на [Fyne](https://fyne.io) для Windows и Android. Встроенный браузер на Windows — отдельный процесс портативного Firefox ESR, на Android — GeckoView внутри APK с экранами на Kotlin.

- [Карта репозитория](#карта-репозитория)
- [Пакеты](#пакеты)
- [Границы пакетов](#границы-пакетов)
- [Платформы и build-теги](#платформы-и-build-теги)
- [Потоки и UI](#потоки-и-ui)
- [Встроенный браузер](#встроенный-браузер)
- [Версия, сборка и CI](#версия-сборка-и-ci)
- [OpenSpec](#openspec)

## Карта репозитория

```text
cmd/mangareader/     точка входа; метаданные приложения для Android
internal/            код приложения (см. «Пакеты»)
tools/               утилиты сборки: android-build, fetch-firefox, version
android/             Gradle-проект Android; экраны браузера на Kotlin
openspec/            спецификации и история изменений (OpenSpec)
docs/                документация (en, ru, images)
testdata/            example.zip — пример архива
.github/workflows/   CI и релизы
FyneApp.toml         метаданные приложения и единственный источник версии
```

## Пакеты

| Пакет | Назначение |
|---|---|
| `cmd/mangareader` | Точка входа: создаёт приложение Fyne, собирает зависимости (`internal/app`), запускает оболочку UI |
| `internal/app` | Сборка зависимостей без виджетов: папка библиотеки, хранилище, сканер, индекс поиска, браузер, ключи настроек |
| `internal/paths` | Папка приложения на ПК (портативный режим): рядом с exe, при `go run` — рабочая папка |
| `internal/storage` | Доступ к файлам библиотеки и настройки: файловая система на ПК, Storage Access Framework (JNI) на Android; метки источника загрузки (NTFS-поток, `Zone.Identifier`) |
| `internal/library` | Сканирование папки, разбор zip и `meta.json`, определение типа файлов, отслеживание изменений (fsnotify), размеры страниц |
| `internal/model` | Модель: `Gallery`, `Tag`, `Key`, натуральная сортировка |
| `internal/search` | Поля поиска, разбор запроса, `Query`, интерфейс `Index` и индекс в памяти |
| `internal/problems` | Список ошибок библиотеки и статус их просмотра |
| `internal/thumbs` | Миниатюры обложек: декодирование в фоне, уменьшение, кэш в памяти |
| `internal/pages` | Загрузка страниц для читалки с приоритетами и геометрия отображения |
| `internal/browser` | Встроенный браузер Windows: профиль Firefox, расширение-мост, запуск, привязка окна, уборка реестра |
| `internal/mobilebrowser` | Мост к браузеру Android (JNI): открыть, очистить данные, получить сообщения о загрузках |
| `internal/catalog` | Каталог библиотеки в SQLite (`library.db`, FTS5, тег `sqlite_fts5`): результаты сканирования между запусками, индекс поиска (`search.Index`), обложки на диске, копия ссылок скачанных файлов; восстанавливаемый кэш |
| `internal/display` | Частота экрана окна приложения на Android (JNI, `DisplayRate.kt`): ограничение 60 Гц; на других платформах — заглушка |
| `internal/appversion` | Версия из `FyneApp.toml` и номер сборки Android |
| `internal/ui` | Оболочка: окно, вкладки, toast-уведомления, связь экранов с сервисами |
| `internal/ui/screens` | Экраны: библиотека, поиск, ошибки, настройки |
| `internal/ui/details` | Страница произведения |
| `internal/ui/reader` | Читалка: постраничный режим и лента |
| `internal/archtest` | Тесты архитектурных границ и структуры документации |
| `tools/android-build` | Сборка APK: `libmangareader.so` через NDK, Java-классы Fyne, лицензии в `assets/`, Gradle |
| `tools/fetch-firefox` | Скачивание и распаковка закреплённой версии Firefox ESR (проверка SHA-256) |
| `tools/version` | Печать версии и номера сборки для Makefile и CI |

## Границы пакетов

Виджеты Fyne (`fyne.io/fyne/v2/widget`, `container`) используются **только** в `internal/ui/...`. Сервисные пакеты (`app`, `library`, `search`, `storage`, `browser` и др.) от виджетов не зависят и тестируются без окна. Это проверяет `internal/archtest` для `GOOS=windows` и `GOOS=android`.

Новый пакет в `internal/` нужно добавить в список в [`internal/archtest/imports_test.go`](../../internal/archtest/imports_test.go) и в таблицу выше — на обоих языках.

## Платформы и build-теги

Платформенный код разделён по тегам и суффиксам файлов:

| Тег | Файлы | Пример |
|---|---|---|
| `windows` / `!windows` | `*_windows.go`, `*_other.go` / `stub_other.go` | окно и реестр браузера, NTFS-потоки |
| `android` / `!android` | `*_android.go`, `other.go`, `*_other.go` | SAF, JNI-мост к GeckoView, частота экрана, метаданные приложения |
| `frameprobe` / `!frameprobe` | `frameprobe_on.go`, `frameprobe_off.go` | замер кадров для разработки (`make … EXTRA_TAGS=frameprobe`) |

На платформах, где функции нет, работает заглушка: например, `internal/browser` на Android сообщает, что браузер не поддерживается, а UI не показывает Windows-настройки браузера. Новая платформенная функция всегда сопровождается заглушкой для остальных платформ.

Linux пока не поддерживается.

## Потоки и UI

Приложение мигрировано на `fyne.Do` (`[Migrations] fyneDo = true` в `FyneApp.toml`): виджеты меняются **только** из основного потока. Сканирование, декодирование миниатюр и страниц, поиск и события браузера выполняются в горутинах и возвращают результат в UI через `fyne.Do`. `make run` запускает приложение с проверками потоков Fyne; релизные сборки — с тегом `migrated_fynedo`, без проверок.

## Встроенный браузер

### Windows

```text
MangaReader ──запуск──▶ firefox.exe -profile browser\profile
     ▲                        │
     │  127.0.0.1 + токен     │ расширение bridge@mangareader.app
     └────────────────────────┘ (адрес страницы загрузки, команды)
```

- `tools/fetch-firefox` распаковывает установщик Firefox ESR (`/ExtractDir`) в `browser\firefox`; версия и SHA-256 закреплены в коде.
- При каждом запуске приложение перезаписывает `browser\profile\user.js` (папка загрузок = папка библиотеки, отключённые обновления и телеметрия, тёмная тема) и пересобирает расширение `bridge@mangareader.app.xpi` из `internal/browser/ext`.
- Расширение сообщает приложению адрес страницы, с которой начата загрузка, по `127.0.0.1` с одноразовым токеном; приложение пишет его в NTFS-поток `:mangareader.source` файла.
- Окно Firefox находится по процессу и получает окно приложения владельцем (Win32): поверх приложения, сворачивается вместе с ним, без кнопки на панели задач.
- После закрытия браузера удаляются служебные значения Firefox в `HKCU\Software\Mozilla\Firefox`, относящиеся к встроенному экземпляру.

### Android

- GeckoView подключён в `android/app/build.gradle.kts`; экраны браузера — Kotlin в `android/app/src/main/kotlin/io/github/mangareader/app/browser/` (`BrowserActivity`, `BrowserEngine`, `DownloadManager`, `DownloadService` и др.).
- Go вызывает Kotlin и получает события через JNI (`internal/mobilebrowser`, `GoBridge.kt`): открыть браузер, очистить данные, сообщить о загрузке (путь и адрес страницы).
- Загрузки пишутся в выбранную SAF-папку через `DownloadService` (foreground-сервис с уведомлением), во время загрузки — `имя.part`.

## Версия, сборка и CI

- Версия — только `FyneApp.toml`; `internal/appversion` проверяет формат и вычисляет `versionCode`. Подробно — [building.md](building.md#версия).
- `Makefile` — обёртка над `go build`, `tools/android-build` и `tools/fetch-firefox`; работает из PowerShell, cmd и Git Bash (на Windows команды выполняет `cmd.exe`).
- CI и релизы — [`.github/workflows/`](../../.github/workflows/); порядок выпуска и ключи — [building.md](building.md#выпуск-релиза).
- Сторонние компоненты, которые попадают в пакеты, перечислены в [`THIRD_PARTY_NOTICES.md`](../../THIRD_PARTY_NOTICES.md).

## OpenSpec

Требования и история решений хранятся в [`openspec/`](../../openspec/) ([OpenSpec](https://github.com/Fission-AI/OpenSpec)):

- `openspec/specs/<возможность>/spec.md` — действующие требования (что приложение должно делать), со сценариями;
- `openspec/changes/archive/` — завершённые изменения: предложение, дизайн с альтернативами, задачи;
- `openspec/config.yaml` — контекст проекта и правила для новых изменений (их читают и люди, и AI-ассистенты).

Порядок работы над изменением — в [CONTRIBUTING](../../CONTRIBUTING.ru.md).

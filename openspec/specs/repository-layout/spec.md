# repository-layout Specification

## Purpose

Что хранится в репозитории и в каком виде: игнорируемые файлы, окончания строк, лицензия MIT и уведомления о сторонних компонентах, отсутствие временных экспериментов и актуальный контекст OpenSpec для будущих изменений.

## Requirements
### Requirement: Игнорируемые файлы
Репозиторий SHALL NOT содержать результаты сборки, скачанные инструменты, пользовательские данные запуска, личные настройки IDE и AI-агентов, а также ключи подписи. `.gitignore` MUST исключать как минимум: `dist/`, `browser/`, `/manga/`, `/settings.json`, генерируемые файлы Gradle в `android/`, `.idea/`, `.vscode/`, `.claude/`, `.cline/`, `.clinerules/`, `*.jks`, `*.keystore`, `*.apk`, `*.aab`, `*.exe`.

#### Scenario: Файлы IDE и агентов
- **WHEN** в рабочей папке есть `.idea/workspace.xml`, `.claude/skills/…` и `.clinerules/workflows/…`
- **THEN** `git status` их не показывает, и `git add .` их не добавляет

#### Scenario: Ключ подписи
- **WHEN** разработчик кладёт `release.jks` в корень проекта или в `android/`
- **THEN** файл не попадает в индекс при `git add .`

#### Scenario: Папка openspec сохраняется
- **WHEN** выполняется `git add .`
- **THEN** `openspec/config.yaml`, `openspec/specs/` и `openspec/changes/` добавляются в индекс

### Requirement: Окончания строк и бинарные файлы
`.gitattributes` SHALL задавать нормализацию текстовых файлов к LF в репозитории независимо от `core.autocrlf` у разработчика. В рабочей копии `*.bat` и `*.cmd` MUST иметь CRLF, а `gradlew` и `*.sh` — LF. Файлы `*.png`, `*.jpg`, `*.jar`, `*.zip`, `*.xpi`, `*.apk` MUST помечаться как бинарные (без преобразования строк и текстового diff). `gradlew` MUST храниться в репозитории с битом исполнения.

#### Scenario: Клон на Windows с autocrlf
- **WHEN** репозиторий клонирован на Windows с `core.autocrlf=true`
- **THEN** `android/gradlew` в рабочей копии имеет окончания LF, а `android/gradlew.bat` — CRLF

#### Scenario: Клон на Linux
- **WHEN** репозиторий клонирован на Linux
- **THEN** `android/gradlew` запускается как `./gradlew` без `chmod` и без ошибок из-за `\r`

#### Scenario: Бинарный файл
- **WHEN** в коммите изменён `testdata/example.zip`
- **THEN** `git diff` сообщает об изменении бинарного файла, содержимое не выводится и не преобразуется

### Requirement: Лицензия и сторонние компоненты
Репозиторий SHALL содержать `LICENSE` с лицензией MIT на код проекта (правообладатель DragonZero000) и `THIRD_PARTY_NOTICES.md` со списком компонентов, которые распространяются вместе с приложением: название, закреплённая версия (где она закреплена в проекте), лицензия и ссылка на текст лицензии. Для компонентов под MPL-2.0 (Firefox ESR, GeckoView) MUST указываться, где получить исходный код именно распространяемой версии. Документ MUST сообщать, что Firefox и его логотип — товарные знаки Mozilla и проект не связан с Mozilla.

#### Scenario: Проверка лицензий
- **WHEN** человек открывает корень репозитория на GitHub
- **THEN** GitHub определяет лицензию как MIT, а в `THIRD_PARTY_NOTICES.md` есть Firefox ESR, GeckoView, Fyne, fsnotify, `golang.org/x/*`, AndroidX и Kotlin с их лицензиями

#### Scenario: Обновление Firefox ESR
- **WHEN** в `tools/fetch-firefox` меняется закреплённая версия Firefox ESR
- **THEN** версия и ссылка на исходники в `THIRD_PARTY_NOTICES.md` указывают на ту же версию

### Requirement: Нет временных экспериментов и мусора
Основная ветка SHALL NOT содержать папку `spikes/` и пустые служебные папки, появившиеся по ошибке (например, `-p/`). Выводы экспериментов MUST оставаться в `design.md` соответствующих изменений OpenSpec.

#### Scenario: Корень репозитория
- **WHEN** выполняется `git ls-files` в основной ветке
- **THEN** в списке нет путей, начинающихся с `spikes/` или `-p/`

#### Scenario: Сборка и тесты без спайков
- **WHEN** папка `spikes/` удалена
- **THEN** `make test`, `make build-windows` и `make build-android` завершаются успешно

### Requirement: Актуальный контекст OpenSpec
`openspec/config.yaml` SHALL описывать этот проект: назначение приложения, стек и версии (Go, Fyne, режим `fyne.Do`), поддерживаемые платформы и build-теги, правило границ пакетов из `internal/archtest`, портативный режим хранения, язык интерфейса, раскладку документации (`README.md`/`README.ru.md`, `docs/en`, `docs/ru`) и команду проверки `make test`. Контекст MUST NOT упоминать компоненты, которых нет в проекте. `rules` MUST требовать: в tasks — задачу проверки `make test`; при изменении поведения для пользователя или сборки — обновление документации на обоих языках; в design — перечисление затронутых платформ; при добавлении пакета в `internal/` — обновление `internal/archtest` и описания архитектуры.

#### Scenario: Новое изменение получает верный контекст
- **WHEN** выполняется `openspec instructions proposal --change <имя> --json`
- **THEN** поле `context` описывает MangaReader на Go и Fyne для Windows и Android и не упоминает SQLite, rate limit или онлайн-каталоги

#### Scenario: Правила для задач
- **WHEN** выполняется `openspec instructions tasks --change <имя> --json`
- **THEN** поле `rules` содержит требование задачи `make test` и обновления документации на двух языках


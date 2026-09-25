## Context

Репозиторий `manga-reader` пуст (только OpenSpec-скаффолд). Предыдущий проект `../nh whatcher` используется как справочник: из него известны формат `meta.json` и типичные ошибки (одна страница вместо навигации, toast через `SetContent`, UI-код в сервисах). Код оттуда не копируется.

Целевые платформы — Windows и Android. Первый источник данных — фиксированная папка с zip-архивами (каждый содержит изображения страниц и `meta.json`, возможно внутри одной корневой подпапки; образец — `example.zip`). Сеть в этом изменении не используется.

Окружение разработчика: Go 1.26.4, `fyne` CLI установлен; наличие Android NDK/SDK не проверено.

## Goals / Non-Goals

**Goals:**
- Собираемый и запускаемый каркас на Windows и Android.
- Архитектурные границы, которые не придётся ломать: UI ↔ сервисы, модель ↔ источник, поиск ↔ хранилище.
- Модель `Gallery`/`Tag` и модель запроса по параметрам — окончательные по форме, покрытые unit-тестами.
- Снятие двух платформенных рисков: путь к внешнему каталогу на Android и работоспособность `modernc.org/sqlite` в `.apk`.

**Non-Goals:**
- Чтение архивов, сканирование, индекс, читалка, сетевой API (см. proposal).

## Decisions

### D1. Структура пакетов

```
cmd/mangareader/main.go        — вход, версия через -ldflags
internal/app/                   — сборка зависимостей (wire-up), без виджетов
internal/ui/                    — оболочка: shell.go, toast.go
internal/ui/screens/            — library.go, search.go, settings.go (заглушки)
internal/paths/                 — корень библиотеки по платформам
internal/model/                 — gallery.go, key.go, tag.go, natsort.go
internal/search/                — field.go, query.go, validate.go, index.go, nop.go
spikes/sqlite-android/          — отдельный go.mod, не входит в основной модуль
testdata/example.zip
```

Зависимости направлены строго: `ui → app → {paths, model, search}`; `search → model`. `paths`, `model`, `search` не импортируют Fyne-виджеты (проверяется тестом на импорты или `go list -deps`).

*Альтернатива:* плоский пакет `internal/core`. Отклонено — к моменту появления API и БД границы пришлось бы вводить заново.

### D2. Навигация — `container.AppTabs`

Три вкладки: Библиотека, Поиск, Настройки. Расположение: `TabLocationBottom` при `fyne.CurrentDevice().IsMobile()`, иначе `TabLocationLeading`. Экраны создаются один раз при старте и хранятся в оболочке; переключение не пересоздаёт их.

*Альтернатива:* собственный роутер со стеком экранов. Понадобится для перехода «Библиотека → Галерея → Читалка» в следующих изменениях; сделаем его тогда поверх вкладок (стек внутри вкладки). Сейчас не нужен.

### D3. Toast — отдельный слой в `container.NewStack`

Содержимое окна: `Stack(tabs, toastLayer)`. `toastLayer` — контейнер, прижимающий карточку уведомления к низу; пустой слой не перехватывает нажатия. Показ: `Show(text)` из любой горутины оборачивает изменения в `fyne.Do`, скрытие — через `time.AfterFunc` + `fyne.Do`. Новое уведомление заменяет текущее и перезапускает таймер.

*Альтернатива:* `widget.PopUp` / `Canvas().Overlays()`. Отклонено — pop-up в Fyne модальный по поведению (перехватывает ввод) и закрывается по нажатию вне него.

### D4. Потокобезопасность UI (явно)

- `FyneApp.toml`: `[Migrations] fyneDo = true` — Fyne сообщает о доступе к UI вне основного потока.
- Любая фоновая работа (в будущем — сканирование, загрузка обложек) идёт в горутинах сервисов и возвращает данные; UI применяет их внутри `fyne.Do`.
- Сервисы не получают ссылок на виджеты; обратная связь — через возвращаемые значения или колбэки, которые UI сам оборачивает в `fyne.Do`.
- Toast — единственный UI-компонент в этом изменении, вызываемый из горутин; его API потокобезопасен по контракту.

### D5. Сеть, API-пути и rate limiting (явно)

В этом изменении нет HTTP-клиента, API-путей и ограничения частоты запросов. Модель подготовлена к ним ключом `<источник>:<id>` и полем `ExternalID`. Требование для будущего сетевого источника фиксируется здесь: ограничение частоты должно применяться **до** отправки запроса (ожидание на мьютексе/token bucket), пути API проверяются спайком против реального сервиса до написания клиента.

### D6. Папка библиотеки

- **Windows/ПК:** `os.UserHomeDir()` + `MangaReader/manga`.
- **Android:** внешний каталог файлов приложения (`Context.getExternalFilesDir(null)` + `/manga`). Fyne отдаёт только внутренний `Storage().RootURI()`, недоступный пользователю. Способ получения — **спайк S1**:
  1. `driver.RunNative` → `*driver.AndroidContext` → JNI-вызов `getExternalFilesDir(null)` (cgo, `//go:build android`);
  2. если (1) не удаётся — построить путь `/storage/emulated/0/Android/data/<appID>/files` из `EXTERNAL_STORAGE` и проверить, что `os.MkdirAll` в нём разрешён.
- Интерфейс пакета: `paths.LibraryDir() (string, error)` и `paths.EnsureLibraryDir() (string, error)`; платформенная часть — через build-теги `android` / `!android`.
- Доступ к файлам — обычные `os`-пути; это позволит `archive/zip.OpenReader` работать напрямую в `local-library`.

*Альтернатива:* выбор папки через SAF (`dialog.NewFolderOpen`). Отклонено по решению пользователя (фиксированная папка) и из-за сложности: content://URI, отсутствие `io.ReaderAt` для zip, сохранение прав между запусками.

### D7. Модель

```go
type Key struct{ Source, ID string }            // "local:example.zip"
type Tag struct{ Type, Name string }            // нормализованы в NewTag
type Page struct{ Name string }                 // открытие — ответственность источника
type Gallery struct {
    Key         Key
    ExternalID  int64
    Title, AltTitle, Scanlator string
    Tags        []Tag       // без дубликатов, порядок добавления
    Uploaded    time.Time
    NumPages, Favorites int
    Pages       []Page      // натуральный порядок
    File        FileInfo    // Path, Size, ModTime
}
```

- `Page` не содержит `Open()`: доступ к данным страницы (zip, сеть) — задача источника, модель остаётся чистыми данными и сериализуемой.
- Натуральная сортировка — собственная реализация (~40 строк), без внешней зависимости.
- `ChooseTitle(en, jp, filename) (title, alt string)` — чистая функция по правилу из спецификации.

### D8. Модель запроса и индекс

```go
type Field int  // Title, Tag, ID, Pages, Uploaded, Favorites, Scanlator, Added, Size
type Op int     // Contains, Has, NotHas, Eq, Lt, Gt, Between
type Value struct{ Text string; Tag model.Tag; Num, Num2 int64; Time, Time2 time.Time }
type Filter struct{ Field Field; Op Op; Value Value }
type Query struct{ Text string; Filters []Filter; Sort Sort; Limit, Offset int }

type FieldInfo struct{ Field Field; Name string; Kind ValueKind; Ops []Op; Sortable bool }
func Fields() []FieldInfo

type Index interface {
    Upsert(ctx context.Context, g model.Gallery) error
    Remove(ctx context.Context, k model.Key) error
    Search(ctx context.Context, q Query) (keys []model.Key, total int, err error)
    SuggestTags(ctx context.Context, tagType, prefix string, limit int) ([]TagCount, error)
}
```

- Типизированный `Value` вместо `any` — ошибки типов ловятся валидацией, а не в рантайме индекса.
- Таблица `Fields()` — единственный источник правды: из неё будут строиться парсер строки (ПК) и панель фильтров (Android).
- `NopIndex` — пустая реализация для этого изменения.

*Альтернатива:* сразу SQLite-реализация. Отложено до результата спайка S2 и изменения `local-library`, где появятся данные для индексации.

### D9. Спайк S2: SQLite на Android

Отдельный модуль `spikes/sqlite-android` (Fyne-окно с одной кнопкой): открыть БД в `Storage().RootURI()`, создать таблицу FTS5, вставить и найти строку, показать результат. Собрать `fyne package -os android`, запустить на устройстве/эмуляторе. Итог (работает / не собирается / падает) записывается в раздел **Spike results** этого документа. Решение:
- работает → `local-library` реализует `Index` на SQLite (схема `galleries`, `tags`, `gallery_tags`, FTS5 по названиям);
- не работает → индекс в памяти с тем же интерфейсом; SQLite пересматривается позже.

### D10. Сборка

Makefile: `run`, `test`, `build-windows` (`go build -ldflags "-H windowsgui -X main.version=…"`), `build-android` (`fyne package -os android -app-id io.github.mangareader.app`), `clean`. Иконка — `Icon.png` в корне (плейсхолдер).

## Risks / Trade-offs

- [Путь к внешнему каталогу на Android не получается из Go без JNI] → спайк S1 с двумя вариантами; крайний запасной вариант — внутренний `RootURI` и импорт файлов позже.
- [Android удаляет `Android/data/<appID>` вместе с приложением — пользователь теряет мангу] → предупреждение на экране «Настройки»; альтернативные места — отдельное будущее изменение.
- [Android 11+ ограничивает доступ к `Android/data` из файловых менеджеров на телефоне] → основной сценарий — копирование по USB с ПК, где каталог виден; фиксируется в README.
- [`modernc.org/sqlite` не собирается или падает на Android] → спайк S2 до начала `local-library`; интерфейс `Index` позволяет заменить реализацию.
- [Android NDK/SDK не установлены] → задача проверки окружения идёт первой; без неё Android-сценарии не отмечаются выполненными.
- [Стек-слой toast перехватывает нажатия] → проверить вручную на обеих платформах; при проблеме — карточка фиксированного размера без растягивания.

## Migration Plan

Не применимо: новый проект, данных и пользователей нет.

## Open Questions

- Нужен ли подкаталог внутри папки библиотеки (вложенные папки с архивами) — влияет на ключ `local:<относительный путь>`; до решения ключ строится от относительного пути, что покрывает оба случая.
- Точное имя приложения на устройстве («MangaReader») и финальная иконка.

## Spike results

### Окружение (2026-09-24)
- Go 1.26.4 windows/amd64, `fyne` CLI v1.7.2, Fyne v2.8.0.
- Android SDK: `%LOCALAPPDATA%\Android\Sdk`, platforms 31/34, build-tools 31/37, NDK 21.4.7075529 (`ANDROID_NDK_HOME` не задан — задаётся при сборке), gcc (mingw64) для cgo.
- Эмуляторы: Pixel_9_Pro_API_35 и др., **только x86_64**. Реального arm64-устройства нет.

### S1: папка библиотеки на Android — ✅ работает
- `driver.RunNative` → JNI `Context.getExternalFilesDir(null)` возвращает `/storage/emulated/0/Android/data/io.github.mangareader.app/files` (запасной путь не понадобился; при его использовании в лог пишется предупреждение).
- **Найдено при проверке:** `os.MkdirAll` из-за umask создаёт `manga` как `drwxr-s---`; группа `ext_data_rw` (USB/MTP, adb) не может писать, `adb push` → `Permission denied`. Исправлено: на Android после создания `chmod 2770` (`fixDirPerm`). После исправления `adb push example.zip` проходит; права уже созданной папки чинятся при следующем запуске.
- Проверено на эмуляторе API 35 x86_64. Копирование через MTP с реального телефона не проверялось.

### S2: `modernc.org/sqlite` на Android — ⚠️ не подтверждено
- Windows: SQLite 3.53.3, FTS5 работает, включая кириллицу.
- Сборка `.apk` (все ABI) проходит — код компилируется под Android.
- Android x86_64 (эмулятор): **падение** `SIGSYS (SECCOMP)`, syscall 6 (`lstat`). Причина: `modernc.org/libc@v1.74.1/libc_linux_amd64.go:110` вызывает «сырые» `SYS_LSTAT`/`SYS_STAT`, которые seccomp-фильтр Android запрещает.
- Android arm64 (реальные телефоны): libc там — полностью транслированный musl (`ccgo_linux_arm64.go`), где `lstat` реализован через `fstatat`; **вероятно, работает, но не проверено** — нет arm64-устройства/образа.
- **Решение не принято** (требуется выбор пользователя): см. вопросы в итоге apply. Варианты: (а) проверить на реальном arm64-телефоне и при успехе использовать SQLite, собирая `.apk` только под arm64; (б) индекс в памяти с тем же интерфейсом `Index`; (в) другой драйвер (cgo `mattn/go-sqlite3` с NDK).

### Прочие наблюдения
- `fyne package` для мобильных не поддерживает `--src` — Makefile делает `cd cmd/mangareader`.
- `fyneDo`: `go run` из корня читает `FyneApp.toml` → проверки потоков включены (нарушений в логе нет). Релизные сборки получают тег `migrated_fynedo` явно, т.к. `go build` в `dist/` не видит `FyneApp.toml`.
- Версия берётся из `-ldflags`, иначе из метаданных Fyne (`FyneApp.toml` / `fyne package`).
- В светлом режиме системы на эмуляторе фон окна чёрный при светлых карточках; в тёмном режиме всё корректно. Похоже на особенность рендера эмулятора (swiftshader) — проверить на реальном устройстве.

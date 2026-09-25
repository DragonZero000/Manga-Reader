## Context

Сейчас `paths.EnsureLibraryDir()` возвращает фиксированный путь (ПК: `~/MangaReader/manga`; Android: `getExternalFilesDir` через JNI + `chmod 2770`), а библиотека работает только с путями: `filepath.WalkDir`, `zip.OpenReader(g.File.Path)` в `ReadArchive`, `OpenPage`, `PageSizes`. Режим читалки хранится в `fyne.App.Preferences()`, которые Fyne на ПК пишет в `%APPDATA%\fyne\<id>\preferences.json` при каждом выходе, если у приложения есть ID (`needsSaveBeforeExit` в `app/preferences.go`).

Исследовано в Fyne 2.8 / fyne CLI 1.7.2:
- `dialog.NewFolderOpen` на Android вызывает `ACTION_OPEN_DOCUMENT_TREE` с `FLAG_GRANT_READ_URI_PERMISSION` (`GoNativeActivity.java`), но не вызывает `takePersistableUriPermission` — после перезапуска доступ пропадёт.
- Fyne умеет перечислять детей дерева (`android.c`, `buildChildDocumentsUriUsingTree`), но не отдаёт размер и дату, а чтение — только потоком (`openInputStream`), без произвольного доступа, нужного `archive/zip`.
- С Android 11 в системном выборе нельзя выбрать корень `Download`, но можно его подпапку.
- Отладочная сборка `fyne package` — `targetSdkVersion=29`, релизная — 35; SAF не зависит от этого.

Сеть, API и rate limiting в этом изменении не используются.

## Goals / Non-Goals

**Goals:** портативная ПК-версия без записи вне папки приложения; Android-библиотека в выбранной пользователем папке без широких разрешений; библиотека, читалка и поиск работают одинаково поверх двух хранилищ.

**Non-Goals:** перенос архивов, корень `Download`, «доступ ко всем файлам», запись на Android, выбор папки на ПК (см. proposal).

## Decisions

### D1. Интерфейс хранилища (`internal/storage`)

```go
type Entry struct {
    RelPath string    // относительный путь, прямые слеши
    Size    int64
    ModTime time.Time
    IsDir   bool
}
type File interface { io.ReaderAt; io.Closer; Size() int64 }

type Storage interface {
    Name() string                                   // для UI: путь (ПК) или «Download/manga» (Android)
    Walk(ctx context.Context, fn func(Entry) error) error // все элементы рекурсивно
    Open(relPath string) (File, error)
}
var ErrUnavailable = errors.New("хранилище недоступно")
```

Пакет без виджетов (добавляется в `archtest`). `Walk` на ПК — `filepath.WalkDir`; на Android — обход дерева SAF. Отбор архивов и пропуск скрытых имён остаются в сканере (одинаковые правила для обеих платформ).

### D2. Библиотека поверх хранилища

- `Scanner` получает `Storage` вместо корня; вместо `fs.FileInfo` использует `Entry` (размер и дата — для инкрементальности и сортировки, как раньше).
- `ReadArchive(f File, relPath string, e Entry)` — `zip.NewReader(f, f.Size())`; открытие и закрытие файла — в сканере.
- `Source.OpenPage` и `PageSizes` открывают архив через `Storage.Open(g.Key.ID)`; `openZip`-подмена в тестах заменяется счётчиком открытий хранилища.
- `Gallery.File.Path` становится отображаемым путём (`Storage.Name()` + относительный путь); для доступа к данным не используется.
- Ошибка `Walk`/`Open` с `ErrUnavailable` пробрасывается из `Source.Scan` — UI по ней показывает экран выбора папки.

### D3. ПК: папка приложения и портативность

- `paths.AppDir()`: `os.Executable()` → `filepath.EvalSymlinks` → каталог; если путь внутри `os.TempDir()` или содержит `go-build` (запуск через `go run`) — `os.Getwd()`. Та же эвристика, что в Fyne (`getProjectPath`).
- Библиотека — `AppDir/manga` (`fsStorage`, папка создаётся при старте); настройки — `AppDir/settings.json`.
- **Fyne без ID на ПК:** в `main` до создания приложения — `app.SetMetadata(fyne.AppMetadata{Name: "MangaReader", Version: version, Migrations: {"fyneDo": true}})` и `fyneapp.New()`. С пустым ID Fyne не создаёт хранилище настроек и не пишет `preferences.json`; заданное имя отключает поиск `FyneApp.toml` рядом с exe. Платформа определяется build-тегом (`android`), а не `fyne.CurrentDevice()`, которому нужно уже созданное приложение.
- Fyne по-прежнему *читает* общие `%APPDATA%\fyne\settings.json` и `theme.json` (если есть) — чтение не нарушает требования «не создавать и не изменять файлы».

*Альтернатива:* подменить `USERPROFILE` для процесса, чтобы Fyne писал в папку приложения. Отклонено — хрупко и влияет на всё, что использует домашний каталог.

### D4. Android: SAF через JNI (`storage/saf_android.go` + `saf_android.c`)

- Выбор: `dialog.NewFolderOpen(cb, win)` Fyne. Сразу в колбэке (пока временный доступ действует) — JNI `ContentResolver.takePersistableUriPermission(treeUri, FLAG_GRANT_READ_URI_PERMISSION)`; `treeUri` сохраняется в настройках (`library.tree`).
- Проверка при старте: `getPersistedUriPermissions()` содержит `treeUri` с правом чтения; иначе — `ErrUnavailable`.
- `Walk`: от `DocumentsContract.getTreeDocumentId(treeUri)` рекурсивно: `buildChildDocumentsUriUsingTree` + `ContentResolver.query` с колонками `DOCUMENT_ID, DISPLAY_NAME, MIME_TYPE, SIZE, LAST_MODIFIED`; папка — `MIME_TYPE_DIR`. Результат одной папки передаётся из C в Go одной строкой (записи через `\x1e`, поля через `\x1f`), Go разбирает. `SecurityException`/`FileNotFound` → `ErrUnavailable`.
- Соответствие `RelPath → documentId` запоминается при `Walk`; `Open(rel)` → `buildDocumentUriUsingTree` → `openFileDescriptor(uri, "r")` → `ParcelFileDescriptor.detachFd()` → `os.NewFile(fd)`; размер — из `Entry` (или `fstat`). `*os.File` поддерживает `ReadAt`; для документов внутренней памяти (`ExternalStorageProvider`) дескриптор — обычный файл, копирования нет.
- Имя для UI: из `treeDocumentId` вида `primary:Download/manga` → `Download/manga`.
- Все JNI-вызовы — через `driver.RunNative` из фоновых горутин (скан, загрузчики страниц); JNI-окружение привязывается к потоку.

*Альтернатива:* `MANAGE_EXTERNAL_STORAGE` или `requestLegacyExternalStorage` — отклонены пользователем (широкие права / не работает в релизной сборке). *Запасной вариант*, если `openFileDescriptor` не даст произвольного доступа: копирование архива во временный файл кэша приложения перед чтением.

### D5. Настройки (`storage.Settings`)

```go
type Settings interface {
    String(key, fallback string) string
    SetString(key, value string)
}
```

ПК — `fileSettings`: JSON-объект в `AppDir/settings.json`, чтение при старте (ошибка/нет файла → пусто), запись при каждом изменении атомарно (временный файл + `os.Rename`), мьютекс. Android — обёртка над `fyne.App.Preferences()` (внутреннее хранилище приложения, не видно пользователю). Читалка получает `Settings` вместо `fyne.App.Preferences()`.

### D6. UI

- `Services` получает `Storage`, `Settings` и (Android) `ChooseFolder(win, done)`.
- Android, первый запуск: `Lifecycle.OnStarted` — если `library.tree` пуст, сразу открыть выбор; после выбора — сохранить, взять постоянное разрешение, пересканировать.
- Библиотека: при `ErrUnavailable` (Android) — экран «Выберите папку с мангой» с кнопкой «Выбрать папку» вместо сетки и пустого состояния; на ПК — toast, как раньше.
- «Настройки»: ПК — путь `AppDir/manga` и «Скопировать путь»; Android — имя папки или «Папка не выбрана» и кнопка «Изменить папку»; предупреждение про удаление папки вместе с приложением убирается.

### D7. Потоки UI (явно)

JNI и файловые операции — только в фоновых горутинах (скан, загрузчики). Колбэк выбора папки Fyne приходит в UI-потоке: взятие разрешения (быстрый JNI-вызов) выполняется сразу, сканирование — в горутине, результат — через `fyne.Do`.

### D8. Удаляется

`paths_android.go` / `paths_android.c` (`getExternalFilesDir`, запасной путь, `chmod`), `paths.EnsureLibraryDir` в прежнем виде, `LibraryDir` в `Services` заменяется на `Storage.Name()`.

## Risks / Trade-offs

- [`takePersistableUriPermission` после диалога Fyne может не сработать, если результат не несёт флаг постоянного доступа] → **задача 1 — проверка на эмуляторе до переработки**. При неудаче — свой вызов `ACTION_OPEN_DOCUMENT_TREE` потребовал бы Java-кода в активности, которой управляет Fyne; тогда остановиться и пересмотреть подход.
- [`openFileDescriptor` вернёт непоследовательный (pipe) дескриптор у нестандартных провайдеров (облачные папки)] → `ReadAt` вернёт ошибку → запасной вариант с копированием во временный файл; для внутренней памяти и SD-карты — обычный файл.
- [Скорость обхода SAF на больших папках] → запросы по папкам, без чтения содержимого; инкрементальность по размеру и дате сохраняется. Замерить на 300 архивах.
- [Пользователь ждёт архивы из корня `Download`] → текст на экране выбора: «Выберите или создайте папку внутри Download, например Download/manga, и складывайте архивы туда».
- [Портативная папка в `Program Files` недоступна для записи] → toast с ошибкой и путём; запасного места нет по требованию.
- [Старые архивы «пропадут» после обновления] → BREAKING в proposal и README; путь старой папки можно упомянуть в README.

## Migration Plan

Переноса данных нет: пользователь перемещает архивы вручную (ПК: из `%USERPROFILE%\MangaReader\manga` в `manga` рядом с exe; Android: из `Android/data/…/files/manga` в выбранную папку, пока старая сборка установлена). Настройки режима читалки на ПК сбрасываются к значению по умолчанию.

## Open Questions

- Показывать ли на Android подсказку со ссылкой на системный файловый менеджер для переноса файлов из корня `Download`.

## Spike results

### SAF на эмуляторе Android 15 (2026-09-24) — ✅ работает

Спайк `spikes/saf-android` (отладочная сборка `fyne package`, targetSdk 29):
- `dialog.ShowFolderOpen` Fyne открывает системный выбор; корень памяти выбрать нельзя («Can’t use this folder»), `Download` → `manga` — можно; система спрашивает «Allow … to access files in manga? This will let … access current and future content». Результат — `content://com.android.externalstorage.documents/tree/primary%3ADownload%2Fmanga`.
- `takePersistableUriPermission` из колбэка Fyne — **успешно**; `getPersistedUriPermissions` содержит дерево с правом чтения.
- Обход: `example.zip` и `series/vol1.zip` (подпапка) с размером и временем изменения; `Download/root-file.zip` (корень) не виден — как ожидалось. 3 файла — ~46–80 мс.
- `openFileDescriptor` → `detachFd` → `os.NewFile` → `zip.NewReader`: произвольный доступ работает (прочитан последний элемент архива, 118 КБ).
- После `force-stop` и повторного запуска — доступ без повторного выбора, обход и чтение работают.
- Файл, добавленный в папку после выдачи разрешения (`adb push` — как чужое приложение/браузер), виден при следующем обходе.

Решение: подход D4 подтверждён, запасной вариант с копированием не нужен.

## Implementation notes (2026-09-24)

- **`AppDir`: только признак `go-build`.** Первая версия (как в D3) считала «внутри `os.TempDir()`» признаком `go run`; при проверке exe, лежащий во временной папке, взял рабочую папку `C:\` и создал `C:\manga`. Приложение могут запустить и прямо из распакованного во временную папку архива — теперь рабочая папка берётся только для исполняемого файла в каталоге с сегментом `go-build…` (`go run`, `go test`).
- **SAF: соответствие «путь → документ» заполняется по ходу обхода.** Сканер открывает архив прямо в колбэке `Walk`, а первая версия запоминала соответствие только в конце обхода — все архивы давали «файл не найден в папке». Исправлено; в конце обхода карта заменяется целиком (удалённые файлы исчезают).
- Ошибка открытия файла подписывается «не удалось открыть», а не «не zip-архив».
- **Проверено на Windows:** exe в отдельной папке создаёт рядом `manga`; `settings.json` появляется после смены режима (`"reader.mode": "strip"`); после штатного закрытия в `%APPDATA%yne`, `%LOCALAPPDATA%yne` и `%USERPROFILE%\MangaReader` не изменилось ни одного файла (сравнение списков с размерами и временем до/после). `go run` создаёт `manga` в корне проекта.
- **Проверено на эмуляторе Android 15:** чистая установка → системный выбор открывается сам → `Download/manga` → 3 архива, включая подпапку; страница произведения и читалка читают архив через SAF; переустановка поверх — выбор сохранён; переименование папки → экран «Выберите папку с мангой» → выбор новой папки → библиотека.
- **Скорость SAF:** 304 новых архива (480 МБ) — первое сканирование 3,6 с (каждый архив открывается через JNI для чтения `meta.json`), повторное без изменений — 85 мс. На ПК для сравнения — 25 мс на 300 архивов.

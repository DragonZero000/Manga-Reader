## 1. Проверка SAF на эмуляторе (до переработки)

- [x] 1.1 Спайк `spikes/saf-android` (отдельный модуль): кнопка → `dialog.NewFolderOpen`; в колбэке JNI `takePersistableUriPermission`; обход дерева с размером и датой; `openFileDescriptor` → `os.NewFile` → `zip.NewReader` для `example.zip`; вывод результатов в окно и logcat
- [x] 1.2 Эмулятор Android 15: создать и выбрать `Download/manga` в диалоге, положить архивы через `adb push` (в папку и подпапку), проверить список и чтение zip; перезапустить приложение и проверить доступ без повторного выбора (`getPersistedUriPermissions`); записать итог в design.md → Spike results; при неудаче — остановиться и пересмотреть подход

## 2. Хранилище и настройки (`internal/storage`)

- [x] 2.1 `Entry`, `File`, `Storage`, `ErrUnavailable`; `fsStorage` (`Walk` через `WalkDir`, `Open` через `os.Open`); тесты во временной папке
- [x] 2.2 `Settings`: `fileSettings` (JSON, атомарная запись, повреждённый файл → значения по умолчанию) и обёртка над `fyne.Preferences`; тесты; `storage` в `archtest`

## 3. Библиотека поверх хранилища

- [x] 3.1 `Scanner` на `Storage.Walk` + `Entry`; `ReadArchive(File, relPath, Entry)`; открытие/закрытие файла в сканере
- [x] 3.2 `Source`: `OpenPage` и `PageSizes` через `Storage.Open`; `ErrUnavailable` пробрасывается из `Scan`; `Gallery.File.Path` — отображаемый путь
- [x] 3.3 Перевести тесты `internal/library` на `fsStorage` во временной папке (счётчик открытий вместо подмены `openZip`); тест недоступного хранилища

## 4. ПК: портативный режим

- [x] 4.1 `paths.AppDir()` (исполняемый файл / `go run` → рабочая папка); тесты эвристики; удалить `paths_android.*` и старые `LibraryDir/EnsureLibraryDir`
- [x] 4.2 `main`: на ПК — `app.SetMetadata(Name, Version, fyneDo)` + `fyneapp.New()` без ID; на Android — как сейчас; платформа по build-тегу
- [x] 4.3 `app.Services`: `Storage` (`fsStorage(AppDir/manga)` с созданием папки) и `Settings` (`AppDir/settings.json`); читалка использует `Settings`

## 5. Android: SAF

- [x] 5.1 `safStorage` (build-тег `android`, cgo): JNI `takePersistableUriPermission`, проверка `getPersistedUriPermissions`, обход дерева с метаданными, `Open` через `openFileDescriptor`/`detachFd`, отображаемое имя; ошибки доступа → `ErrUnavailable`
- [x] 5.2 `app.Services` на Android: `Settings` через `Preferences`, `Storage` из сохранённого `library.tree`, `ChooseFolder(win, done)` (диалог Fyne + постоянное разрешение + сохранение)

## 6. UI

- [x] 6.1 Библиотека: экран «Выберите папку с мангой» с кнопкой при `ErrUnavailable` (Android); первый запуск без папки — сразу системный выбор; после выбора — сканирование
- [x] 6.2 «Настройки»: ПК — путь `AppDir/manga`; Android — имя папки / «Папка не выбрана» и «Изменить папку»; убрать предупреждение про `Android/data`
- [x] 6.3 Пустое состояние библиотеки показывает `Storage.Name()`

## 7. Проверка

- [x] 7.1 `make test` проходит, включая `-race` для `internal/storage`, `internal/library`, `internal/ui/...`
- [x] 7.2 Windows: `dist\mangareader.exe` в отдельной папке — создаётся `manga` рядом, `settings.json` появляется после смены режима, в `%APPDATA%\fyne` и профиле нет новых/изменённых файлов приложения (сравнить до/после); `go run` — `manga` в корне проекта
- [x] 7.3 Android (эмулятор): первый запуск → выбор папки → `Download/manga`; архивы через `adb push` в папку и подпапку видны; чтение страниц; перезапуск без повторного выбора; «Изменить папку»; отзыв доступа → экран выбора папки; замер скана 300 архивов записать в design.md
- [x] 7.4 README: новые папки на ПК и Android, перенос архивов из старых папок

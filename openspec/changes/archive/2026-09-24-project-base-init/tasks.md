## 1. Окружение и каркас модуля

- [x] 1.1 Проверить окружение: `go version` (≥1.26), `fyne version`, наличие Android SDK/NDK (`ANDROID_HOME`, `ANDROID_NDK_HOME`); записать результат в design.md → Spike results
- [x] 1.2 Создать `go.mod` (модуль `mangareader`, go 1.26, `fyne.io/fyne/v2 v2.8.x`), `FyneApp.toml` (Name `MangaReader`, ID `io.github.mangareader.app`, `[Migrations] fyneDo = true`), плейсхолдер `Icon.png`
- [x] 1.3 Создать `cmd/mangareader/main.go` с `var version = "dev"`, открывающий пустое окно «MangaReader»; `go run ./cmd/mangareader` запускается
- [x] 1.4 Makefile: `run`, `test`, `build-windows`, `build-android`, `clean`; `.gitignore` для `dist/`
- [x] 1.5 Перенести `example.zip` в `testdata/example.zip`

## 2. Модель произведения (`internal/model`)

- [x] 2.1 `Tag`, `NewTag` с нормализацией (регистр, пробелы), `String()` → `тип:имя`, список известных типов и `Known()`; unit-тесты по сценариям спецификации
- [x] 2.2 `Key`, `ParseKey`, `String()`; тесты: `local:example.zip`, строка без `:`, пустой источник
- [x] 2.3 Натуральная сортировка имён (`NaturalLess`) без внешних зависимостей; тесты: `1, 2, 10`, регистр, имена без цифр, одинаковые префиксы
- [x] 2.4 `Gallery`, `Page`, `FileInfo`; `AddTag` без дубликатов, `SortPages`, `Cover() (Page, bool)`; тесты, включая галерею без страниц
- [x] 2.5 `ChooseTitle(en, jp, filename)`; тесты: оба названия, только японское, ни одного

## 3. Модель запроса и контракт индекса (`internal/search`)

- [x] 3.1 `Field`, `Op`, `ValueKind`, `FieldInfo`, `Fields()` — таблица полей из спецификации; тест, что все 9 полей описаны и у каждого есть операции
- [x] 3.2 `Value`, `Filter`, `Sort`, `Query`, `NewQuery()` с сортировкой по умолчанию (`added`, по убыванию)
- [x] 3.3 `Query.Validate()`: операция допустима для поля, тип значения, диапазон `Between`, неотрицательные лимит/смещение, нормализация тега в фильтре; ошибки указывают фильтр; табличные тесты по сценариям спецификации
- [x] 3.4 Интерфейс `Index`, `TagCount`, реализация `NopIndex`; тест, что `NopIndex.Search` возвращает 0 результатов без ошибки

## 4. Папка библиотеки (`internal/paths`)

- [x] 4.1 `LibraryDir()` для ПК (`!android`): `~/MangaReader/manga`; `EnsureLibraryDir()` через `os.MkdirAll`, не трогает существующие файлы; тесты с временным HOME
- [x] 4.2 Спайк S1: получить внешний каталог файлов приложения на Android (вариант 1 — `driver.RunNative` + JNI `getExternalFilesDir`; вариант 2 — путь из `EXTERNAL_STORAGE`); записать итог в design.md → Spike results
- [x] 4.3 Реализовать `LibraryDir()` для `android` по итогу S1

## 5. Оболочка UI (`internal/ui`, `internal/app`)

- [x] 5.1 `internal/app`: создание Fyne-приложения, вызов `EnsureLibraryDir`, `NopIndex`, передача зависимостей в UI; ошибки не завершают приложение
- [x] 5.2 `ui.Toast`: слой в `container.NewStack`, `Show(text)` потокобезопасен (`fyne.Do`), автоскрытие через 3 с, новый toast заменяет текущий
- [x] 5.3 `ui.Shell`: `AppTabs` (Библиотека, Поиск, Настройки), `TabLocationBottom` на мобильных, `TabLocationLeading` на ПК; экраны создаются один раз
- [x] 5.4 Экраны-заглушки: «Библиотека» (пустое состояние + путь к папке), «Поиск» (поле ввода, пустой результат через `NopIndex`), «Настройки» (полный путь библиотеки, версия, предупреждение об удалении папки вместе с приложением на Android)
- [x] 5.5 Toast с ошибкой, если папку библиотеки не удалось создать
- [x] 5.6 Тест/проверка импортов: `internal/paths`, `internal/model`, `internal/search` не зависят от `fyne.io/fyne/v2/widget` и `container` (`go list -deps`)

## 6. Спайк S2: SQLite на Android

- [x] 6.1 Модуль `spikes/sqlite-android` (отдельный go.mod): окно с кнопкой, создание БД в `Storage().RootURI()`, таблица FTS5, вставка и поиск строки, результат в label
- [x] 6.2 Запуск спайка на Windows
- [ ] 6.3 Сборка `fyne package -os android` и запуск на устройстве/эмуляторе; записать итог и решение (SQLite / индекс в памяти) в design.md → Spike results
  - Итог: сборка ✓, Windows ✓, эмулятор x86_64 ✗ (seccomp `lstat` в modernc/libc amd64), arm64 не проверен; решение не принято

## 7. Сборка и проверка

- [x] 7.1 `make test` проходит (`go test ./...`, `go vet ./...`)
- [x] 7.2 `make build-windows`: запуск `.exe`, ручная проверка навигации, возврата на экран без сброса состояния, toast поверх экрана без предупреждений `fyneDo` в логе
- [ ] 7.3 `make build-android`: установка `.apk`, запуск, нижние вкладки, папка библиотеки создана и видна с ПК по USB
  - Эмулятор API 35: установка, нижние вкладки, папка, `adb push` ✓; USB/MTP на реальном телефоне не проверен
- [x] 7.4 README: сборка, где лежит папка библиотеки на каждой платформе, как копировать архивы на Android

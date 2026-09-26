## 1. Замер «до»

- [x] 1.1 Журнал сканирования: число открытых архивов в строке `библиотека: …` (счётчик в `Scanner`); замер на S25 Ultra с 300 архивами (design D9): время до сетки, время первого сканирования, открытия при повторном запуске; результаты в design.md

## 2. Нормализация и общий набор тестов индекса

- [x] 2.1 `search.Normalize` и `NormVersion` (`golang.org/x/text/unicode/norm` — прямая зависимость); парсер и `MemIndex` на ней; тесты: ё/е, регистр кириллицы, полноширинные, составные символы
- [x] 2.2 Пакет `internal/search/indextest` с `Run(t, newIndex)`: перенести сценарии из `memindex_test.go`/`search_test.go` (слова, фильтры, сортировки с натуральным порядком, лимит/смещение, подсказки тегов) + спецсимволы `% _ \`, короткие слова, нормализация; `MemIndex` проходит набор

## 3. Каталог SQLite

- [x] 3.1 `go get github.com/mattn/go-sqlite3`; тег `sqlite_fts5` в Makefile (`TAGS`, `test`, `run`), `tools/android-build` (`buildTags` + тест), шаги `go vet`/`go test` в `ci.yml` и `release.yml`; `internal/catalog/require_tag.go` (design D1)
- [x] 3.2 `catalog.Open(path, root)`: схема (design D2), PRAGMA, `user_version`, пересоздание при другой версии/повреждении (включая `-wal`/`-shm`), смена `root` очищает таблицы, `Reset(root)`, `Close`; коллация `NATSORT` (`NATURAL` — ключевое слово SQL) в `ConnectHook`; тесты: новый файл, мусор вместо файла, старая версия, другой root
- [x] 3.3 `catalog.Index` (design D4): Upsert/Remove/Search/SuggestTags; экранирование LIKE; проверка согласованности `docs`/`files` при открытии; проходит `indextest.Run`

## 4. Кэш сканера в каталоге

- [x] 4.1 `library.ScanStore`, `StoredEntry`, `ScanDelta`; `NewScanner(st, store)` загружает кэш; в конце `Scan` — один `Save` с изменениями (без временных «занят»); `NewSource(st, idx, store)`; тесты на фейковом хранилище: без изменений — `Save` пустой, новый/изменённый/удалённый/ошибка
- [x] 4.2 Сохранение ошибок с видом (design D3) и восстановление с тем же текстом и `errors.Is`; тест для каждого вида
- [x] 4.3 `catalog` реализует `ScanStore`: `files` + индекс + удаление обложек в одной транзакции; интеграционный тест: сканирование → новый сканер над тем же каталогом → повторное сканирование без изменений не открывает архивов (`openHook`), результат тот же
- [x] 4.4 `Source.LoadCatalog()`: галереи и ошибки из кэша без обхода папки; журнал `каталог: загружено N галерей за M`

## 5. Ссылки: две копии

- [x] 5.1 Таблица `links` в каталоге; `ScanDelta.Links` (ссылки из `sourcer` после разбора) и удаление по `Delete` в транзакции `Save`; `readEntry`: запасная ссылка из `ScanStore`, если нет ни в `meta.json`, ни в `sourcer`
- [x] 5.2 `library.LoadLinks(path, backup)`: отсутствующий/повреждённый `links.json` восстанавливается из каталога и записывается; `catalog.ImportLinks` для нового каталога; тесты: мусор в `links.json`, удалённый каталог, потеря метки (фейковый `sourcer`), удалённый файл чистит обе копии

## 6. Обложки на диске

- [x] 6.1 `thumbs.Store`, `Cache.SetStore`; воркер: сначала `Load` (JPEG → RGBA), иначе декодирование + `jpeg.Encode` q85 + `Save`; тесты: второй кэш над тем же хранилищем не открывает архив, другой размер области — декодирование заново
- [x] 6.2 `catalog` реализует `thumbs.Store` (таблица `covers`, проверка `w`/`h`); удаление обложек удалённых и изменённых файлов в `Save` сканера; тест

## 7. Подключение

- [x] 7.1 `internal/app`: открытие каталога (ПК — рядом с `settings.json`, Android — личная папка), запасной режим без каталога (`MemIndex`), `SetFolder` → `Reset`, порядок ссылок (design D10); `NewForTest` без изменения API для тестов UI
- [x] 7.2 `internal/ui/shell.go`: при запуске `LoadCatalog` в фоне → показ через `fyne.Do` → `AutoRefresh`; закрытие каталога при выходе; тест: оболочка над каталогом с галереями показывает их до сканирования
- [x] 7.3 `internal/archtest`: пакеты `catalog` и `search/indextest` в списке сервисных; `go list` с тегом `sqlite_fts5`

## 8. Документация и лицензии

- [x] 8.1 `THIRD_PARTY_NOTICES.md`: `github.com/mattn/go-sqlite3` (MIT), SQLite (public domain), `golang.org/x/text` (BSD-3) — если ещё не указан
- [x] 8.2 `docs/{ru,en}/user-guide.md`: каталог `library.db` (где лежит, что это кэш, можно удалить; ссылки скачанных файлов хранятся и в нём), поиск без учёта ё и ширины символов; `docs/{ru,en}/building.md`: тег `sqlite_fts5`, первая сборка компилирует SQLite; `docs/{ru,en}/architecture.md`: пакет `internal/catalog`, `library.db` в карте файлов

## 9. Проверка

- [x] 9.1 `make test`, `make build-windows`, `make build-android`; локально те же команды, что в CI: `go vet -tags sqlite_fts5 ./...`, `go test -tags sqlite_fts5 ./...`, `go run ./tools/android-build`
- [x] 9.2 **Замер «после»** (design D9) на S25 Ultra; проверки вручную: повреждённый `library.db` пересоздаётся, обложки после перезапуска без открытия архивов (журнал), поиск `елка`/`ЁЛКА` по тестовому архиву; результаты в design.md; удалить тестовые архивы с телефона
- [ ] 9.3 GitHub Actions: после коммита на отдельной ветке и push (с разрешения пользователя) — `gh run watch` для CI; оба задания (тесты Windows и сборка APK) зелёные; шаги `release.yml` проверяются локально теми же командами (релизный тег не ставится)

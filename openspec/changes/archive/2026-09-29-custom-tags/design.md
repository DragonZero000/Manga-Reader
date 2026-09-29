## Context

- Изменение `user-data` даёт:
  - `user.db` с миграциями;
  - `works(uid, root, rel, fingerprint, orphaned_at)` и `Ensure/Lookup`;
  - сверку по отпечатку;
  - порядок в `Source.Scan`: обход → `Observer.Scanned` (сверка) → публикация и индекс.
- `model.Gallery.Tags` заполняется из `meta.json` и кэшируется в `library.db` (`files.gallery`, JSON).
- Индекс: таблицы `tags(key, type, name, name_norm)` и `docs_fts.hay` в каталоге; `MemIndex` с общими проверками `search/indextest`; `SuggestTags` уже есть.
- Страница произведения (`internal/ui/details`) строит чипы через `TagGroups(g.Tags)`. Чип — `widget.Button` на сером фоне.
- Fyne 2.8: у `fyne.TextStyle` есть `Strikethrough`; `theme.ColorNameWarning` (жёлтый/оранжевый), `theme.ColorNameError`.

## Goals / Non-Goals

**Goals:** свои тэги любого типа и скрытие оригинальных на странице произведения; хранение в `user.db`; поиск по действующим, своим и скрытым тэгам; автодополнение; согласованность индекса с пользовательскими данными.

**Non-Goals:** массовое редактирование, глобальное скрытие, экспорт, изменение архивов, тэги на карточках.

## Decisions

### D1. Модель: наложение отдельно от оригинала

```go
// model
type TagOrigin uint8 // OriginMeta, OriginCustom, OriginHidden
type Gallery struct {
    ...
    Tags   []Tag // из meta.json, как сейчас
    Custom []Tag `json:"-"` // свои, в порядке добавления
    Hidden []Tag `json:"-"` // скрытые оригинальные
}
func (g Gallery) EffectiveTags() []Tag        // Tags − Hidden, затем Custom
func (g Gallery) TagViews() []TagView         // все с происхождением (для режима редактирования)
```

`json:"-"`: наложение не попадает в кэш сканера `files.gallery`, потому что его источник — `user.db`. Скрытый тэг, которого больше нет в `meta.json` (архив изменили), в `TagViews` не показывается и в индексе не участвует; запись о нём остаётся и удаляется при сбросе.

- Отклонено: хранить итоговый список в `Tags`. Тогда теряется, что оригинал, и режим редактирования и `hidden-tag:` становятся невозможны.

### D2. Миграция `user.db` v2

```sql
CREATE TABLE user_tags(
  uid   INTEGER NOT NULL REFERENCES works(uid) ON DELETE CASCADE,
  kind  INTEGER NOT NULL,          -- 1 свой, 2 скрытый
  type  TEXT NOT NULL, name TEXT NOT NULL,   -- нормализованы model.NewTag
  seq   INTEGER NOT NULL,          -- порядок добавления своих
  PRIMARY KEY(uid, kind, type, name));
CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT);
INSERT INTO meta VALUES('epoch', <случайные 16 байт hex>);
```

API `userdata`:
- `Overlays(root) map[rel]Overlay` — одним запросом `JOIN works` для записей, которые не сироты;
- `Overlay(uid)`;
- `AddCustom(uid, tag)` → `ErrDuplicate`, если такой свой уже есть;
- `RemoveCustom`, `Hide`, `Unhide`, `Reset(uid)`, `Epoch()`.

Проверка дубликата с оригинальными тэгами — в слое `app` (D4), потому что `userdata` не знает `meta.json`.

### D3. Наложение в `library.Source`

```go
type Overlay interface {
    Apply(gs []model.Gallery)                  // заполняет Custom/Hidden по rel
}
func (s *Source) SetOverlay(o Overlay)
func (s *Source) Refresh(k model.Key) (model.Gallery, error) // перечитать наложение одной галереи
```

- `Scan`: обход → `Observer.Scanned` (сверка `user-data`, перенос по отпечатку) → `Overlay.Apply` к `Galleries`, `Added`, `Changed` → публикация → `updateIndex`. Перенесённый по отпечатку файл приходит как `Added`, поэтому индексируется уже со своими тэгами.
- `LoadCatalog`: `Apply` к галереям из каталога. Индекс не трогается, он хранит наложение (D5, D6).
- `Refresh(k)` берёт `scanMu`, так что правка, завершившаяся во время сканирования, не будет перезаписана его устаревшим результатом. Метод перечитывает наложение, заменяет галерею в новом срезе (список неизменяемый, копирование при записи) и делает `index.Upsert`.

Адаптер `Overlay` в `internal/app` вызывает `userdata.Overlays(rootKey)`.

### D4. Сервис правки в `internal/app`

```go
func (s *Services) TagsAvailable() bool
func (s *Services) EditTags(k model.Key, op TagOp) (model.Gallery, error) // не из UI-потока
// TagOp: AddCustom(tag) | RemoveCustom(tag) | Hide(tag) | Unhide(tag) | Reset
```

Шаги:
1. `Source.Get(k)`.
2. Проверка ввода и дубликата. Дубликат — если тэг есть среди оригинальных (включая скрытые) или своих: `ErrTagExists`. Ввод: пустое имя, больше 100 символов или `"` дают `ErrTagInvalid`.
3. `userdata.Ensure(root, rel, fp)`, операция, затем `Source.Refresh(k)`.

Ошибки типизированы, текст для пользователя — в UI (`screens.ErrorText` / ключи i18n).

### D5. Индекс: происхождение тэга

- Каталог: `tags(key, type, name, name_norm, src INTEGER, PRIMARY KEY(key, type, name, src))`, где `src`: 0 — оригинальный видимый, 1 — свой, 2 — скрытый. В `docs_fts.hay` попадают только действующие тэги (`src` 0 и 1).
- `search.Value` получает `TagScope` (`ScopeEffective` — нулевое значение, `ScopeCustom`, `ScopeHidden`). Фильтр тэга: `src IN (0,1)` / `= 1` / `= 2`.
- `queryFields`: `custom-tag` и `hidden-tag` (`isTag`, тип не задан, своя область). Разбор `-` у них работает как у других тэговых полей.
- `SuggestTags`: только `src IN (0,1)`, `COUNT(DISTINCT key)`.
- `MemIndex`: в `doc` отдельные множества для каждой области; `indextest` получает сценарии со своими и скрытыми тэгами, одни и те же для обоих индексов.
- `catalog.schemaVersion` повышается (4; если `user-data` и `custom-tags` выходят в одном релизе — одно повышение до 3 на оба изменения).

### D6. Согласованность индекса и `user.db`

Индекс в `library.db` хранит результат наложения. Если `user.db` заменили (восстановление из резервной копии, повреждение → новая база, база недоступна), индекс рассогласуется. Решение — эпоха:
- `library.db` `meta.overlay_epoch` хранит эпоху, с которой построен индекс;
- ожидаемая эпоха — `userdata.Epoch()` или `none`, если база недоступна;
- при запуске, если эпохи отличаются, после `LoadCatalog` в фоне выполняется `Source.Reindex` (`Apply` + `Upsert` всех галерей под `scanMu`) и эпоха записывается.

При обычной работе эпоха не меняется, и переиндексации нет.

### D7. Интерфейс страницы произведения

- **Чип `tagChip`** (свой виджет, `internal/ui/details`): фон и текст (`canvas.Text`), обработка нажатия, в режиме редактирования — маленькая кнопка ✕ или ↺.
  - Оригинальный: как сейчас.
  - Свой: фон `ColorNameWarning` с прозрачностью около 35 % и рамка `ColorNameWarning`, текст `ColorNameForeground`. Цвет — на фоне, а не на тексте: жёлтый текст на светлой теме нечитаем.
  - Скрытый: текст `ColorNameError` + `Strikethrough`.
- **Раздел тэгов**: заголовок-строка с кнопкой «Изменить тэги» (`DocumentCreateIcon`) ↔ «Готово». В режиме редактирования у каждой группы «+»; внизу `Select` «Добавить в другую группу» (известные типы, которых нет среди групп) и «Сбросить к оригиналу…» (`dialog.NewConfirm`).
- **Поле ввода** появляется в строке группы вместо «+»: `widget.Entry` + «Добавить»/«Отмена»; ошибка валидатора показывается под полем. Подсказки — встроенный список под полем внутри прокрутки, а не всплывающее окно. При открытии поля прокрутка сдвигается так, чтобы поле было в верхней трети экрана. Так на телефоне клавиатура не перекрывает поле и подсказки, и не нужно позиционировать popup.
- **Подсказки**: `SuggestTags(type, prefix, 10 + число тэгов группы)` в фоне с паузой 150 мс после ввода и токеном (устаревшие ответы отбрасываются), затем исключение уже имеющихся имён и обрезка до 10.
- `Details` получает зависимость-интерфейс `TagEditor` (`Available`, `Edit(k, op, cb)`, `Suggest(type, prefix, cb)`) и `notify func(string)` для toast. Реализация — в `Shell` поверх `Services`. Так `details` тестируется без базы.
- После успешной правки через `d.do`: если страница открыта с той же галереей (токен), обновляется `d.g`, раздел перестраивается с сохранением прокрутки и режима; вызывается `search.Rerun()`.

### Потоки и платформы

- `EditTags`, `Suggest`, `Reindex` выполняются только в горутинах. Результаты применяются в UI через `fyne.Do` (`d.do`) с проверкой токена страницы. Из UI-потока `user.db` и индекс не вызываются.
- Windows и Android ведут себя одинаково. Отличается только ввод: на телефоне поле прокручивается выше клавиатуры (D7). Платформенного кода и stub нет.
- Сторонних компонентов не добавляется.

## Risks / Trade-offs

- [`Strikethrough` не рисуется драйвером на Android GL] → проверить на эмуляторе; запасной вариант — `canvas.Line` поверх текста.
- [Контраст жёлтого фона в светлой и тёмной теме] → проверить обе темы, при необходимости подобрать прозрачность.
- [Второе полное пересканирование, если `user-data` и `custom-tags` в разных релизах] → по возможности выпускать вместе с одним повышением версии каталога.
- [Правка во время сканирования] → `Refresh` ждёт `scanMu`; UI получает результат после окончания сканирования (обычно доли секунды).
- [Устаревшие скрытые записи после изменения `meta.json`] → не влияют на показ и поиск; очищаются сбросом.

## Migration Plan

- `user.db` v1 → v2 (таблицы `user_tags`, `meta`, эпоха). Каталог новой версии пересоздаётся и заполняется с наложением.
- Откат: прежняя версия не открывает `user.db` v2 (`ErrNewerSchema`) и работает без пользовательских данных; файл сохраняется.

## Open Questions

- Нужна ли в будущем область с типом, например `custom-tag:character:alice`? Пока `custom-tag` и `hidden-tag` ищут по имени любого типа.

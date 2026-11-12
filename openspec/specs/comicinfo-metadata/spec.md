# comicinfo-metadata Specification

## Purpose

Чтение `ComicInfo.xml` из архива как второго источника метаданных рядом с `meta.json`: соответствие полей модели галереи и приоритет `meta.json`.

## Requirements
### Requirement: Чтение ComicInfo.xml
Для каждого архива система SHALL искать файл `ComicInfo.xml` (имя без учёта регистра) на любой глубине; если их несколько, используется наименее вложенный. Файл читается не более чем на 4 МиБ. Разбор MUST поддерживать UTF-8 (с BOM и без). Некорректный XML MUST давать предупреждение сканирования, а галерея MUST строиться так, как будто файла нет. Архив без `meta.json` и с корректным `ComicInfo.xml` MUST считаться архивом с метаданными (`HasMeta` истинно).

#### Scenario: Архив с ComicInfo.xml
- **WHEN** сканируется `testdata/cbz-example.cbz`
- **THEN** галерея создаётся, а в ней есть метаданные из `ComicInfo.xml` и `meta.json`

#### Scenario: Только ComicInfo.xml
- **WHEN** архив содержит страницы и `ComicInfo.xml` с `<Title>X</Title>`, но не содержит `meta.json`
- **THEN** название галереи «X» и `HasMeta` истинно

#### Scenario: Некорректный XML
- **WHEN** `ComicInfo.xml` не является корректным XML
- **THEN** галерея создаётся как для архива без метаданных, а в предупреждениях сканирования есть сообщение о некорректном `ComicInfo.xml`

### Requirement: Соответствие полей ComicInfo.xml модели галереи
Система SHALL переносить поля `ComicInfo.xml` в галерею так:
- `Title` → название; если пусто, то `Series` (с добавлением `Number`, если он задан);
- `AlternateSeries` → альтернативное (японское) название;
- `Tags` — значения через запятую, пробелы по краям и пустые значения отбрасываются → теги типа `tag`;
- `Characters` — аналогично → `character`;
- `LanguageISO` → тег `language`;
- `Format` → тег `category`;
- `Writer` и `Penciller` — значения через запятую → `artist`;
- `Publisher` → `group`;
- `Year`, `Month`, `Day` → дата загрузки в UTC (если нет месяца или дня, подставляется 1; если нет года, дата не задаётся);
- `PageCount` → число страниц;
- `Web` → ссылка на работу, только если URL корректный (`http` или `https`) и его путь не пустой и не `/`.

Поля `Manga` и `AgeRating` MUST игнорироваться. Теги MUST нормализоваться по тем же правилам, что теги из `meta.json`.

#### Scenario: Теги через запятую
- **WHEN** в `ComicInfo.xml` указано `<Tags>horror, supernatural, mystery</Tags>`
- **THEN** галерея имеет теги `tag:horror`, `tag:supernatural`, `tag:mystery`

#### Scenario: Название из серии
- **WHEN** `Title` пуст, `Series` равен «S», `Number` равен «3»
- **THEN** название галереи «S 3»

#### Scenario: Ссылка на главную страницу
- **WHEN** `Web` равен `https://examplesite.com/`
- **THEN** ссылка на работу не задаётся

#### Scenario: Ссылка на работу
- **WHEN** `Web` равен `https://examplesite.com/g/123`
- **THEN** ссылка на работу равна `https://examplesite.com/g/123`

#### Scenario: Возрастной рейтинг и направление чтения
- **WHEN** `ComicInfo.xml` содержит `<AgeRating>12+</AgeRating>` и `<Manga>YesAndRightToLeft</Manga>`
- **THEN** ни одно из этих значений не попадает в галерею

### Requirement: Приоритет meta.json над ComicInfo.xml
Если в архиве есть и `meta.json`, и `ComicInfo.xml`, система SHALL сначала применять `meta.json`, а `ComicInfo.xml` MUST заполнять только пустые поля: название, альтернативное название, ссылку, дату, число страниц. Теги из `ComicInfo.xml` MUST добавляться к тегам из `meta.json`, если такого тега ещё нет (сравнение по типу и имени без учёта регистра). Непустое значение из `meta.json` MUST NOT заменяться значением из `ComicInfo.xml`.

#### Scenario: Конфликт названий
- **WHEN** в `meta.json` `title.english` равно «A», а в `ComicInfo.xml` `Title` равен «B»
- **THEN** название галереи «A»

#### Scenario: Недостающая ссылка
- **WHEN** в `meta.json` нет ссылки, а в `ComicInfo.xml` `Web` равен `https://site.example/g/1`
- **THEN** ссылка на работу равна `https://site.example/g/1`

#### Scenario: Дубликат тега
- **WHEN** `meta.json` содержит тег `tag:horror`, а `ComicInfo.xml` — `<Tags>Horror, mystery</Tags>`
- **THEN** галерея имеет по одному тегу `tag:horror` и `tag:mystery`

#### Scenario: Пример архива
- **WHEN** сканируется `testdata/cbz-example.cbz`
- **THEN** название «The Hollow Shrine», альтернативное «うつろの社», 5 страниц, теги `tag:horror`, `tag:supernatural`, `tag:mystery`, `character:shizuka`, `artist:s. watanuki`, `language:english`, `category:manga`, без дублей

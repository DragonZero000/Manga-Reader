## ADDED Requirements

### Requirement: Ссылка на произведение
Модель `Gallery` SHALL содержать необязательное поле «ссылка на произведение» (`SourceURL`) — абсолютный адрес `http` или `https`. Значение MUST выбираться по приоритету: ссылка из `meta.json`; иначе — адрес страницы, с которой файл скачан (см. `embedded-browser`, «Источник загрузки»); иначе поле пустое.

#### Scenario: Ссылка из meta.json важнее
- **WHEN** в `meta.json` указан `https://a.example/g/1/`, а файл скачан со страницы `https://b.example/x`
- **THEN** ссылка на произведение — `https://a.example/g/1/`

#### Scenario: Ссылка из источника загрузки
- **WHEN** в `meta.json` ссылки нет, а файл скачан со страницы `https://b.example/x`
- **THEN** ссылка на произведение — `https://b.example/x`

#### Scenario: Ссылки нет
- **WHEN** ссылки нет в `meta.json`, а источник загрузки неизвестен
- **THEN** поле пустое

## ADDED Requirements

### Requirement: Ссылка из meta.json
При разборе `meta.json` сканер SHALL искать ссылку на произведение в полях верхнего уровня `url`, `source`, `link`, `source_url`, `gallery_url` в этом порядке и MUST использовать первое значение, которое является строкой с абсолютным адресом `http` или `https`. Поля другого типа или с некорректным адресом MUST пропускаться без предупреждения.

#### Scenario: Поле url
- **WHEN** `meta.json` содержит `"url": "https://site.example/g/535147/"`
- **THEN** ссылка галереи — `https://site.example/g/535147/`

#### Scenario: Первое поле некорректно
- **WHEN** `meta.json` содержит `"url": "не ссылка"` и `"source": "https://site.example/g/1/"`
- **THEN** ссылка галереи — `https://site.example/g/1/`

#### Scenario: Ссылки нет
- **WHEN** `meta.json` не содержит ни одного из полей
- **THEN** ссылка из `meta.json` пустая, предупреждений нет

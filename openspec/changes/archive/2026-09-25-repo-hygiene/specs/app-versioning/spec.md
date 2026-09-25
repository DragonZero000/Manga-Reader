## ADDED Requirements

### Requirement: Единый источник версии
Версия приложения SHALL задаваться только в `FyneApp.toml` (поле `Version` в `[Details]`). Сборки для всех платформ (`make build-windows`, `make package-windows`, `make build-android`, `make build-android-release`) MUST брать версию оттуда; `Makefile` и другие файлы сборки MUST NOT содержать свою копию номера версии. Явное переопределение (`make build-windows VERSION=…`) MUST NOT поддерживаться, чтобы исполняемый файл и APK одной сборки не расходились.

#### Scenario: Смена версии
- **WHEN** в `FyneApp.toml` версия изменена на `0.2.0` и выполнены `make build-windows` и `make build-android`
- **THEN** приложение Windows пишет в лог `MangaReader 0.2.0`, а APK имеет `versionName` `0.2.0`

#### Scenario: Поиск копий версии
- **WHEN** в репозитории ищется строка текущей версии в файлах сборки (`Makefile`, `tools/`, `android/`)
- **THEN** она не найдена: версия есть только в `FyneApp.toml` (и в документации как пример)

#### Scenario: Запуск в режиме разработки
- **WHEN** приложение запущено через `make run`
- **THEN** в логе указана версия `dev`

### Requirement: Формат версии
Версия SHALL иметь вид `MAJOR.MINOR.PATCH` из неотрицательных целых без ведущих нулей и суффиксов, где `MINOR` и `PATCH` не больше 99, а `MAJOR` не больше 2099. Сборка MUST останавливаться с понятным сообщением, если версия в `FyneApp.toml` не соответствует формату.

#### Scenario: Неверная версия
- **WHEN** в `FyneApp.toml` указано `Version = "0.2.0-beta"` или `"1.100.0"`
- **THEN** `make build-android` и `make build-windows` завершаются ошибкой с текстом, какой формат ожидается

#### Scenario: Верная версия
- **WHEN** в `FyneApp.toml` указано `Version = "1.2.3"`
- **THEN** сборка проходит

### Requirement: Номер сборки Android вычисляется из версии
Android `versionCode` SHALL вычисляться из версии по формуле `MAJOR*10000 + MINOR*100 + PATCH` и MUST NOT задаваться вручную. Поэтому любая версия, большая по правилам semver, MUST давать больший `versionCode`.

#### Scenario: Вычисление
- **WHEN** версия `0.1.0`, `0.2.0` и `1.2.3`
- **THEN** `versionCode` равен `100`, `200` и `10203` соответственно

#### Scenario: Порядок версий
- **WHEN** сравниваются `0.9.99` и `0.10.0`
- **THEN** `versionCode` второй (`1000`) больше, чем первой (`999`)

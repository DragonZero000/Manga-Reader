## 1. Сборка

- [x] 1.1 `tools/fetch-firefox`: база `en-US` с новой SHA-256; список языковых пакетов `{язык, sha}` (сейчас `ru`) с загрузкой `win64/xpi/<язык>.xpi`, проверкой суммы и копированием в `browser\firefox\langpacks\`
- [x] 1.2 Чтение id пакета из его `manifest.json` при сборке (для имени файла в профиле)
- [x] 1.3 Проверить `make build-windows`: папка `browser\firefox\` содержит `langpacks\ru.xpi`, размер дистрибутива вырос не больше чем на 1 МБ

## 2. Профиль

- [x] 2.1 Установка пакета языка приложения в `profile\extensions\<id>.xpi` при запуске браузера (по образцу встроенного расширения); отсутствие пакета → English
- [x] 2.2 `user.js`: `intl.locale.requested` = язык приложения (`en-US` для English); `intl.accept_languages` не записывается; тесты `config_test.go`
- [x] 2.3 Передать язык приложения в `browser.Options`

## 3. Расширение MangaReader

- [x] 3.1 `manifest.json` с `default_locale` и `__MSG_…__`; `_locales/en/messages.json`, `_locales/ru/messages.json`
- [x] 3.2 Тест: одинаковый набор ключей во всех `_locales`

## 4. Проверка на Windows

- [x] 4.1 Русский и English: меню и настройки Firefox, подсказка кнопки MangaReader
- [x] 4.2 Переход существующего профиля с русской сборки: закладки, расширения, cookies, поисковик на месте
- [x] 4.3 Accept-Language по умолчанию и после изменения в настройках Firefox (сохраняется после перезапуска)

## 5. Документация и проверка

- [x] 5.1 `THIRD_PARTY_NOTICES.md`: сборка `en-US` и языковые пакеты
- [x] 5.2 `docs/en/building.md`, `docs/ru/building.md`: языковые пакеты и обновление их сумм вместе с ESR; `docs/*/user-guide.md`: язык браузера следует языку приложения
- [x] 5.3 `make test`, `make build-windows`

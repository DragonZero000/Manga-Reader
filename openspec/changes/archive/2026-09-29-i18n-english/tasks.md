## 1. Пакет i18n

- [x] 1.1 `internal/i18n`: загрузка встроенных `locales/*.json` через go-i18n, `Init`, `T`, `N`, `Lang`, `Available` (с `_name`), English до `Init`; `go.mod` — go-i18n прямой зависимостью
- [x] 1.2 `format.go`: `Size`, `Date`, `DateTime` для en/ru с тестами
- [x] 1.3 Тест полноты: одинаковый набор ключей во всех `locales/*.json`, недостающий ключ → английский текст
- [x] 1.4 Тест на кириллические строковые литералы в не-тестовом коде `internal/ui` (через `go/ast`, комментарии не учитываются)
- [x] 1.5 Добавить `internal/i18n` в `internal/archtest` и в `docs/en/architecture.md`, `docs/ru/architecture.md`

## 2. Выбор языка

- [x] 2.1 Ключ `ui.language` (auto/en/ru) и определение языка при запуске: явный → системный базовый язык ∈ Available → en; тесты
- [x] 2.2 Вызов определения языка в `cmd/mangareader` до `NewShell` (Windows и Android, проверить доступность JVM для `go-locale` на Android)
- [x] 2.3 Копии `base.en.json`/`base.ru.json` Fyne и подмена переводов Fyne под системную локаль; тест: системная `ru`, язык `en` → «Cancel»
- [x] 2.4 Раздел «Язык / Language» в настройках: варианты из `Available()`, подсказка о перезапуске на выбранном языке

## 3. Типизированные ошибки

- [x] 3.1 `search.ParseError{Code, Field, Fragment}` вместо текстовых ошибок разбора; английский `Error()`; обновить тесты `search`
- [x] 3.2 Английские `Error()` у ошибок `library`, `storage`, `catalog`, `browser`, `mobilebrowser`; новые типы для ошибок, показываемых пользователю (например, `browser.NotFoundError`)
- [x] 3.3 `problems.Item.Err` вместо `Reason`; вкладка «Ошибки» переводит причину
- [x] 3.4 `internal/ui/messages.go`: `errText(err)` — вид → ключ перевода, неизвестная → пояснение + `err.Error()`; тесты на все виды
- [x] 3.5 Повысить `catalog.schemaVersion`; тест: каталог прежней версии пересоздаётся

## 4. Перевод UI

- [x] 4.1 Оболочка, toast, вкладки, библиотека, пустые состояния, выбор папки
- [x] 4.2 Поиск: подсказка поля, статусы, справка (`helpExamples` — пояснения через ключи, запросы как есть)
- [x] 4.3 Страница произведения: кнопки, подписи групп тегов (`knownTypeOrder`, «Прочее»), строки сведений, форматы даты и размера
- [x] 4.4 Читалка, вкладка «Ошибки», настройки (все разделы, включая браузер Windows и Android), диалоги подтверждения
- [x] 4.5 Формы множественного числа («N галерей», «Новые ошибки: N», «Найдено: N»)
- [x] 4.6 Тесты UI: хелпер `i18n.SetForTest("ru")` для существующих тестов; тест экранов на English без кириллицы в видимых подписях

## 5. Логи

- [x] 5.1 Все сообщения `log.*` в Go на английском
- [x] 5.2 Все сообщения `Log.*` в Kotlin на английском

## 6. Браузер Android

- [x] 6.1 Перенести строки Kotlin в `res/values/strings.xml` (English) и `res/values-ru/strings.xml`
- [x] 6.2 Поле `Lang` в `mobilebrowser.Settings` и передача через JNI
- [x] 6.3 Строки браузера на языке приложения: `Lang` (ресурсы с конфигурацией языка; `attachBaseContext` не подходит — Intent с настройками ещё недоступен) в `BrowserActivity`, `BrowserEngine`, `DownloadManager` и уведомлениях `DownloadService`
- [ ] 6.4 `GeckoRuntimeSettings.locales` = язык приложения; проверить Accept-Language и служебные страницы на устройстве

## 7. Документация и конфигурация

- [x] 7.1 `openspec/config.yaml`: раздел Language (двуязычный интерфейс, `internal/i18n`, `ui.language`, логи на English, комментарии на русском), хранилище — `library.db`; правило для задач «новая строка UI → ключ во всех `locales/*.json`, строки Android → во всех `values-*`»
- [x] 7.2 README.md / README.ru.md: интерфейс на английском и русском
- [x] 7.3 `docs/en/user-guide.md`, `docs/ru/user-guide.md`: настройка языка, перезапуск
- [x] 7.4 `THIRD_PARTY_NOTICES.md`: копии переводов Fyne (BSD-3); go-i18n как прямая зависимость
- [ ] 7.5 Release notes: однократное полное сканирование после обновления

## 8. Проверка

- [x] 8.1 Пройти все экраны на English и Русском на Windows и Android (обрезанные подписи, диалоги Fyne, браузер Android)
- [x] 8.2 `make test`, `make build-windows`, `make build-android`

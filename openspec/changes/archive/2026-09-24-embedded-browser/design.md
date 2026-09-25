## Context

Fyne рисует весь интерфейс в одном OpenGL-canvas и не умеет встраивать нативные виджеты: WebView внутри окна невозможен. У Gecko нет API встраивания на Windows. Поэтому «браузер внутри» реализуется как управляемый процесс Firefox с отдельным окном, привязанным к главному (так же устроен встроенный браузер Telegram Desktop: отдельное окно со своей шапкой).

Опоры в коде:
- `driver.NativeWindow.RunNative` (Fyne 2.8) даёт `driver.WindowsWindowContext{HWND}` главного окна; вызывается в UI-потоке.
- `paths.AppDir()` — папка приложения (портативный режим), `storage.Settings` — настройки.
- `library-watch` пересканирует папку после загрузки, `library-errors` пропускает `.part` и показывает невалидные файлы.
- `golang.org/x/sys/windows` уже в `go.sum`.

Сеть: приложение само сетевых запросов не делает (браузер — отдельный процесс). Rate limiting не требуется. Сеть используется только при сборке (`make browser`) для загрузки Firefox ESR.

## Goals / Non-Goals

**Goals:** портативный Firefox без следов вне папки приложения; окно «как в Telegram»; загрузки сразу в `manga`; настройки и очистка данных из приложения; ссылка на произведение.

**Non-Goals:** Android-браузер, встраивание в canvas, вкладки, автообновление Firefox (см. proposal).

## Decisions

### D1. Раскладка файлов

```
MangaReader\
├── mangareader.exe
├── settings.json          browser.home, browser.search (+ существующие)
├── manga\                 ◀── папка загрузок
└── browser\
    ├── firefox\           Firefox ESR: содержимое core\ из распакованного установщика
    └── profile\
        ├── user.js        ◀── пишет приложение перед каждым запуском
        ├── chrome\userChrome.css        ◀── пишет приложение
        └── extensions\bridge@mangareader.app.xpi   ◀── расширение MangaReader, пишет приложение
```

При `go run` — те же пути от рабочей папки (`browser\` в `.gitignore`).

### D2. Настройки профиля (без политик)

**Пересмотрено после проверки пользователем:** корпоративные политики (`policies.json`) заставляют Firefox показывать «Ваш браузер управляется Вашей организацией» (в новой странице настроек Firefox 153 надпись не скрывается `userContent.css`). Поэтому всё задаётся настройками профиля в `user.js`, а `policies.json`, оставшийся от прежней версии, удаляется перед запуском.

| Было политикой | Настройка `user.js` |
|---|---|
| `DisableAppUpdate` | `app.update.disabledForTesting=true`, `app.update.auto=false` (S5: папка `%LOCALAPPDATA%\Mozilla\updates` не создаётся) |
| `DisableTelemetry`, `DisableFirefoxStudies` | `datareporting.policy.dataSubmissionEnabled=false`, `toolkit.telemetry.enabled=false`, `app.shield.optoutstudies.enabled=false` |
| `DisableDefaultBrowserAgent`, `DontCheckDefaultBrowser` | `default-browser-agent.enabled=false`, `browser.shell.checkDefaultBrowser=false` |
| `OverrideFirstRunPage`, `OverridePostUpdatePage`, `UserMessaging`, `SkipTermsOfUse` | `browser.preonboarding.enabled=false`, `browser.aboutwelcome.enabled=false`, `startup.homepage_welcome_url=""`, `browser.startup.homepage_override.mstone="ignore"`, рекомендации CFR выключены |
| `FirefoxHome`, `FirefoxSuggest`, `DisablePocket` | `browser.newtabpage.activity-stream.showSponsored*=false`, `…feeds.section.topstories=false`, `browser.urlbar.suggest.quicksuggest.*=false`, `extensions.pocket.enabled=false` |
| `DownloadDirectory`, `PromptForDownloadLocation` | `browser.download.folderList=2`, `browser.download.dir=<manga>`, `browser.download.useDownloadDir=true` |
| `Homepage` | `browser.startup.homepage=<адрес>`, `browser.startup.page=1` |
| `DisplayBookmarksToolbar` | `browser.toolbars.bookmarks.visibility="never"` |
| `SearchEngines.Default` | **нет аналога.** Поисковик выбирается в настройках самого Firefox (кнопка «Поисковик» открывает `about:preferences#search`); выбор хранится в профиле. Запись `search.json.mozlz4` в обход Firefox отклонена: файл защищён хешем, а подмена прямо запрещена Firefox. |
| `ExtensionSettings` (force_installed) | расширение кладётся в `profile\extensions\`; `xpinstall.signatures.required=false` (ESR это позволяет), `extensions.autoDisableScopes=0`, `extensions.enabledScopes=15` |

Остальное в `user.js`: `toolkit.legacyUserProfileCustomizations.stylesheets=true`; тёмная тема (`extensions.activeThemeID="firefox-compact-dark@mozilla.org"`, `layout.css.prefers-color-scheme.content-override=0`, `browser.theme.*-theme=0`); `browser.tabs.inTitlebar=0` (системный заголовок окна — полоса вкладок скрыта); `browser.download.always_ask_before_handling_new_types=false`, `browser.download.open_pdf_internally=false`; кэш в профиле; `browser.startup.preXulSkeletonUI=false`; раскладка шапки `browser.uiCustomization.state`. Процесс запускается с `MOZ_CRASHREPORTER_DISABLE=1`.

Шапка (`browser.uiCustomization.state`, `nav-bar`): назад, вперёд, обновить, адрес, `alltabs-button` («Все вкладки»), `new-tab-button`, закладки, загрузки, расширения, `bridge_mangareader_app-browser-action` (кнопка «MangaReader»). Ссылки с `target=_blank` открываются в новой вкладке (настройки `browser.link.open_newwindow*` больше не переопределяются).

`userChrome.css`: скрыть `#TabsToolbar` и `#PersonalToolbar`.

Генерация — чистые функции `browser.UserJS(cfg)`, `browser.UserChrome()`, `browser.Extension(port, token)` — покрываются golden-тестами.

### D3. Запуск и единственный экземпляр

```
Open(url) ─▶ процесс жив? ─да─▶ firefox.exe -profile P <url>  (remoting передаёт URL в живой экземпляр)
                │                   + SetForegroundWindow
                │нет
                ▼
           записать policies.json, user.js, userChrome.css
           exec firefox.exe -profile P [url]   (без -no-remote)
           goroutine: ждать окно ─▶ привязать (D4); ждать выхода процесса
```

Без `-no-remote` Firefox (≥ 67) находит запущенный экземпляр по пути профиля и передаёт ему URL; установленный Firefox имеет другой профиль и другой install-hash, поэтому не перехватывает. Проверяется спайком S1.

Процесс-лаунчер Firefox может завершиться сразу, передав работу дочернему; поэтому «жив ли браузер» определяется поиском окон класса `MozillaWindowClass`, чей процесс имеет образ `browser\firefox\firefox.exe` (`QueryFullProcessImageName`), а не PID запущенного процесса.

### D4. Привязка окна (Win32)

- Главный HWND: `fyne.Do(func(){ w.(driver.NativeWindow).RunNative(...) })` — один раз при старте.
- Найденному окну браузера: `SetWindowLongPtr(GWLP_HWNDPARENT, main)` (владелец) и снять `WS_EX_APPWINDOW` → окно поверх главного, сворачивается с ним, без кнопки на панели задач.
- Пока браузер жив, раз в 500 мс проверяются новые окна Firefox (всплывающие окна входа, диалоги) и тоже привязываются.
- При выходе из приложения (`SetOnStopped`): `WM_CLOSE` всем окнам Firefox, ожидание до 5 с, затем завершение процесса.

*Альтернатива:* `SetParent` в область главного окна — отклонено: проблемы с фокусом, клавиатурой, DPI и GPU-процессом Firefox; выигрыш только визуальный.

**Потоки:** Win32-вызовы к окнам Firefox — из горутины `browser` (разрешено для окон другого процесса); к главному окну — только через `RunNative` в UI-потоке. Обновление виджетов (состояние кнопки, toast) — через `fyne.Do`.

### D5. Очистка данных (по S3)

Очистку выполняет сам Firefox при следующем запуске: приложение добавляет в генерируемый `user.js` одноразовую настройку

```
user_pref("privacy.sanitize.pending", "[{\"id\":\"mangareader\",\"itemsToClear\":[…],\"options\":{}}]");
```

- cookies и данные сайтов — `["cookies","offlineApps","cache"]`;
- история — `["history","formdata","downloads"]`;
- обе кнопки — объединённый список.

Запрошенные очистки хранятся в настройке приложения `browser.clear` (множество «cookies», «history»). При следующем запуске браузера `user.js` пишется с `privacy.sanitize.pending`, после чего `browser.clear` сбрасывается: `user.js` перезаписывается при каждом запуске, поэтому очистка однократна (Firefox сам сбрасывает pending в `prefs.js` в `"[]"`). Если браузер открыт, приложение предлагает закрыть его (`Close`), иначе pending применится при следующем открытии.

*Отклонено:* удаление файлов профиля (история и закладки — одна `places.sqlite`; восстановление закладок из резервной копии теряет свежие), SQL-очистка (нужен sqlite), headless-запуск с `sanitizeOnShutdown` (нужен канал управления для штатного закрытия).

### D6. Источник загрузки (пересмотрено по S5)

**Проблема (найдена пользователем):** `ReferrerUrl` в `Zone.Identifier` — это Referer, который Firefox *отправил*. По умолчанию действует политика `strict-origin-when-cross-origin`: при загрузке с другого домена (API, CDN, перенаправление) уходит только адрес сайта, и ссылка ведёт на главную страницу. S5 воспроизвёл: сервер файла получил `Referer="http://127.0.0.1:8765/"`, хотя страница — `/g/535147/`.

**Решение:** адрес страницы сообщает встроенное расширение (D10): при `downloads.onCreated` оно берёт адрес активной вкладки последнего активного окна — это страница, где нажали «Скачать»; при `downloads.onChanged` → `complete` отправляет приложению `{file, page}`. S5: пришло `page=http://127.0.0.1:8765/g/535147/` при обрезанном Referer и перенаправлении API → CDN.

Приложение (мост, D10) проверяет, что `file` лежит внутри папки загрузок и `page` — `http(s)`, и записывает адрес в NTFS-поток файла `<файл>:mangareader.source` (одна строка, UTF-8). Поток переезжает вместе с файлом при переименовании и перемещении по NTFS и не меняет метку загрузки Windows. Затем мост сообщает библиотеке `Source.Invalidate(relPath)` (кэш сканера для файла сбрасывается — запись потока не обязана менять время изменения файла) и запрашивает пересканирование.

Чтение (`storage.FS.SourceURL`, Windows): сначала `:mangareader.source`, иначе `ReferrerUrl` из `:Zone.Identifier` (файлы, скачанные другими браузерами). Только абсолютный `http(s)`.

*Отклонено:* `network.http.referer.*` (сайт может задать свою политику заголовком, и полный Referer уходил бы всем сторонним сайтам), чтение `sessionstore` (запаздывает, неточно при нескольких вкладках), WebDriver BiDi (браузер показывает «управляется удалённо»).

### D7. Сборка дистрибутива

`make browser`: скачать `Firefox Setup <ESR>.exe` (ru, win64) с `archive.mozilla.org` по закреплённой версии, проверить SHA-256 (закреплён в Makefile), распаковать `"Firefox Setup.exe" /ExtractDir=dist\MangaReader\browser\firefox` (спайк S2; запасной путь — распаковка 7z из `core\`). `make build-windows` кладёт exe в `dist\MangaReader\`; `make package-windows` — zip папки.

### D8. Пакеты и UI

```
internal/browser/            без виджетов; *_windows.go + заглушки для других ОС
  config.go    Config (home, search, dirs), Policies/UserJS/UserChrome
  process.go   Browser: Open(url), Running(), Close(), OnStateChange
  window_windows.go  поиск окон, привязка, закрытие
  clean.go     запрошенные очистки → privacy.sanitize.pending
  registry_windows.go  уборка записей HKCU\Software\Mozilla\Firefox (D9)
internal/ui/screens/settings.go   раздел «Браузер» (только Windows)
internal/ui/screens/library.go    кнопка «Браузер»
internal/ui/details               кнопка «Открыть в браузере» (Windows → browser.Open, Android → fyne.App.OpenURL)
```

`browser` добавляется в `archtest` (не импортирует виджеты).

Добавлено (D10): `internal/browser/extension.go` (сборка xpi, файлы расширения через `embed`), `internal/browser/bridge.go` (HTTP-сервер на 127.0.0.1), `internal/storage/source_windows.go` (запись/чтение `:mangareader.source`).

### D9. Записи Firefox в реестре (по S1)

Firefox до чтения `user.js` пишет в `HKCU\Software\Mozilla\Firefox` значения, имена которых начинаются с пути установки (`<firefox>\firefox.exe|Launcher`, `<firefox>|DisableTelemetry`, …) в подключах `Launcher`, `PreXULSkeletonUISettings`, `DllPrefetchExperiment`, `Default Browser Agent`, и создаёт пустой ключ `Installer\<хеш пути установки>`. Отключить это настройками нельзя.

`browser.CleanRegistry(firefoxDir)`:
- обходит `HKCU\Software\Mozilla\Firefox` рекурсивно и удаляет **только значения, имя которых начинается с `firefoxDir`** (без учёта регистра) — значения других установок Firefox не трогаются;
- ключи `Installer\*`: перед первым запуском браузера запоминается список подключей; после выхода удаляются появившиеся **пустые** подключи. Хеш пути не вычисляется (алгоритм Firefox — внутренняя деталь).
- вызывается после выхода процесса браузера и при запуске приложения (уборка после аварийного завершения; без списка «до» — только значения).

Пока браузер открыт, записи существуют — это принятое ограничение (так же работают портативные сборки PortableApps).

## Risks / Trade-offs

- [Firefox переопределяет владельца окна или возвращает кнопку на панель задач] → повторная привязка в цикле 500 мс; спайк S1 до основной работы.
- [Привязка окна другого процесса связывает очереди ввода: зависание одного может подвешивать другое] → Firefox редко зависает целиком; при `IsHungAppWindow` привязка снимается.
- [Полоса вкладок скрыта — новая вкладка открывается незаметно] → кнопка «Все вкладки» в шапке; всплывающие окна (`window.open` с размерами) остаются окнами и привязываются.
- [Другой процесс на компьютере пишет в порт моста] → без токена запрос отклоняется; принимаются только пути внутри папки загрузок.
- [Расширение берёт не ту вкладку (загрузка из фоновой вкладки)] → берётся активная вкладка окна, где началась загрузка; редкий случай — адрес будет неточным, как и без расширения.
- [Размер дистрибутива +~345 МБ] → принятое решение (максимальная портативность).
- [Устаревание встроенного Firefox (безопасность)] → ESR получает исправления; версия обновляется новой сборкой приложения, в README — рекомендация обновляться.
- [Firefox пишет в реестр вне папки приложения] → D9: уборка после закрытия и при старте; пока браузер открыт, записи есть. `SkeletonUILock-*` в `%LOCALAPPDATA%\Mozilla\Firefox` Firefox удаляет сам при закрытии; `browser.startup.preXulSkeletonUI=false` в `user.js` дополнительно отключает skeleton UI.
- [Уборка реестра удалит чужое значение] → удаляются только значения с префиксом нашего пути и только новые пустые ключи `Installer`.
- [Папка приложения без прав записи] → `policies.json`/профиль не создать → ошибка с путём, как для `manga`.
- [Встроенные поисковики зависят от локали сборки] → список берётся из спайка для ru-сборки, в настройках — только проверенные имена.

## Migration Plan

Новые файлы появляются при первом открытии браузера. Откат — удалить `browser\`; остальные данные не затрагиваются.

## Результаты спайков (2026-09-24, Firefox ESR 153.3.0esr ru win64)

Спайк — `spikes/firefox-window` (Fyne-окно + Win32), проверено при запущенном у пользователя Firefox Developer Edition.

- **S2 — распаковка.** `"Firefox Setup 153.3.0esr.exe" /ExtractDir=<папка>` → код 0, реестр не меняется; результат — `<папка>\core\` (firefox.exe и всё нужное) + `setup.exe`. Размер `core` — **344 МБ** (больше оценки ~250 МБ). SHA-256 установщика сверяется с `SHA256SUMS` из того же каталога archive.mozilla.org.
- **S1 — окно.** Окно `MozillaWindowClass` находится по образу процесса за 0,2–1,4 с; PID окна ≠ PID запущенного процесса (лаунчер) — поиск по образу обязателен. `GWLP_HWNDPARENT` держится (через 3 с владелец тот же), `WS_EX_APPWINDOW` не выставляется обратно; окно скрывается при сворачивании главного и возвращается при восстановлении. Второй запуск `firefox.exe -profile P <url>` без `-no-remote` завершается за ~0,24 с и открывает URL в **нашем** окне, не в запущенном Developer Edition. `WM_CLOSE` закрывает браузер за 0,1 с, процессов не остаётся.
- **S1 — следы вне папки.** Профиль пользователя и `%APPDATA%\Mozilla` не затрагиваются. **Firefox пишет в `HKCU\Software\Mozilla\Firefox`** значения с путём установки (`Launcher`, `PreXULSkeletonUISettings`, `DllPrefetchExperiment`, `Default Browser Agent`) и пустой ключ `Installer\<хеш пути>` — до чтения `user.js`, prefs (`browser.launcherProcess.enabled`, `browser.startup.preXulSkeletonUI`) это не отключают. В `%LOCALAPPDATA%\Mozilla\Firefox` на время работы создаётся `SkeletonUILock-*` и удаляется при закрытии. → решение D9.
- **S3 — очистка.** Механизм Firefox `privacy.sanitize.pending` (JSON-список `{id, itemsToClear, options}`) выполняется при следующем запуске до загрузки страниц и сбрасывается Firefox в `"[]"`. `["history","formdata","downloads"]` → визиты и загрузки 0, закладки на месте; `["cookies","offlineApps","cache"]` → cookies и `localStorage` 0, история и закладки на месте. Удалять файлы профиля и тащить sqlite не нужно. → D5 пересмотрен.
- **S4 — источник загрузки.** Firefox пишет в `Zone.Identifier` (NTFS ADS) скачанного файла `ReferrerUrl=<страница>` и `HostUrl=<файл>`. BiDi и `places.sqlite` не нужны. → D6 пересмотрен.
- **S5 — расширение и работа без политик** (`spikes/firefox-extension`, после проверки пользователем). Страница на `127.0.0.1:8765/g/535147/`, файл — через API `localhost:8766/api/download` с перенаправлением на `/files/…`: сервер файла получил `Referer="http://127.0.0.1:8765/"` (обрезан), расширение прислало `page=http://127.0.0.1:8765/g/535147/`. Неподписанное расширение работает и из политики `force_installed`, и из `profile\extensions\` (при `xpinstall.signatures.required=false`, `extensions.autoDisableScopes=0`); `fetch` на `127.0.0.1` с токеном доходит. С `policies.json` страница настроек показывает «Ваш браузер управляется Вашей организацией», `userContent.css` её не скрывает; без `policies.json` надписи нет, окна «Условий использования» нет (`browser.preonboarding.enabled=false`), загрузки идут в `browser.download.dir`, `%LOCALAPPDATA%\Mozilla\updates` не создаётся (`app.update.disabledForTesting`). Кнопка расширения: `windows.update({state:"minimized"})` сворачивает окно (первый клик при открытой панели загрузок только закрывает панель). Для `localhost` Firefox не пишет `Zone.Identifier` — поэтому свой поток `:mangareader.source`.

## Open Questions

- Нет. D5, D6, D9 подтверждены пользователем 2026-09-24.

### D10. Встроенное расширение и мост (по S5)

**Расширение** `bridge@mangareader.app` (Manifest V2, фоновый скрипт), исходники в `internal/browser/ext/` (`embed`):
- `browser_action` «MangaReader»: `windows.update(окно, {state:"minimized"})`, затем `POST /show`;
- `downloads.onCreated` → адрес активной вкладки; `downloads.onChanged(complete)` → `POST /download {file, page, url}`;
- `config.js` — `const BRIDGE = {port, token}`: приложение собирает xpi перед каждым запуском браузера в `profile\extensions\bridge@mangareader.app.xpi` (изменённый файл Firefox переустанавливает сам).

S5: неподписанное расширение из папки профиля включается без вопросов при `xpinstall.signatures.required=false` + `extensions.autoDisableScopes=0` (ESR); без политик надписи «управляется организацией» нет.

**Мост** (`browser.Bridge`): `net.Listen("tcp", "127.0.0.1:0")`, случайный токен 32 байта (`crypto/rand`) на запуск приложения; запросы без заголовка `X-MangaReader-Token` с верным токеном отклоняются (403), тело ≤ 64 КБ. Обработчики:
- `/download` → запись `:mangareader.source` (D6) и колбэк `OnDownload(relPath)` → в UI `fyne.Do(library.Invalidate+RequestScan)`;
- `/show` → `Browser` сам выводит главное окно (владельца, HWND известен по `SetOwner`) на передний план: `ShowWindow(SW_RESTORE)` + `SetForegroundWindow`; окно браузера к этому моменту свёрнуто — Windows отдаёт активность владельцу. Затем необязательный колбэк `OnShow`.

Мост запускается при запуске браузера и останавливается при его закрытии (новый сеанс — новые порт и токен, xpi пересобирается). Порт другим программам не объявляется; токен известен только расширению (в профиле приложения).

**Потоки:** обработчики HTTP — в горутинах `net/http`; всё, что касается виджетов, — через `fyne.Do`.

*Отклонено:* native messaging (требует ключ в `HKCU\Software\Mozilla\NativeMessagingHosts` и отдельный исполняемый файл-посредник), `3rdparty`-политика для передачи порта (включает надпись «управляется организацией»).

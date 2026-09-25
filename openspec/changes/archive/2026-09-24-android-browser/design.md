## Context

После `android-build-gradle` APK собирается Gradle: `GoNativeActivity` (Fyne, NativeActivity) загружает `libmangareader.so`, в проекте можно держать Kotlin-код и AAR. Go работает с Java через JNI (`driver.AndroidContext`: VM, Env, Ctx; пример — `internal/storage/saf_android.c`). Библиотека на Android — SAF-дерево (`storage.SAF`), сейчас только чтение; постоянный доступ берётся только на чтение (`TakePersistable`). Настройки — `fyne.Preferences` через `storage.Settings`.

На Windows браузер — отдельный процесс Firefox; источник загрузки сообщает расширение. На Android браузер — часть процесса приложения, всё известно напрямую.

Минимум Android 11 (API 30), arm64.

**Сеть:** загрузки выполняет сам движок GeckoView (cookies, перенаправления, POST — как в Firefox); приложение само запросов не делает, rate limiting не нужен.
**Потоки:** Go ↔ Kotlin через JNI; всё, что меняет виджеты Fyne, — через `fyne.Do`; UI браузера — в главном потоке Android, запись файлов — в фоновом `Executor`.

## Goals / Non-Goals

**Goals:** функциональное соответствие настольному браузеру на движке Firefox; загрузки любого вида в папку манги; точная ссылка; вкладки переживают закрытие экрана браузера.

**Non-Goals:** восстановление после выгрузки процесса, отдельная задача в недавних, Firefox Account (см. proposal).

## Decisions

### D1. Компоненты

```
 процесс io.github.mangareader.app
 ┌──────────────────────────────────────────────────────────────────────┐
 │ (без своего Application: BrowserEngine.init — из экрана или clearData)│
 │                                                                      │
 │ BrowserEngine (object)                                               │
 │   runtime: GeckoRuntime (один на процесс)                            │
 │   tabs: TabManager ── [Tab(GeckoSession, title, url)…], active       │
 │   downloads: DownloadManager ── Executor + DownloadService           │
 │   bookmarks: BookmarkStore (JSON, filesDir/browser/bookmarks.json)   │
 │   extensions: ExtensionManager (WebExtensionController)              │
 │   settings: BrowserSettings (приходят из Go при каждом открытии)     │
 │                                                                      │
 │ GoNativeActivity (Fyne)            BrowserActivity                   │
 │   Go ── JNI: GoBridge.openBrowser ─▶ GeckoView + BrowserToolbar      │
 │   Go ◀─ JNI: GoBridge.nativeOnDownloaded(rel, page)                  │
 │                                     TabsSheet, BookmarksSheet,       │
 │                                     ExtensionsSheet, DownloadsSheet  │
 └──────────────────────────────────────────────────────────────────────┘
```

Вкладки — в `BrowserEngine`, а не в `BrowserActivity`: экран только подключает `GeckoView.setSession(activeTab.session)` в `onStart` и отключает в `onStop`. Поэтому закрытие экрана не теряет вкладки (требование «Возврат в читалку и сохранение вкладок»).

### D2. Навигация между экранами

- Открыть браузер: `startActivity(Intent(BrowserActivity).addFlags(FLAG_ACTIVITY_REORDER_TO_FRONT))` с extras (D6). `BrowserActivity` в той же задаче (`taskAffinity` по умолчанию), `launchMode="singleTop"`.
- «MangaReader»: `startActivity(Intent(GoNativeActivity).addFlags(FLAG_ACTIVITY_REORDER_TO_FRONT))` — браузер уходит под читалку, не уничтожаясь.
- «Назад»: `session.goBack()`, если `canGoBack`; иначе `finish()` — экран закрывается, вкладки остаются в `BrowserEngine`.

### D3. Панель браузера

`BrowserToolbar : LinearLayout` — один View; `BrowserActivity` кладёт его в `CoordinatorLayout` сверху или снизу по `settings.toolbarPosition`. Состав: строка адреса (`EditText`: адрес → загрузить; иначе — поиск по шаблону поисковика), назад/вперёд/обновить, «Вкладки [N]», «MangaReader», ☆, меню (`PopupMenu`: новая вкладка, закладки, загрузки, расширения). Кнопки назад/вперёд и ☆ обновляются по `NavigationDelegate`/`ProgressDelegate`. Стиль — тёмный, по мотивам Firefox для Android (Material 3, без лишних элементов).

### D4. Загрузки (onExternalResponse)

`ContentDelegate.onExternalResponse(session, response: WebResponse)` — движок уже выполнил запрос (перенаправления, POST, cookies, blob) и отдаёт поток тела. Порядок:

```
 onExternalResponse
   page  = tab.url (адрес вкладки в момент ответа — страница, где нажали «Скачать»)
   name  = из Content-Disposition, иначе из пути URI; очистка недопустимых символов
   uri   = SAF createDocument(tree, "<name>.part")  (конфликт имён → " (1)", проверка по listChildren)
   DownloadService.start(name)                     (foreground, уведомление с прогрессом)
   executor: copy(response.body → openOutputStream(uri)), прогресс по Content-Length
   успех  → renameDocument(uri, name) → GoBridge.nativeOnDownloaded(relPath, page)
   ошибка → deleteDocument(uri), уведомление «Не удалось скачать name»
   DownloadService.stop, когда активных загрузок нет
```

`.part` уже пропускается сканером (`library-errors`). `DownloadService`: `foregroundServiceType="dataSync"`, `POST_NOTIFICATIONS` запрашивается перед первой загрузкой (Android 13+); без разрешения загрузка всё равно идёт, но без уведомления.

*Альтернатива:* Android `DownloadManager` — отклонено: не пишет в SAF-дерево и теряет cookies/POST.

### D5. Мост Go ↔ Kotlin

- **Go → Kotlin:** JNI: `Intent().setClassName(ctx, "io.github.mangareader.app.browser.BrowserActivity")` с extras `url`, `settings` (JSON), `tree` и `FLAG_ACTIVITY_REORDER_TO_FRONT`, затем `ctx.startActivity` (S-B1: классы приложения искать не нужно). Очистка данных — тем же способом: Intent с действием `io.github.mangareader.app.CLEAR` и extras, `BrowserActivity` не показывается (отдельный невидимый `ClearActivity` или обработка в `MangaApp`). По образцу `saf_android.c`.
- **Kotlin → Go:** `external fun nativeOnDownloaded(rel: String, page: String)` в `GoBridge`. `GoNativeActivity` загружает `mangareader` через `System.loadLibrary` (S-G1), поэтому JNI находит экспортированную C-функцию `Java_io_github_mangareader_app_GoBridge_nativeOnDownloaded` (cgo), которая вызывает Go.
- Go при `nativeOnDownloaded`: `links.Set(rel, page)`, `Source.Invalidate(rel)` (в горутине), `fyne.Do(library.RequestScan)`.

### D6. Настройки

Хранятся в `storage.Settings` Go (`browser.home`, `browser.search`, `browser.toolbar`), экран «Настройки» → раздел «Браузер» (Android-вариант карточки). При каждом `openBrowser` Go передаёт JSON `{home, searchTemplate, toolbar}`. Поисковики — список в Go: `Google https://www.google.com/search?q=%s`, `DuckDuckGo https://duckduckgo.com/?q=%s`, `Bing`, `Startpage`, `Ecosia`.

Очистка: `GoBridge.clearData(flags)` → `runtime.storageController.clearData(COOKIES|DOM_STORAGES|NETWORK_CACHE|…)` или `(HISTORY|DOWNLOADS|FORM_DATA)`, результат — toast через `fyne.Do`. Если движок ещё не создан, он создаётся для очистки.

Настройки движка (`GeckoRuntimeSettings`): `preferredColorScheme = DARK`, `aboutConfigEnabled = false`, `consoleOutput = false`, телеметрия выключена (`telemetryDelegate` не задаётся); `configFilePath` не используется.

### D7. Источник загрузки (`links.json`)

`internal/library/links.go` (без виджетов): `Links` — map «relPath → URL» в файле личной папки приложения (`fyne.App.Storage().RootURI()`), атомарная запись. `storage.SAF` получает необязательный источник ссылок (`SourceURL(rel)` из `Links`) — тот же интерфейс `sourcer`, что на Windows; `Links.Prune(существующие пути)` после сканирования.

### D8. Запись через SAF и доступ на запись

`saf_android.c`: `createDocument(tree, docId, mime, name)`, `openOutputStream`/`openFileDescriptor("w")`, `renameDocument`, `deleteDocument` — для Kotlin это обычные вызовы `DocumentsContract`, для Go они нужны только для проверки доступа. `TakePersistable` запрашивает `FLAG_GRANT_READ_URI_PERMISSION | FLAG_GRANT_WRITE_URI_PERMISSION`. `HasWriteAccess(tree)` — по `getPersistedUriPermissions().isWritePermission`; нет записи → сообщение и выбор папки перед открытием браузера.

### D9. Расширения

`runtime.webExtensionController` (+ `enableExtensionProcessSpawning()` при создании движка — S-B3):
- `promptDelegate.onInstallPromptRequest` → диалог «Добавить <имя>? Разрешения: …» (как в Firefox);
- установка по кнопке «Добавить в Firefox» на addons.mozilla.org идёт штатно (AMO распознаёт GeckoView как Firefox для Android);
- `list()` → экран «Расширения» (переключатель вкл/выкл — `enable`/`disable`, удаление — `uninstall`); расширения хранятся в профиле GeckoView в личной папке приложения.

Спайк S-B3 проверяет, что AMO показывает «Добавить в Firefox» для GeckoView и что uBlock работает.

### D10. Версия GeckoView

Release-канал, версия закреплена в `app/build.gradle.kts` (`geckoviewVersion`); compileSdk — по требованию AAR (для 156 — 37.1), Kotlin — через KGP нужной версии в корневом `build.gradle.kts`, `.so` сжимаются (`useLegacyPackaging`). Обновление — сменой версии и пересборкой; в README — рекомендация обновлять приложение. Только `arm64-v8a` (`abiFilters`, GeckoView AAR содержит все архитектуры — лишние отсекаются).

## Risks / Trade-offs

- [Go → Kotlin: класс `GoBridge` недоступен из потока Go по `FindClass`] → поиск через `ClassLoader` активности; спайк S-B1.
- [AMO может не показывать «Добавить в Firefox» для GeckoView-приложения] → спайк S-B3; запасной вариант — установка по ссылке на `.xpi` из списка рекомендованных расширений в приложении.
- [Android убивает процесс в фоне — вкладки теряются] → принято (non-goal); загрузки защищены foreground service.
- [Размер APK +60 МБ, обновления движка раз в 4 недели] → принято пользователем; версия закреплена.
- [Файл, созданный скриптом (blob), и POST в GeckoView] → идут через `onExternalResponse`; спайк S-B2 проверяет оба.
- [Конфликт имён при параллельных загрузках] → выбор имени и `createDocument` под общим мьютексом `DownloadManager`.
- [Доступ только на чтение у существующих установок] → однократный повторный выбор папки (требование «Доступ на запись»); после `android-build-gradle` приложение и так переустанавливается.

## Migration Plan

Новые данные браузера (профиль GeckoView, закладки, `links.json`) — в личной папке приложения; удаление приложения удаляет их. Откат — сборка без модуля браузера: кнопка «Браузер» на Android скрывается, «Открыть в браузере» снова открывает системный браузер.

## Результаты спайков (2026-09-25, `spikes/android-browser`, Galaxy S25 Ultra, Android 16)

- **Сборка:** GeckoView **156.0.20260921121718** (release) требует **compileSdk 37.1** (`compileSdk { version = release(37) { minorApiLevel = 1 } }`; AGP сам установил SDK Platform 37.1) и стандартную библиотеку Kotlin 2.4 — встроенный в AGP 9.4.1 компилятор Kotlin 2.2 её не читает, поэтому в корневой `build.gradle.kts` добавлен `id("org.jetbrains.kotlin.android") version "2.4.10" apply false` (AGP берёт компилятор этой версии). Репозиторий `https://maven.mozilla.org/maven2/`.
- **Размер:** `libxul.so` — 152 МБ. Без сжатия `.so` APK — 230–250 МБ; с `packaging.jniLibs.useLegacyPackaging = true` — **124 МБ** (libxul сжимается до 70 МБ; при установке распаковывается, занятое место ~250 МБ). Proposal исправлен.
- **S-B1 (экран):** `BrowserActivity` открывается из Go через JNI `Intent.setClassName(ctx, "…BrowserActivity")` — искать класс приложения через `FindClass` не нужно. Браузер и читалка — одна задача (одна карточка в недавних); «MangaReader» (`REORDER_TO_FRONT` на `GoNativeActivity`) и «Назад» (`finish()`) возвращают в читалку; вкладка (сессия в объекте процесса) сохраняется: после повторного открытия — та же страница.
- **S-B2 (загрузки):** через `ContentDelegate.onExternalResponse` пришли **все четыре** способа: прямая ссылка, API на другом домене с перенаправлением на CDN (сервер получил обрезанный `Referer=http://127.0.0.1:8765/`, браузер записал страницу `/g/535147/`), POST-форма (`id=535147` дошёл), blob (`blob:…` URI). Запись: `DocumentsContract.createDocument(<имя>.part)` → поток → `renameDocument`. Имя — из `Content-Disposition`. Kotlin → Go: `external fun nativeOnDownloaded` находит экспорт `Java_…_GoBridge_nativeOnDownloaded` из `libmangareader.so`; все четыре сообщения дошли до Go с адресом страницы. `GetStringUTFChars` отдаёт модифицированный UTF-8 — для имён с символами вне BMP в основной реализации строки передаются через `GetStringChars` (UTF-16).
- **S-B3 (расширения):** AMO показывает GeckoView «Добавить в Firefox»; `.xpi` приходит в `onExternalResponse` с типом `application/x-xpinstall` → `webExtensionController.install(uri)` → `PromptDelegate.onInstallPromptRequest(ext, permissions, origins, dataCollection)` → `PermissionPromptResponse(true, false, false)` → установлено. После полного перезапуска процесса расширение на месте (`list()`). **Обязателен `webExtensionController.enableExtensionProcessSpawning()`** — иначе фоновые скрипты расширений не работают. uBlock Origin 1.75.0 на `edition.cnn.com` заблокировал 27 запросов (значок кнопки через `WebExtension.ActionDelegate.onBrowserAction` → `badgeText`); страницы на `127.0.0.1` uBlock не фильтрует. В логе — безобидное «Native manifests are not supported on android» (запрос `storage.managed`).

## Open Questions

- Нет.

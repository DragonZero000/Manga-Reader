## 1. Спайки (после `android-build-gradle`; результаты — в design.md)

- [x] 1.1 S-B1: `BrowserActivity` с GeckoView в Gradle-сборке; открытие из Go по JNI (`GoBridge.openBrowser`), возврат в читалку с `REORDER_TO_FRONT`, вкладка сохраняется
- [x] 1.2 S-B2: `onExternalResponse` для прямой ссылки, перенаправления на другой домен, POST-формы и blob → запись `.part` в SAF-дерево, переименование; `nativeOnDownloaded` (экспорт `Java_…` из Go) доходит до Go с адресом страницы
- [x] 1.3 S-B3: установка uBlock Origin с addons.mozilla.org в GeckoView, работа во вкладках, сохранение после перезапуска

## 2. Хранилище и мост в Go

- [x] 2.1 SAF: постоянный доступ на чтение и запись, `HasWriteAccess`; повторный выбор папки перед браузером при доступе только на чтение
- [x] 2.2 `library/links.go`: `links.json` в личной папке (атомарная запись, `Set`, `Prune`); SAF-хранилище отдаёт `SourceURL` из `Links`; тесты
- [x] 2.3 JNI-мост: `openBrowser(url, settingsJson, treeUri)`, экспорт `Java_…GoBridge_nativeOnDownloaded` → `Links.Set` + `Invalidate` + `RequestScan` (`fyne.Do`), `clearData`

## 3. Движок и экран (Kotlin)

- [x] 3.1 `BrowserEngine` (без отдельного `MangaApp`: движок создаётся лениво из экрана или `GoBridge.clearData`) (GeckoRuntime, тёмная тема, без телеметрии), `TabManager`
- [x] 3.2 `BrowserActivity`: подключение активной вкладки, «Назад» по истории / в читалку, «MangaReader», открытие URL из Intent в новой вкладке
- [x] 3.3 `BrowserToolbar`: адрес/поиск, назад/вперёд/обновить, «Вкладки [N]», «MangaReader», ☆, меню; положение сверху/снизу
- [x] 3.4 Вкладки: список карточек, переключение, закрытие, новая вкладка, `target=_blank`/`window.open` → новая вкладка, последняя закрытая → новая с домашней

## 4. Загрузки

- [x] 4.1 `DownloadManager`: имя из Content-Disposition/URI, конфликт « (1)», `.part` → переименование, удаление `.part` при ошибке, адрес вкладки → `nativeOnDownloaded`
- [x] 4.2 `DownloadService` (foreground, dataSync): уведомление с прогрессом, `POST_NOTIFICATIONS`, остановка без активных загрузок; список загрузок в меню

## 5. Закладки, расширения, настройки

- [x] 5.1 `BookmarkStore` (JSON), ☆ на панели, экран «Закладки» (открыть, удалить)
- [x] 5.2 `ExtensionManager`: подтверждение установки, экран «Расширения» (вкл/выкл, удаление)
- [x] 5.3 Go: раздел «Браузер» в настройках на Android (домашняя страница, поисковик, панель, очистка с подтверждением и toast); кнопка «Браузер» в библиотеке и «Открыть в браузере» на Android → встроенный браузер

## 6. Проверка

- [x] 6.1 Тесты Go: `Links`, мост (без JNI — через интерфейс), настройки; `make test`
- [x] 6.2 На телефоне (Galaxy S25 Ultra, Android 16; расширение после перезапуска и HTML-«архив» — в спайке S-B3 и library-errors): загрузка с перенаправлением/POST/blob → галерея и точная ссылка; HTML-«архив» → ошибка; сворачивание во время загрузки 300 МБ; вкладки после «MangaReader» и повторного открытия; закладка и расширение после перезапуска; очистка; панель сверху/снизу; тёмная тема
- [x] 6.3 README: браузер на Android, разрешения, размер, обновления движка

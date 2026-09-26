## 1. Замер «до»

- [x] 1.1 Скрипт суммы PSS процессов приложения (`dumpsys meminfo`, раздел «Total PSS by process»); замер M0–M3 из design D9 на текущем коде (S25 Ultra); результаты в design.md

## 2. Движок: выгрузка и восстановление

- [x] 2.1 `Tab`: заменяемая `session`, `state`, `downloads`, `lastUsed`, `loaded`; `setup(tab, session)` с проверкой `session === tab.session` во всех делегатах; `onSessionStateChange` сохраняет `state`
- [x] 2.2 `BrowserEngine`: `unload(tab)`, `ensureLoaded(tab)` (restoreState или loadUri), `activeSession()`; `select`, `newTab`, `onNewSession`, `close` через них
- [x] 2.3 Политика `enforce()` (design D3): `foreground`, активная + последняя по `lastUsed`, в фоне — только активная, вкладки с загрузками не трогаются
- [x] 2.4 `onKill`/`onCrash` (design D6): вкладка выгружена, активная при открытом экране — восстановление через `onTabsChanged`
- [x] 2.5 `clearData("history")`: `state = null` у выгруженных (design D7)

## 3. Экран и загрузки

- [x] 3.1 `BrowserActivity`: `attach()` через `ensureLoaded`; `onStart`/`onStop` — `foreground`, `setActive`, `setPriorityHint`, `enforce()` (design D4); все `tab.session?.…` заменить на `BrowserEngine.activeSession()`; «Назад» и закладки — тоже
- [x] 3.2 `DownloadManager.start(…, onFinish)`: один вызов по DONE/FAILED (из потока загрузки; движок переходит в главный поток); движок считает `tab.downloads`

## 4. Проверка на устройстве

- [x] 4.1 `make build-android`; сценарии спеки вручную на S25 Ultra: 3 вкладки (в списке все, загружены 2), «MangaReader» и возврат (активная без перезагрузки), выбор выгруженной (страница и «Назад» на месте), очистка истории
- [x] 4.2 **Замер «после»** M0–M3; результаты и сравнение в design.md
- [x] 4.3 Загрузка архива из вкладки 1, пока открываются вкладки 2 и 3 и нажимается «MangaReader»: загрузка завершается, файл в библиотеке. Убитый процесс вкладки: `adb shell run-as io.github.mangareader.app kill <pid процесса :tab>` (APK из `make build-android` отладочный) — вкладка восстанавливается с той же страницей

## 5. Документация и проверка

- [x] 5.1 `docs/ru/user-guide.md` и `docs/en/user-guide.md`, раздел браузера Android: неактивные вкладки выгружаются для экономии памяти и загружаются заново при выборе; загрузки не прерываются
- [x] 5.2 `make test`, `make build-android`

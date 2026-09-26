## 1. Замер кадров (сначала)

- [x] 1.1 `internal/ui/frameprobe_on.go` (`//go:build frameprobe`) — бесконечная `fyne.Animation`, кольцевой буфер интервалов, журнал раз в 5 с (n, p50, p95, max, >25 мс); `frameprobe_off.go` — заглушка; запуск из `NewShell`; тест расчёта перцентилей в чистой функции
- [x] 1.2 `tools/android-build`: флаг `-tags` (добавляется к `migrated_fynedo`); Makefile: `EXTRA_TAGS ?=` для `run`, `build-windows` и `build-android` (`TAGS` уже занят внутренней константой)
- [x] 1.3 **Замер A (база)** на S25 Ultra: `make build-android EXTRA_TAGS=frameprobe`, сценарии из design D5 (библиотека, лента 34 стр., постраничный); результаты записать в design.md (Open Questions)

## 2. Страницы в RGBA без копий

- [x] 2.1 `internal/pages`: экспортируемая `ToRGBA(image.Image) *image.RGBA` (без копии для RGBA с `Min=(0,0)`); `scaleToBox` рисует в RGBA, без уменьшения возвращает `ToRGBA(src)`; тесты на тип результата и сохранение пикселей (JPEG/YCbCr, NRGBA с прозрачностью)
- [x] 2.2 `Slice` через `SubImage` полной ширины (при другом типе или `Stride` — сначала `ToRGBA`); тест: куски — участки одного `Pix`, `Pix[0:4]` куска совпадает с пикселем исходника в строке `y`, сумма `Bounds()` кусков совпадает с полным размером
- [x] 2.3 `ImageScaleFastest` в `internal/ui/reader/paged.go`, `strip.go` и `internal/ui/details/details.go`; тест читалки: изображения результата — `*image.RGBA`

## 3. Лента: работа только при смене окна

- [x] 3.1 `stripView.updateVisible` по design D7: запоминать `from/to`, видимые страницы и `top`; при неизменном окне — выход без `NewGeneration`, `request` и `content.Refresh()`; при смене только видимых — новые приоритеты без `Refresh`; сброс запомненного в `releaseAll` (его вызывают `layout`, `show`, `reset`) и `setHeight`; в `goTo` сброс не нужен — смену смещения ловит сравнение окна
- [x] 3.2 Тесты: прокрутка на несколько пикселей внутри окна не меняет поколение загрузчика и не вызывает обновление содержимого (счётчик в тестовом хуке); вход новой страницы в окно запрашивает её и освобождает вышедшие; `goTo` после прокрутки работает

## 4. Миниатюры

- [x] 4.1 `thumbs.Cache`: постоянные воркеры и стек заданий (LIFO) вместо горутины на каждый запрос; `Load` возвращает `cancel`; задание без живых ожидающих выбрасывается без декодирования
- [x] 4.2 Кэш по байтам (`New(open, limitBytes, workers)`, вытеснение LRU по объёму), удалить `DefaultCapacity`; `Downscale` в RGBA, маленькие изображения через `pages.ToRGBA`
- [x] 4.3 `SetBox(w, h)` (каждая сторона ≤ 512) и очисткой кэша при изменении; тесты: отмена до начала декодирования (счётчик `decodes`), LIFO, лимит байт, размер миниатюры ≤ области и ≤ 512, объединение одинаковых запросов
- [x] 4.4 `internal/app`: параметры кэша миниатюр в `newServices` — 2 воркера и 48 МБ в `app_android.go`, `min(4, NumCPU)` и 64 МБ в `app_desktop.go`; `NewForTest` обновить
- [x] 4.5 `galleryGrid`/`galleryCard`: хранить и вызывать `cancel` при переиспользовании карточки; `SetBox` по `coverSize × Canvas().Scale()`, `Load` только после появления канваса; `ImageScaleFastest` у обложки; обновить тесты UI
- [x] 4.6 **Замер B** (после групп 2–4, без 60 Гц): те же сценарии; результаты в design.md; если p95 уже ≈ 16,7 мс и рывков нет — отметить это (60 Гц всё равно делается, для 120-Гц экранов)

## 5. Частота экрана (Android)

- [x] 5.1 Kotlin `android/app/src/main/kotlin/io/github/mangareader/app/DisplayRate.kt`: `apply(Activity, Boolean)` в UI-потоке Android — `preferredDisplayModeId` (то же разрешение, ближайший к 60 Гц режим), запасной `preferredRefreshRate = 60f`, при выключении — сброс; ошибки в `Log.w`
- [x] 5.2 Пакет `internal/display`: `Supported`, `SetMax60(bool) error`; `display_android.go` + C-вызов через загрузчик классов приложения (по образцу `mobilebrowser`), `display_other.go` — заглушка; добавить пакет в `internal/archtest/imports_test.go`
- [x] 5.3 `internal/app`: `KeyDisplayMax60 = "display.max60"`, `DisplayMax60(settings) bool` (по умолчанию `true`) с тестом
- [x] 5.4 Применение: при старте оболочки и на `OnEnteredForeground` (только если `display.Supported`); ошибки — в журнал
- [x] 5.5 `settings.go`: раздел «Экран» с флажком «Ограничить 60 Гц» и пояснением (только при `display.Supported`); сохранение и немедленное применение; тест: на ПК раздела нет, значение по умолчанию и сохранение (через подмену функции применения)
- [x] 5.6 **Замер C**: «Ограничить 60 Гц» включено и выключено; `adb shell dumpsys display` подтверждает активный режим 60 Гц; результаты и вывод (нужен ли форк драйвера — шаг 4) в design.md

## 6. Документация

- [x] 6.1 `docs/ru/user-guide.md` и `docs/en/user-guide.md`: раздел «Настройки» — «Экран» / «Ограничить 60 Гц» (Android)
- [x] 6.2 `docs/ru/building.md` и `docs/en/building.md`: замер кадров (`make build-android EXTRA_TAGS=frameprobe`, `adb logcat | grep frameprobe`, как читать p95 и ограничение замера); `docs/{ru,en}/architecture.md`: пакет `internal/display` в таблице пакетов и платформ

## 7. Проверка

- [x] 7.1 `make test`; `make build-windows` и `make build-android` (изменился код сборки)
- [x] 7.2 Визуально на устройстве и ПК: нет потери качества страниц (мелкий текст), обложек и обложки на странице произведения; лента при медленной и быстрой прокрутке без пустых мест

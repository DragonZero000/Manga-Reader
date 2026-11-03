# Правки копии Fyne

Основа: `fyne.io/fyne/v2` v2.8.0

Это копия модуля из кэша модулей Go с правками MangaReader. Подключается
через `replace fyne.io/fyne/v2 => ./third_party/fyne` в `go.mod` корня;
`require` там остаётся той же версии, что указана выше (это проверяет
`internal/archtest`). Лицензия Fyne — BSD-3-Clause (`LICENSE`), правки под
той же лицензией. Отправлять их в Fyne не планируется.

Цель правок — темп кадров по vsync экрана (изменение `frame-pacing`): на
Android кадры идут по `AChoreographer`, на Windows — по vsync монитора, в
простое кадров нет, анимации получают время кадра.

## Правила

- Каждая правка в коде помечена комментарием `// mangareader:` (в C —
  `/* mangareader: */`): `grep -rn "mangareader:" third_party/fyne`
  находит все места.
- Новые файлы целиком наши; у них пометка в первой строке комментария.
- Каждая правка описана ниже: файл, что изменено, зачем.

## Перенос на новую версию Fyne

1. Скопировать новый модуль из кэша (`go mod download -json fyne.io/fyne/v2@vX`
   покажет папку) во временную папку, снять атрибут «только чтение».
2. Перенести в неё новые файлы целиком и правки по списку ниже (поиск
   `mangareader:` в старой копии). Если код вокруг правки изменился, понять
   её смысл по описанию ниже, а не по строкам.
3. Заменить `third_party/fyne`, обновить версию здесь и в `require` корня.
4. `make test`, `make build-windows`, dev-APK; замер кадров на телефоне и ПК
   (`docs/ru/building.md`, «Замер кадров») — `1v` и `miss` не хуже прежних.

Тесты самой копии: `go test ./internal/pacing ./internal/animation ./container`.
Часть тестов Fyne (`internal/driver/glfw` с настоящим окном, меню `widget.Entry`)
падает на ПК с масштабом экрана не 100% или с языком системы не английским —
так же и в исходном Fyne 2.8.0, правки тут ни при чём.

## Правки

### Новые пакеты `driver/pacing` и `internal/pacing`

- `driver/pacing` — публичная часть: `FrameTime`, `RefreshRate`, `Mode`,
  `PresentSource`, `SetObserver`, `SetSampler`, `SetRateHint(rate float64)`,
  тип `Frame` (`Sample` — значение `SetSampler` в момент рисования кадра:
  на Android наблюдатель получает кадр позже, когда придёт время показа).
  Единственная точка связи приложения с темпом кадров (в приложении —
  только через `internal/ui/frameclock`).
- `internal/pacing` — состояние, которое заполняют драйверы: `BeginFrame` /
  `EndFrame` (время кадра), `SetPeriod`, `SetMode`, `SetPresentSource`,
  `Report` (наблюдатель), `TakeSample` (драйвер — сразу после рисования
  кадра), `RateHint`, `NextVsync` (сетка vsync).

Перенос: файлы целиком.

### Анимации: время кадра

- `internal/animation/runner.go`: `tickAnimation` берёт `now` из
  `pacing.FrameTime()` вместо `time.Now()`, следующий цикл повторяющейся
  анимации начинается от этого же `now`; новый метод `Running()` (идут ли
  анимации) для мобильного драйвера.
- `internal/animation/animation.go`: `newAnim` — начало анимации от
  `pacing.FrameTime()`.
- `internal/driver/common/canvas.go`: новый метод `IsDirty()` (флаг без
  сброса) для мобильного драйвера.

### Вкладки: выбор без обновления содержимого

- `container/tabs.go`: `baseTabsRenderer.applyTheme` (вызывается при каждом
  выборе вкладки через `Refresh`) обновляет содержимое всех вкладок, только
  если тема или её вариант сменились с прошлого раза (поля
  `contentTheme`, `contentVariant`, функция `sameTheme`). Раньше `Refresh`
  всего содержимого при каждом выборе освобождал текстуры изображений во
  всех вкладках (обложки загружались заново) и стоил на эмуляторе
  50–100 мс. При смене темы Fyne и так обновляет всё дерево окна.
- `container/apptabs_mangareader_test.go` (новый): выбор вкладки не
  обновляет содержимое, смена темы — обновляет.

### ПК (`internal/driver/glfw`)

- `loop.go`: такт обёрнут в `pacing.BeginFrame(frameTimeAt(time.Now()))` /
  `pacing.EndFrame()`; после такта период таймера — `tickPeriod` (тикер
  `Reset`, если изменился). В `repaintWindow` вокруг `SwapBuffers` —
  время начала и `swapped(start)`.
- `window_desktop.go`: после создания контекста (`RunWithContext`) не на
  Wayland — `enableVsync()`; после создания окна — `updateRefreshRate()` и
  `watchMonitorsOnce()`; в `moved` — `updateRefreshRate()`.
- `pacing.go` (новый): `swapped` — проверка vsync и кадр наблюдателю.
- `pacing_windows.go` (новый): `SwapInterval(1)`, режим `vsync`; таймер
  цикла `1/(1,1 × частота монитора)`, в простое (> 250 мс без кадра) 1/60 с;
  обнаружение выключенного в драйвере vsync (30 кадров подряд друг за
  другом, `SwapBuffers` < ½ периода и в среднем чаще 0,95 периода) →
  таймер с частотой монитора, режим `timer`, строка в журнал; время кадра
  и время показа — по vblank DWM (`DwmGetCompositionTimingInfo(NULL)`,
  структура упакована, поля читаются по смещениям 12 и 28).
- `pacing_other.go` (новый): вне Windows всё как в Fyne (таймер 60 Гц,
  время кадра — время такта).
- `pacing_desktop.go` (новый): частота монитора, на котором центр окна;
  смена монитора — проверка vsync заново; колбэк смены конфигурации
  мониторов.

### Android (`internal/driver/mobile`)

- `driver.go`: поля темпа в `driver`; в `Run` таймер 60 Гц заменён каналом
  меток vsync `app.Vsync()` (таймер остаётся, если канала нет — не
  Android); после `fyne.Do`, событий касания, клавиш, размера и жизненного
  цикла — `requestFrame()`; счёт пальцев на экране (`touches`); в
  `handlePaint` время кадра от Choreographer (`takeFrameTime`),
  `BeginFrame` / `EndFrame`, результат `Publish` — в `reportFrame`, в
  конце — `afterPaint`.
- `pacing.go` (новый): `requestFrame` / `onVsync` (кадр по колбэку;
  во время движения следующий колбэк заказывается до рисования),
  `afterPaint` (нужен ли кадр: грязный холст, анимации, палец, хвост 4
  кадра), `updateFrameRate` (просьба частоты `RateHint` на время движения и
  500 мс после), `reportFrame` (кадры наблюдателю по порядку, когда время
  показа известно).
- `app/app.go`: в `PublishResult` поля `FrameID`, `Swapped`, `Presented` и
  тип `Presented`.
- `app/android.go`: в `mainUI` вокруг `eglSwapBuffers` — `beforeSwap` /
  `afterSwap`; после создания и уничтожения поверхности — `surfaceChanged`;
  текущее окно и частота для `ANativeWindow_setFrameRate` (запросы из канала
  `frameRates`, новое окно получает частоту); в `runInputQueue` после
  `ALooper_prepare` — `pacingInitLooper`, после каждого `ALooper_pollOnce` —
  `pacingPoll`.
- `app/pacing_android.{c,go}` (новые): на потоке ввода кадровые колбэки
  `AChoreographer_postFrameCallback64` (метки vsync в канал `Vsync()`;
  колбэк, не пришедший за 250 мс, заказывается снова) и
  `AChoreographer_registerRefreshRateCallback` (период); на потоке GL
  `ANativeWindow_setFrameRate` (`-lnativewindow`) и метки
  `EGL_ANDROID_get_frame_timestamps` (`EGL_DISPLAY_PRESENT_TIME_ANDROID`),
  без них — время после `eglSwapBuffers` на сетке vsync. Метки EGL
  запрашиваются, только пока задан наблюдатель.
- `app/pacing_other.go` (новый): вне Android `Vsync()` — `nil`,
  `RequestVsync` и `SetFrameRate` ничего не делают.

### Android: касания по кадрам (`internal/driver/mobile`)

Сенсор присылает точки неровно и чаще кадров, а Fyne применял каждую
сразу: сдвиг прокрутки за кадр скакал (ручной замер на телефоне 120 Гц —
`dy` 0,63 против 0,01 у автопрокрутки).

- `event/touch/touch.go`: поле `Time` в `touch.Event` (ноль — неизвестно).
- `app/android.go`: `processEvent` передаёт время события
  (`AMotionEvent_getEventTime`, через `monoTime`) и промежуточные точки
  пакета движения (`AMotionEvent_getHistorical*`) отдельными `TypeMove`.
- `touchresample.go` (новый): `touchResampler` — движения копятся до кадра,
  в кадре одно положение на «метка vsync − 5 мс» (`FrameTime − период − 5 мс`; интерполяция или
  экстраполяция по двум последним точкам не дальше `min(Δ/2, 8 мс)`, как
  `InputConsumer` в Android); назад относительно движения не идёт, в конце
  касания назад не поправляет (иначе развернулась бы инерция `tapUpCanvas`).
  Тесты — `touchresample_test.go`.
- `driver.go`: при Choreographer `TypeBegin` начинает последовательность в
  `touchFrames`, `TypeMove` копится там (без Choreographer или без времени —
  сразу, как в Fyne), перед `TypeEnd` применяется накопленное движение.
- `pacing.go`: `applyTouches` — в `handlePaint` перед `TickAnimations`.

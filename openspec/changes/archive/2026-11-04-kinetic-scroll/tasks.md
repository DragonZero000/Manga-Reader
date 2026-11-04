## 1. Физика броска

- [x] 1.1 `third_party/fyne/internal/fling`: `Spline` — порт `SplineOverScroller` (константы AOSP, таблица `SPLINE_POSITION`, `Distance`, `Duration`, `Position(t)`, `Velocity(t)`), dp и 160 точек на дюйм (design D1)
- [x] 1.2 Тесты: путь и длительность для 1000/3000/8000 dp/с совпадают с формулами AOSP; `Position` монотонна от 0 до `Distance`; `Velocity(0) = v0`, в конце 0
- [x] 1.3 `Start` / `Stop` / `Active`: `fyne.Animation` по осям, `Dragged` со сдвигом за такт, остановка оси на краю `*widget.Scroll`, `DragEnd` и `onEnd` по окончании, пределы 50 и 8000 dp/с, flywheel при том же знаке; тесты на тестовом драйвере (сдвиг за такт по времени кадра, край, сложение скоростей, обратный бросок, `Stop`)

## 2. Скорость пальца и отрыв (Android)

- [x] 2.1 `velocityTracker` в `internal/driver/mobile` (design D2): точки в единицах Fyne, LSQ2 за 100 мс, «палец стоял» > 40 мс → 0; тесты (постоянная скорость, разгон, неровные точки сенсора, остановка перед отрывом)
- [x] 2.2 Точки в трекер из `TypeBegin`/`TypeMove` (вместе с промежуточными); в `tapUpCanvas` вместо горутины Fyne: `*widget.Scroll` и скорость ≥ 50 dp/с → `fling.Start` (и `draggingOuter` своей составляющей), иначе `DragEnd` сразу (design D3)
- [x] 2.3 Касание во время броска (design D4): `fling.Stop()` в `TypeBegin`, пометка последовательности; `canvas.tapUp` без колбэков нажатия, второго и двойного нажатия для помеченной последовательности без перетаскивания; `TouchUp` как обычно; тест на мобильном canvas
- [x] 2.4 `third_party/fyne/PATCHES.md`: раздел «Инерция прокрутки», пометки `// mangareader:`

## 3. Замер

- [x] 3.1 `internal/pacing`: `Frame.Motion` (`""`, `"touch"`, `"fling"`), `SetMotion`; запись в `TakeSample`; мобильный драйвер — палец на экране, `internal/fling` — бросок; `driver/pacing.Fling(target, v)` и `frameclock.Fling`
- [x] 3.2 `frameprobe`: `dyt`, `dyf` (тройки по виду движения среднего кадра) в сводке и `formatReport`; тесты
- [x] 3.3 `screens.ScrollTarget.Draggable()`; сценарии `fling-grid` и `fling-strip` в `probe_on.go` (design D5)
- [x] 3.4 ПК: прогон тестов (`EXTRA_TAGS=frameprobe`) — `fling-*`: `dyf < 0,1`, `1v ≥ 95%`

## 4. Проверка на эмуляторе

- [x] 4.1 Эмулятор x86_64 dev-сборка: бросок свайпом (`adb shell input swipe` коротко и быстро) — сетка и лента продолжают движение и останавливаются; касание во время инерции останавливает её и не открывает карточку; свайп в постраничном режиме листает сразу; увеличенная страница без инерции; край библиотеки

## 5. Документация

- [x] 5.1 `docs/en/architecture.md` и `docs/ru/architecture.md`: инерция (физика Android, по кадрам, остановка касанием, только прокрутки), `internal/fling` в копии Fyne
- [x] 5.2 `docs/en/building.md` и `docs/ru/building.md`: сценарии `fling-grid`, `fling-strip`, поля `dyt`, `dyf`
- [x] 5.3 `docs/en/user-guide.md` и `docs/ru/user-guide.md`: бросок продолжает прокрутку, касание останавливает её без нажатия

## 6. Проверка и сдача

- [x] 6.1 `make test` и тесты копии Fyne (`go test ./internal/fling ./internal/driver/mobile ./internal/pacing` в `third_party/fyne`) зелёные; `make build-windows`
- [x] 6.2 `dist/mangareader.apk` и `dist/mangareader-dev.apk` (arm64) пользователю: тесты (`fling-*`) и ручной замер (`dyt ≤ 0,15`), самостоятельная проверка инерции; результаты в design.md — закрыто по решению пользователя «по ощущениям всё работает нормально»: тесты `fling-*` по критерию (`dyf` 0,015/0,024, `1v` 99,8%), последний ручной замер — `dyt` 0,259, `tlat` 21,1 мс до правок 7.2; замер после 7.2 не снимался

## 7. По отчёту с телефона (2026-11-04)

- [x] 7.1 Пересчёт касаний: задержка по факту прихода точек (наибольший возраст свежей точки за 32 кадра, 5 мс … 5 мс + 2 периода) вместо постоянных 5 мс; интерполяция при любом промежутке; тест-модель доставки пачками (с 5 мс — сдвиги 3,96/8,33, с подбором — ровные); `tlat` в сводке (`pacing.TouchLatency`); дельты `frame-pacing`, `android-build`; `PATCHES.md`, `docs/{en,ru}/{architecture,building}.md`
- [x] 7.2 Опоздание кадра — задним числом, по точкам, пришедшим позже и существовавшим уже к кадру, без паузы перед ними (тест паузы пальца); без рывка к накопленному движению при отрыве с броском (`willFling`); дельта `frame-pacing`, `PATCHES.md`, `docs/{en,ru}/architecture.md`

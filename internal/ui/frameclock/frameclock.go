// Package frameclock — единственная обёртка приложения над темпом кадров
// драйвера (fyne.io/fyne/v2/driver/pacing из копии Fyne в third_party/fyne):
// время кадра для анимаций и бюджетов, частота экрана, наблюдатель кадров
// для замера, запрос частоты на время движения.
//
// Откат на исходный Fyne: убрать replace из go.mod и заменить этот файл
// версией на time.Now() (частота 60, режим "timer", наблюдатель не вызывается).
package frameclock

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/pacing"
)

// Frame — нарисованный кадр: время vsync, период, время SwapBuffers и показа.
type Frame = pacing.Frame

// Now — время vsync, к которому готовится текущий кадр; вне кадра —
// time.Now(). Подходит как поле now func() time.Time.
func Now() time.Time { return pacing.FrameTime() }

// RefreshRate — частота экрана окна, Гц.
func RefreshRate() float64 { return pacing.RefreshRate() }

// Mode — режим темпа кадров: "choreographer", "vsync", "dwm" или "timer".
func Mode() string { return pacing.Mode() }

// PresentSource — источник времени показа кадров: "egl", "swap" или "dwm".
func PresentSource() string { return pacing.PresentSource() }

// Observe задаёт функцию, которую драйвер вызывает в UI-потоке после каждого
// нарисованного кадра (nil — снять).
func Observe(fn func(Frame)) { pacing.SetObserver(fn) }

// Sample задаёт функцию, значение которой попадает в Frame.Sample в момент
// рисования кадра (nil — снять).
func Sample(fn func() float64) { pacing.SetSampler(fn) }

// SetRateHint задаёт частоту экрана (Гц), которую Android просит у системы
// на время движения; 0 — не просить.
func SetRateHint(rate float64) { pacing.SetRateHint(rate) }

// Fling бросает target со скоростью v (единиц Fyne в секунду по осям, знак —
// направление сдвига пальца) так же, как инерция после отрыва пальца. Для
// сценариев замера; только UI-поток. false — бросок не начат.
func Fling(target fyne.Draggable, v fyne.Delta) bool { return pacing.Fling(target, v) }

// FlingActive — идёт ли бросок.
func FlingActive() bool { return pacing.FlingActive() }

// TouchLatency — задержка пересчёта касаний на время кадра (Android); 0 —
// касаний не было.
func TouchLatency() time.Duration { return pacing.TouchLatency() }

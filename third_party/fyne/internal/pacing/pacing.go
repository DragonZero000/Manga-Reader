// mangareader: пакет добавлен в копию Fyne (third_party/fyne/PATCHES.md).

// Package pacing хранит состояние темпа кадров, которое заполняют драйверы:
// время кадра, период vsync, режим темпа, источник времени показа и
// наблюдатель кадров. Приложению оно доступно через fyne.io/fyne/v2/driver/pacing.
package pacing

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// Режимы темпа кадров (Mode).
const (
	ModeChoreographer = "choreographer" // Android: кадровые колбэки AChoreographer
	ModeVsync         = "vsync"         // ПК: SwapInterval(1)
	ModeDWM           = "dwm"           // ПК, vsync выключен в драйвере: ожидание композиции DWM
	ModeTimer         = "timer"         // таймер цикла драйвера
)

// Источники времени показа кадра (PresentSource).
const (
	PresentEGL  = "egl"  // EGL_ANDROID_get_frame_timestamps
	PresentSwap = "swap" // после SwapBuffers, на сетке vsync, если она известна
	PresentDWM  = "dwm"  // первый vblank DWM после SwapBuffers
)

// Frame — один нарисованный кадр.
type Frame struct {
	Time    time.Time     // vsync, к которому готовится кадр (время анимаций)
	Period  time.Duration // период vsync в момент кадра
	Swapped time.Time     // после SwapBuffers
	Present time.Time     // показ на экране (точный или оценка, см. PresentSource)

	// Sample — значение функции SetSampler в момент рисования кадра (не в
	// момент отчёта: на Android отчёт о кадре приходит позже); HasSample —
	// функция была задана.
	Sample    float64
	HasSample bool
}

var (
	frameMu   sync.Mutex
	frameTime time.Time // время текущего кадра; ноль — вне кадра

	period   atomic.Int64 // период vsync, нс; 0 — неизвестен
	mode     atomic.Value // string
	source   atomic.Value // string
	observer atomic.Pointer[func(Frame)]
	sampler  atomic.Pointer[func() float64]
	rateHint atomic.Uint64 // math.Float64bits частоты, Гц
)

func init() {
	mode.Store(ModeTimer)
	source.Store(PresentSwap)
}

// BeginFrame задаёт время кадра, который драйвер начинает готовить.
func BeginFrame(t time.Time) {
	frameMu.Lock()
	frameTime = t
	frameMu.Unlock()
}

// EndFrame — кадр готов; FrameTime снова возвращает time.Now().
func EndFrame() {
	frameMu.Lock()
	frameTime = time.Time{}
	frameMu.Unlock()
}

// FrameTime — время текущего кадра; вне кадра — time.Now().
func FrameTime() time.Time {
	frameMu.Lock()
	t := frameTime
	frameMu.Unlock()
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// SetPeriod задаёт период vsync экрана окна (0 — неизвестен).
func SetPeriod(d time.Duration) { period.Store(int64(d)) }

// Period — период vsync; если неизвестен — 1/60 с.
func Period() time.Duration {
	if p := period.Load(); p > 0 {
		return time.Duration(p)
	}
	return time.Second / 60
}

// RefreshRate — частота экрана окна, Гц.
func RefreshRate() float64 { return float64(time.Second) / float64(Period()) }

// SetMode задаёт режим темпа кадров (ModeChoreographer, ModeVsync, ModeDWM, ModeTimer).
func SetMode(m string) { mode.Store(m) }

// Mode — режим темпа кадров.
func Mode() string { return mode.Load().(string) }

// SetPresentSource задаёт источник времени показа (PresentEGL, PresentSwap, PresentDWM).
func SetPresentSource(s string) { source.Store(s) }

// PresentSource — источник времени показа кадров.
func PresentSource() string { return source.Load().(string) }

// SetObserver задаёт наблюдателя кадров (nil — нет).
func SetObserver(fn func(Frame)) {
	if fn == nil {
		observer.Store(nil)
		return
	}
	observer.Store(&fn)
}

// Observed — наблюдатель задан: драйверу стоит собирать время показа.
func Observed() bool { return observer.Load() != nil }

// SetSampler задаёт функцию, значение которой драйвер записывает в
// Frame.Sample в момент рисования кадра (nil — снять). Вызывается в UI-потоке.
func SetSampler(fn func() float64) {
	if fn == nil {
		sampler.Store(nil)
		return
	}
	sampler.Store(&fn)
}

// TakeSample записывает в f значение функции SetSampler. Драйвер вызывает
// его в UI-потоке сразу после рисования кадра.
func TakeSample(f *Frame) {
	if fn := sampler.Load(); fn != nil {
		f.Sample, f.HasSample = (*fn)(), true
	}
}

// Report передаёт кадр наблюдателю. Вызывается в UI-потоке после кадра.
func Report(f Frame) {
	if fn := observer.Load(); fn != nil {
		(*fn)(f)
	}
}

// SetRateHint задаёт частоту (Гц), которую драйвер Android просит у системы
// на время движения; 0 — не просить.
func SetRateHint(rate float64) { rateHint.Store(math.Float64bits(max(rate, 0))) }

// RateHint — частота, которую просить на время движения; 0 — не просить.
func RateHint() float64 { return math.Float64frombits(rateHint.Load()) }

// NextVsync — первый vsync сетки (vsync, period) не раньше t; при
// неизвестной сетке — t.
func NextVsync(t, vsync time.Time, period time.Duration) time.Time {
	if vsync.IsZero() || period <= 0 {
		return t
	}
	d := t.Sub(vsync)
	k := d / period
	if d > 0 && d%period != 0 {
		k++
	}
	return vsync.Add(k * period)
}

// mangareader: пакет добавлен в копию Fyne (third_party/fyne/PATCHES.md).

// Package pacing — темп кадров драйвера: время кадра для анимаций, частота
// экрана, режим темпа, наблюдатель нарисованных кадров (замер) и разрешение
// просить у системы наибольшую частоту экрана на время движения (Android).
//
// Android: кадры по AChoreographer. Windows: vsync монитора окна. Прочие
// платформы: таймер Fyne, FrameTime — time.Now(), частота 60 Гц.
package pacing

import (
	"time"

	"fyne.io/fyne/v2/internal/pacing"
)

// Frame — один нарисованный кадр.
type Frame = pacing.Frame

// FrameTime — время vsync, к которому готовится текущий кадр; вне кадра —
// time.Now(). Анимации считают прогресс от него.
func FrameTime() time.Time { return pacing.FrameTime() }

// RefreshRate — частота экрана или монитора окна, Гц.
func RefreshRate() float64 { return pacing.RefreshRate() }

// Mode — режим темпа кадров: "choreographer", "vsync", "dwm" (vsync выключен
// в драйвере, кадры ждут композиции DWM) или "timer".
func Mode() string { return pacing.Mode() }

// PresentSource — откуда известно время показа кадров (Frame.Present):
// "egl", "swap" или "dwm".
func PresentSource() string { return pacing.PresentSource() }

// SetObserver задаёт функцию, которую драйвер вызывает в UI-потоке после
// каждого нарисованного кадра (nil — снять). На Android с точным временем
// показа (EGL) кадры приходят с задержкой в несколько кадров, по порядку.
func SetObserver(fn func(Frame)) { pacing.SetObserver(fn) }

// SetSampler задаёт функцию, значение которой драйвер записывает в
// Frame.Sample сразу после рисования кадра (nil — снять). На Android кадр
// попадает к наблюдателю позже, поэтому то, что было на экране в этом
// кадре (например, смещение прокрутки), читается здесь, а не в наблюдателе.
func SetSampler(fn func() float64) { pacing.SetSampler(fn) }

// SetRateHint задаёт частоту экрана (Гц), которую драйвер Android просит у
// системы на время движения (анимации, палец на экране): обычно наибольшая
// частота режимов с текущим разрешением. 0 — не просить, частоту выбирает
// система. Окно получает просьбу через ANativeWindow_setFrameRate.
func SetRateHint(rate float64) { pacing.SetRateHint(rate) }

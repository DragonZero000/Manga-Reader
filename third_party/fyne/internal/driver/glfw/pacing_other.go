//go:build !windows

// mangareader: темп кадров вне Windows — как в Fyne (third_party/fyne/PATCHES.md).

package glfw

import "time"

// presentAfter: время показа неизвестно, кадр считается показанным сразу
// после отправки.
func presentAfter(submitted time.Time) time.Time { return submitted }

// enableVsync: SwapInterval остаётся, как в Fyne.
func enableVsync() {}

// checkVsync: vsync не проверяется.
func checkVsync(_, _ time.Time) {}

// waitVblank: vsync не проверяется.
func waitVblank() {}

// resetVsyncCheck: vsync не проверяется.
func resetVsyncCheck() {}

// tickPeriod: таймер цикла 60 Гц, как в Fyne.
func tickPeriod(time.Time) time.Duration { return time.Second / 60 }

// frameTimeAt: время кадра — время такта.
func frameTimeAt(now time.Time) time.Time { return now }

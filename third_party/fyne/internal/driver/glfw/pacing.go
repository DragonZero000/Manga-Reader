// mangareader: темп кадров ПК (third_party/fyne/PATCHES.md).

package glfw

import (
	"time"

	"fyne.io/fyne/v2/internal/pacing"
)

// lastSwap — когда вернулся последний SwapBuffers. Только главный поток.
var lastSwap time.Time

// swapped вызывается после SwapBuffers, начатого в start: проверка vsync и
// кадр наблюдателю driver/pacing. Только главный поток.
func swapped(start time.Time) {
	now := time.Now()
	checkVsync(start, now)
	waitVblank()
	lastSwap = now
	reportFrame(start, now)
}

// reportFrame передаёт наблюдателю driver/pacing кадр, отправленный
// SwapBuffers с start по swapped. Время показа — первый vblank после start:
// блокирующий SwapBuffers возвращается у самой границы vblank, и оценка по
// возврату прыгала бы на соседний vblank.
func reportFrame(start, swapped time.Time) {
	if !pacing.Observed() {
		return
	}
	f := pacing.Frame{
		Time:    pacing.FrameTime(),
		Period:  pacing.Period(),
		Swapped: swapped,
		Present: presentAfter(start),
	}
	pacing.TakeSample(&f)
	pacing.Report(f)
}

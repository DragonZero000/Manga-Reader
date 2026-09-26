//go:build frameprobe

package ui

import (
	"log"
	"time"

	"fyne.io/fyne/v2"
)

// frameProbePeriod — как часто писать сводку в журнал.
const frameProbePeriod = 5 * time.Second

// startFrameProbe запускает замер шага цикла UI: бесконечная анимация
// получает Tick на каждом кадре драйвера; интервалы между тиками раз в
// frameProbePeriod сводятся и пишутся в журнал (на Android — logcat).
// Вызывается из UI-потока.
func startFrameProbe() {
	var (
		last      time.Time
		intervals []time.Duration
		since     = time.Now()
	)
	a := fyne.NewAnimation(time.Second, func(float32) {
		now := time.Now()
		if !last.IsZero() {
			intervals = append(intervals, now.Sub(last))
		}
		last = now
		if now.Sub(since) >= frameProbePeriod {
			log.Print(summarizeFrames(intervals))
			intervals = intervals[:0]
			since = now
		}
	})
	a.RepeatCount = fyne.AnimationRepeatForever
	a.Curve = fyne.AnimationLinear
	a.Start()
	log.Print("frameprobe: замер кадров включён")
}

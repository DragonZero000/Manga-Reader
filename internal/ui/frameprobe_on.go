//go:build frameprobe

package ui

import (
	"log"
	"runtime/metrics"
	"time"

	"fyne.io/fyne/v2"
)

// frameRecorder — интервалы между кадрами текущей сводки и сводки отчёта.
// Только UI-поток.
type frameRecorder struct {
	label     string // сценарий или «manual»
	last      time.Time
	intervals []time.Duration
	gc        *metrics.Float64Histogram // паузы GC в начале сводки
	anim      *fyne.Animation           // счётчик кадров; nil — замер не идёт
	lines     []string                  // сводки текущего отчёта
}

var frames frameRecorder

// startFrames запускает счётчик кадров: бесконечная анимация получает Tick
// на каждом кадре драйвера, интервалы между тиками — шаг цикла UI. Работает
// только во время тестов или ручного замера. Вызывается из UI-потока.
func startFrames() {
	if frames.anim != nil {
		return
	}
	frames.lines = nil
	frames.last = time.Time{}
	a := fyne.NewAnimation(time.Second, func(float32) {
		now := time.Now()
		if !frames.last.IsZero() {
			frames.intervals = append(frames.intervals, now.Sub(frames.last))
		}
		frames.last = now
	})
	a.RepeatCount = fyne.AnimationRepeatForever
	a.Curve = fyne.AnimationLinear
	a.Start()
	frames.anim = a
}

// stopFrames останавливает счётчик кадров и возвращает сводки отчёта.
func stopFrames() []string {
	if frames.anim != nil {
		frames.anim.Stop()
		frames.anim = nil
	}
	return frames.lines
}

// probeBegin начинает сводку label (прежние интервалы отбрасываются).
func probeBegin(label string) {
	frames.label = label
	frames.intervals = frames.intervals[:0]
	frames.gc = readGCPauses()
}

// probeEnd добавляет сводку в отчёт и пишет её в журнал (на Android — logcat).
func probeEnd() {
	st := summarizeFrames(frames.label, frames.intervals)
	st.GC = gcPauses(frames.gc, readGCPauses())
	st.HeapLive = readHeapLive()
	line := st.String()
	log.Print(line)
	frames.lines = append(frames.lines, line)
}

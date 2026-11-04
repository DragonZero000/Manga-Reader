//go:build frameprobe

package ui

import (
	"image/color"
	"log"
	"runtime/metrics"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"

	"mangareader/internal/ui/frameclock"
)

// frameRecorder — показанные кадры текущей сводки и сводки отчёта.
// Только UI-поток.
type frameRecorder struct {
	label string    // сценарий или «manual»
	begin time.Time // начало сводки: кадры, отправленные раньше, не входят
	recs  []frameRec
	gc    *metrics.Float64Histogram // паузы GC в начале сводки
	anim  *fyne.Animation           // перерисовка на каждом такте; nil — замер не идёт
	lines []string                  // сводки текущего отчёта
}

var frames frameRecorder

// tabSwitch — переключение вкладки, кадр которого ещё не показан (диагностика
// сценария tabs, только журнал). Только UI-поток.
var tabSwitch struct {
	pending  bool
	tab      int
	at, done time.Time // начало и конец SelectIndex
}

// probeTabSwitch переключает вкладку select и запоминает время для журнала.
func probeTabSwitch(tab int, sel func()) {
	at := time.Now()
	sel()
	tabSwitch.pending, tabSwitch.tab, tabSwitch.at, tabSwitch.done = true, tab, at, time.Now()
}

// logTabSwitch пишет в журнал, сколько заняли SelectIndex и путь до
// SwapBuffers кадра, где вкладка показана (раскладка, рисование, обмен).
func logTabSwitch(f frameclock.Frame) {
	if !tabSwitch.pending || !f.Swapped.After(tabSwitch.done) {
		return
	}
	tabSwitch.pending = false
	ms := func(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
	log.Printf("frameprobe: tab %d: select %.1fms, then to swap %.1fms",
		tabSwitch.tab, ms(tabSwitch.done.Sub(tabSwitch.at)), ms(f.Swapped.Sub(tabSwitch.done)))
}

// frameDirty — объект вне дерева окна: Refresh по нему только помечает
// холст грязным.
var frameDirty = canvas.NewRectangle(color.Transparent)

// startFrames начинает запись показанных кадров (наблюдатель frameclock).
// Пока замер идёт, холст c помечается грязным на каждом такте: кадр
// рисуется на каждом vsync, как при прокрутке, и промежутки между показами —
// темп кадров, а не простой. Работает только во время тестов или ручного
// замера. Вызывается из UI-потока.
func startFrames(c fyne.Canvas) {
	if frames.anim != nil {
		return
	}
	frames.lines = nil
	frames.recs = frames.recs[:0]
	frameclock.Observe(recordFrame)
	a := fyne.NewAnimation(time.Second, func(float32) { c.Refresh(frameDirty) })
	a.RepeatCount = fyne.AnimationRepeatForever
	a.Curve = fyne.AnimationLinear
	a.Start()
	frames.anim = a
}

// recordFrame — наблюдатель кадров: время показа и смещение прокрутки.
// На Android с точным временем показа кадры приходят с задержкой в
// несколько кадров и иногда по два сразу, поэтому смещение берётся из
// Frame.Sample — его драйвер записал в момент рисования кадра.
func recordFrame(f frameclock.Frame) {
	logTabSwitch(f)
	if f.Swapped.Before(frames.begin) {
		return // кадр прошлой сводки, пришедший с задержкой
	}
	frames.recs = append(frames.recs, frameRec{
		Present: f.Present, Period: f.Period,
		Offset: float32(f.Sample), HasOffset: f.HasSample, Motion: f.Motion,
	})
}

// stopFrames останавливает запись кадров и возвращает сводки отчёта.
func stopFrames() []string {
	if frames.anim != nil {
		frames.anim.Stop()
		frames.anim = nil
	}
	frameclock.Observe(nil)
	frameclock.Sample(nil)
	return frames.lines
}

// probeBegin начинает сводку label (прежние кадры отбрасываются); offset —
// смещение прокрутки для dy (nil — сценарий без прокрутки).
func probeBegin(label string, offset func() float32) {
	frames.label = label
	frames.begin = time.Now()
	if offset != nil {
		frameclock.Sample(func() float64 { return float64(offset()) })
	} else {
		frameclock.Sample(nil)
	}
	frames.recs = frames.recs[:0]
	frames.gc = readGCPauses()
}

// probeEnd добавляет сводку в отчёт и пишет её в журнал (на Android — logcat).
func probeEnd() {
	st := summarizeFrames(frames.label, frames.recs)
	frameclock.Sample(nil)
	st.GC = gcPauses(frames.gc, readGCPauses())
	st.HeapLive = readHeapLive()
	if st.DYT >= 0 { // палец был на экране
		st.TouchLatency = frameclock.TouchLatency()
	}
	line := st.String()
	log.Print(line)
	frames.lines = append(frames.lines, line)
}

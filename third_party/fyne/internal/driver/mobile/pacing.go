// mangareader: кадры для наблюдателя driver/pacing (third_party/fyne/PATCHES.md).

package mobile

import (
	"time"

	"fyne.io/fyne/v2/internal/driver/mobile/app"
	"fyne.io/fyne/v2/internal/pacing"
)

// maxPaced — сколько кадров держать в ожидании времени показа: больше
// бывает, только если поверхность пересоздана и метки EGL потеряны.
const maxPaced = 16

// pacedFrame — нарисованный кадр, время показа которого ещё не пришло.
type pacedFrame struct {
	id uint64
	f  pacing.Frame
}

// reportFrame передаёт наблюдателю кадр, начатый в t и отправленный на
// экран с результатом res, и кадры, время показа которых стало известно.
// Только цикл драйвера.
func (d *driver) reportFrame(t time.Time, res app.PublishResult) {
	if !pacing.Observed() {
		d.paced = d.paced[:0]
		return
	}
	f := pacing.Frame{Time: t, Period: pacing.Period(), Swapped: res.Swapped}
	pacing.TakeSample(&f)
	if res.FrameID == 0 { // время показа неизвестно: сразу после SwapBuffers
		if f.Swapped.IsZero() {
			f.Swapped = time.Now()
		}
		f.Present = f.Swapped
		pacing.Report(f)
		return
	}
	if len(d.paced) >= maxPaced {
		d.paced = d.paced[:copy(d.paced, d.paced[1:])]
	}
	d.paced = append(d.paced, pacedFrame{id: res.FrameID, f: f})
	for _, p := range res.Presented {
		for i := range d.paced {
			if d.paced[i].id == p.ID {
				d.paced[i].f.Present = p.Present
				break
			}
		}
	}
	n := 0
	for ; n < len(d.paced) && !d.paced[n].f.Present.IsZero(); n++ {
		pacing.Report(d.paced[n].f)
	}
	d.paced = d.paced[:copy(d.paced, d.paced[n:])]
}

// frameTail — сколько кадров ещё заказывать после последнего нарисованного:
// отложенные обновления (раскладка, текстуры) успевают попасть на экран.
const frameTail = 4

// rateHold — сколько держать запрос наибольшей частоты после движения.
const rateHold = 500 * time.Millisecond

// rateSustain — анимация считается движением для запроса частоты, если
// холст перерисовывался на каждом vsync хотя бы столько: прокрутка — да,
// мигание курсора (затухание 100 мс) — нет.
const rateSustain = 200 * time.Millisecond

// vsyncLost — заказанный кадр, не пришедший за это время, заказывается снова.
const vsyncLost = 250 * time.Millisecond

// requestFrame заказывает кадр на ближайший vsync (Choreographer). Пока
// окно не на экране, кадры не заказываются. Только цикл драйвера.
func (d *driver) requestFrame() {
	if d.vsync == nil || d.glctx == nil {
		return
	}
	now := time.Now()
	if !d.vsyncAt.IsZero() && now.Sub(d.vsyncAt) < vsyncLost {
		return // уже заказан
	}
	d.vsyncAt = now
	app.RequestVsync()
}

// onVsync — кадровый колбэк Choreographer с меткой vsync t: кадр
// готовится к следующему vsync. Пока идёт движение, следующий колбэк
// заказывается сразу, до рисования: кадр дольше периода не ждёт лишний
// vsync, метка уже лежит в канале.
func (d *driver) onVsync(t time.Time) {
	d.vsyncAt = time.Time{}
	d.frameTime = t.Add(pacing.Period())
	if d.animation.Running() || d.touches > 0 {
		d.requestFrame()
	}
	d.sendPaintEvent()
}

// applyTouches применяет к окну w положение каждого пальца для кадра со
// временем frame (движения, накопленные с прошлого кадра). Время кадра —
// vsync показа, на период позже метки Choreographer; положение пальца, как
// в Android, берётся от самой метки (frame − период): к началу кадра точки
// новее «frame − 5 мс» ещё не пришли, и положение всегда угадывалось бы
// экстраполяцией. Только цикл драйвера.
func (d *driver) applyTouches(w *window, frame time.Time) {
	vsync := frame.Add(-pacing.Period())
	for _, seq := range d.touchFrames.sequences() {
		if x, y, ok := d.touchFrames.take(seq, vsync); ok {
			d.tapMoveCanvas(w, x, y, seq)
		}
	}
}

// takeFrameTime — время кадра от Choreographer или, без него, time.Now().
func (d *driver) takeFrameTime() time.Time {
	t := d.frameTime
	d.frameTime = time.Time{}
	if t.IsZero() {
		return time.Now()
	}
	return t
}

// afterPaint решает, нужен ли следующий кадр: холст грязный, идут
// анимации, палец на экране или не кончился хвост после нарисованного
// кадра. Иначе кадры не заказываются до события или fyne.Do.
func (d *driver) afterPaint(drawn bool, c *canvas) {
	if d.vsync == nil {
		return
	}
	moving := d.animation.Running() || d.touches > 0
	if drawn {
		d.tail = frameTail
		d.drawnRun++
	} else {
		d.tail = max(d.tail-1, 0)
		d.drawnRun = 0
	}
	d.updateFrameRate(d.rateMotion())
	if moving || c.IsDirty() || d.tail > 0 {
		d.requestFrame()
	}
}

// rateMotion — движение для запроса частоты: палец на экране или анимация,
// перерисовывающая холст на каждом vsync не меньше rateSustain.
func (d *driver) rateMotion() bool {
	if d.touches > 0 {
		return true
	}
	sustain := int(rateSustain.Seconds()*pacing.RefreshRate() + 0.5)
	return d.animation.Running() && d.drawnRun >= sustain
}

// updateFrameRate просит наибольшую частоту экрана (pacing.RateHint) на
// время движения и снимает просьбу через rateHold после него.
func (d *driver) updateFrameRate(moving bool) {
	now := time.Now()
	if moving {
		d.lastMotion = now
	}
	want := float32(0)
	if hint := pacing.RateHint(); hint > 0 && now.Sub(d.lastMotion) < rateHold {
		want = float32(hint)
	}
	if want != d.frameRate {
		d.frameRate = want
		app.SetFrameRate(want)
	}
	if want == 0 {
		return
	}
	wait := rateHold - now.Sub(d.lastMotion)
	if d.rateTimer == nil {
		d.rateTimer = time.AfterFunc(wait, func() {
			d.queuedFuncs.In() <- func() {
				d.updateFrameRate(d.rateMotion())
			}
		})
		return
	}
	d.rateTimer.Reset(wait)
}

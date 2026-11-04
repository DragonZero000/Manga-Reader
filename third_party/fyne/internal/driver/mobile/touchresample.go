// mangareader: движения касаний по кадрам (third_party/fyne/PATCHES.md).

package mobile

import (
	"time"

	"fyne.io/fyne/v2/internal/driver/mobile/event/touch"
)

// Пересчёт касаний на время кадра — как InputConsumer в Android.
const (
	// resampleLatency — на сколько раньше времени кадра берётся положение
	// пальца, если точки приходят сразу: точка новее этого момента обычно
	// уже пришла, и положение интерполируется, а не угадывается.
	resampleLatency = 5 * time.Millisecond
	// resampleMinDelta, resampleMaxDelta — промежуток между двумя последними
	// точками, при котором по ним можно экстраполировать.
	resampleMinDelta = 2 * time.Millisecond
	resampleMaxDelta = 20 * time.Millisecond
	// resampleMaxPrediction — дальше последней точки не экстраполировать.
	resampleMaxPrediction = 8 * time.Millisecond
	// touchHistory — сколько точек одной последовательности хранить (сенсор
	// до 480 Гц при задержке до resampleLatency + 2 периода 60 Гц).
	touchHistory = 64
	// touchLagWindow — за сколько последних кадров с движением помнить
	// возраст свежей точки (около 0,25 с при 120 Гц).
	touchLagWindow = 32
)

// touchSample — точка касания с временем события.
type touchSample struct {
	x, y float32
	t    time.Time
}

// touchTrack — одна последовательность касания: точки по времени и
// положение, применённое в прошлом кадре.
type touchTrack struct {
	samples []touchSample
	applied touchSample
	pending bool // есть точки после applied

	// доставка точек: самая ранняя точка, пришедшая после прошлого кадра
	// (hasFresh — такие есть), время прошлого кадра и самая свежая точка,
	// которая к нему уже была
	freshMin                time.Time
	hasFresh                bool
	lastFrame, newestAtLast time.Time
}

// touchResampler копит движения касаний до кадра и в кадре отдаёт одно
// положение на каждую последовательность (design D6 frame-pacing): сдвиг
// прокрутки за кадр не зависит от того, сколько точек прислал сенсор между
// кадрами. Только цикл драйвера.
//
// Задержка пересчёта подбирается по тому, как приходят точки. В
// NativeActivity касания идут через ViewRootImpl: система отдаёт их пачкой
// раз за кадр со своего Choreographer, и пачка этого кадра приходит то до
// нашего кадрового колбэка, то после. Тогда свежая точка то моложе 5 мс, то
// старше на период, и положение на «vsync − 5 мс» угадывалось бы
// экстраполяцией через кадр — сдвиг скакал бы, как без пересчёта. Поэтому
// задержка — наибольшее опоздание за последние touchLagWindow кадров, от
// resampleLatency до resampleLatency + 2 периода: положение всегда между
// настоящими точками, ценой задержки на кадр там, где точки приходят
// пачками. Опоздание кадра — возраст самой свежей точки, которая к нему
// была, и засчитывается оно, только когда позже пришли точки, существовавшие
// уже к тому кадру (доставка запоздала), без паузы перед ними: остановка
// пальца — не опоздание.
type touchResampler struct {
	tracks map[touch.Sequence]*touchTrack

	lags    [touchLagWindow]time.Duration // опоздания по кадрам
	lagNext int                           // куда писать следующее
	latency time.Duration                 // задержка последнего кадра (замер)
}

// Latency — задержка пересчёта, выбранная в последнем кадре с движением
// (0 — касаний ещё не было).
func (r *touchResampler) Latency() time.Duration { return r.latency }

// noteLag запоминает возраст свежей точки в кадре.
func (r *touchResampler) noteLag(d time.Duration) {
	r.lags[r.lagNext] = max(d, 0)
	r.lagNext = (r.lagNext + 1) % touchLagWindow
}

// resampleDelay — задержка пересчёта при периоде кадров period.
func (r *touchResampler) resampleDelay(period time.Duration) time.Duration {
	d := resampleLatency
	for _, l := range r.lags {
		d = max(d, l)
	}
	return min(d, resampleLatency+2*period)
}

// begin начинает последовательность seq: точка уже применена (TypeBegin).
func (r *touchResampler) begin(seq touch.Sequence, x, y float32, t time.Time) {
	if r.tracks == nil {
		r.tracks = make(map[touch.Sequence]*touchTrack)
	}
	s := touchSample{x, y, t}
	r.tracks[seq] = &touchTrack{samples: []touchSample{s}, applied: s}
}

// move добавляет точку движения; false — последовательность не начата
// (движение надо применить сразу).
func (r *touchResampler) move(seq touch.Sequence, x, y float32, t time.Time) bool {
	tr := r.tracks[seq]
	if tr == nil {
		return false
	}
	if n := len(tr.samples); n > 0 && t.Before(tr.samples[n-1].t) {
		t = tr.samples[n-1].t // время не идёт назад
	}
	if len(tr.samples) >= touchHistory {
		tr.samples = tr.samples[:copy(tr.samples, tr.samples[1:])]
	}
	tr.samples = append(tr.samples, touchSample{x, y, t})
	tr.pending = true
	if !tr.hasFresh || t.Before(tr.freshMin) {
		tr.freshMin = t
	}
	tr.hasFresh = true
	return true
}

// end завершает последовательность seq. Если после применённого положения
// пришли точки и последняя из них впереди него, возвращает её (её надо
// применить перед TypeEnd) и true. Назад не возвращает: инерция Fyne после
// отрыва пальца продолжает последний сдвиг, и шаг назад развернул бы её.
func (r *touchResampler) end(seq touch.Sequence) (x, y float32, ok bool) {
	tr := r.tracks[seq]
	if tr == nil {
		return 0, 0, false
	}
	delete(r.tracks, seq)
	n := len(tr.samples)
	last := tr.samples[n-1]
	if !tr.pending || (last.x == tr.applied.x && last.y == tr.applied.y) {
		return 0, 0, false
	}
	if n > 1 {
		prev := tr.samples[n-2]
		if (last.x-tr.applied.x)*(last.x-prev.x)+(last.y-tr.applied.y)*(last.y-prev.y) < 0 {
			return 0, 0, false
		}
	}
	return last.x, last.y, true
}

// sequences — начатые последовательности.
func (r *touchResampler) sequences() []touch.Sequence {
	seqs := make([]touch.Sequence, 0, len(r.tracks))
	for seq := range r.tracks {
		seqs = append(seqs, seq)
	}
	return seqs
}

// take — положение пальца seq для кадра с меткой vsync frame (время
// кадрового колбэка) при периоде кадров period: на frame − задержка
// (resampleDelay). false — с прошлого кадра нечего применять.
func (r *touchResampler) take(seq touch.Sequence, frame time.Time, period time.Duration) (x, y float32, ok bool) {
	tr := r.tracks[seq]
	if tr == nil {
		return 0, 0, false
	}
	last := tr.samples[len(tr.samples)-1]
	// прошлый кадр видел точки с опозданием: пришли точки не новее его,
	// и перед ними не было паузы
	if tr.hasFresh && !tr.lastFrame.IsZero() && !tr.freshMin.After(tr.lastFrame) &&
		tr.freshMin.Sub(tr.newestAtLast) <= resampleMaxDelta {
		r.noteLag(tr.lastFrame.Sub(tr.newestAtLast))
	}
	tr.hasFresh = false
	tr.lastFrame, tr.newestAtLast = frame, last.t
	if !tr.pending {
		return 0, 0, false
	}
	r.latency = r.resampleDelay(period)
	s, ok := resample(tr.samples, frame.Add(-r.latency))
	if !ok {
		return 0, 0, false // все точки новее: подождут следующего кадра
	}
	// не назад относительно направления последнего движения
	if (s.x-tr.applied.x)*(last.x-tr.applied.x)+(s.y-tr.applied.y)*(last.y-tr.applied.y) < 0 {
		s = tr.applied
	}
	tr.pending = s.t.Before(last.t)
	if s.x == tr.applied.x && s.y == tr.applied.y {
		return 0, 0, false
	}
	tr.applied = s
	// старее предпоследней точки до момента кадра точки больше не нужны
	i := 0
	for i+2 < len(tr.samples) && !tr.samples[i+2].t.After(s.t) {
		i++
	}
	tr.samples = tr.samples[:copy(tr.samples, tr.samples[i:])]
	return s.x, s.y, true
}

// resample — положение в момент at по точкам samples (по времени):
// интерполяция между соседними точками, а если новее at точки нет —
// экстраполяция по двум последним не дальше min(Δ/2, resampleMaxPrediction).
// false — все точки новее at.
func resample(samples []touchSample, at time.Time) (touchSample, bool) {
	i := -1 // последняя точка не новее at
	for j, s := range samples {
		if s.t.After(at) {
			break
		}
		i = j
	}
	if i < 0 {
		return touchSample{}, false
	}
	a := samples[i]
	if i+1 < len(samples) { // интерполяция при любом промежутке: сенсор 480 Гц
		b := samples[i+1] // присылает точки чаще resampleMinDelta
		d := b.t.Sub(a.t)
		if d <= 0 {
			return a, true
		}
		return lerp(a, b, at.Sub(a.t), d, at), true
	}
	if i == 0 {
		return a, true
	}
	p := samples[i-1]
	d := a.t.Sub(p.t)
	if d < resampleMinDelta || d > resampleMaxDelta {
		return a, true
	}
	at = minTime(at, a.t.Add(min(d/2, resampleMaxPrediction)))
	return lerp(p, a, at.Sub(p.t), d, at), true
}

// lerp — точка между a и b через dt из d (dt > d — продолжение за b).
func lerp(a, b touchSample, dt, d time.Duration, at time.Time) touchSample {
	k := float32(float64(dt) / float64(d))
	return touchSample{x: a.x + (b.x-a.x)*k, y: a.y + (b.y-a.y)*k, t: at}
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

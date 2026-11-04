// mangareader: скорость пальца для инерции прокрутки (third_party/fyne/PATCHES.md).

package mobile

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/internal/driver/mobile/event/touch"
)

// Параметры как у VelocityTracker в Android.
const (
	// velocityHorizon — по точкам за какое время до последней считается скорость.
	velocityHorizon = 100 * time.Millisecond
	// velocityStopped — пауза между точками (и между последней точкой и
	// отрывом), после которой считается, что палец стоял.
	velocityStopped = 40 * time.Millisecond
	// velocityHistory — сколько точек хранить.
	velocityHistory = 32
)

// velocitySample — точка пальца в единицах Fyne.
type velocitySample struct {
	x, y float64
	t    time.Time
}

// velocityTracker считает скорость пальца по сырым точкам касания (до
// пересчёта на время кадра), как VelocityTracker в Android: квадратичный
// метод наименьших квадратов (LSQ2) по точкам за последние 100 мс. Только
// цикл драйвера.
type velocityTracker struct {
	samples []velocitySample
}

// add добавляет точку; пауза дольше velocityStopped забывает прежние точки.
func (v *velocityTracker) add(x, y float32, t time.Time) {
	if n := len(v.samples); n > 0 {
		last := v.samples[n-1].t
		if t.Sub(last) > velocityStopped {
			v.samples = v.samples[:0]
		} else if t.Before(last) {
			t = last
		}
	}
	if len(v.samples) >= velocityHistory {
		v.samples = v.samples[:copy(v.samples, v.samples[1:])]
	}
	v.samples = append(v.samples, velocitySample{float64(x), float64(y), t})
}

// reset забывает все точки.
func (v *velocityTracker) reset() { v.samples = v.samples[:0] }

// velocity — скорость пальца (единиц Fyne в секунду) на момент отрыва at.
// Нулевой at пропускает проверку «палец стоял».
func (v *velocityTracker) velocity(at time.Time) (vx, vy float64) {
	n := len(v.samples)
	if n < 2 {
		return 0, 0
	}
	last := v.samples[n-1]
	if !at.IsZero() && at.Sub(last.t) > velocityStopped {
		return 0, 0
	}
	first := n - 1
	for first > 0 && last.t.Sub(v.samples[first-1].t) <= velocityHorizon {
		first--
	}
	pts := v.samples[first:]
	if len(pts) < 2 {
		return 0, 0
	}
	ts := make([]float64, len(pts))
	xs := make([]float64, len(pts))
	ys := make([]float64, len(pts))
	for i, s := range pts {
		ts[i] = s.t.Sub(last.t).Seconds() // ≤ 0, у последней точки 0
		xs[i], ys[i] = s.x, s.y
	}
	if len(pts) < 3 {
		dt := ts[len(ts)-1] - ts[0]
		if dt <= 0 {
			return 0, 0
		}
		return (xs[len(xs)-1] - xs[0]) / dt, (ys[len(ys)-1] - ys[0]) / dt
	}
	return slope(ts, xs), slope(ts, ys)
}

// slope — производная в t = 0 многочлена второй степени a + b·t + c·t²,
// подогнанного методом наименьших квадратов к точкам (ts, ys), то есть b.
// Если все t совпадают, 0.
func slope(ts, ys []float64) float64 {
	// нормальные уравнения: суммы степеней t и t^k·y
	var s0, s1, s2, s3, s4, y0, y1, y2 float64
	s0 = float64(len(ts))
	for i, t := range ts {
		t2 := t * t
		s1 += t
		s2 += t2
		s3 += t2 * t
		s4 += t2 * t2
		y0 += ys[i]
		y1 += ys[i] * t
		y2 += ys[i] * t2
	}
	// | s0 s1 s2 | |a|   |y0|
	// | s1 s2 s3 | |b| = |y1|
	// | s2 s3 s4 | |c|   |y2|
	det := s0*(s2*s4-s3*s3) - s1*(s1*s4-s3*s2) + s2*(s1*s3-s2*s2)
	if det > -1e-30 && det < 1e-30 {
		// вырожденный набор (точки одного момента): линейная оценка
		return 0
	}
	detB := s0*(y1*s4-s3*y2) - y0*(s1*s4-s3*s2) + s2*(s1*y2-y1*s2)
	return detB / det
}

// trackVelocity передаёт точку касания e трекеру скорости; begin — начало
// последовательности (прежние точки забываются). Координаты — в единицах Fyne.
func (d *driver) trackVelocity(w *window, e touch.Event, begin bool) {
	if e.Time.IsZero() {
		return
	}
	if d.velocities == nil {
		d.velocities = make(map[touch.Sequence]*velocityTracker)
	}
	v := d.velocities[e.Sequence]
	if v == nil {
		v = &velocityTracker{}
		d.velocities[e.Sequence] = v
	}
	if begin {
		v.reset()
	}
	s := w.canvas.Scale()
	v.add(e.X/s, e.Y/s, e.Time)
}

// liftVelocity — скорость пальца при отрыве e (единиц Fyne в секунду);
// трекер последовательности забывается.
func (d *driver) liftVelocity(w *window, e touch.Event) fyne.Delta {
	v := d.velocities[e.Sequence]
	delete(d.velocities, e.Sequence)
	if v == nil {
		return fyne.Delta{}
	}
	vx, vy := v.velocity(e.Time)
	return fyne.NewDelta(float32(vx), float32(vy))
}

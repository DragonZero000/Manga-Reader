// mangareader: пакет добавлен в копию Fyne (third_party/fyne/PATCHES.md).

package fling

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/internal/pacing"
	"fyne.io/fyne/v2/internal/widget"
)

// Пределы скорости броска, единиц Fyne в секунду (ViewConfiguration в AOSP).
const (
	MinVelocity = 50.0
	MaxVelocity = 8000.0
)

// Offsetter — цель броска, не являющаяся *widget.Scroll, но сообщающая своё
// смещение: бросок останавливает ось, когда смещение по ней перестало меняться.
type Offsetter interface {
	ScrollOffset() fyne.Position
}

// StartAnimation запускает анимацию; тесты подменяют, чтобы вести кадры вручную.
var StartAnimation = func(a *fyne.Animation) { a.Start() }

// StopAnimation останавливает анимацию (подменяется вместе с StartAnimation).
var StopAnimation = func(a *fyne.Animation) { a.Stop() }

// axis — бросок по одной оси.
type axis struct {
	spline Spline
	sign   float64 // направление сдвига пальца: +1 или -1
	done   float64 // путь, уже отданный цели
	active bool
}

// velocity — скорость оси со знаком в момент t.
func (a *axis) velocity(t time.Duration) float64 {
	if !a.active {
		return 0
	}
	return a.sign * a.spline.Velocity(t)
}

type flinger struct {
	target   fyne.Draggable
	onEnd    func()
	x, y     axis
	duration time.Duration
	anim     *fyne.Animation
	started  time.Time // время кадра, с которого идёт анимация
}

// current — идущий бросок; только UI-поток.
var current *flinger

// Active — идёт ли бросок.
func Active() bool { return current != nil }

// Stop останавливает идущий бросок: цель получает DragEnd. true — бросок шёл.
func Stop() bool {
	f := current
	if f == nil {
		return false
	}
	f.finish()
	return true
}

// Start бросает target со скоростью v (единиц Fyne в секунду по осям, знак —
// направление сдвига пальца); onEnd вызывается после остановки. Скорость по
// оси меньше MinVelocity ось не бросает, больше MaxVelocity ограничивается.
// Бросок по тому же target в ту же сторону прибавляет скорость к текущей
// (как OverScroller.fling в Android). false — бросать нечего.
func Start(target fyne.Draggable, v fyne.Delta, onEnd func()) bool {
	vx, vy := clampVelocity(float64(v.DX)), clampVelocity(float64(v.DY))
	if vx == 0 && vy == 0 {
		return false
	}
	if old := current; old != nil {
		if old.target == target {
			now := old.elapsed()
			if cx := old.x.velocity(now); cx*vx > 0 {
				vx = clampVelocity(vx + cx)
			}
			if cy := old.y.velocity(now); cy*vy > 0 {
				vy = clampVelocity(vy + cy)
			}
			// тот же target: DragEnd не вызывается, бросок продолжается
			StopAnimation(old.anim)
			current = nil
			if onEnd == nil {
				onEnd = old.onEnd
			}
		} else {
			old.finish()
		}
	}

	f := &flinger{target: target, onEnd: onEnd}
	f.x = newAxis(vx)
	f.y = newAxis(vy)
	f.duration = max(f.x.spline.Duration, f.y.spline.Duration)
	f.anim = &fyne.Animation{Duration: f.duration, Curve: fyne.AnimationLinear, Tick: f.tick}
	f.started = pacing.FrameTime()
	current = f
	pacing.SetMotion(pacing.MotionFling)
	StartAnimation(f.anim)
	return true
}

func clampVelocity(v float64) float64 {
	if math.Abs(v) < MinVelocity {
		return 0
	}
	return math.Copysign(math.Min(math.Abs(v), MaxVelocity), v)
}

func newAxis(v float64) axis {
	if v == 0 {
		return axis{}
	}
	return axis{spline: NewSpline(v), sign: math.Copysign(1, v), active: true}
}

// elapsed — сколько идёт бросок по времени кадра.
func (f *flinger) elapsed() time.Duration {
	return min(max(pacing.FrameTime().Sub(f.started), 0), f.duration)
}

// tick — такт анимации: прогресс 0…1 по длительности броска.
func (f *flinger) tick(p float32) {
	if current != f {
		return
	}
	t := time.Duration(float64(p) * float64(f.duration))
	var d fyne.Delta
	before := f.offset()
	if f.x.active {
		d.DX = f.x.step(t)
	}
	if f.y.active {
		d.DY = f.y.step(t)
	}
	if d.DX != 0 || d.DY != 0 {
		f.target.Dragged(&fyne.DragEvent{Dragged: d})
		after := f.offset()
		if f.x.active && d.DX != 0 && after.X == before.X {
			f.x.active = false
		}
		if f.y.active && d.DY != 0 && after.Y == before.Y {
			f.y.active = false
		}
	}
	if p >= 1 || (!f.x.active && !f.y.active) {
		f.finish()
	}
}

// step — сдвиг оси за такт к моменту t (со знаком); конец пути выключает ось.
func (a *axis) step(t time.Duration) float32 {
	pos := a.spline.Position(t)
	d := pos - a.done
	a.done = pos
	if t >= a.spline.Duration {
		a.active = false
	}
	return float32(a.sign * d)
}

// offset — смещение цели; для целей без смещения — фиктивное, ось тогда
// кончается только по времени.
func (f *flinger) offset() fyne.Position {
	switch s := any(f.target).(type) {
	case *widget.Scroll:
		return s.Offset
	case Offsetter:
		return s.ScrollOffset()
	}
	return fyne.Position{X: float32(math.NaN()), Y: float32(math.NaN())}
}

// finish заканчивает бросок: DragEnd цели и onEnd.
func (f *flinger) finish() {
	if current != f {
		return
	}
	current = nil
	StopAnimation(f.anim)
	if pacing.CurrentMotion() == pacing.MotionFling {
		pacing.SetMotion("")
	}
	f.target.DragEnd()
	if f.onEnd != nil {
		f.onEnd()
	}
}

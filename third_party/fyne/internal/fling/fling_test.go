// mangareader: тесты пакета, добавленного в копию Fyne.

package fling

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/internal/pacing"
)

// fakeScroll — цель броска с пределами смещения.
type fakeScroll struct {
	y, max   float32
	dragged  []fyne.Delta
	dragEnds int
}

func (s *fakeScroll) Dragged(e *fyne.DragEvent) {
	s.dragged = append(s.dragged, e.Dragged)
	s.y = min(max(s.y-e.Dragged.DY, 0), s.max)
}
func (s *fakeScroll) DragEnd()                    { s.dragEnds++ }
func (s *fakeScroll) ScrollOffset() fyne.Position { return fyne.NewPos(0, s.y) }

// harness заменяет анимацию Fyne: кадры идут вручную.
type harness struct {
	t    *testing.T
	anim *fyne.Animation
	t0   time.Time
	now  time.Time
}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, t0: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)}
	h.now = h.t0
	pacing.BeginFrame(h.now)
	StartAnimation = func(a *fyne.Animation) { h.anim = a }
	StopAnimation = func(a *fyne.Animation) {
		if h.anim == a {
			h.anim = nil
		}
	}
	t.Cleanup(func() {
		Stop()
		pacing.EndFrame()
		pacing.SetMotion("")
		StartAnimation = func(a *fyne.Animation) { a.Start() }
		StopAnimation = func(a *fyne.Animation) { a.Stop() }
	})
	return h
}

// frame — кадр через d: время кадра и такт анимации.
func (h *harness) frame(d time.Duration) {
	h.now = h.now.Add(d)
	pacing.BeginFrame(h.now)
	if h.anim == nil {
		return
	}
	since := h.now.Sub(h.t0)
	p := float32(1)
	if since < h.anim.Duration {
		p = float32(float64(since) / float64(h.anim.Duration))
	}
	h.anim.Tick(p)
}

func (h *harness) run(frame time.Duration) {
	for i := 0; h.anim != nil && i < 100000; i++ {
		h.frame(frame)
	}
}

func total(s *fakeScroll) float64 {
	var sum float64
	for _, d := range s.dragged {
		sum += float64(d.DY)
	}
	return sum
}

func TestStartTotalDistance(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{y: 0, max: 1e9}
	ended := 0
	if !Start(s, fyne.Delta{DY: -3000}, func() { ended++ }) {
		t.Fatal("бросок не начат")
	}
	if !Active() || pacing.CurrentMotion() != pacing.MotionFling {
		t.Fatal("бросок не активен")
	}
	if h.anim == nil || h.anim.Duration != NewSpline(3000).Duration || h.anim.Curve == nil {
		t.Fatalf("анимация: %+v", h.anim)
	}
	h.run(8333 * time.Microsecond)
	want := NewSpline(3000).Distance
	if got := total(s); math.Abs(-got-want) > 0.5 {
		t.Errorf("путь %v, ждали %v", -got, want)
	}
	if Active() || ended != 1 || s.dragEnds != 1 || pacing.CurrentMotion() != "" {
		t.Errorf("конец: active=%v ended=%d dragEnds=%d motion=%q", Active(), ended, s.dragEnds, pacing.CurrentMotion())
	}
	if float64(s.y) < want-1 {
		t.Errorf("смещение %v", s.y)
	}
}

func TestShiftPerFrameFollowsFrameTime(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{max: 1e9}
	Start(s, fyne.Delta{DY: -3000}, nil)
	sp := NewSpline(3000)
	// кадр с шагом 16 мс даёт ровно путь за эти 16 мс
	h.frame(16 * time.Millisecond)
	if len(s.dragged) != 1 {
		t.Fatalf("сдвигов: %d", len(s.dragged))
	}
	want := sp.Position(16 * time.Millisecond)
	if got := -float64(s.dragged[0].DY); math.Abs(got-want) > 0.5 {
		t.Errorf("сдвиг %v, ждали %v", got, want)
	}
	// пропущенный кадр (33 мс) сдвигает на путь за 33 мс, не на «один шаг»
	h.frame(33 * time.Millisecond)
	want = sp.Position(49*time.Millisecond) - sp.Position(16*time.Millisecond)
	if got := -float64(s.dragged[1].DY); math.Abs(got-want) > 0.5 {
		t.Errorf("сдвиг %v, ждали %v", got, want)
	}
}

func TestEdgeStopsAxis(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{max: 100}
	ended := 0
	Start(s, fyne.Delta{DY: -5000}, func() { ended++ })
	h.run(8 * time.Millisecond)
	if s.y != 100 || Active() || ended != 1 || s.dragEnds != 1 {
		t.Errorf("y=%v active=%v ended=%d", s.y, Active(), ended)
	}
	// остановка раньше конца броска
	if n := len(s.dragged); time.Duration(n)*8*time.Millisecond >= NewSpline(5000).Duration {
		t.Errorf("тактов: %d, бросок дошёл до конца по времени", n)
	}
}

func TestVelocityLimits(t *testing.T) {
	newHarness(t)
	s := &fakeScroll{max: 1e9}
	if Start(s, fyne.Delta{DY: -49}, nil) || Active() {
		t.Fatal("49 < 50: бросок не начинается")
	}
	if !Start(s, fyne.Delta{DY: -50}, nil) {
		t.Fatal("50: бросок начинается")
	}
	Stop()
	Start(s, fyne.Delta{DY: -20000}, nil)
	if got, want := current.y.spline.V0, MaxVelocity; got != want {
		t.Errorf("скорость %v, ждали %v", got, want)
	}
}

func TestFlywheel(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{max: 1e9}
	Start(s, fyne.Delta{DY: -2000}, nil)
	h.frame(100 * time.Millisecond)
	cur := NewSpline(2000).Velocity(100 * time.Millisecond)
	Start(s, fyne.Delta{DY: -1000}, nil)
	if s.dragEnds != 0 {
		t.Error("тот же target: DragEnd не вызывается")
	}
	if got, want := current.y.spline.V0, 1000+cur; math.Abs(got-want) > 1 {
		t.Errorf("сумма скоростей %v, ждали %v", got, want)
	}
	if current.y.sign != -1 || current.y.done != 0 {
		t.Errorf("ось: %+v", current.y)
	}
}

func TestReverseStartsFresh(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{y: 5000, max: 1e9}
	Start(s, fyne.Delta{DY: -2000}, nil)
	h.frame(100 * time.Millisecond)
	Start(s, fyne.Delta{DY: 1500}, nil)
	if got := current.y.spline.V0; got != 1500 || current.y.sign != 1 {
		t.Errorf("обратный бросок: %+v", current.y)
	}
}

func TestStop(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{max: 1e9}
	ended := 0
	if Stop() {
		t.Fatal("нечего останавливать")
	}
	Start(s, fyne.Delta{DY: -3000}, func() { ended++ })
	h.frame(50 * time.Millisecond)
	n := len(s.dragged)
	if !Stop() || Active() || ended != 1 || s.dragEnds != 1 {
		t.Fatalf("Stop: active=%v ended=%d dragEnds=%d", Active(), ended, s.dragEnds)
	}
	h.frame(50 * time.Millisecond)
	if len(s.dragged) != n {
		t.Error("после Stop сдвигов нет")
	}
}

func TestOtherTargetStopsPrevious(t *testing.T) {
	h := newHarness(t)
	a, b := &fakeScroll{max: 1e9}, &fakeScroll{max: 1e9}
	Start(a, fyne.Delta{DY: -3000}, nil)
	h.frame(30 * time.Millisecond)
	Start(b, fyne.Delta{DY: -3000}, nil)
	if a.dragEnds != 1 || current.target != fyne.Draggable(b) {
		t.Errorf("a.dragEnds=%d", a.dragEnds)
	}
}

func TestBothAxes(t *testing.T) {
	h := newHarness(t)
	s := &fakeScroll{max: 1e9}
	Start(s, fyne.Delta{DX: 1000, DY: -3000}, nil)
	h.frame(16 * time.Millisecond)
	if s.dragged[0].DX <= 0 || s.dragged[0].DY >= 0 {
		t.Errorf("знаки: %+v", s.dragged[0])
	}
	if current.duration != NewSpline(3000).Duration {
		t.Errorf("длительность — большей оси: %v", current.duration)
	}
}

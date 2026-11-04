// mangareader: тесты скорости пальца.

package mobile

import (
	"math"
	"testing"
	"time"
)

var vt0 = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func near(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: %v, ждали %v", name, got, want)
	}
}

func TestVelocityConstant(t *testing.T) {
	var v velocityTracker
	// 1000 единиц/с по y вверх, 500 по x, точки каждые 4 мс (сенсор 240 Гц)
	for i := 0; i <= 25; i++ {
		ms := time.Duration(i*4) * time.Millisecond
		v.add(float32(100+0.5*float64(i*4)), float32(800-1.0*float64(i*4)), vt0.Add(ms))
	}
	vx, vy := v.velocity(vt0.Add(100*time.Millisecond + 5*time.Millisecond))
	near(t, "vx", vx, 500, 1)
	near(t, "vy", vy, -1000, 1)
}

func TestVelocityAccelerating(t *testing.T) {
	var v velocityTracker
	// y = 5000·t² (ускорение 10000): скорость в конце 5000·2·t
	for i := 0; i <= 40; i++ {
		ts := float64(i*4) / 1000
		v.add(0, float32(5000*ts*ts), vt0.Add(time.Duration(i*4)*time.Millisecond))
	}
	_, vy := v.velocity(vt0.Add(160 * time.Millisecond))
	near(t, "vy", vy, 10000*0.16, 20) // квадратичная подгонка точна для параболы
}

func TestVelocityUnevenSamples(t *testing.T) {
	var v velocityTracker
	// постоянная скорость 2000, но точки идут неровно: 1, 7, 2, 9, 3 мс
	at := time.Duration(0)
	for _, gap := range []int{0, 1, 7, 2, 9, 3, 8, 2, 10, 3, 6, 4, 7, 2, 9, 3} {
		at += time.Duration(gap) * time.Millisecond
		v.add(0, float32(2000*at.Seconds()), vt0.Add(at))
	}
	_, vy := v.velocity(vt0.Add(at))
	near(t, "vy", vy, 2000, 1)
}

func TestVelocityFingerStopped(t *testing.T) {
	var v velocityTracker
	for i := 0; i <= 10; i++ {
		v.add(0, float32(i*8), vt0.Add(time.Duration(i*8)*time.Millisecond))
	}
	if _, vy := v.velocity(vt0.Add(80*time.Millisecond + 30*time.Millisecond)); vy == 0 {
		t.Error("30 мс после последней точки: ещё движется")
	}
	if vx, vy := v.velocity(vt0.Add(80*time.Millisecond + 500*time.Millisecond)); vx != 0 || vy != 0 {
		t.Errorf("палец стоял полсекунды: %v %v", vx, vy)
	}
	// стоял, потом двинулся: прежние точки забыты
	v.add(0, 100, vt0.Add(700*time.Millisecond))
	v.add(0, 101, vt0.Add(704*time.Millisecond))
	_, vy := v.velocity(vt0.Add(705 * time.Millisecond))
	near(t, "vy после паузы", vy, 250, 1)
}

func TestVelocityFewPoints(t *testing.T) {
	var v velocityTracker
	if vx, vy := v.velocity(vt0); vx != 0 || vy != 0 {
		t.Error("без точек")
	}
	v.add(0, 0, vt0)
	if _, vy := v.velocity(vt0); vy != 0 {
		t.Error("одна точка")
	}
	v.add(0, 10, vt0.Add(10*time.Millisecond))
	_, vy := v.velocity(vt0.Add(10 * time.Millisecond))
	near(t, "две точки", vy, 1000, 1e-6)
	v.reset()
	if _, vy := v.velocity(vt0); vy != 0 {
		t.Error("после reset")
	}
}

func TestVelocityHorizon(t *testing.T) {
	var v velocityTracker
	// первые 200 мс — медленно (100/с), последние 100 мс — быстро (3000/с)
	y := 0.0
	for ms := 0; ms <= 300; ms += 5 {
		if ms > 200 {
			y += 3000 * 0.005
		} else if ms > 0 {
			y += 100 * 0.005
		}
		v.add(0, float32(y), vt0.Add(time.Duration(ms)*time.Millisecond))
	}
	_, vy := v.velocity(vt0.Add(300 * time.Millisecond))
	if vy < 2500 {
		t.Errorf("скорость по последним 100 мс: %v", vy)
	}
}

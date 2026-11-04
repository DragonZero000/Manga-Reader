// mangareader: тесты пакета, добавленного в копию Fyne.

package fling

import (
	"math"
	"testing"
	"time"
)

// Путь и длительность по формулам AOSP, посчитанные отдельно от кода:
//
//	l = ln(0.35·v / (0.015 · 9.80665·39.37·160·0.84))
//	duration = 1000·exp(l / (r−1)) мс, distance = f·c·exp(r/(r−1)·l), r = ln0.78/ln0.9
func TestSplineAOSP(t *testing.T) {
	for _, v := range []float64{1000, 3000, 8000} {
		s := NewSpline(v)
		coeff := 0.015 * 9.80665 * 39.37 * 160 * 0.84
		l := math.Log(0.35 * v / coeff)
		r := math.Log(0.78) / math.Log(0.9)
		wantMs := 1000 * math.Exp(l/(r-1))
		wantDist := coeff * math.Exp(r/(r-1)*l)
		if math.Abs(s.Distance-wantDist) > 1e-6*wantDist {
			t.Errorf("v=%v: путь %v, ждали %v", v, s.Distance, wantDist)
		}
		if got := float64(s.Duration) / float64(time.Millisecond); math.Abs(got-wantMs) > 1e-3 {
			t.Errorf("v=%v: длительность %v мс, ждали %v", v, got, wantMs)
		}
		t.Logf("v=%v distance=%.1f duration=%v", v, s.Distance, s.Duration)
	}
}

func TestSplineGrowsWithVelocity(t *testing.T) {
	a, b, c := NewSpline(1000), NewSpline(3000), NewSpline(8000)
	if !(a.Distance < b.Distance && b.Distance < c.Distance) || !(a.Duration < b.Duration && b.Duration < c.Duration) {
		t.Fatalf("%+v %+v %+v", a, b, c)
	}
	if NewSpline(0).Distance != 0 || NewSpline(0).Duration != 0 {
		t.Fatal("нулевая скорость — нулевой бросок")
	}
}

func TestSplinePosition(t *testing.T) {
	for _, v := range []float64{1000, 3000, 8000} {
		s := NewSpline(v)
		prev := -1.0
		for ms := time.Duration(0); ms <= s.Duration+50*time.Millisecond; ms += 5 * time.Millisecond {
			p := s.Position(ms)
			if p < prev-1e-9 || p > s.Distance+1e-9 {
				t.Fatalf("v=%v t=%v: %v после %v (путь %v)", v, ms, p, prev, s.Distance)
			}
			prev = p
		}
		if s.Position(0) != 0 || s.Position(s.Duration) != s.Distance || s.Position(s.Duration*2) != s.Distance {
			t.Errorf("v=%v: границы пути", v)
		}
	}
}

func TestSplineVelocity(t *testing.T) {
	for _, v := range []float64{1000, 3000, 8000} {
		s := NewSpline(v)
		// в начале скорость табличная и близка к заданной (в AOSP так же)
		if got := s.Velocity(0); math.Abs(got-v) > 0.1*v {
			t.Errorf("v=%v: начальная скорость %v", v, got)
		}
		if s.Velocity(s.Duration) != 0 || s.Velocity(2*s.Duration) != 0 {
			t.Errorf("v=%v: в конце скорость не 0", v)
		}
		if mid := s.Velocity(s.Duration / 2); mid >= s.Velocity(0) || mid <= 0 {
			t.Errorf("v=%v: торможение %v", v, mid)
		}
		// скорость — производная пути
		t0 := s.Duration / 3
		dt := time.Millisecond
		num := (s.Position(t0+dt) - s.Position(t0)) / dt.Seconds()
		if got := s.Velocity(t0); math.Abs(got-num) > 0.05*got {
			t.Errorf("v=%v: скорость %v, производная %v", v, got, num)
		}
	}
}

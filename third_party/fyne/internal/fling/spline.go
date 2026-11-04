// mangareader: пакет добавлен в копию Fyne (third_party/fyne/PATCHES.md).

// Package fling — инерция прокрутки после отрыва пальца: физика Android
// (SplineOverScroller из AOSP) и бросок, который шаг за шагом сдвигает
// перетаскиваемый виджет по кадрам.
package fling

import (
	"math"
	"time"
)

// Константы AOSP (OverScroller, ViewConfiguration).
const (
	inflexion    = 0.35
	startTension = 0.5
	endTension   = 1.0
	p1           = startTension * inflexion
	p2           = 1.0 - endTension*(1.0-inflexion)
	nbSamples    = 100

	scrollFriction = 0.015 // ViewConfiguration.getScrollFriction
	// density — 160 точек на дюйм на единицу Fyne (dp), как в Android
	// при плотности 1.
	ppi           = 160.0
	physicalCoeff = 9.80665 * 39.37 * ppi * 0.84
)

var (
	decelerationRate = math.Log(0.78) / math.Log(0.9)
	decelMinusOne    = decelerationRate - 1.0

	// splinePosition — доля пройденного пути по доле времени (i/nbSamples).
	splinePosition [nbSamples + 1]float64
)

func init() {
	// Таблица строится тем же способом, что в AOSP: бисекция по кривой
	// Безье с точками (0, P1, P2, 1).
	xMin := 0.0
	for i := 0; i < nbSamples; i++ {
		alpha := float64(i) / nbSamples
		xMax := 1.0
		var x, coef float64
		for {
			x = xMin + (xMax-xMin)/2.0
			coef = 3.0 * x * (1.0 - x)
			tx := coef*((1.0-x)*p1+x*p2) + x*x*x
			if math.Abs(tx-alpha) < 1e-5 {
				break
			}
			if tx > alpha {
				xMax = x
			} else {
				xMin = x
			}
		}
		splinePosition[i] = coef*((1.0-x)*startTension+x) + x*x*x
	}
	splinePosition[nbSamples] = 1.0
}

// Spline — бросок с начальной скоростью V0 (единиц Fyne в секунду, по
// модулю) по формуле Android.
type Spline struct {
	V0       float64       // начальная скорость, > 0
	Distance float64       // полный путь
	Duration time.Duration // длительность
}

func splineDeceleration(v float64) float64 {
	return math.Log(inflexion * math.Abs(v) / (scrollFriction * physicalCoeff))
}

// NewSpline считает путь и длительность броска со скоростью v0 (по модулю).
func NewSpline(v0 float64) Spline {
	v0 = math.Abs(v0)
	if v0 == 0 {
		return Spline{}
	}
	l := splineDeceleration(v0)
	ms := 1000.0 * math.Exp(l/decelMinusOne)
	dist := scrollFriction * physicalCoeff * math.Exp(decelerationRate/decelMinusOne*l)
	return Spline{V0: v0, Distance: dist, Duration: time.Duration(ms * float64(time.Millisecond))}
}

// coefs — доля пути и доля скорости (производная по доле времени) в момент t.
func (s Spline) coefs(t time.Duration) (distance, velocity float64) {
	if s.Duration <= 0 || t >= s.Duration {
		return 1, 0
	}
	if t <= 0 {
		return 0, splinePosition[1] * nbSamples
	}
	frac := float64(t) / float64(s.Duration)
	idx := int(nbSamples * frac)
	tInf := float64(idx) / nbSamples
	tSup := float64(idx+1) / nbSamples
	dInf, dSup := splinePosition[idx], splinePosition[idx+1]
	velocity = (dSup - dInf) / (tSup - tInf)
	distance = dInf + (frac-tInf)*velocity
	return distance, velocity
}

// Position — путь, пройденный к моменту t (0…Distance).
func (s Spline) Position(t time.Duration) float64 {
	d, _ := s.coefs(t)
	return d * s.Distance
}

// Velocity — скорость броска в момент t; после конца — 0.
func (s Spline) Velocity(t time.Duration) float64 {
	_, v := s.coefs(t)
	if s.Duration <= 0 {
		return 0
	}
	return v * s.Distance / s.Duration.Seconds()
}

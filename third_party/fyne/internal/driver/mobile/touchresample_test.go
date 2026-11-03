// mangareader: тесты пересчёта касаний на время кадра.

package mobile

import (
	"math"
	"testing"
	"time"
)

var touchT0 = time.Unix(1000, 0)

func ms(v float64) time.Time { return touchT0.Add(time.Duration(v * float64(time.Millisecond))) }

// Палец идёт с постоянной скоростью 1 px/мс, сенсор присылает точки
// неровно: сдвиг за кадр 120 Гц одинаковый, сколько бы точек ни пришло.
func TestTouchResamplerEvenShift(t *testing.T) {
	var r touchResampler
	r.begin(0, 0, 0, ms(0))
	times := []float64{3, 7, 8, 12, 16.5, 19, 24, 25, 29, 33.5, 37, 41, 42, 46, 50.5, 54, 58, 62, 63, 67}
	period := 1000.0 / 120
	next := 0
	var prev float32
	var shifts []float64
	for f := 2; f <= 7; f++ {
		frame := float64(f) * period
		// до кадра приходят точки, отправленные сенсором не позже кадра
		for next < len(times) && times[next] <= frame {
			r.move(0, 0, float32(times[next]), ms(times[next]))
			next++
		}
		if _, y, ok := r.take(0, ms(frame)); ok {
			if f > 2 {
				shifts = append(shifts, float64(y-prev))
			}
			prev = y
		}
	}
	if len(shifts) != 5 {
		t.Fatalf("сдвиги %v", shifts)
	}
	for _, s := range shifts {
		if math.Abs(s-period) > 0.01 {
			t.Fatalf("сдвиги %v, ждали по %.2f", shifts, period)
		}
	}
}

func TestResample(t *testing.T) {
	two := []touchSample{{0, 0, ms(0)}, {0, 10, ms(10)}}
	// интерполяция
	if s, ok := resample(two, ms(4)); !ok || s.y != 4 {
		t.Fatalf("интерполяция: %+v %v", s, ok)
	}
	// экстраполяция не дальше Δ/2
	if s, _ := resample(two, ms(25)); s.y != 15 {
		t.Fatalf("экстраполяция: %+v", s)
	}
	// и не дальше resampleMaxPrediction
	far := []touchSample{{0, 0, ms(0)}, {0, 20, ms(20)}}
	if s, _ := resample(far, ms(40)); s.y != 28 {
		t.Fatalf("предел: %+v", s)
	}
	// точки слишком далеко друг от друга — без экстраполяции
	slow := []touchSample{{0, 0, ms(0)}, {0, 30, ms(30)}}
	if s, _ := resample(slow, ms(40)); s.y != 30 {
		t.Fatalf("Δ > resampleMaxDelta: %+v", s)
	}
	// одна точка
	if s, ok := resample(two[:1], ms(5)); !ok || s.y != 0 {
		t.Fatalf("одна точка: %+v %v", s, ok)
	}
	// все точки новее
	if _, ok := resample(two, ms(-1)); ok {
		t.Fatal("точки из будущего")
	}
}

func TestTouchResamplerBeginEnd(t *testing.T) {
	var r touchResampler
	// нажатие без движения: перед TypeEnd применять нечего
	r.begin(1, 5, 5, ms(0))
	if _, _, ok := r.end(1); ok {
		t.Fatal("нажатие без движения")
	}
	// движение не начатой последовательности — сразу
	if r.move(2, 0, 0, ms(0)) {
		t.Fatal("без begin")
	}
	// конец касания: последняя точка, ещё не применённая
	r.begin(3, 0, 0, ms(0))
	r.move(3, 0, 4, ms(4))
	r.move(3, 0, 8, ms(8))
	if _, y, ok := r.take(3, ms(11)); !ok || y != 6 {
		t.Fatalf("кадр: %v %v", y, ok)
	}
	r.move(3, 0, 12, ms(12))
	if _, y, ok := r.end(3); !ok || y != 12 {
		t.Fatalf("конец: %v %v", y, ok)
	}
	if len(r.sequences()) != 0 {
		t.Fatal("последовательность не снята")
	}
}

// Экстраполяция ушла дальше, чем палец: в конце касания назад не
// возвращаемся (инерция Fyne развернулась бы).
func TestTouchResamplerNoStepBack(t *testing.T) {
	var r touchResampler
	r.begin(0, 0, 0, ms(0))
	r.move(0, 0, 10, ms(10))
	r.move(0, 0, 20, ms(20))
	if _, y, ok := r.take(0, ms(30)); !ok || y != 25 {
		t.Fatalf("экстраполяция: %v %v", y, ok)
	}
	if _, _, ok := r.end(0); ok {
		t.Fatal("шаг назад в конце касания")
	}
}

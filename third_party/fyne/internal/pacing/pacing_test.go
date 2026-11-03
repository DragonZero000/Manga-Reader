// mangareader: тесты пакета, добавленного в копию Fyne.

package pacing

import (
	"testing"
	"time"
)

func TestFrameTime(t *testing.T) {
	at := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	BeginFrame(at)
	if got := FrameTime(); !got.Equal(at) {
		t.Fatalf("в кадре: %v", got)
	}
	EndFrame()
	if got := FrameTime(); time.Since(got) > time.Second {
		t.Fatalf("вне кадра: %v", got)
	}
}

func TestPeriod(t *testing.T) {
	SetPeriod(0)
	if Period() != time.Second/60 || RefreshRate() < 59.9 || RefreshRate() > 60.1 {
		t.Fatalf("по умолчанию: %v %v", Period(), RefreshRate())
	}
	SetPeriod(time.Second / 120)
	if r := RefreshRate(); r < 119.9 || r > 120.1 {
		t.Fatalf("120 Гц: %v", r)
	}
	SetPeriod(0)
}

func TestReport(t *testing.T) {
	var got []Frame
	SetObserver(func(f Frame) { got = append(got, f) })
	if !Observed() {
		t.Fatal("наблюдатель не задан")
	}
	Report(Frame{Period: time.Millisecond})
	SetObserver(nil)
	Report(Frame{})
	if len(got) != 1 || got[0].Period != time.Millisecond || Observed() {
		t.Fatalf("%v", got)
	}
}

func TestNextVsync(t *testing.T) {
	v := time.Unix(100, 0)
	p := 10 * time.Millisecond
	ms := time.Millisecond
	for _, c := range []struct{ t, want time.Duration }{
		{0, 0}, {1 * ms, 10 * ms}, {10 * ms, 10 * ms}, {25 * ms, 30 * ms},
		{-5 * ms, 0}, {-10 * ms, -10 * ms}, {-15 * ms, -10 * ms},
	} {
		if got := NextVsync(v.Add(c.t), v, p); !got.Equal(v.Add(c.want)) {
			t.Errorf("%v: %v, ждали %v", c.t, got.Sub(v), c.want)
		}
	}
	at := v.Add(3 * ms)
	if got := NextVsync(at, time.Time{}, p); !got.Equal(at) {
		t.Errorf("без сетки: %v", got)
	}
}

func TestRateHint(t *testing.T) {
	SetRateHint(120)
	if RateHint() != 120 {
		t.Fatal(RateHint())
	}
	SetRateHint(-1)
	if RateHint() != 0 {
		t.Fatal(RateHint())
	}
}

func TestTakeSample(t *testing.T) {
	var f Frame
	TakeSample(&f)
	if f.HasSample {
		t.Fatal("без функции значения нет")
	}
	y := 12.5
	SetSampler(func() float64 { return y })
	defer SetSampler(nil)
	TakeSample(&f)
	y = 99 // значение берётся в момент кадра, не позже
	if !f.HasSample || f.Sample != 12.5 {
		t.Fatalf("%+v", f)
	}
}

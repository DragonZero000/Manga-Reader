package ui

import (
	"math"
	"runtime"
	"runtime/metrics"
	"strings"
	"testing"
	"time"
)

func TestSummarizeFrames(t *testing.T) {
	if st := summarizeFrames("grid", nil); st.N != 0 || st.Label != "grid" {
		t.Fatalf("пустой замер: %+v", st)
	}
	ms := time.Millisecond
	var in []time.Duration
	for i := 1; i <= 100; i++ {
		in = append(in, time.Duration(i)*ms) // 1..100 мс, в обратном порядке не важно
	}
	in[0], in[99] = in[99], in[0]
	st := summarizeFrames("strip", in)
	if st.N != 100 || st.P50 != 50*ms || st.P95 != 95*ms || st.Max != 100*ms {
		t.Fatalf("сводка: %+v", st)
	}
	if st.Slow25 != 75 { // 26..100 мс
		t.Fatalf("дольше 25 мс: %d", st.Slow25)
	}
	if st.Slow != 25 { // дольше 1,5 × 50 = 75 мс: 76..100
		t.Fatalf("рывков: %d", st.Slow)
	}
	if in[0] != 100*ms {
		t.Fatal("входной срез не должен меняться")
	}
	if got := summarizeFrames("", []time.Duration{16 * ms}); got.P50 != 16*ms || got.P95 != 16*ms || got.Slow != 0 {
		t.Fatalf("один интервал: %+v", got)
	}
}

// Порог рывка зависит от частоты экрана: на 240 Гц интервал 8 мс — рывок,
// хотя он короче 25 мс.
func TestSummarizeFramesHighRefresh(t *testing.T) {
	in := make([]time.Duration, 100)
	for i := range in {
		in[i] = 4166 * time.Microsecond
	}
	in[10], in[20] = 8*time.Millisecond, 30*time.Millisecond
	st := summarizeFrames("grid", in)
	if st.Slow != 2 || st.Slow25 != 1 {
		t.Fatalf("рывков %d, дольше 25 мс %d", st.Slow, st.Slow25)
	}
}

func TestGCPauses(t *testing.T) {
	inf := math.Inf(1)
	buckets := []float64{math.Inf(-1), 0.0001, 0.001, 0.002, inf}
	before := &metrics.Float64Histogram{Counts: []uint64{0, 3, 1, 0}, Buckets: buckets}
	after := &metrics.Float64Histogram{Counts: []uint64{0, 5, 1, 1}, Buckets: buckets}
	st := gcPauses(before, after)
	// 2 паузы в [0,1; 1) мс (середина 0,55 мс) и одна в [2 мс; ∞) — 2 мс
	want := time.Duration(2*0.00055*float64(time.Second)) + 2*time.Millisecond
	if st.N != 3 || absDur(st.Total-want) > time.Microsecond || st.Max != 2*time.Millisecond {
		t.Fatalf("паузы: %+v, ждали всего %v", st, want)
	}
	if got := gcPauses(nil, after); got.N != 0 {
		t.Fatalf("без снимка: %+v", got)
	}
	if readGCPauses() == nil {
		t.Fatalf("нет метрики %s", gcPauseMetric)
	}
	runtime.GC() // живая куча известна после первой сборки
	if readHeapLive() == 0 {
		t.Fatalf("нет метрики %s", heapLiveMetric)
	}
}

func TestFrameStatsString(t *testing.T) {
	st := frameStats{Label: "grid", N: 3, P50: 16700 * time.Microsecond, Slow: 1, Slow25: 1,
		GC: gcStats{N: 2, Total: 1500 * time.Microsecond, Max: time.Millisecond}, HeapLive: 150 << 20}
	s := st.String()
	for _, want := range []string{"frameprobe[grid]:", "n=3", "p50=16.7ms", "slow=1", ">25ms=1", "gc=1.5ms/2", "gcmax=1.0ms", "heap=150MB"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q: нет %q", s, want)
		}
	}
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func TestFormatReport(t *testing.T) {
	at := time.Date(2026, 11, 2, 20, 30, 5, 0, time.UTC)
	h := reportHeader{Kind: "tests", Version: "0.1.3-dev", Platform: "android/arm64", Time: at,
		Width: 412, Height: 915, Scale: 3.5, Max60: "on", Grid: "3 columns", Galleries: 199}
	got := formatReport(h, []string{"frameprobe[grid]: n=1", "frameprobe[strip]: n=2"})
	want := "MangaReader 0.1.3-dev performance report (tests)\n" +
		"date: 2026-11-02 20:30:05\n" +
		"platform: android/arm64\n" +
		"screen: 412x915, scale 3.50\n" +
		"limit 60 Hz: on\n" +
		"grid: 3 columns, galleries: 199\n\n" +
		"frameprobe[grid]: n=1\nframeprobe[strip]: n=2\n"
	if got != want {
		t.Fatalf("отчёт:\n%s\nwant:\n%s", got, want)
	}
	h.Max60 = ""
	if strings.Contains(formatReport(h, nil), "60 Hz") {
		t.Fatal("строка 60 Гц без настройки")
	}
	if n := reportName(at); n != "perf-20261102-203005.txt" {
		t.Fatalf("имя %q", n)
	}
}

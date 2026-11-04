package ui

import (
	"math"
	"runtime"
	"runtime/metrics"
	"strings"
	"testing"
	"time"
)

func TestSummarizeIntervals(t *testing.T) {
	if st := summarizeIntervals("grid", nil); st.N != 0 || st.Label != "grid" {
		t.Fatalf("пустой замер: %+v", st)
	}
	ms := time.Millisecond
	var in []time.Duration
	for i := 1; i <= 100; i++ {
		in = append(in, time.Duration(i)*ms) // 1..100 мс, в обратном порядке не важно
	}
	in[0], in[99] = in[99], in[0]
	st := summarizeIntervals("strip", in)
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
	if got := summarizeIntervals("", []time.Duration{16 * ms}); got.P50 != 16*ms || got.P95 != 16*ms || got.Slow != 0 {
		t.Fatalf("один интервал: %+v", got)
	}
}

// Порог рывка зависит от частоты экрана: на 240 Гц интервал 8 мс — рывок,
// хотя он короче 25 мс.
func TestSummarizeIntervalsHighRefresh(t *testing.T) {
	in := make([]time.Duration, 100)
	for i := range in {
		in[i] = 4166 * time.Microsecond
	}
	in[10], in[20] = 8*time.Millisecond, 30*time.Millisecond
	st := summarizeIntervals("grid", in)
	if st.Slow != 2 || st.Slow25 != 1 {
		t.Fatalf("рывков %d, дольше 25 мс %d", st.Slow, st.Slow25)
	}
}

// shownFrames — кадры, показанные через ks vsync друг после друга (период
// period); сдвиг прокрутки за кадр — shift × k.
func shownFrames(period time.Duration, shift float32, ks ...int) []frameRec {
	at := time.Unix(1000, 0)
	recs := []frameRec{{Present: at, Period: period, HasOffset: true}}
	var y float32
	for _, k := range ks {
		at = at.Add(time.Duration(k) * period)
		y += shift * float32(k)
		recs = append(recs, frameRec{Present: at, Period: period, Offset: y, HasOffset: true})
	}
	return recs
}

func TestSummarizeFramesPacing(t *testing.T) {
	period := time.Second / 120
	ks := make([]int, 100)
	for i := range ks {
		ks[i] = 1
	}
	st := summarizeFrames("grid", shownFrames(period, 12.5, ks...))
	if st.N != 100 || st.OneV != 1 || st.Miss != 0 || st.DY != 0 {
		t.Fatalf("ровно по vsync: %+v", st)
	}
	// кадр держится то 1, то 3 vsync
	for i := range ks {
		ks[i] = 1 + 2*(i%2)
	}
	st = summarizeFrames("grid", shownFrames(period, 12.5, ks...))
	if st.OneV != 0.5 || math.Abs(st.Miss-0.5) > 1e-9 { // Σ(k−1)=100, Σk=200
		t.Fatalf("1 и 3 vsync: 1v=%v miss=%v", st.OneV, st.Miss)
	}
	if st.DY != 0 { // сдвиг пропорционален времени: пропуск vsync — не неровность
		t.Fatalf("dy=%v", st.DY)
	}
}

func TestSummarizeFramesUnevenness(t *testing.T) {
	period := time.Second / 120
	at := time.Unix(1000, 0)
	frame := func(y float32) frameRec {
		at = at.Add(period)
		return frameRec{Present: at, Period: period, Offset: y, HasOffset: true}
	}
	// сдвиги 10 и 15 через кадр: |15 − 10| / 12,5 = 0,4
	var recs []frameRec
	var y float32
	for i := range 101 {
		recs = append(recs, frame(y))
		y += 10 + 5*float32(i%2)
	}
	if st := summarizeFrames("manual", recs); math.Abs(st.DY-0.4) > 0.01 {
		t.Fatalf("скачки: dy=%v", st.DY)
	}
	// плавный разгон и торможение — почти 0
	recs, y = recs[:0], 0
	for i := range 101 {
		recs = append(recs, frame(y))
		y += float32(50 - math.Abs(float64(i-50)))
	}
	if st := summarizeFrames("manual", recs); st.DY > 0.02 {
		t.Fatalf("разгон: dy=%v", st.DY)
	}
	// смена направления: тройки через разворот не в счёт
	recs = []frameRec{frame(0), frame(10), frame(20), frame(30), frame(20), frame(10), frame(0)}
	if st := summarizeFrames("manual", recs); st.DY != 0 {
		t.Fatalf("разворот: dy=%v", st.DY)
	}
}

// Промежутки длиннее frameIdleVsyncs — простой: в 1v, miss и dy не входят,
// но остаются в интервалах (max).
func TestSummarizeFramesIdle(t *testing.T) {
	period := time.Second / 60
	st := summarizeFrames("tabs", shownFrames(period, 10, 1, 1, 20, 1, 2))
	if st.N != 5 || st.Max != 20*period {
		t.Fatalf("интервалы: %+v", st)
	}
	if st.OneV != 0.75 || math.Abs(st.Miss-0.2) > 1e-9 { // k: 1,1,1,2 → Σ(k−1)=1, Σk=5
		t.Fatalf("1v=%v miss=%v", st.OneV, st.Miss)
	}
}

func TestSummarizeFramesNoOffset(t *testing.T) {
	recs := shownFrames(time.Second/60, 0, 1, 1, 1)
	for i := range recs {
		recs[i].HasOffset = false
	}
	if st := summarizeFrames("paged", recs); st.DY != -1 || st.OneV != 1 {
		t.Fatalf("%+v", st)
	}
	if st := summarizeFrames("x", nil); st.OneV != -1 || st.Miss != -1 || st.DY != -1 || st.N != 0 {
		t.Fatalf("пусто: %+v", st)
	}
	// стоящая прокрутка (нулевые сдвиги) в dy не входит
	if st := summarizeFrames("x", shownFrames(time.Second/60, 0, 1, 1, 1)); st.DY != -1 {
		t.Fatalf("без движения: dy=%v", st.DY)
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
		OneV: 0.9734, Miss: 0.012, DY: 0.0456,
		GC: gcStats{N: 2, Total: 1500 * time.Microsecond, Max: time.Millisecond}, HeapLive: 150 << 20}
	s := st.String()
	for _, want := range []string{"frameprobe[grid]:", "n=3", "p50=16.7ms", "slow=1", ">25ms=1",
		"1v=97.3%", "miss=1.2%", "dy=0.046", "gc=1.5ms/2", "gcmax=1.0ms", "heap=150MB"} {
		if !strings.Contains(s, want) {
			t.Fatalf("%q: нет %q", s, want)
		}
	}
	st.OneV, st.Miss, st.DY = -1, -1, -1
	if s := st.String(); !strings.Contains(s, "1v=- miss=- dy=-") {
		t.Fatalf("без данных: %q", s)
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
		Width: 412, Height: 915, Scale: 3.5, Refresh: 120.00001, Pacing: "choreographer", Present: "egl",
		Max60: "on", Grid: "3 columns", Galleries: 199}
	got := formatReport(h, []string{"frameprobe[grid]: n=1", "frameprobe[strip]: n=2"})
	want := "MangaReader 0.1.3-dev performance report (tests)\n" +
		"date: 2026-11-02 20:30:05\n" +
		"platform: android/arm64\n" +
		"screen: 412x915, scale 3.50\n" +
		"refresh: 120 Hz\n" +
		"pacing: choreographer, present: egl\n" +
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

// dyt и dyf считаются по тройкам, у которых средний кадр с пальцем или с
// инерцией: неровность пальца не попадает в dyf и наоборот.
func TestSummarizeFramesMotion(t *testing.T) {
	period := time.Second / 120
	at := time.Unix(1000, 0)
	var recs []frameRec
	var y float32
	add := func(shift float32, motion string) {
		recs = append(recs, frameRec{Present: at, Period: period, Offset: y, HasOffset: true, Motion: motion})
		at = at.Add(period)
		y += shift
	}
	for i := range 60 { // палец: сдвиг скачет 10/15
		add(10+5*float32(i%2), "touch")
	}
	for range 60 { // инерция: ровно
		add(12, "fling")
	}
	st := summarizeFrames("manual", recs)
	if st.DYT < 0.3 || st.DYT > 0.5 {
		t.Errorf("dyt=%v, ждали около 0,4", st.DYT)
	}
	if st.DYF > 0.02 { // одна тройка на стыке с пальцем
		t.Errorf("dyf=%v, ждали около 0", st.DYF)
	}
	if st.DY <= st.DYF {
		t.Errorf("dy=%v должен включать неровность пальца", st.DY)
	}
	if st := summarizeFrames("x", shownFrames(period, 12.5, 1, 1, 1, 1)); st.DYT != -1 || st.DYF != -1 {
		t.Errorf("без вида движения: dyt=%v dyf=%v", st.DYT, st.DYF)
	}
	if s := (frameStats{DY: 0.1, DYT: 0.25, DYF: -1}).String(); !strings.Contains(s, "dy=0.100 dyt=0.250 dyf=-") {
		t.Errorf("строка: %s", s)
	}
}

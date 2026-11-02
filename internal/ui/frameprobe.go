package ui

import (
	"fmt"
	"math"
	"runtime/metrics"
	"slices"
	"strings"
	"time"
)

// frameSlow25 — прежний фиксированный порог рывка: поле «>25ms» сводки
// оставлено для сравнения с прошлыми замерами.
const frameSlow25 = 25 * time.Millisecond

// frameSlowFactor — интервал дольше frameSlowFactor × медианы считается
// рывком: порог имеет смысл на экране любой частоты.
const frameSlowFactor = 1.5

// gcPauseMetric — гистограмма пауз сборщика мусора (runtime/metrics);
// heapLiveMetric — живая куча после последней сборки.
const (
	gcPauseMetric  = "/sched/pauses/total/gc:seconds"
	heapLiveMetric = "/gc/heap/live:bytes"
)

// frameStats — сводка интервалов между кадрами для журнала замера
// (сборка с тегом frameprobe).
type frameStats struct {
	Label         string // сценарий замера
	N             int
	P50, P95, Max time.Duration
	Slow          int // интервалов дольше frameSlowFactor × P50
	Slow25        int // интервалов дольше frameSlow25
	GC            gcStats
	HeapLive      uint64 // живая куча в конце сводки, байт
}

// gcStats — паузы сборщика мусора за время сводки.
type gcStats struct {
	N          uint64
	Total, Max time.Duration
}

// summarizeFrames считает сводку; intervals не изменяется.
func summarizeFrames(label string, intervals []time.Duration) frameStats {
	st := frameStats{Label: label}
	if len(intervals) == 0 {
		return st
	}
	s := slices.Clone(intervals)
	slices.Sort(s)
	st.N, st.P50, st.P95, st.Max = len(s), percentile(s, 50), percentile(s, 95), s[len(s)-1]
	slow := time.Duration(float64(st.P50) * frameSlowFactor)
	for _, d := range s {
		if d > slow {
			st.Slow++
		}
		if d > frameSlow25 {
			st.Slow25++
		}
	}
	return st
}

// percentile — значение p-го процентиля отсортированного среза (ближайший ранг).
func percentile(sorted []time.Duration, p int) time.Duration {
	i := (len(sorted)*p + 99) / 100 // ceil(n·p/100)
	return sorted[max(0, min(len(sorted)-1, i-1))]
}

// gcPauses — паузы между двумя снимками гистограммы gcPauseMetric. Пауза
// оценивается серединой своего интервала гистограммы (у крайних
// бесконечных интервалов — конечной границей); Max — верхняя граница
// наибольшего интервала с паузами.
func gcPauses(before, after *metrics.Float64Histogram) gcStats {
	var st gcStats
	if before == nil || after == nil || len(before.Counts) != len(after.Counts) {
		return st
	}
	for i, c := range after.Counts {
		d := c - before.Counts[i]
		if d == 0 {
			continue
		}
		lo, hi := after.Buckets[i], after.Buckets[i+1]
		mid := (lo + hi) / 2
		switch {
		case math.IsInf(lo, -1):
			mid, lo = hi, hi
		case math.IsInf(hi, 1):
			mid, hi = lo, lo
		}
		st.N += d
		st.Total += time.Duration(mid * float64(d) * float64(time.Second))
		st.Max = time.Duration(hi * float64(time.Second))
	}
	return st
}

// readGCPauses — снимок гистограммы пауз сборщика мусора (nil — метрики нет).
func readGCPauses() *metrics.Float64Histogram {
	s := []metrics.Sample{{Name: gcPauseMetric}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindFloat64Histogram {
		return nil
	}
	return s[0].Value.Float64Histogram()
}

// readHeapLive — живая куча после последней сборки, байт (0 — метрики нет).
func readHeapLive() uint64 {
	s := []metrics.Sample{{Name: heapLiveMetric}}
	metrics.Read(s)
	if s[0].Value.Kind() != metrics.KindUint64 {
		return 0
	}
	return s[0].Value.Uint64()
}

func (s frameStats) String() string {
	ms := func(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }
	return fmt.Sprintf("frameprobe[%s]: n=%d p50=%s p95=%s max=%s slow=%d >25ms=%d gc=%s/%d gcmax=%s heap=%dMB",
		s.Label, s.N, ms(s.P50), ms(s.P95), ms(s.Max), s.Slow, s.Slow25, ms(s.GC.Total), s.GC.N, ms(s.GC.Max), s.HeapLive>>20)
}

// reportHeader — шапка отчёта тестов производительности dev-сборки.
type reportHeader struct {
	Kind      string // «tests» — сценарии, «manual» — ручной замер
	Version   string
	Platform  string // GOOS/GOARCH
	Time      time.Time
	Width     float32 // размер холста, логических единиц
	Height    float32
	Scale     float32 // физических пикселей на логическую единицу
	Max60     string  // «Ограничить 60 Гц»: on/off; "" — настройки нет
	Grid      string  // плотность сетки: колонки (телефон) или размер карточек (ПК)
	Galleries int
}

// formatReport — текст отчёта: шапка и строки сводок (по одной на сценарий).
func formatReport(h reportHeader, lines []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "MangaReader %s performance report (%s)\n", h.Version, h.Kind)
	fmt.Fprintf(&b, "date: %s\n", h.Time.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "platform: %s\n", h.Platform)
	fmt.Fprintf(&b, "screen: %.0fx%.0f, scale %.2f\n", h.Width, h.Height, h.Scale)
	if h.Max60 != "" {
		fmt.Fprintf(&b, "limit 60 Hz: %s\n", h.Max60)
	}
	fmt.Fprintf(&b, "grid: %s, galleries: %d\n\n", h.Grid, h.Galleries)
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

// reportName — имя файла отчёта по времени окончания замера.
func reportName(t time.Time) string { return "perf-" + t.Format("20060102-150405") + ".txt" }

package ui

import (
	"fmt"
	"slices"
	"time"
)

// frameSlow — интервал между кадрами, который считается рывком.
const frameSlow = 25 * time.Millisecond

// frameStats — сводка интервалов между кадрами для журнала замера
// (сборка с тегом frameprobe).
type frameStats struct {
	N             int
	P50, P95, Max time.Duration
	Slow          int // интервалов дольше frameSlow
}

// summarizeFrames считает сводку; intervals не изменяется.
func summarizeFrames(intervals []time.Duration) frameStats {
	if len(intervals) == 0 {
		return frameStats{}
	}
	s := slices.Clone(intervals)
	slices.Sort(s)
	st := frameStats{N: len(s), P50: percentile(s, 50), P95: percentile(s, 95), Max: s[len(s)-1]}
	for _, d := range s {
		if d > frameSlow {
			st.Slow++
		}
	}
	return st
}

// percentile — значение p-го процентиля отсортированного среза (ближайший ранг).
func percentile(sorted []time.Duration, p int) time.Duration {
	i := (len(sorted)*p + 99) / 100 // ceil(n·p/100)
	return sorted[max(0, min(len(sorted)-1, i-1))]
}

func (s frameStats) String() string {
	ms := func(d time.Duration) string { return fmt.Sprintf("%.1fms", float64(d.Microseconds())/1000) }
	return fmt.Sprintf("frameprobe: n=%d p50=%s p95=%s max=%s >25ms=%d", s.N, ms(s.P50), ms(s.P95), ms(s.Max), s.Slow)
}

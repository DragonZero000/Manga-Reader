package ui

import (
	"testing"
	"time"
)

func TestSummarizeFrames(t *testing.T) {
	if st := summarizeFrames(nil); st.N != 0 {
		t.Fatalf("пустой замер: %+v", st)
	}
	ms := time.Millisecond
	var in []time.Duration
	for i := 1; i <= 100; i++ {
		in = append(in, time.Duration(i)*ms) // 1..100 мс, в обратном порядке не важно
	}
	in[0], in[99] = in[99], in[0]
	st := summarizeFrames(in)
	if st.N != 100 || st.P50 != 50*ms || st.P95 != 95*ms || st.Max != 100*ms {
		t.Fatalf("сводка: %+v", st)
	}
	if st.Slow != 75 { // 26..100 мс
		t.Fatalf("рывков: %d", st.Slow)
	}
	if in[0] != 100*ms {
		t.Fatal("входной срез не должен меняться")
	}
	if got := summarizeFrames([]time.Duration{16 * ms}); got.P50 != 16*ms || got.P95 != 16*ms {
		t.Fatalf("один интервал: %+v", got)
	}
}

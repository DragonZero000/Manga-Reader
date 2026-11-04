package stats

import (
	"sort"

	"mangareader/internal/model"
)

// Поля Сводки в порядке показа; у уникальных тегов — SummaryUnique + тип.
const (
	SummaryWorks       = "works"
	SummaryPages       = "pages"
	SummarySize        = "size"
	SummaryAvgPages    = "avg_pages"
	SummaryMedianPages = "median_pages"
	SummaryAvgSize     = "avg_size"
	SummaryMedianSize  = "median_size"
	SummaryDates       = "dates"
	SummaryUnique      = "unique."
)

// SummaryFields — все поля Сводки в порядке показа (их можно скрыть).
func SummaryFields() []string {
	out := []string{SummaryWorks, SummaryPages, SummarySize, SummaryAvgPages,
		SummaryMedianPages, SummaryAvgSize, SummaryMedianSize, SummaryDates}
	for _, typ := range TypeBlocks {
		out = append(out, SummaryUnique+typ)
	}
	return out
}

// summary считает блок «Сводка»; unique — уникальные теги по типам.
func summary(gs []model.Gallery, t sum, unique map[string]int) *Summary {
	s := &Summary{Works: t.works, Pages: t.pages, Size: t.size, Unique: unique}
	if s.Unique == nil {
		s.Unique = map[string]int{}
	}
	if len(gs) == 0 {
		return s
	}
	pages := make([]int64, len(gs))
	sizes := make([]int64, len(gs))
	for i := range gs {
		pages[i] = int64(len(gs[i].Pages))
		sizes[i] = gs[i].File.Size
		if up := gs[i].Uploaded; !up.IsZero() {
			if !s.HasDates || up.Before(s.Earliest) {
				s.Earliest = up
			}
			if !s.HasDates || up.After(s.Latest) {
				s.Latest = up
			}
			s.HasDates = true
		}
	}
	n := float64(len(gs))
	s.AvgPages, s.AvgSize = float64(t.pages)/n, float64(t.size)/n
	s.MedianPages, s.MedianSize = median(pages), median(sizes)
	return s
}

// median — медиана; при чётном числе значений — среднее двух средних.
// Сортирует переданный срез.
func median(v []int64) float64 {
	sort.Slice(v, func(i, j int) bool { return v[i] < v[j] })
	n := len(v)
	if n%2 == 1 {
		return float64(v[n/2])
	}
	return float64(v[n/2-1]+v[n/2]) / 2
}

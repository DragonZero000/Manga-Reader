package stats

import (
	"math"
	"sort"
	"time"

	"mangareader/internal/model"
)

// mb — мегабайт в байтах (1 МБ = 1024×1024).
const mb = 1 << 20

// MBBytes переводит мегабайты в байты так же, как поиск разбирает «size:5mb»:
// корзины и условия перехода совпадают до байта.
func MBBytes(v float64) int64 { return int64(math.Round(v * mb)) }

// bucketRows делит n значений на корзины по возрастающим границам: корзина
// b — (bounds[b-1], bounds[b]], последняя — больше последней границы.
// idx возвращает номер корзины значения i.
func bucketRows(n int, bounds []float64, idx func(i int) int, kind RowKind) []Row {
	counts := make([]int, len(bounds)+1)
	for i := 0; i < n; i++ {
		counts[idx(i)]++
	}
	rows := make([]Row, len(counts))
	for b := range counts {
		ref := RowRef{Kind: kind}
		if b > 0 {
			ref.Lo = bounds[b-1]
		}
		if b < len(bounds) {
			ref.Hi = bounds[b]
		}
		rows[b] = Row{Ref: ref, Count: counts[b], Share: share(float64(counts[b]), float64(n))}
	}
	return rows
}

// pageRows — корзины по числу страниц.
func pageRows(gs []model.Gallery, bounds []float64) []Row {
	return bucketRows(len(gs), bounds, func(i int) int {
		n := float64(len(gs[i].Pages))
		return sort.Search(len(bounds), func(b int) bool { return n <= bounds[b] })
	}, RowPages)
}

// sizeIndex — номер корзины объёма для размера в байтах.
func sizeIndex(size int64, bounds []float64) int {
	return sort.Search(len(bounds), func(b int) bool { return size <= MBBytes(bounds[b]) })
}

// sizeBlock — корзины объёма и вся выборка по величине by (объём или
// страницы): по убыванию, с asc — по возрастанию; при равенстве — по
// названию (натурально), затем по ключу.
func sizeBlock(gs []model.Gallery, bounds []float64, by Mode, asc bool) *SizeBlock {
	b := &SizeBlock{Rows: bucketRows(len(gs), bounds, func(i int) int {
		return sizeIndex(gs[i].File.Size, bounds)
	}, RowSize), By: by}
	b.Largest = make([]Large, len(gs))
	for i := range gs {
		b.Largest[i] = Large{Key: gs[i].Key, Title: gs[i].Title, Size: gs[i].File.Size, Pages: len(gs[i].Pages)}
	}
	value := func(l *Large) int64 {
		if by == ModePages {
			return int64(l.Pages)
		}
		return l.Size
	}
	sort.Slice(b.Largest, func(i, j int) bool {
		a, c := &b.Largest[i], &b.Largest[j]
		if va, vc := value(a), value(c); va != vc {
			return va > vc != asc
		}
		if a.Title != c.Title {
			return model.NaturalLess(a.Title, c.Title)
		}
		return model.NaturalLess(a.Key.String(), c.Key.String())
	})
	return b
}

// uploadedRows — по строке на каждый год загрузки (UTC) от самого раннего до
// самого позднего, затем «Без даты», если такие есть.
func uploadedRows(gs []model.Gallery) []Row {
	byYear := map[int]int{}
	noDate := 0
	lo, hi := math.MaxInt, math.MinInt
	for i := range gs {
		up := gs[i].Uploaded
		if up.IsZero() {
			noDate++
			continue
		}
		y := up.UTC().Year()
		byYear[y]++
		lo, hi = min(lo, y), max(hi, y)
	}
	var rows []Row
	for y := lo; y <= hi; y++ {
		rows = append(rows, Row{Ref: RowRef{Kind: RowUploaded, Year: y}, Count: byYear[y],
			Share: share(float64(byYear[y]), float64(len(gs)))})
	}
	if noDate > 0 {
		rows = append(rows, Row{Ref: RowRef{Kind: RowNoDate}, Count: noDate,
			Share: share(float64(noDate), float64(len(gs)))})
	}
	return rows
}

// growth считает рост библиотеки по времени изменения файла в поясе loc.
func growth(gs []model.Gallery, loc *time.Location) *Growth {
	byMonth := map[int]int{} // год*12 + (месяц-1)
	lo, hi := math.MaxInt, math.MinInt
	for i := range gs {
		mt := gs[i].File.ModTime
		if mt.IsZero() {
			continue
		}
		t := mt.In(loc)
		k := t.Year()*12 + int(t.Month()) - 1
		byMonth[k]++
		lo, hi = min(lo, k), max(hi, k)
	}
	g := &Growth{}
	if len(byMonth) == 0 {
		return g
	}
	n := float64(len(gs))
	monthRow := func(k int) Row {
		return Row{Ref: RowRef{Kind: RowAddedMonth, Year: k / 12, Month: k%12 + 1},
			Count: byMonth[k], Share: share(float64(byMonth[k]), n)}
	}
	if hi-lo < monthsForYears {
		for k := lo; k <= hi; k++ {
			g.Months = append(g.Months, monthRow(k))
		}
		return g
	}
	g.ByYear = true
	for y := lo / 12; y <= hi/12; y++ {
		gy := GrowthYear{}
		cnt := 0
		for m := 0; m < 12; m++ {
			row := monthRow(y*12 + m)
			cnt += row.Count
			gy.Months = append(gy.Months, row)
		}
		gy.Row = Row{Ref: RowRef{Kind: RowAddedYear, Year: y}, Count: cnt, Share: share(float64(cnt), n)}
		g.Years = append(g.Years, gy)
	}
	return g
}

// quality считает блок «Качество метаданных» без учёта исключений тегов.
func quality(gs []model.Gallery) []Row {
	var noMeta, noArtist, noLang, noTags int
	for i := range gs {
		g := &gs[i]
		if !g.HasMeta {
			noMeta++
		}
		tags := g.EffectiveTags()
		if len(tags) == 0 {
			noTags++
		}
		var artist, lang bool
		for _, t := range tags {
			artist = artist || t.Type == model.TagTypeArtist
			lang = lang || t.Type == model.TagTypeLanguage
		}
		if !artist {
			noArtist++
		}
		if !lang {
			noLang++
		}
	}
	n := float64(len(gs))
	row := func(q string, c int) Row {
		return Row{Ref: RowRef{Kind: RowQuality, Quality: q}, Count: c, Share: share(float64(c), n)}
	}
	return []Row{
		row(QualityNoMeta, noMeta), row(QualityNoArtist, noArtist),
		row(QualityNoLanguage, noLang), row(QualityNoTags, noTags),
	}
}

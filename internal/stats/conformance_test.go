package stats

import (
	"context"
	"fmt"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
)

// testLibrary — библиотека для проверки соответствия строк статистики и
// поиска: теги всех типов, свои и скрытые, граничные размеры и страницы,
// годы загрузки и месяцы изменения на длинной истории. Имена своих и
// скрытых тегов не совпадают с именами тегов других типов (допущение
// спецификации).
func testLibrary() []model.Gallery {
	var gs []model.Gallery
	artists := []string{"a1", "a2", "a3", "a4", "a5"}
	langs := []string{"japanese", "english", "translated"}
	for i := 0; i < 60; i++ {
		pages := 1 + (i*7)%90
		size := int64(1+i*i%40) * mb / 2
		switch i {
		case 0:
			pages, size = 20, 5*mb // ровно на границах
		case 1:
			pages, size = 21, 5*mb+1
		case 2:
			pages, size = 500, 200*mb
		case 3:
			pages, size = 501, 200*mb+1
		}
		gl := g(fmt.Sprintf("g%02d.zip", i), pages, size)
		gl.HasMeta = i%7 != 0
		if i%6 != 0 {
			gl.AddTag(model.NewTag("artist", artists[i%len(artists)]))
		}
		if i%5 != 0 {
			gl.AddTag(model.NewTag("language", langs[i%len(langs)]))
		}
		gl.AddTag(model.NewTag("tag", fmt.Sprintf("t%d", i%9)))
		if i%4 == 0 {
			gl.AddTag(model.NewTag("group", "g"+fmt.Sprint(i%3)))
			gl.AddTag(model.NewTag("parody", "p"+fmt.Sprint(i%2)))
			gl.AddTag(model.NewTag("character", "ch"+fmt.Sprint(i%5)))
			gl.AddTag(model.NewTag("category", "doujinshi"))
		}
		if i%3 == 0 {
			gl.AddTag(model.NewTag("tag", "spam"))
			gl.Hidden = append(gl.Hidden, model.NewTag("tag", "spam"))
		}
		if i%8 == 0 {
			gl.Custom = append(gl.Custom, model.NewTag("tag", "fav"))
		}
		if i%10 == 0 {
			gl.Custom = append(gl.Custom, model.NewTag("character", "alice"))
		}
		if i%9 != 0 {
			gl.Uploaded = time.Date(2018+i%7, time.Month(1+i%12), 1+i%28, 12, 0, 0, 0, time.UTC)
		}
		gl.File.ModTime = time.Date(2021+i%6, time.Month(1+(i*5)%12), 1+i%28, 12, 0, 0, 0, time.Local)
		gs = append(gs, gl)
	}
	return gs
}

// allRows собирает ссылки и значения всех строк результата.
func allRows(r Result) map[RowRef]int {
	out := map[RowRef]int{}
	add := func(rows []Row) {
		for _, row := range rows {
			out[row.Ref] = row.Count
		}
	}
	addTop := func(top *Top) {
		for _, row := range top.Rows {
			out[row.Ref] = row.Works
		}
	}
	for _, top := range r.Tops {
		addTop(top)
	}
	add(r.Pages)
	add(r.Size.Rows)
	add(r.Uploaded)
	for _, row := range r.Growth.Months {
		add([]Row{row})
	}
	for _, y := range r.Growth.Years {
		add([]Row{y.Row})
		add(y.Months)
	}
	addTop(&r.Yours.Custom)
	addTop(&r.Yours.Hidden)
	add(r.Quality)
	return out
}

// CheckConformance проверяет: для каждой строки статистики запрос перехода
// находит ровно столько произведений, сколько в строке. idx уже заполнен
// testLibrary().
func CheckConformance(t *testing.T, idx search.Index) {
	t.Helper()
	all := testLibrary()
	for _, tc := range []struct {
		filter string
		opt    Options
	}{
		{"", Options{}},
		{"", Options{PageBounds: []float64{20, 21, 500}, SizeBounds: []float64{0.5, 2.5, 5, 200}}},
		{"language:japanese", Options{}},
		{"-tag:t3 pages:>10", Options{}},
	} {
		sample := all
		if tc.filter != "" {
			q, err := search.Parse(tc.filter)
			if err != nil {
				t.Fatal(err)
			}
			keys, _, err := idx.Search(context.Background(), q)
			if err != nil {
				t.Fatal(err)
			}
			want := map[model.Key]bool{}
			for _, k := range keys {
				want[k] = true
			}
			sample = nil
			for _, gl := range all {
				if want[gl.Key] {
					sample = append(sample, gl)
				}
			}
		}
		r := compute(t, sample, tc.opt)
		rows := allRows(r)
		if len(rows) < 50 {
			t.Fatalf("фильтр %q: слишком мало строк (%d), проверка бессмысленна", tc.filter, len(rows))
		}
		for ref, count := range rows {
			text := Query(ref, tc.filter)
			if text == "" {
				if ref.Kind != RowNoDate {
					t.Errorf("фильтр %q, строка %+v: пустой запрос", tc.filter, ref)
				}
				continue
			}
			q, err := search.Parse(text)
			if err != nil {
				t.Errorf("фильтр %q, строка %+v: запрос %q: %v", tc.filter, ref, text, err)
				continue
			}
			keys, _, err := idx.Search(context.Background(), q)
			if err != nil {
				t.Errorf("запрос %q: %v", text, err)
				continue
			}
			if len(keys) != count {
				t.Errorf("запрос %q: найдено %d, в строке %d", text, len(keys), count)
			}
		}
	}
}

func TestConformanceMemIndex(t *testing.T) {
	idx := search.NewMemIndex()
	for _, gl := range testLibrary() {
		if err := idx.Upsert(context.Background(), gl); err != nil {
			t.Fatal(err)
		}
	}
	CheckConformance(t, idx)
}

func TestQueryRows(t *testing.T) {
	for _, tc := range []struct {
		ref    RowRef
		filter string
		want   string
	}{
		{RowRef{Kind: RowTag, Tag: model.NewTag("artist", "x")}, "language:japanese", `language:japanese artist:"x"`},
		{RowRef{Kind: RowTag, Tag: model.NewTag("tag", `a"b`)}, "  ", `tag:"ab"`},
		{RowRef{Kind: RowCustom, Tag: model.NewTag("tag", "fav")}, "", `custom-tag:"fav" tag:"fav"`},
		{RowRef{Kind: RowHidden, Tag: model.NewTag("tag", "spam")}, "", `hidden-tag:"spam"`},
		{RowRef{Kind: RowPages, Lo: 0, Hi: 20}, "", "pages:1..20"},
		{RowRef{Kind: RowPages, Lo: 20, Hi: 50}, "", "pages:21..50"},
		{RowRef{Kind: RowPages, Lo: 500}, "", "pages:>500"},
		{RowRef{Kind: RowSize, Hi: 5}, "", "size:<=5mb"},
		{RowRef{Kind: RowSize, Lo: 5, Hi: 10}, "", "size:>5mb size:<=10mb"},
		{RowRef{Kind: RowSize, Lo: 2.5, Hi: 10}, "", "size:>2.5mb size:<=10mb"},
		{RowRef{Kind: RowSize, Lo: 200}, "", "size:>200mb"},
		{RowRef{Kind: RowUploaded, Year: 2024}, "", "uploaded:2024"},
		{RowRef{Kind: RowAddedYear, Year: 2024}, "", "added:2024"},
		{RowRef{Kind: RowAddedMonth, Year: 2024, Month: 3}, "", "added:2024-03"},
		{RowRef{Kind: RowQuality, Quality: QualityNoArtist}, "", "missing:artist"},
		{RowRef{Kind: RowNoDate}, "", ""},
	} {
		if got := Query(tc.ref, tc.filter); got != tc.want {
			t.Errorf("Query(%+v, %q) = %q, want %q", tc.ref, tc.filter, got, tc.want)
		}
	}
}

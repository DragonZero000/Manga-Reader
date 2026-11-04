package stats

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	"mangareader/internal/model"
)

// g строит галерею: pages страниц, size байт, теги "тип:имя".
func g(id string, pages int, size int64, tags ...string) model.Gallery {
	gl := model.Gallery{
		Key:     model.LocalKey(id),
		Title:   id,
		HasMeta: true,
		File:    model.FileInfo{Size: size, ModTime: time.Date(2026, 1, 15, 12, 0, 0, 0, time.Local)},
	}
	for i := 0; i < pages; i++ {
		gl.Pages = append(gl.Pages, model.Page{Name: fmt.Sprintf("%d.jpg", i+1)})
	}
	for _, t := range tags {
		gl.AddTag(model.ParseTag(t))
	}
	return gl
}

func compute(t *testing.T, gs []model.Gallery, opt Options) Result {
	t.Helper()
	r, err := Compute(context.Background(), gs, opt)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func topNames(top *Top) []string {
	var out []string
	for _, r := range top.Rows {
		out = append(out, r.Tag.String())
	}
	return out
}

func TestAllBlocksOrder(t *testing.T) {
	ids := AllBlocks()
	if len(ids) != 14 || ids[0] != BlockSummary || ids[13] != BlockQuality {
		t.Fatalf("блоки: %v", ids)
	}
	if typ, ok := TypeOf("language"); !ok || typ != "language" {
		t.Errorf("TypeOf(language) = %q, %v", typ, ok)
	}
	if _, ok := TypeOf(BlockPages); ok {
		t.Error("pages — не топ типа")
	}
}

func TestSummaryAveragesAndMedian(t *testing.T) {
	gs := []model.Gallery{g("a", 10, 100), g("b", 20, 200), g("c", 90, 900)}
	s := compute(t, gs, Options{}).Summary
	if s.Works != 3 || s.Pages != 120 || s.AvgPages != 40 || s.MedianPages != 20 {
		t.Errorf("страницы: %+v", s)
	}
	if s.Size != 1200 || s.AvgSize != 400 || s.MedianSize != 200 {
		t.Errorf("объём: %+v", s)
	}
	// чётное число: среднее двух средних
	if s := compute(t, append(gs, g("d", 30, 300)), Options{}).Summary; s.MedianPages != 25 {
		t.Errorf("медиана чётного числа: %v", s.MedianPages)
	}
	if s.HasDates {
		t.Error("дат загрузки нет, но HasDates истинен")
	}
}

func TestSummaryDates(t *testing.T) {
	a, b := g("a", 1, 1), g("b", 1, 1)
	a.Uploaded = time.Date(2021, 3, 1, 0, 0, 0, 0, time.UTC)
	b.Uploaded = time.Date(2024, 5, 9, 0, 0, 0, 0, time.UTC)
	s := compute(t, []model.Gallery{b, a, g("c", 1, 1)}, Options{}).Summary
	if !s.HasDates || !s.Earliest.Equal(a.Uploaded) || !s.Latest.Equal(b.Uploaded) {
		t.Errorf("диапазон дат: %+v", s)
	}
}

func TestSummaryUniqueTagsWithExclusion(t *testing.T) {
	var gs []model.Gallery
	for _, a := range []string{"a", "b", "c", "d", "x"} {
		gs = append(gs, g("w"+a, 1, 1, "artist:"+a, "language:japanese"))
	}
	if n := compute(t, gs, Options{}).Summary.Unique["artist"]; n != 5 {
		t.Fatalf("авторов %d", n)
	}
	opt := Options{Excluded: map[BlockID]map[string]bool{"artist": {"artist:x": true}}}
	r := compute(t, gs, opt)
	if n := r.Summary.Unique["artist"]; n != 4 {
		t.Errorf("с исключением авторов %d", n)
	}
	if r.Summary.Works != 5 || r.Summary.Unique["language"] != 1 {
		t.Errorf("исключение затронуло остальное: %+v", r.Summary)
	}
	if got := topNames(r.Tops["artist"]); len(got) != 4 {
		t.Errorf("топ авторов: %v", got)
	}
}

func TestEffectiveTagsOverlay(t *testing.T) {
	own := g("own", 1, 1)
	own.Custom = []model.Tag{model.NewTag("artist", "alice")}
	hid := g("hid", 1, 1, "language:english")
	hid.Hidden = []model.Tag{model.NewTag("language", "english")}
	r := compute(t, []model.Gallery{own, hid}, Options{})
	if got := topNames(r.Tops["artist"]); !reflect.DeepEqual(got, []string{"artist:alice"}) {
		t.Errorf("свой тег: %v", got)
	}
	if got := topNames(r.Tops["language"]); len(got) != 0 {
		t.Errorf("скрытый тег в топе: %v", got)
	}
}

func TestTopOrderAndShare(t *testing.T) {
	gs := []model.Gallery{
		g("1", 10, 10, "artist:b"), g("2", 10, 10, "artist:b"),
		g("3", 10, 10, "artist:a10"), g("4", 10, 10, "artist:a2"), g("5", 10, 10, "artist:c", "artist:c"),
	}
	top := compute(t, gs, Options{}).Tops["artist"]
	if want := []string{"artist:b", "artist:a2", "artist:a10", "artist:c"}; !reflect.DeepEqual(topNames(top), want) {
		t.Errorf("порядок: %v, want %v", topNames(top), want)
	}
	if top.Rows[0].Works != 2 || top.Rows[0].Share != 0.4 {
		t.Errorf("доля: %+v", top.Rows[0])
	}
}

func TestTopShareOverHundred(t *testing.T) {
	gs := []model.Gallery{
		g("1", 1, 1, "language:japanese", "language:translated"),
		g("2", 1, 1, "language:japanese", "language:translated"),
	}
	for _, row := range compute(t, gs, Options{}).Tops["language"].Rows {
		if row.Share != 1 {
			t.Errorf("%s: доля %v", row.Tag, row.Share)
		}
	}
}

func TestTopModes(t *testing.T) {
	gs := []model.Gallery{
		g("1", 5, 10<<20, "artist:a"), g("2", 5, 10<<20, "artist:a"), g("3", 5, 10<<20, "artist:a"),
		g("4", 100, 50<<20, "artist:b"),
	}
	cases := map[Mode][]string{
		ModeWorks: {"artist:a", "artist:b"},
		ModePages: {"artist:b", "artist:a"},
		ModeSize:  {"artist:b", "artist:a"},
	}
	for mode, want := range cases {
		top := compute(t, gs, Options{Modes: map[BlockID]Mode{"artist": mode}}).Tops["artist"]
		if got := topNames(top); !reflect.DeepEqual(got, want) {
			t.Errorf("режим %s: %v, want %v", mode, got, want)
		}
	}
	top := compute(t, gs, Options{Modes: map[BlockID]Mode{"artist": ModeSize}}).Tops["artist"]
	if top.Rows[0].Value != 50<<20 || top.Rows[1].Value != 30<<20 || top.Rows[0].Share != 50.0/80 {
		t.Errorf("значения режима объёма: %+v", top.Rows)
	}
	// неизвестный режим — по умолчанию
	if top := compute(t, gs, Options{Modes: map[BlockID]Mode{"artist": "x"}}).Tops["artist"]; top.Mode != ModeWorks {
		t.Errorf("режим %q", top.Mode)
	}
}

func counts(rows []Row) []int {
	out := make([]int, len(rows))
	for i, r := range rows {
		out[i] = r.Count
	}
	return out
}

func TestPageBuckets(t *testing.T) {
	gs := []model.Gallery{g("a", 8, 1), g("b", 20, 1), g("c", 21, 1), g("d", 600, 1)}
	rows := compute(t, gs, Options{}).Pages
	if want := []int{2, 1, 0, 0, 0, 1}; !reflect.DeepEqual(counts(rows), want) {
		t.Errorf("корзины: %v, want %v", counts(rows), want)
	}
	if rows[0].Ref.Lo != 0 || rows[0].Ref.Hi != 20 || rows[5].Ref.Lo != 500 || rows[5].Ref.Hi != 0 {
		t.Errorf("границы: %+v %+v", rows[0].Ref, rows[5].Ref)
	}
	if rows[0].Share != 0.5 {
		t.Errorf("доля: %v", rows[0].Share)
	}
	rows = compute(t, gs, Options{PageBounds: []float64{10, 600}}).Pages
	if want := []int{1, 3, 0}; !reflect.DeepEqual(counts(rows), want) {
		t.Errorf("свои границы: %v, want %v", counts(rows), want)
	}
}

func TestSizeBuckets(t *testing.T) {
	gs := []model.Gallery{
		g("a", 1, 2*mb+mb*8/10), g("b", 1, 5*mb), g("c", 1, 30*mb+mb*3/10),
	}
	r := compute(t, gs, Options{})
	if want := []int{2, 0, 0, 1, 0, 0, 0}; !reflect.DeepEqual(counts(r.Size.Rows), want) {
		t.Errorf("корзины: %v, want %v", counts(r.Size.Rows), want)
	}
	// 5 МБ ровно — в «до 5», 5,000001 — уже выше
	over := []model.Gallery{g("x", 1, 5*mb+1)}
	if got := compute(t, over, Options{}).Size.Rows; got[0].Count != 0 || got[1].Count != 1 {
		t.Errorf("граница: %v", counts(got))
	}
	r = compute(t, gs, Options{SizeBounds: []float64{1, 2.5, 10}})
	if want := []int{0, 0, 2, 1}; !reflect.DeepEqual(counts(r.Size.Rows), want) {
		t.Errorf("дробные границы: %v, want %v", counts(r.Size.Rows), want)
	}
}

func TestLargestWorks(t *testing.T) {
	var gs []model.Gallery
	for i := 0; i < 15; i++ {
		gs = append(gs, g(fmt.Sprintf("w%02d", i), 1, int64(i+1)*mb))
	}
	got := compute(t, gs, Options{}).Size.Largest
	if len(got) != 15 || got[0].Key != model.LocalKey("w14") || got[0].Size != 15*mb || got[14].Size != 1*mb {
		t.Errorf("самые большие: %+v", got)
	}
	asc := compute(t, gs, Options{Ascending: map[BlockID]bool{BlockLargest: true}}).Size.Largest
	if asc[0].Key != model.LocalKey("w00") || asc[14].Key != model.LocalKey("w14") {
		t.Errorf("по возрастанию: %v … %v", asc[0].Key, asc[14].Key)
	}
}

func TestLargestByPages(t *testing.T) {
	small, long := g("small", 110, 20*mb), g("big", 8, 30*mb)
	tie := g("a-tie", 8, 1*mb)
	r := compute(t, []model.Gallery{long, small, tie}, Options{LargestBy: ModePages})
	got := r.Size.Largest
	if r.Size.By != ModePages || got[0].Key != model.LocalKey("small") || got[0].Pages != 110 {
		t.Fatalf("по страницам: %+v", got)
	}
	// при равенстве страниц — по названию
	if got[1].Title != "a-tie" || got[2].Title != "big" {
		t.Errorf("равные страницы: %v, %v", got[1].Title, got[2].Title)
	}
}

func TestTopAscending(t *testing.T) {
	gs := []model.Gallery{g("1", 1, 1, "artist:a"), g("2", 1, 1, "artist:a"), g("3", 1, 1, "artist:a"), g("4", 1, 1, "artist:b"), g("5", 1, 1, "artist:c")}
	top := compute(t, gs, Options{Ascending: map[BlockID]bool{"artist": true}}).Tops["artist"]
	var names []string
	for _, r := range top.Rows {
		names = append(names, r.Tag.Name)
	}
	if !reflect.DeepEqual(names, []string{"b", "c", "a"}) {
		t.Errorf("по возрастанию: %v", names)
	}
}

func TestUploadedYears(t *testing.T) {
	a, b, c := g("a", 1, 1), g("b", 1, 1), g("c", 1, 1)
	a.Uploaded = time.Date(2020, 6, 1, 0, 0, 0, 0, time.UTC)
	b.Uploaded = time.Date(2022, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := compute(t, []model.Gallery{a, b, c, g("d", 1, 1)}, Options{}).Uploaded
	var years []int
	for _, r := range rows {
		years = append(years, r.Count)
	}
	if want := []int{1, 0, 1, 2}; !reflect.DeepEqual(years, want) {
		t.Fatalf("строки: %v, want %v", years, want)
	}
	if rows[1].Ref.Year != 2021 || rows[3].Ref.Kind != RowNoDate {
		t.Errorf("строки: %+v", rows)
	}
	// последняя секунда года в UTC остаётся в своём году
	last := g("e", 1, 1)
	last.Uploaded = time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC).In(time.FixedZone("x", 5*3600))
	if rows := compute(t, []model.Gallery{last}, Options{}).Uploaded; len(rows) != 1 || rows[0].Ref.Year != 2023 {
		t.Errorf("год в UTC: %+v", rows)
	}
	// ни у кого нет даты — только «Без даты»
	if rows := compute(t, []model.Gallery{g("f", 1, 1)}, Options{}).Uploaded; len(rows) != 1 || rows[0].Ref.Kind != RowNoDate {
		t.Errorf("без дат: %+v", rows)
	}
}

func gm(id string, y int, m time.Month) model.Gallery {
	gl := g(id, 1, 1)
	gl.File.ModTime = time.Date(y, m, 10, 12, 0, 0, 0, time.Local)
	return gl
}

func TestGrowthShortHistory(t *testing.T) {
	gs := []model.Gallery{gm("a", 2026, 1), gm("b", 2026, 1), gm("c", 2026, 11)}
	gr := compute(t, gs, Options{}).Growth
	if gr.ByYear || len(gr.Months) != 11 {
		t.Fatalf("рост: годы=%v, месяцев %d", gr.ByYear, len(gr.Months))
	}
	if gr.Months[0].Count != 2 || gr.Months[5].Count != 0 || gr.Months[10].Ref.Month != 11 {
		t.Errorf("месяцы: %v", counts(gr.Months))
	}
}

func TestGrowthLongHistory(t *testing.T) {
	gs := []model.Gallery{gm("a", 2022, 5), gm("b", 2024, 3), gm("c", 2026, 11)}
	gr := compute(t, gs, Options{}).Growth
	if !gr.ByYear || len(gr.Years) != 5 || gr.Years[0].Row.Ref.Year != 2022 || gr.Years[4].Row.Ref.Year != 2026 {
		t.Fatalf("рост: %+v", gr)
	}
	y := gr.Years[2]
	if y.Row.Count != 1 || len(y.Months) != 12 || y.Months[2].Count != 1 || y.Months[2].Ref.Month != 3 {
		t.Errorf("2024: %+v", y)
	}
	if gr.Years[1].Row.Count != 0 {
		t.Errorf("пропущенный год: %+v", gr.Years[1])
	}
	// 23 месяца между крайними — ещё месяцы, 24 — уже годы
	if gr := compute(t, []model.Gallery{gm("a", 2024, 1), gm("b", 2025, 12)}, Options{}).Growth; gr.ByYear {
		t.Error("23 месяца: ожидались месяцы")
	}
	if gr := compute(t, []model.Gallery{gm("a", 2024, 1), gm("b", 2026, 1)}, Options{}).Growth; !gr.ByYear {
		t.Error("24 месяца: ожидались годы")
	}
}

func TestGrowthUsesLocation(t *testing.T) {
	gl := g("a", 1, 1)
	gl.File.ModTime = time.Date(2026, 3, 31, 23, 30, 0, 0, time.UTC)
	loc := time.FixedZone("plus3", 3*3600)
	if m := compute(t, []model.Gallery{gl}, Options{Loc: loc}).Growth.Months; m[0].Ref.Month != 4 {
		t.Errorf("месяц в поясе +3: %+v", m[0].Ref)
	}
}

func TestYoursTags(t *testing.T) {
	var gs []model.Gallery
	for i := 0; i < 3; i++ {
		x := g(fmt.Sprintf("c%d", i), 1, 1, "tag:orig")
		x.Custom = []model.Tag{model.NewTag("tag", "fav")}
		gs = append(gs, x)
	}
	for i := 0; i < 2; i++ {
		x := g(fmt.Sprintf("h%d", i), 1, 1, "tag:spam", "tag:keep")
		x.Hidden = []model.Tag{model.NewTag("tag", "spam"), model.NewTag("tag", "absent")}
		gs = append(gs, x)
	}
	y := compute(t, gs, Options{}).Yours
	if y.WithCustom != 3 || y.WithHidden != 2 {
		t.Errorf("произведений: свои %d, скрытые %d", y.WithCustom, y.WithHidden)
	}
	if got := topNames(&y.Custom); !reflect.DeepEqual(got, []string{"tag:fav"}) || y.Custom.Rows[0].Works != 3 {
		t.Errorf("свои: %v", got)
	}
	if got := topNames(&y.Hidden); !reflect.DeepEqual(got, []string{"tag:spam"}) || y.Hidden.Rows[0].Works != 2 {
		t.Errorf("скрытые: %v", got)
	}
	// исключение действует только в своём блоке
	opt := Options{Excluded: map[BlockID]map[string]bool{"tag": {"tag:fav": true}}}
	r := compute(t, gs, opt)
	if len(r.Yours.Custom.Rows) != 1 {
		t.Errorf("исключение в топе tag убрало тег из «Ваших»: %v", topNames(&r.Yours.Custom))
	}
	opt = Options{Excluded: map[BlockID]map[string]bool{ExcludeCustom: {"tag:fav": true}}}
	r = compute(t, gs, opt)
	if len(r.Yours.Custom.Rows) != 0 || r.Yours.WithCustom != 3 {
		t.Errorf("исключение в «Ваших»: %+v", r.Yours)
	}
	if got := topNames(r.Tops["tag"]); !contains(got, "tag:fav") {
		t.Errorf("исключение в «Ваших» убрало тег из топа tag: %v", got)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestQuality(t *testing.T) {
	bare := g("bare", 1, 1)
	bare.HasMeta = false
	ownArtist := g("own", 1, 1)
	ownArtist.HasMeta = false
	ownArtist.Custom = []model.Tag{model.NewTag("artist", "alice")}
	full := g("full", 1, 1, "artist:x", "language:japanese")
	opt := Options{Excluded: map[BlockID]map[string]bool{"artist": {"artist:x": true}}}
	got := counts(compute(t, []model.Gallery{bare, ownArtist, full}, opt).Quality)
	// без meta, без автора, без языка, без тегов; исключения не влияют
	if want := []int{2, 1, 2, 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("качество: %v, want %v", got, want)
	}
}

func TestDisabledBlocksAreNotComputed(t *testing.T) {
	gs := []model.Gallery{g("a", 5, 5, "artist:a", "language:japanese")}
	off := map[BlockID]bool{}
	for _, id := range AllBlocks() {
		off[id] = true
	}
	delete(off, "language")
	r := compute(t, gs, Options{Disabled: off})
	if r.Summary != nil || r.Pages != nil || r.Size != nil || r.Uploaded != nil || r.Growth != nil || r.Yours != nil || r.Quality != nil {
		t.Errorf("выключенные блоки посчитаны: %+v", r)
	}
	if len(r.Tops) != 1 || r.Tops["language"] == nil {
		t.Errorf("топы: %v", r.Tops)
	}
	if r.Works != 1 {
		t.Errorf("Works = %d", r.Works)
	}
}

func TestComputeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Compute(ctx, []model.Gallery{g("a", 1, 1)}, Options{}); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

func TestEmptyLibrary(t *testing.T) {
	r := compute(t, nil, Options{})
	if r.Works != 0 || r.Summary.Works != 0 || r.Growth == nil || len(r.Uploaded) != 0 {
		t.Errorf("пустая выборка: %+v", r)
	}
	for _, row := range r.Pages {
		if row.Count != 0 || row.Share != 0 {
			t.Errorf("корзина пустой выборки: %+v", row)
		}
	}
}

func TestParseBounds(t *testing.T) {
	ok := map[string][]float64{
		"20, 50,100":                          {20, 50, 100},
		"1":                                   {1},
		"1, 2.5, 10":                          {1, 2.5, 10},
		" 5 , 10 ":                            {5, 10},
		"0.5,1":                               {0.5, 1},
		"1,2,3,4,5,6,7,8,9,10,11,12,13,14,15": {1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
	}
	for s, want := range ok {
		got, err := ParseBounds(s, false)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("ParseBounds(%q) = %v, %v; want %v", s, got, err, want)
		}
	}
	bad := map[string]string{
		"50, 20":                                 BoundsNotRising,
		"20, 20":                                 BoundsNotRising,
		"":                                       BoundsNotNumber,
		"0, 5":                                   BoundsNotNumber,
		"-1":                                     BoundsNotNumber,
		"a, 5":                                   BoundsNotNumber,
		"5,,10":                                  BoundsNotNumber,
		"2,5":                                    "",
		"1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16": BoundsCount,
		"1e3":                                    BoundsNotNumber,
		"2,5.":                                   BoundsNotNumber,
	}
	for s, code := range bad {
		_, err := ParseBounds(s, false)
		var be *BoundsError
		if code == "" {
			if err != nil {
				t.Errorf("ParseBounds(%q): %v", s, err)
			}
			continue
		}
		if !errors.As(err, &be) || be.Code != code {
			t.Errorf("ParseBounds(%q): %v, want %s", s, err, code)
		}
	}
	var be *BoundsError
	if _, err := ParseBounds("20, 2.5", true); !errors.As(err, &be) || be.Code != BoundsNotInteger || be.Value != "2.5" {
		t.Errorf("дробные страницы: %v", err)
	}
	if got := FormatBounds([]float64{1, 2.5, 10}); got != "1, 2.5, 10" {
		t.Errorf("FormatBounds = %q", got)
	}
}

func BenchmarkCompute20000(b *testing.B) {
	gs := library(20000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Compute(context.Background(), gs, Options{}); err != nil {
			b.Fatal(err)
		}
	}
}

// library строит n галерей с разнообразными тегами, датами и размерами.
func library(n int) []model.Gallery {
	gs := make([]model.Gallery, n)
	for i := range gs {
		gl := g(fmt.Sprintf("g%05d.zip", i), 5+i%120, int64(1+i%97)*mb/3,
			fmt.Sprintf("artist:a%d", i%700), fmt.Sprintf("group:g%d", i%300),
			fmt.Sprintf("parody:p%d", i%150), fmt.Sprintf("character:c%d", i%900),
			fmt.Sprintf("language:l%d", i%4), "category:doujinshi",
			fmt.Sprintf("tag:t%d", i%40), fmt.Sprintf("tag:u%d", i%1500), fmt.Sprintf("tag:v%d", i%2500))
		gl.Uploaded = time.Date(2015+i%10, time.Month(1+i%12), 1, 0, 0, 0, 0, time.UTC)
		gl.File.ModTime = time.Date(2018+i%8, time.Month(1+i%12), 5, 0, 0, 0, 0, time.Local)
		gs[i] = gl
	}
	return gs
}

// Подсчёт 20 000 галерей укладывается в бюджет кадра (50 мс); порог с запасом
// на медленный CI.
func TestCompute20000Speed(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий")
	}
	gs := library(20000)
	start := time.Now()
	compute(t, gs, Options{})
	if d := time.Since(start); d > 400*time.Millisecond {
		t.Errorf("Compute(20000) занял %v", d)
	}
}

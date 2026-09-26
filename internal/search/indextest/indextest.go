// Package indextest — общий набор проверок реализаций search.Index: индекс
// в памяти и индекс в каталоге обязаны давать одинаковые результаты.
package indextest

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
)

// NewIndex создаёт пустой индекс для одного подтеста.
type NewIndex func(t *testing.T) search.Index

// Run прогоняет все проверки над индексами, созданными newIndex.
func Run(t *testing.T, newIndex NewIndex) {
	t.Run("Matching", func(t *testing.T) { testMatching(t, newIndex) })
	t.Run("AllAndOrder", func(t *testing.T) { testAllAndOrder(t, newIndex) })
	t.Run("SortFields", func(t *testing.T) { testSortFields(t, newIndex) })
	t.Run("Remove", func(t *testing.T) { testRemove(t, newIndex) })
	t.Run("Upsert", func(t *testing.T) { testUpsertReplaces(t, newIndex) })
	t.Run("SuggestTags", func(t *testing.T) { testSuggestTags(t, newIndex) })
	t.Run("Normalize", func(t *testing.T) { testNormalize(t, newIndex) })
	t.Run("SpecialChars", func(t *testing.T) { testSpecialChars(t, newIndex) })
	t.Run("Concurrent", func(t *testing.T) { testConcurrent(t, newIndex) })
}

// ExampleG повторяет метаданные testdata/example.zip.
func ExampleG() model.Gallery {
	g := model.Gallery{
		Key:        model.LocalKey("example.zip"),
		ExternalID: 535147,
		Title:      "english name",
		AltTitle:   "japanease name",
		Uploaded:   time.Unix(1728950787, 0).UTC(),
		NumPages:   2,
		Favorites:  806,
		Pages:      []model.Page{{Name: "example/1.jpg"}, {Name: "example/2.jpg"}},
		File:       model.FileInfo{Size: 31759, ModTime: time.Date(2026, 9, 24, 10, 0, 0, 0, time.Local)},
	}
	for _, t := range [][2]string{{"category", "doujinshi"}, {"language", "japanese"}, {"tag", "tag 1"},
		{"tag", "tag 2"}, {"tag", "tag 3"}, {"parody", "parody 1"}, {"artist", "artist 1"}, {"character", "character 1"}} {
		g.AddTag(model.NewTag(t[0], t[1]))
	}
	return g
}

// PlainG — галерея без метаданных: одна страница, название = имя файла.
func PlainG(id string, mod time.Time) model.Gallery {
	return model.Gallery{Key: model.LocalKey(id), Title: id, Pages: []model.Page{{Name: "1.jpg"}},
		File: model.FileInfo{Size: 10, ModTime: mod}}
}

func fill(t *testing.T, idx search.Index, gs ...model.Gallery) search.Index {
	t.Helper()
	for _, g := range gs {
		if err := idx.Upsert(context.Background(), g); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

// Find выполняет запрос из строки и возвращает ID ключей по порядку.
func Find(t *testing.T, idx search.Index, query string) []string {
	t.Helper()
	q, err := search.Parse(query)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	return run(t, idx, q)
}

func run(t *testing.T, idx search.Index, q search.Query) []string {
	t.Helper()
	keys, total, err := idx.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search(%q): %v", q.Text, err)
	}
	if q.Limit == 0 && q.Offset == 0 && total != len(keys) {
		t.Fatalf("total %d != len %d", total, len(keys))
	}
	out := []string{}
	for _, k := range keys {
		out = append(out, k.ID)
	}
	return out
}

func testMatching(t *testing.T, newIndex NewIndex) {
	plain := PlainG("plain.zip", time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	idx := fill(t, newIndex(t), ExampleG(), plain)
	ex := []string{"example.zip"}
	none := []string{}
	cases := map[string][]string{
		"glis":                 ex, // часть слова в названии
		"gl":                   ex, // короче трёх символов
		"e":                    ex, // один символ («plain.zip» без «e»)
		"japan":                ex, // тег и альтернативное название
		"english doujin":       ex, // слова в разных полях
		"english manhwa":       none,
		"ENGLISH":              ex,
		"535147":               ex, // ID
		"example.zip":          ex, // имя файла
		"language:japanese":    ex, // «тип:имя» тоже в полях
		"tag:japanese":         ex, // тег любого типа
		"artist:japanese":      none,
		"tag:japan":            none, // тег — только точное имя
		`tag:"tag 1"`:          ex,
		`english -tag:"tag 3"`: none,
		`-tag:"tag 3"`:         {"plain.zip"},
		"pages:2":              ex,
		"pages:>2":             none,
		"pages:1":              {"plain.zip"},
		"pages:1..2":           {"example.zip", "plain.zip"},
		"favorites:<10":        none, // у plain нет значения
		"favorites:>800":       ex,
		"id:535147":            ex,
		"uploaded:2024":        ex,
		"uploaded:2024-10-15":  ex,
		"uploaded:>2024-10":    none,
		"uploaded:<2024-11":    ex,
		"size:>20k":            ex,
		"size:<1k":             {"plain.zip"},
		"title:glish":          ex,
		"title:japanease":      ex,
		"scanlator:x":          none,
		"added:2026-09-24":     ex,
		"added:<2026-02":       {"plain.zip"},
		"plain":                {"plain.zip"},
		"zzzz":                 none,
	}
	for q, want := range cases {
		if got := Find(t, idx, q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q → %v, want %v", q, got, want)
		}
	}
}

func testAllAndOrder(t *testing.T, newIndex NewIndex) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	var gs []model.Gallery
	for i, id := range []string{"b10.zip", "b2.zip", "a.zip"} {
		g := PlainG(id, base)
		if i == 2 {
			g.File.ModTime = base.Add(time.Hour) // новее — первым
		}
		g.AddTag(model.NewTag("tag", "tag 1"))
		gs = append(gs, g)
	}
	idx := fill(t, newIndex(t), gs...)
	want := []string{"a.zip", "b2.zip", "b10.zip"} // при равенстве — натуральный порядок
	if got := Find(t, idx, `tag:"tag 1"`); !reflect.DeepEqual(got, want) {
		t.Fatalf("порядок %v, want %v", got, want)
	}
	if got := Find(t, idx, ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("пустой запрос: %v", got)
	}
	q := search.NewQuery()
	q.Limit, q.Offset = 1, 1
	keys, total, err := idx.Search(context.Background(), q)
	if err != nil || total != 3 || len(keys) != 1 || keys[0].ID != "b2.zip" {
		t.Fatalf("limit/offset: %v %d %v", keys, total, err)
	}
	q.Limit, q.Offset = 0, 5
	if keys, total, _ := idx.Search(context.Background(), q); total != 3 || len(keys) != 0 {
		t.Fatalf("смещение за концом: %v %d", keys, total)
	}
}

// testSortFields: сортировка по каждому полю в обе стороны; при равенстве —
// ключ в натуральном порядке по возрастанию.
func testSortFields(t *testing.T, newIndex NewIndex) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(id, title string, ext int64, pages, fav int, size int64, up, add int) model.Gallery {
		g := model.Gallery{Key: model.LocalKey(id), Title: title, ExternalID: ext, Favorites: fav,
			Pages: make([]model.Page, pages), File: model.FileInfo{Size: size, ModTime: base.Add(time.Duration(add) * time.Hour)}}
		if up > 0 {
			g.Uploaded = base.AddDate(0, 0, up)
		}
		return g
	}
	idx := fill(t, newIndex(t),
		mk("g1.zip", "Beta", 30, 5, 2, 300, 3, 1),
		mk("g2.zip", "alpha", 10, 5, 9, 100, 1, 3),
		mk("g10.zip", "Gamma", 20, 1, 9, 200, 2, 2),
	)
	cases := []struct {
		field search.Field
		asc   []string
	}{
		{search.FieldTitle, []string{"g2.zip", "g1.zip", "g10.zip"}}, // без учёта регистра
		{search.FieldID, []string{"g2.zip", "g10.zip", "g1.zip"}},
		{search.FieldPages, []string{"g10.zip", "g1.zip", "g2.zip"}},     // 1, 5, 5 → g1 < g2
		{search.FieldFavorites, []string{"g1.zip", "g2.zip", "g10.zip"}}, // 2, 9, 9 → g2 < g10
		{search.FieldSize, []string{"g2.zip", "g10.zip", "g1.zip"}},
		{search.FieldUploaded, []string{"g2.zip", "g10.zip", "g1.zip"}},
		{search.FieldAdded, []string{"g1.zip", "g10.zip", "g2.zip"}},
	}
	desc := map[search.Field][]string{
		search.FieldTitle:     {"g10.zip", "g1.zip", "g2.zip"},
		search.FieldID:        {"g1.zip", "g10.zip", "g2.zip"},
		search.FieldPages:     {"g1.zip", "g2.zip", "g10.zip"}, // равные 5 — ключи по возрастанию
		search.FieldFavorites: {"g2.zip", "g10.zip", "g1.zip"},
		search.FieldSize:      {"g1.zip", "g10.zip", "g2.zip"},
		search.FieldUploaded:  {"g1.zip", "g10.zip", "g2.zip"},
		search.FieldAdded:     {"g2.zip", "g10.zip", "g1.zip"},
	}
	for _, c := range cases {
		q := search.NewQuery()
		q.Sort = search.Sort{Field: c.field}
		if got := run(t, idx, q); !reflect.DeepEqual(got, c.asc) {
			t.Errorf("сортировка %v ↑: %v, want %v", c.field, got, c.asc)
		}
		q.Sort.Desc = true
		if got := run(t, idx, q); !reflect.DeepEqual(got, desc[c.field]) {
			t.Errorf("сортировка %v ↓: %v, want %v", c.field, got, desc[c.field])
		}
	}
}

func testRemove(t *testing.T, newIndex NewIndex) {
	idx := fill(t, newIndex(t), ExampleG())
	if err := idx.Remove(context.Background(), model.LocalKey("example.zip")); err != nil {
		t.Fatal(err)
	}
	if got := Find(t, idx, "english"); len(got) != 0 {
		t.Fatalf("после удаления: %v", got)
	}
	if err := idx.Remove(context.Background(), model.LocalKey("missing.zip")); err != nil {
		t.Fatalf("удаление отсутствующего: %v", err)
	}
}

func testUpsertReplaces(t *testing.T, newIndex NewIndex) {
	g := ExampleG()
	idx := fill(t, newIndex(t), g)
	g.Title = "renamed title"
	g.Tags = nil
	g.AddTag(model.NewTag("tag", "fresh"))
	fill(t, idx, g)
	if got := Find(t, idx, "renamed"); len(got) != 1 {
		t.Fatalf("новое название не найдено: %v", got)
	}
	if got := Find(t, idx, `tag:"tag 1"`); len(got) != 0 {
		t.Fatalf("старый тег остался: %v", got)
	}
	if got := Find(t, idx, "tag:fresh"); len(got) != 1 {
		t.Fatalf("новый тег не найден: %v", got)
	}
}

func testSuggestTags(t *testing.T, newIndex NewIndex) {
	idx := fill(t, newIndex(t), ExampleG())
	ctx := context.Background()
	got, err := idx.SuggestTags(ctx, "", "TAG", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []search.TagCount{{Tag: model.NewTag("tag", "tag 1"), Count: 1}, {Tag: model.NewTag("tag", "tag 2"), Count: 1}, {Tag: model.NewTag("tag", "tag 3"), Count: 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	got, _ = idx.SuggestTags(ctx, "language", "", 10)
	if len(got) != 1 || got[0].Tag.Name != "japanese" {
		t.Fatalf("по типу: %v", got)
	}
	got, _ = idx.SuggestTags(ctx, "", "", 2)
	if len(got) != 2 {
		t.Fatalf("лимит: %v", got)
	}
	// число произведений и порядок: больше — раньше
	g2 := PlainG("two.zip", time.Now())
	g2.AddTag(model.NewTag("tag", "tag 2"))
	fill(t, idx, g2)
	got, _ = idx.SuggestTags(ctx, "tag", "tag", 10)
	if len(got) != 3 || got[0].Tag.Name != "tag 2" || got[0].Count != 2 {
		t.Fatalf("счётчики: %v", got)
	}
}

func testNormalize(t *testing.T, newIndex NewIndex) {
	fir := PlainG("fir.zip", time.Now())
	fir.Title = "Ёлка"
	fir.AddTag(model.NewTag("tag", "Ёжик"))
	wide := PlainG("wide.zip", time.Now())
	wide.Title = "ABC comic"
	idx := fill(t, newIndex(t), fir, wide)
	for q, want := range map[string][]string{
		"ёлка":        {"fir.zip"},
		"ЕЛКА":        {"fir.zip"},
		"елк":         {"fir.zip"},
		"title:Елка":  {"fir.zip"},
		"tag:ежик":    {"fir.zip"},
		"tag:ЁЖИК":    {"fir.zip"},
		"ＡＢＣ":         {"wide.zip"},
		"title:ａｂｃ":   {"wide.zip"},
		"-tag:ежик":   {"wide.zip"},
		"елка ＡＢＣ":    {},
		"ЁЖИК":        {"fir.zip"}, // имя тега в свободном тексте
		"tag:ёжик ёл": {"fir.zip"},
	} {
		if got := Find(t, idx, q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q → %v, want %v", q, got, want)
		}
	}
	got, _ := idx.SuggestTags(context.Background(), "", "еж", 10)
	if len(got) != 1 || got[0].Tag != model.NewTag("tag", "Ёжик") {
		t.Errorf("подсказка по нормализованному префиксу: %v", got)
	}
}

// testSpecialChars: % _ \ в запросе — обычные символы.
func testSpecialChars(t *testing.T, newIndex NewIndex) {
	mk := func(id, title string) model.Gallery {
		g := PlainG(id, time.Now())
		g.Title = title
		return g
	}
	idx := fill(t, newIndex(t),
		mk("pct.zip", "100% manga"),
		mk("num.zip", "1000 manga"),
		mk("und.zip", "a_b"),
		mk("axb.zip", "axb"),
		mk("bs.zip", `back\slash`),
	)
	for q, want := range map[string][]string{
		"100%":         {"pct.zip"},
		"a_b":          {"und.zip"},
		`k\s`:          {"bs.zip"},
		"%":            {"pct.zip"},
		"_":            {"und.zip"},
		"title:100%":   {"pct.zip"},
		"title:a_b":    {"und.zip"},
		`title:"k\sl"`: {"bs.zip"},
	} {
		if got := Find(t, idx, q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q → %v, want %v", q, got, want)
		}
	}
}

func testConcurrent(t *testing.T, newIndex NewIndex) {
	idx := newIndex(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if err := idx.Upsert(ctx, PlainG(fmt.Sprintf("g%d-%d.zip", i, j), time.Now())); err != nil {
					t.Error(err)
				}
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if _, _, err := idx.Search(ctx, search.NewQuery()); err != nil {
					t.Error(err)
				}
				if _, err := idx.SuggestTags(ctx, "", "", 5); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
	if _, total, _ := idx.Search(ctx, search.NewQuery()); total != 200 {
		t.Fatalf("всего %d", total)
	}
}

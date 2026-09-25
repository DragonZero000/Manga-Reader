package search

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"mangareader/internal/model"
)

// exampleG повторяет метаданные testdata/example.zip.
func exampleG() model.Gallery {
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

func plainG(id string, mod time.Time) model.Gallery {
	return model.Gallery{Key: model.LocalKey(id), Title: id, Pages: []model.Page{{Name: "1.jpg"}},
		File: model.FileInfo{Size: 10, ModTime: mod}}
}

func newIdx(t *testing.T, gs ...model.Gallery) *MemIndex {
	t.Helper()
	idx := NewMemIndex()
	for _, g := range gs {
		if err := idx.Upsert(context.Background(), g); err != nil {
			t.Fatal(err)
		}
	}
	return idx
}

func find(t *testing.T, idx Index, query string) []string {
	t.Helper()
	q, err := Parse(query)
	if err != nil {
		t.Fatalf("Parse(%q): %v", query, err)
	}
	keys, total, err := idx.Search(context.Background(), q)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	if total != len(keys) {
		t.Fatalf("total %d != len %d", total, len(keys))
	}
	out := []string{}
	for _, k := range keys {
		out = append(out, k.ID)
	}
	return out
}

func TestMemIndexMatching(t *testing.T) {
	plain := plainG("plain.zip", time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local))
	idx := newIdx(t, exampleG(), plain)
	ex := []string{"example.zip"}
	none := []string{}
	cases := map[string][]string{
		"glis":                 ex, // часть слова в названии
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
	}
	for q, want := range cases {
		if got := find(t, idx, q); !reflect.DeepEqual(got, want) {
			t.Errorf("%q → %v, want %v", q, got, want)
		}
	}
}

func TestMemIndexAllAndOrder(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.Local)
	var gs []model.Gallery
	for i, id := range []string{"b10.zip", "b2.zip", "a.zip"} {
		g := plainG(id, base)
		if i == 2 {
			g.File.ModTime = base.Add(time.Hour) // новее — первым
		}
		g.AddTag(model.NewTag("tag", "tag 1"))
		gs = append(gs, g)
	}
	idx := newIdx(t, gs...)
	want := []string{"a.zip", "b2.zip", "b10.zip"}
	if got := find(t, idx, `tag:"tag 1"`); !reflect.DeepEqual(got, want) {
		t.Fatalf("порядок %v, want %v", got, want)
	}
	if got := find(t, idx, ""); !reflect.DeepEqual(got, want) {
		t.Fatalf("пустой запрос: %v", got)
	}
	q := NewQuery()
	q.Limit, q.Offset = 1, 1
	keys, total, err := idx.Search(context.Background(), q)
	if err != nil || total != 3 || len(keys) != 1 || keys[0].ID != "b2.zip" {
		t.Fatalf("limit/offset: %v %d %v", keys, total, err)
	}
}

func TestMemIndexRemove(t *testing.T) {
	idx := newIdx(t, exampleG())
	if err := idx.Remove(context.Background(), model.LocalKey("example.zip")); err != nil {
		t.Fatal(err)
	}
	if got := find(t, idx, "english"); len(got) != 0 {
		t.Fatalf("после удаления: %v", got)
	}
}

func TestSuggestTags(t *testing.T) {
	idx := newIdx(t, exampleG())
	got, err := idx.SuggestTags(context.Background(), "", "TAG", 10)
	if err != nil {
		t.Fatal(err)
	}
	want := []TagCount{{model.NewTag("tag", "tag 1"), 1}, {model.NewTag("tag", "tag 2"), 1}, {model.NewTag("tag", "tag 3"), 1}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	got, _ = idx.SuggestTags(context.Background(), "language", "", 10)
	if len(got) != 1 || got[0].Tag.Name != "japanese" {
		t.Fatalf("по типу: %v", got)
	}
	got, _ = idx.SuggestTags(context.Background(), "", "", 2)
	if len(got) != 2 {
		t.Fatalf("лимит: %v", got)
	}
}

func TestMemIndexConcurrent(t *testing.T) {
	idx := NewMemIndex()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				idx.Upsert(context.Background(), plainG(fmt.Sprintf("g%d-%d.zip", i, j), time.Now()))
			}
		}(i)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				idx.Search(context.Background(), NewQuery())
				idx.SuggestTags(context.Background(), "", "", 5)
			}
		}()
	}
	wg.Wait()
	_, total, _ := idx.Search(context.Background(), NewQuery())
	if total != 400 {
		t.Fatalf("всего %d", total)
	}
}

// BenchmarkSearch300 — поиск по 300 произведениям с 5 тегами каждое.
func BenchmarkSearch300(b *testing.B) {
	idx := NewMemIndex()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 300; i++ {
		g := model.Gallery{Key: model.LocalKey(fmt.Sprintf("g%03d.zip", i)), ExternalID: int64(100000 + i),
			Title: fmt.Sprintf("Gallery %03d — test title", i), Uploaded: base.Add(time.Duration(i) * time.Hour),
			Pages: make([]model.Page, 20), Favorites: i, File: model.FileInfo{Size: int64(i) << 20, ModTime: base}}
		for j := 0; j < 5; j++ {
			g.AddTag(model.NewTag("tag", fmt.Sprintf("tag %d", (i+j)%17)))
		}
		g.AddTag(model.NewTag("artist", fmt.Sprintf("artist %d", i%29)))
		idx.Upsert(context.Background(), g)
	}
	for _, query := range []string{"title", `tag:"tag 1" pages:>10 -artist:"artist 3"`, "zzzz"} {
		q, err := Parse(query)
		if err != nil {
			b.Fatal(err)
		}
		b.Run(query, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				idx.Search(context.Background(), q)
			}
		})
	}
}

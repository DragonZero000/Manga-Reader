package search

import (
	"reflect"
	"testing"
	"time"

	"mangareader/internal/model"
)

func sortIDs(gs []model.Gallery) []string {
	ids := make([]string, len(gs))
	for i, g := range gs {
		ids[i] = g.Key.ID
	}
	return ids
}

func sortFixture() []model.Gallery {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(id, title string, pages int, size int64, add int, up int) model.Gallery {
		g := model.Gallery{Key: model.LocalKey(id), Title: title, Pages: make([]model.Page, pages),
			File: model.FileInfo{Size: size, ModTime: base.Add(time.Duration(add) * time.Hour)}}
		if up > 0 {
			g.Uploaded = base.AddDate(0, 0, up)
		}
		return g
	}
	return []model.Gallery{
		mk("a10.zip", "Beta", 5, 300, 1, 3),
		mk("a2.zip", "alpha", 5, 100, 3, 0), // нет даты выпуска
		mk("b.zip", "", 0, 200, 2, 1),       // нет названия и страниц
	}
}

func TestSortGalleriesFields(t *testing.T) {
	cases := []struct {
		field    Field
		asc, dsc []string
	}{
		{FieldTitle, []string{"b.zip", "a2.zip", "a10.zip"}, []string{"a10.zip", "a2.zip", "b.zip"}},
		{FieldPages, []string{"b.zip", "a2.zip", "a10.zip"}, []string{"a2.zip", "a10.zip", "b.zip"}}, // равные: a2 раньше a10
		{FieldSize, []string{"a2.zip", "b.zip", "a10.zip"}, []string{"a10.zip", "b.zip", "a2.zip"}},
		{FieldAdded, []string{"a10.zip", "b.zip", "a2.zip"}, []string{"a2.zip", "b.zip", "a10.zip"}},
		{FieldUploaded, []string{"a2.zip", "b.zip", "a10.zip"}, []string{"a10.zip", "b.zip", "a2.zip"}}, // пустая дата — наименьшая
	}
	for _, c := range cases {
		gs := sortFixture()
		SortGalleries(gs, Sort{Field: c.field})
		if got := sortIDs(gs); !reflect.DeepEqual(got, c.asc) {
			t.Errorf("%v ↑: %v, want %v", c.field, got, c.asc)
		}
		SortGalleries(gs, Sort{Field: c.field, Desc: true})
		if got := sortIDs(gs); !reflect.DeepEqual(got, c.dsc) {
			t.Errorf("%v ↓: %v, want %v", c.field, got, c.dsc)
		}
	}
}

func TestSortGalleriesEqualKeyOrder(t *testing.T) {
	gs := []model.Gallery{
		{Key: model.LocalKey("a10.zip")}, {Key: model.LocalKey("a2.zip")}, {Key: model.LocalKey("a1.zip")},
	}
	for _, desc := range []bool{false, true} {
		SortGalleries(gs, Sort{Field: FieldPages, Desc: desc})
		if want := []string{"a1.zip", "a2.zip", "a10.zip"}; !reflect.DeepEqual(sortIDs(gs), want) {
			t.Errorf("desc=%v: %v, want %v", desc, sortIDs(gs), want)
		}
	}
}

func TestSortGalleriesZeroSortIsDefault(t *testing.T) {
	a, b := sortFixture(), sortFixture()
	SortGalleries(a, Sort{})
	SortGalleries(b, DefaultSort)
	if !reflect.DeepEqual(sortIDs(a), sortIDs(b)) {
		t.Fatalf("%v != %v", sortIDs(a), sortIDs(b))
	}
}

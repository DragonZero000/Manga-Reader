package app

import (
	"path/filepath"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/userdata"
)

func pagesOf(names ...string) []model.Page {
	out := make([]model.Page, len(names))
	for i, n := range names {
		out[i] = model.Page{Name: n}
	}
	return out
}

func TestResume(t *testing.T) {
	five := pagesOf("1", "2", "3", "4", "5")
	cases := []struct {
		name       string
		p          userdata.Progress
		pages      []model.Page
		page       int
		ok, approx bool
	}{
		{"страница на месте", userdata.Progress{Page: "3", Index: 2, Total: 5}, five, 2, true, false},
		{"удалены страницы перед ней", userdata.Progress{Page: "4", Index: 3, Total: 5}, pagesOf("1", "4", "5"), 1, true, false},
		{"имя пропало", userdata.Progress{Page: "x", Index: 3, Total: 5}, five, 3, true, true},
		{"имя пропало, страниц стало меньше", userdata.Progress{Page: "x", Index: 40, Total: 87}, five, 4, true, true},
		{"первая страница", userdata.Progress{Page: "1", Index: 0, Total: 5}, five, 0, false, false},
		{"имя пропало, номер 0", userdata.Progress{Page: "x", Index: 0, Total: 5}, five, 0, false, false},
		{"дочитано", userdata.Progress{Page: "5", Index: 4, Total: 5, Finished: true}, five, 0, false, false},
		{"дочитано, добавлены страницы", userdata.Progress{Page: "5", Index: 4, Total: 5, Finished: true},
			pagesOf("1", "2", "3", "4", "5", "6", "7"), 5, true, false},
		{"дочитано, имя пропало", userdata.Progress{Page: "x", Index: 4, Total: 5, Finished: true}, five, 0, false, false},
		{"нет страниц", userdata.Progress{Page: "1", Index: 3}, nil, 0, false, false},
	}
	for _, c := range cases {
		page, ok, approx := Resume(c.p, c.pages)
		if page != c.page || ok != c.ok || approx != c.approx {
			t.Errorf("%s: (%d, %v, %v), ждали (%d, %v, %v)", c.name, page, ok, approx, c.page, c.ok, c.approx)
		}
	}
}

func testProgress(t *testing.T) (*ProgressService, *userdata.Store) {
	t.Helper()
	store, _, err := userdata.Open(filepath.Join(t.TempDir(), userdata.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return newProgressService(store, func() string { return "root" }), store
}

func gallery(rel string, pages ...string) model.Gallery {
	return model.Gallery{Key: model.LocalKey(rel), Fingerprint: "fp-" + rel, Pages: pagesOf(pages...)}
}

// Позиция видна сразу (кэш) и после перезагрузки из базы; «дочитано» после
// сохранений не перезаписывается; сброс удаляет прогресс.
func TestProgressService(t *testing.T) {
	p, _ := testProgress(t)
	g := gallery("a.zip", "1", "2", "3")
	if p.Has(g) {
		t.Fatal("прогресс до чтения")
	}
	p.Save(g, 1)
	if page, ok, _ := p.Resume(g); !ok || page != 1 {
		t.Fatalf("сразу после Save: %d %v", page, ok)
	}
	p.Save(g, 2)
	p.Finish(g)
	if !p.Flush(time.Second) {
		t.Fatal("очередь не записана")
	}
	p.Reload()
	if pr, ok := p.Get(g); !ok || !pr.Finished || pr.Page != "3" {
		t.Fatalf("после перезагрузки: %+v %v", pr, ok)
	}
	if !p.Has(g) {
		t.Fatal("дочитанное произведение — с прогрессом")
	}
	if _, ok, _ := p.Resume(g); ok {
		t.Fatal("дочитанное без новых страниц не продолжается")
	}

	p.Reset(g)
	p.Flush(time.Second)
	p.Reload()
	if p.Has(g) {
		t.Fatal("прогресс после сброса")
	}
}

// Перенос файла с тем же содержимым: после сверки позиция видна под новым путём.
func TestProgressFollowsMove(t *testing.T) {
	p, store := testProgress(t)
	changed := 0
	p.SetOnChanged(func() { changed++ })
	p.Save(gallery("a.zip", "1", "2", "3"), 2)
	p.Flush(time.Second)
	if err := store.Reconcile("root", map[string]string{"b.zip": "fp-a.zip"}, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	p.Reload()
	if page, ok, _ := p.Resume(gallery("b.zip", "1", "2", "3")); !ok || page != 2 {
		t.Fatalf("после переноса: %d %v", page, ok)
	}
	if changed != 1 {
		t.Fatalf("onChanged вызван %d раз", changed)
	}
}

// Без пользовательских данных служба ничего не делает и не падает.
func TestProgressNil(t *testing.T) {
	var p *ProgressService
	g := gallery("a.zip", "1", "2")
	p.Save(g, 1)
	p.Finish(g)
	p.Reset(g)
	p.Reload()
	if p.Has(g) || !p.Flush(time.Millisecond) {
		t.Fatal("nil-служба")
	}
}

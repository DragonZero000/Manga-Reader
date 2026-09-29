package ui

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
	"mangareader/internal/userdata"
)

// Правка тегов через оболочку: сохранение, индекс, повтор текущего поиска,
// подсказки.
func TestShellEditTags(t *testing.T) {
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store, _, err := userdata.Open(filepath.Join(t.TempDir(), userdata.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	idx := search.NewMemIndex()
	src := library.NewDirSource(dir, idx)
	svc := app.NewForTestWithUserData(src, idx, storage.NewMemSettings(), store)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := NewShell(a, svc)
	q := make(chan func(), 4096)
	do := func(f func()) { q <- f }
	s.do = do
	s.Details.SetDispatcher(do)
	s.Search().SetDispatcher(do)
	pump := func() {
		for {
			select {
			case f := <-q:
				f()
			default:
				return
			}
		}
	}

	ed := shellTags{s}
	if !ed.Available() {
		t.Fatal("правка тегов недоступна при открытых пользовательских данных")
	}
	s.Search().SetQuery("custom-tag:alice")
	waitUI(t, pump, "поиск выполнен", func() bool { return s.Search().StatusText() == "Ничего не найдено" })

	done := make(chan error, 1)
	var got model.Gallery
	ed.Edit(model.LocalKey("example.zip"), app.TagOp{Kind: app.TagAdd, Tag: model.NewTag("character", "alice")},
		func(g model.Gallery, err error) { got = g; done <- err })
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Custom, []model.Tag{model.NewTag("character", "alice")}) {
		t.Fatalf("галерея после правки: %v", got.Custom)
	}
	// текущий поиск выполнен заново и видит новый тег
	waitUI(t, pump, "поиск повторён", func() bool { return len(s.Search().Results()) == 1 })

	names := make(chan []string, 1)
	ed.Suggest("character", "ali", 10, func(n []string) { names <- n })
	if n := <-names; !reflect.DeepEqual(n, []string{"alice"}) {
		t.Fatalf("подсказки: %q", n)
	}

	ed.Edit(model.LocalKey("example.zip"), app.TagOp{Kind: app.TagAdd, Tag: model.NewTag("tag", "tag 1")},
		func(_ model.Gallery, err error) { done <- err })
	if err := <-done; err == nil {
		t.Fatal("дубликат оригинального тега добавлен")
	}
}

// Без пользовательских данных правка тегов недоступна.
func TestShellTagsUnavailable(t *testing.T) {
	s, _, _ := newTestShell(t)
	if (shellTags{s}).Available() {
		t.Fatal("правка тегов доступна без пользовательских данных")
	}
}

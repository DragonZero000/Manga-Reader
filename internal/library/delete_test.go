package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// removeFS — папка с безвозвратным удалением на любой платформе.
type removeFS struct {
	*storage.FS
	root string
}

func (s removeFS) CanTrash(string) (bool, error) { return false, nil }

func (s removeFS) Delete(rel string, _ bool) error {
	return os.Remove(filepath.Join(s.root, filepath.FromSlash(rel)))
}

// После удаления галереи нет в списке и индексе сразу, а после сканирования
// не остаётся ни ошибки, ни ссылки на страницу.
func TestSourceDelete(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "keep.zip", file{"1.png", img})
	writeZip(t, dir, "series/gone.zip", file{"1.png", img})
	links := LoadLinks(filepath.Join(t.TempDir(), "links.json"))
	links.Set("series/gone.zip", "https://site.example/g/1/")
	idx := search.NewMemIndex()
	src := NewSource(WithLinks(removeFS{storage.NewFS(dir), dir}, links), idx)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if !src.CanDelete() {
		t.Fatal("удаление через обёртку ссылок недоступно")
	}
	gone := model.LocalKey("series/gone.zip")
	inIndex := func() bool {
		q, err := search.Parse("gone")
		if err != nil {
			t.Fatal(err)
		}
		keys, _, err := idx.Search(context.Background(), q)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range keys {
			if k == gone {
				return true
			}
		}
		return false
	}
	if !inIndex() {
		t.Fatal("до удаления галерея должна находиться поиском")
	}

	if err := src.Delete(gone, true); err != nil {
		t.Fatal(err)
	}
	if _, ok := src.Get(gone); ok {
		t.Fatal("галерея осталась в списке")
	}
	if len(src.Galleries()) != 1 || src.Galleries()[0].Key != model.LocalKey("keep.zip") {
		t.Fatalf("список после удаления: %v", src.Galleries())
	}
	if inIndex() {
		t.Fatal("галерея осталась в индексе")
	}

	res, err := src.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Galleries) != 1 || len(res.Errors) != 0 {
		t.Fatalf("после сканирования: %d галерей, ошибки %v", len(res.Galleries), res.Errors)
	}
	exists := map[string]bool{}
	for _, g := range src.Galleries() {
		exists[g.Key.ID] = true
	}
	links.Prune(func(rel string) bool { return exists[rel] })
	if u := links.Get("series/gone.zip"); u != "" {
		t.Fatalf("ссылка удалённого файла осталась: %q", u)
	}

	// файла уже нет — ошибка, список не меняется
	if err := src.Delete(gone, true); err == nil {
		t.Fatal("повторное удаление должно давать ошибку")
	}
	if len(src.Galleries()) != 1 {
		t.Fatal("список изменился после ошибки")
	}
}

// hidden скрывает методы удаления хранилища.
type hidden struct{ storage.Storage }

func TestSourceDeleteUnsupported(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "a.zip", file{"1.png", pngBytes(t, 2, 2)})
	src := NewSource(hidden{storage.NewFS(dir)}, nil)
	if src.CanDelete() {
		t.Fatal("хранилище без удаления")
	}
	if err := src.Delete(model.LocalKey("a.zip"), true); !errors.Is(err, storage.ErrUnsupported) {
		t.Fatalf("ожидалась ErrUnsupported, получено %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.zip")); err != nil {
		t.Fatal(err)
	}
	if err := NewSource(nil, nil).Delete(model.LocalKey("a.zip"), true); !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("без папки: %v", err)
	}
}

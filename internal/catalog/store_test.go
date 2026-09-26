//go:build sqlite_fts5

package catalog

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

func copyExample(t *testing.T, dir, rel string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, rel), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func sourceOver(c *Catalog, dir string) *library.Source {
	return library.NewSourceWithStore(storage.NewFS(dir), c.Index(), c.ScanStore())
}

func findKeys(t *testing.T, idx search.Index, query string) []string {
	t.Helper()
	q, err := search.Parse(query)
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := idx.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, k := range keys {
		out = append(out, k.ID)
	}
	return out
}

// Сканирование → «перезапуск» (новый каталог над тем же файлом) → галереи,
// ошибки и поиск доступны до сканирования; сканирование без изменений не
// открывает ни одного файла.
func TestCatalogRestart(t *testing.T) {
	dir, db := t.TempDir(), filepath.Join(t.TempDir(), FileName)
	copyExample(t, dir, "a.zip")
	copyExample(t, dir, "b.zip")
	if err := os.WriteFile(filepath.Join(dir, "page.html"), []byte("<!doctype html><html><body>x</body></html>"), 0o644); err != nil {
		t.Fatal(err)
	}
	c1 := openTest(t, db, dir)
	if _, err := sourceOver(c1, dir).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := openTest(t, db, dir)
	if c2.Fresh() {
		t.Fatal("каталог не должен пересоздаваться")
	}
	src := sourceOver(c2, dir)
	cached := src.LoadCatalog()
	if len(cached.Galleries) != 2 || len(cached.Errors) != 1 {
		t.Fatalf("из каталога: %d галерей, %d ошибок", len(cached.Galleries), len(cached.Errors))
	}
	var ue *library.UnsupportedError
	if !errors.As(cached.Errors[0].Err, &ue) || ue.Kind != library.KindHTML {
		t.Fatalf("причина ошибки: %v", cached.Errors[0].Err)
	}
	if g, ok := src.Get(model.LocalKey("a.zip")); !ok || g.Title != "english name" || len(g.Pages) != 2 {
		t.Fatalf("галерея из каталога: %+v", g)
	}
	if got := findKeys(t, c2.Index(), "english"); len(got) != 2 {
		t.Fatalf("поиск до сканирования: %v", got)
	}
	res, err := src.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if res.Opened != 0 || len(res.Galleries) != 2 || len(res.Added)+len(res.Changed)+len(res.Removed) != 0 {
		t.Fatalf("сканирование без изменений: открыто %d, %d галерей, изменений %d",
			res.Opened, len(res.Galleries), len(res.Added)+len(res.Changed)+len(res.Removed))
	}

	// удалённый файл уходит из каталога и индекса
	os.Remove(filepath.Join(dir, "b.zip"))
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := findKeys(t, c2.Index(), "english"); len(got) != 1 || got[0] != "a.zip" {
		t.Fatalf("после удаления: %v", got)
	}
	if n := count(t, c2, "files"); n != 2 {
		t.Fatalf("записей файлов %d, ожидалось 2 (a.zip, page.html)", n)
	}
}

// Индекс разошёлся с галереями — при открытии перестраивается.
func TestCatalogRebuildsIndex(t *testing.T) {
	dir, db := t.TempDir(), filepath.Join(t.TempDir(), FileName)
	copyExample(t, dir, "a.zip")
	c1 := openTest(t, db, dir)
	if _, err := sourceOver(c1, dir).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	c1.db.Exec(`DELETE FROM docs`)
	c1.db.Exec(`DELETE FROM docs_fts`)
	c1.Close()

	c2 := openTest(t, db, dir)
	if got := findKeys(t, c2.Index(), "english"); len(got) != 1 {
		t.Fatalf("индекс не перестроен: %v", got)
	}
}

func TestStoreLinks(t *testing.T) {
	c := openTest(t, filepath.Join(t.TempDir(), FileName), "root")
	st := c.ScanStore()
	if err := st.Save(library.ScanDelta{Links: map[string]string{"g.zip": "https://site.example/g/1/"}}); err != nil {
		t.Fatal(err)
	}
	if st.StoredLink("g.zip") != "https://site.example/g/1/" || st.StoredLink("x.zip") != "" {
		t.Fatal("ссылки")
	}
	if err := st.Save(library.ScanDelta{Delete: []string{"g.zip"}}); err != nil {
		t.Fatal(err)
	}
	if st.StoredLink("g.zip") != "" {
		t.Fatal("ссылка удалённого файла осталась")
	}
}

// Обложки: сохраняются для области; обложки изменённых и удалённых файлов
// удаляются при сохранении сканирования.
func TestCovers(t *testing.T) {
	c := openTest(t, filepath.Join(t.TempDir(), FileName), "root")
	cv := c.Covers()
	cv.Save("local:a.zip@1", "a.zip", 200, 300, []byte("jpeg-a"))
	cv.Save("local:b.zip@1", "b.zip", 200, 300, []byte("jpeg-b"))
	if d, ok := cv.Load("local:a.zip@1", 200, 300); !ok || string(d) != "jpeg-a" {
		t.Fatalf("обложка: %q %v", d, ok)
	}
	if _, ok := cv.Load("local:a.zip@1", 100, 150); ok {
		t.Fatal("обложка для другой области не должна подходить")
	}
	st := c.ScanStore()
	if err := st.Save(library.ScanDelta{
		Put:    map[string]library.StoredEntry{"a.zip": {Size: 1, Err: library.ErrEmpty}},
		Delete: []string{"b.zip"},
	}); err != nil {
		t.Fatal(err)
	}
	if _, ok := cv.Load("local:a.zip@1", 200, 300); ok {
		t.Fatal("обложка изменённого файла осталась")
	}
	if _, ok := cv.Load("local:b.zip@1", 200, 300); ok {
		t.Fatal("обложка удалённого файла осталась")
	}
}

//go:build !android

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/model"
	"mangareader/internal/paths"
	"mangareader/internal/thumbs"
)

// Кэш обложек ПК — cache/thumbs в папке приложения; после сканирования
// обложки галерей, которых нет в библиотеке, удаляются.
func TestPortableCoverCachePrune(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, paths.LibraryDirName)
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "a.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := newPortable("test", dir)
	if s.Covers == nil {
		t.Fatal("кэша обложек нет")
	}
	if _, err := s.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, ok := s.Library.Get(model.LocalKey("a.zip"))
	if !ok {
		t.Fatal("галерея не найдена")
	}
	store := thumbs.NewDirStore(filepath.Join(dir, "cache", "thumbs"))
	store.Save(thumbs.CacheKey(g), "a.zip", 10, 20, []byte("a"))
	store.Save("local:gone.zip@1", "gone.zip", 10, 20, []byte("gone"))

	s.PruneCovers()
	s.Close() // дожидается уборки в фоне
	if _, ok := store.Load(thumbs.CacheKey(g), 10, 20); !ok {
		t.Fatal("обложка галереи библиотеки удалена")
	}
	if _, ok := store.Load("local:gone.zip@1", 10, 20); ok {
		t.Fatal("обложка удалённого архива осталась")
	}
}

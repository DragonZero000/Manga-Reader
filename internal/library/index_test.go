package library

import (
	"context"
	"os"
	"testing"

	"mangareader/internal/search"
)

// Сканирование заполняет индекс поиска, удаление архива убирает его из индекса.
func TestScanFillsMemIndex(t *testing.T) {
	dir := t.TempDir()
	p := copyExample(t, dir, "example.zip")
	idx := search.NewMemIndex()
	src := NewDirSource(dir, idx)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	q, err := search.Parse("english")
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := idx.Search(context.Background(), q)
	if err != nil || len(keys) != 1 || keys[0].ID != "example.zip" {
		t.Fatalf("поиск после сканирования: %v, %v", keys, err)
	}

	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	keys, _, _ = idx.Search(context.Background(), q)
	if len(keys) != 0 {
		t.Fatalf("после удаления архива: %v", keys)
	}
}

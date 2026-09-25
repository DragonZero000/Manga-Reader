package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

func TestLinksPersistAndPrune(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sub", "links.json")
	l := LoadLinks(p)
	if err := l.Set("g-1.zip", "https://site.example/g/1/"); err != nil {
		t.Fatal(err)
	}
	if err := l.Set("g-2.zip", "javascript:alert(1)"); err == nil {
		t.Error("не http(s) — ошибка")
	}
	l.Set("old.zip", "https://site.example/g/0/")

	l = LoadLinks(p) // «перезапуск»
	if got := l.Get("g-1.zip"); got != "https://site.example/g/1/" {
		t.Fatalf("после перезапуска: %q", got)
	}
	if err := l.Prune(func(rel string) bool { return rel == "g-1.zip" }); err != nil {
		t.Fatal(err)
	}
	l = LoadLinks(p)
	if l.Get("old.zip") != "" || l.Get("g-1.zip") == "" {
		t.Fatalf("после Prune: old=%q g-1=%q", l.Get("old.zip"), l.Get("g-1.zip"))
	}
}

func TestLinksCorruptFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "links.json")
	os.WriteFile(p, []byte("не json"), 0o644)
	if l := LoadLinks(p); l.Get("x") != "" {
		t.Fatal("повреждённый файл — пустой набор")
	}
}

func TestScanUsesLinks(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "g-1.zip", file{"1.png", img})
	writeZip(t, dir, "meta.zip", file{"1.png", img}, file{"meta.json", []byte(`{"url":"https://a.example/g/9/"}`)})
	links := LoadLinks(filepath.Join(t.TempDir(), "links.json"))
	links.Set("g-1.zip", "https://site.example/g/1/")
	links.Set("meta.zip", "https://b.example/x")

	src := NewSource(WithLinks(storage.NewFS(dir), links), nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"g-1.zip":  "https://site.example/g/1/",
		"meta.zip": "https://a.example/g/9/", // meta.json важнее
	} {
		if g, _ := src.Get(model.LocalKey(rel)); g.SourceURL != want {
			t.Errorf("%s: %q, ожидалось %q", rel, g.SourceURL, want)
		}
	}
}

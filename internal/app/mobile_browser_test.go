package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/library"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

func TestMobileBrowserSettings(t *testing.T) {
	st := storage.NewMemSettings()
	got := MobileBrowserSettings(st)
	if got.Home != "" || got.Search != mobilebrowser.Engines[0].Template || got.Toolbar != mobilebrowser.ToolbarBottom {
		t.Fatalf("по умолчанию: %+v", got)
	}
	st.SetString(KeyBrowserHome, "https://example.org")
	st.SetString(KeyBrowserSearch, "DuckDuckGo")
	st.SetString(KeyBrowserToolbar, mobilebrowser.ToolbarTop)
	got = MobileBrowserSettings(st)
	if got.Home != "https://example.org" || got.Search != "https://duckduckgo.com/?q=%s" || got.Toolbar != "top" {
		t.Fatalf("заданные: %+v", got)
	}
	st.SetString(KeyBrowserToolbar, "сбоку")
	if MobileBrowserSettings(st).Toolbar != mobilebrowser.ToolbarBottom {
		t.Fatal("неизвестное положение — снизу")
	}
}

// Загрузка из браузера Android: адрес страницы запоминается, файл
// разбирается заново и получает ссылку; удалённый файл забывается.
func TestOnDownloadedAndPrune(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "g-1.zip")
	os.WriteFile(p, data, 0o644)
	links := library.LoadLinks(filepath.Join(t.TempDir(), "links.json"))
	src := library.NewSource(library.WithLinks(storage.NewFS(dir), links), nil)
	svc := NewForTest(src, search.NopIndex{}, storage.NewMemSettings())
	svc.Links = links
	src.Scan(context.Background())

	svc.OnDownloaded("g-1.zip", "https://site.example/g/1/")
	src.Scan(context.Background())
	if g, _ := src.Get(model.LocalKey("g-1.zip")); g.SourceURL != "https://site.example/g/1/" {
		t.Fatalf("ссылка после загрузки: %q", g.SourceURL)
	}

	os.Remove(p)
	src.Scan(context.Background())
	svc.PruneLinks()
	if links.Get("g-1.zip") != "" {
		t.Fatal("ссылка удалённого файла должна быть забыта")
	}
}

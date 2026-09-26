//go:build sqlite_fts5

package catalog

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/storage"
)

const pageURL = "https://site.example/g/1/"

// Сценарии спеки «Ссылки скачанных файлов в каталоге» на Android-модели:
// links.json + каталог.
func TestLinksTwoCopies(t *testing.T) {
	dir, app := t.TempDir(), t.TempDir()
	db, linksPath := filepath.Join(app, FileName), filepath.Join(app, "links.json")
	copyExample(t, dir, "g.zip")

	start := func() (*Catalog, *library.Links, *library.Source) {
		c := openTest(t, db, dir)
		links := library.LoadLinksWithBackup(linksPath, c)
		if c.Fresh() {
			if err := c.ImportLinks(links.All()); err != nil {
				t.Fatal(err)
			}
		}
		src := library.NewSourceWithStore(library.WithLinks(storage.NewFS(dir), links), c.Index(), c.ScanStore())
		src.LoadCatalog()
		if _, err := src.Scan(context.Background()); err != nil {
			t.Fatal(err)
		}
		return c, links, src
	}
	url := func(src *library.Source) string {
		g, _ := src.Get(model.LocalKey("g.zip"))
		return g.SourceURL
	}

	// скачан браузером: ссылка в links.json, после разбора — и в каталоге
	c, links, _ := start()
	if err := links.Set("g.zip", pageURL); err != nil {
		t.Fatal(err)
	}
	c.Close()
	c, _, src := start()
	// файл уже в каталоге без ссылки — как после загрузки: разбор заново
	src.Invalidate("g.zip")
	src.Scan(context.Background())
	if url(src) != pageURL || c.ScanStore().StoredLink("g.zip") != pageURL {
		t.Fatalf("ссылка после разбора: галерея %q, каталог %q", url(src), c.ScanStore().StoredLink("g.zip"))
	}
	c.Close()

	// повреждённый links.json — восстанавливается из каталога
	if err := os.WriteFile(linksPath, []byte("{мусор"), 0o644); err != nil {
		t.Fatal(err)
	}
	c, links, src = start()
	if links.Get("g.zip") != pageURL || url(src) != pageURL {
		t.Fatalf("после повреждения links.json: links %q, галерея %q", links.Get("g.zip"), url(src))
	}
	if data, _ := os.ReadFile(linksPath); !strings.Contains(string(data), pageURL) {
		t.Fatalf("links.json не записан заново: %s", data)
	}
	c.Close()

	// удалённый каталог — ссылки переносятся из links.json
	for _, p := range []string{db, db + "-wal", db + "-shm"} {
		os.Remove(p)
	}
	c, _, src = start()
	if !c.Fresh() || c.ScanStore().StoredLink("g.zip") != pageURL || url(src) != pageURL {
		t.Fatalf("после удаления каталога: fresh=%v, каталог %q, галерея %q",
			c.Fresh(), c.ScanStore().StoredLink("g.zip"), url(src))
	}
	c.Close()
}

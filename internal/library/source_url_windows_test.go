package library

import (
	"context"
	"os"
	"testing"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

func TestScanSourceURLPriority(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	zone := []byte("[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://b.example/x\r\n")
	withMeta := writeZip(t, dir, "meta.zip", file{"1.png", img}, file{"meta.json", []byte(`{"url":"https://a.example/g/1/"}`)})
	plain := writeZip(t, dir, "plain.zip", file{"1.png", img})
	writeZip(t, dir, "none.zip", file{"1.png", img})
	for _, p := range []string{withMeta, plain} {
		if err := os.WriteFile(p+":Zone.Identifier", zone, 0o644); err != nil {
			t.Skipf("нет альтернативных потоков: %v", err)
		}
	}
	src := NewSource(storage.NewFS(dir), nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	for rel, want := range map[string]string{
		"meta.zip":  "https://a.example/g/1/",
		"plain.zip": "https://b.example/x",
		"none.zip":  "",
	} {
		g, _ := src.Get(model.LocalKey(rel))
		if g.SourceURL != want {
			t.Errorf("%s: %q, ожидалось %q", rel, g.SourceURL, want)
		}
	}
}

func TestScanSourceStreamAndInvalidate(t *testing.T) {
	dir := t.TempDir()
	p := writeZip(t, dir, "g.zip", file{"1.png", pngBytes(t, 2, 2)})
	// сайт обрезал Referer до адреса сайта
	if err := os.WriteFile(p+":Zone.Identifier", []byte("[ZoneTransfer]\r\nZoneId=3\r\nReferrerUrl=https://site.example/\r\n"), 0o644); err != nil {
		t.Skipf("нет альтернативных потоков: %v", err)
	}
	src := NewSource(storage.NewFS(dir), nil)
	src.Scan(context.Background())
	if g, _ := src.Get(model.LocalKey("g.zip")); g.SourceURL != "https://site.example/" {
		t.Fatalf("до записи адреса: %q", g.SourceURL)
	}

	// браузер сообщил адрес страницы: поток важнее ReferrerUrl
	info := stat(t, p)
	if err := storage.WriteSourceURL(p, "https://site.example/g/535147/"); err != nil {
		t.Fatal(err)
	}
	setMtime(t, p, info.ModTime()) // даже если время изменения файла не изменилось
	src.Invalidate("g.zip")
	res, _ := src.Scan(context.Background())
	if g, _ := src.Get(model.LocalKey("g.zip")); g.SourceURL != "https://site.example/g/535147/" {
		t.Fatalf("после Invalidate: %q", g.SourceURL)
	}
	if len(res.Changed) != 1 || len(res.Added) != 0 {
		t.Errorf("галерея должна считаться изменённой: added=%d changed=%d", len(res.Added), len(res.Changed))
	}
	// Invalidate неизвестного файла — без последствий
	src.Invalidate("нет.zip")
}

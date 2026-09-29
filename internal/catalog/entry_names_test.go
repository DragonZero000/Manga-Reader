//go:build sqlite_fts5

package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/library"
	"mangareader/internal/model"
)

// Ревизия правил разбора: нет значения → 0; пишется в транзакции Save;
// Reset (смена папки) её не трогает; переживает повторное открытие.
func TestParseRevision(t *testing.T) {
	db := filepath.Join(t.TempDir(), FileName)
	c := openTest(t, db, "root")
	st := c.ScanStore()
	if rev, err := st.ParseRevision(); err != nil || rev != 0 {
		t.Fatalf("новый каталог: ревизия %d, %v", rev, err)
	}
	if err := st.Save(library.ScanDelta{ParseRevision: library.ParseRevision}); err != nil {
		t.Fatal(err)
	}
	if err := c.Reset("other"); err != nil {
		t.Fatal(err)
	}
	c.Close()

	c2 := openTest(t, db, "other")
	if rev, err := c2.ScanStore().ParseRevision(); err != nil || rev != library.ParseRevision {
		t.Fatalf("после Reset и открытия: ревизия %d, %v", rev, err)
	}
}

// writeSJISZip — архив с двумя страницами в папках, названных в Shift-JIS
// («作品», «漫画»): после сохранения в каталог и «перезапуска» имена страниц
// должны остаться побайтно прежними и различными.
func writeSJISZip(t *testing.T, dir string) (names []string, data [][]byte) {
	t.Helper()
	names = []string{
		string([]byte{0x8D, 0xEC, 0x95, 0x69}) + "/001.png",
		string([]byte{0x96, 0x9F, 0x89, 0xE6}) + "/001.png",
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, n := range names {
		var img bytes.Buffer
		if err := png.Encode(&img, image.NewGray(image.Rect(0, 0, i+2, i+3))); err != nil {
			t.Fatal(err)
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, NonUTF8: true, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		w.Write(img.Bytes())
		data = append(data, img.Bytes())
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sjis.zip"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return names, data
}

// Имена не в UTF-8 после перезапуска: галерея из каталога открывает каждую
// страницу своей записью.
func TestNonUTF8NamesSurviveRestart(t *testing.T) {
	dir, db := t.TempDir(), filepath.Join(t.TempDir(), FileName)
	names, data := writeSJISZip(t, dir)

	c1 := openTest(t, db, dir)
	if _, err := sourceOver(c1, dir).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	c1.Close()

	c2 := openTest(t, db, dir)
	src := sourceOver(c2, dir)
	res := src.LoadCatalog()
	if len(res.Galleries) != 1 {
		t.Fatalf("галерей из каталога: %d", len(res.Galleries))
	}
	g := res.Galleries[0]
	if len(g.Pages) != 2 || g.Pages[0].Name != names[0] || g.Pages[1].Name != names[1] {
		t.Fatalf("страницы из каталога %q, ожидались %q", g.Pages, names)
	}
	for i, n := range names {
		rc, err := src.OpenPage(model.LocalKey("sjis.zip"), n)
		if err != nil {
			t.Fatalf("%q: %v", n, err)
		}
		got, _ := io.ReadAll(rc)
		rc.Close()
		if !bytes.Equal(got, data[i]) {
			t.Errorf("%q: прочитаны не те байты", n)
		}
	}
}

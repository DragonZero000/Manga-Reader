package library

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/storage"
)

func scanDir(t *testing.T, dir string) ScanResult {
	t.Helper()
	res, err := NewScanner(storage.NewFS(dir)).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// Zip открывается при любом расширении, тип определяется по содержимому.
func TestScanZipAnyExtension(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "a.cbz", file{"1.jpg", img})
	writeZip(t, dir, "b.CBZ", file{"1.jpg", img})
	writeZip(t, dir, "book.dat", file{"1.jpg", img})
	writeZip(t, dir, "noext", file{"1.jpg", img})
	res := scanDir(t, dir)
	if len(res.Galleries) != 4 || len(res.Errors) != 0 {
		t.Fatalf("галереи %v, ошибки %v", keys(res.Galleries), res.Errors)
	}
	for _, k := range keys(res.Galleries) {
		switch k {
		case "local:a.cbz", "local:b.CBZ", "local:book.dat", "local:noext":
		default:
			t.Errorf("неожиданный ключ %s", k)
		}
	}
}

// .zip, внутри которого картинка: причина — изображение.
func TestScanFakeZip(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "fake.zip", pngBytes(t, 2, 2))
	res := scanDir(t, dir)
	var ue *UnsupportedError
	if len(res.Errors) != 1 || !errors.As(res.Errors[0].Err, &ue) || ue.Kind != KindImage {
		t.Fatalf("ошибки: %v", res.Errors)
	}
}

// Образец .cbz из testdata открывается.
func TestScanCbzExample(t *testing.T) {
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "cbz-example.cbz"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "cbz-example.cbz", data)
	res := scanDir(t, dir)
	if len(res.Galleries) != 1 || res.Galleries[0].Key.String() != "local:cbz-example.cbz" || len(res.Errors) != 0 {
		t.Fatalf("галереи %v, ошибки %v", keys(res.Galleries), res.Errors)
	}
}

// Прежняя ошибка «zip с расширением .cbz» перепроверяется, остальные
// неизменённые файлы не открываются.
func TestRecheckOldCbzError(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	p := writeZip(t, dir, "old.cbz", file{"1.jpg", img})
	q := writeFile(t, dir, "other.jpg", img)
	pi, qi := stat(t, p), stat(t, q)
	store := newMemStore()
	store.entries["old.cbz"] = StoredEntry{Size: pi.Size(), ModTime: pi.ModTime(), Err: RestoreError("unsupported:1:.cbz", "zip archive with extension \".cbz\" - only .zip files are supported")}
	store.entries["other.jpg"] = StoredEntry{Size: qi.Size(), ModTime: qi.ModTime(), Err: &UnsupportedError{Kind: KindImage}}
	oldRevision(store)

	opens := countOpens(t)
	res, err := NewScannerWithStore(storage.NewFS(dir), store).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 1 || len(res.Added) != 1 || res.Added[0].Key.ID != "old.cbz" {
		t.Fatalf("открыто %d, добавлены %v", opens.Load(), keys(res.Added))
	}
	if len(res.Errors) != 1 || res.Errors[0].RelPath != "other.jpg" {
		t.Errorf("ошибки: %v", res.Errors)
	}
}

// Zip с .webp из temp/ (если есть в рабочей копии) открывается.
func TestScanWebpZipFromTemp(t *testing.T) {
	src := filepath.Join("..", "..", "temp")
	if _, err := os.Stat(src); err != nil {
		t.Skip("temp/ нет")
	}
	const name = "kono_subarashii_sekai_ni_shukufuku_wo_vol24_ch143.zip"
	if _, err := os.Stat(filepath.Join(src, name)); err != nil {
		t.Skip(name + " нет в temp/")
	}
	// в temp/ могут лежать посторонние файлы — важен только этот архив
	res := scanDir(t, src)
	found := false
	for _, g := range res.Galleries {
		if g.Key.ID == name {
			found = true
		}
	}
	for _, e := range res.Errors {
		if e.RelPath == name {
			t.Fatalf("%s: %v", name, e.Err)
		}
	}
	if !found {
		t.Fatalf("галереи %v, ошибки %v", keys(res.Galleries), res.Errors)
	}
}

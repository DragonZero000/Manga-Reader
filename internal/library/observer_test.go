package library

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// recObserver запоминает события; в Scanned проверяет, что список галерей
// источника ещё прежний (наблюдатель вызывается до публикации).
type recObserver struct {
	src       *Source
	scans     []ScanResult
	published []int // len(src.Galleries()) в момент Scanned
	deleted   []string
}

func (o *recObserver) Scanned(res ScanResult) {
	o.scans = append(o.scans, res)
	o.published = append(o.published, len(o.src.Galleries()))
}

func (o *recObserver) Deleted(rel string) { o.deleted = append(o.deleted, rel) }

func TestObserverScanned(t *testing.T) {
	dir := t.TempDir()
	a := copyExample(t, dir, "a.zip")
	writeFile(t, dir, "broken.zip", []byte("не zip"))
	src := NewSource(storage.NewFS(dir), search.NewMemIndex())
	o := &recObserver{src: src}
	src.SetObserver(o)

	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(o.scans) != 1 {
		t.Fatalf("вызовов Scanned: %d", len(o.scans))
	}
	res := o.scans[0]
	if len(res.Galleries) != 1 || res.Galleries[0].Key.ID != "a.zip" || res.Galleries[0].Fingerprint == "" {
		t.Fatalf("галереи: %+v", res.Galleries)
	}
	if len(res.Errors) != 1 || res.Errors[0].RelPath != "broken.zip" {
		t.Fatalf("ошибки: %+v", res.Errors)
	}
	if o.published[0] != 0 {
		t.Fatalf("Scanned вызван после публикации списка: %d галерей", o.published[0])
	}

	// неуспешный обход (папка пропала) — наблюдатель не вызывается
	os.Remove(a)
	os.Remove(filepath.Join(dir, "broken.zip"))
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Scan(context.Background()); err == nil {
		t.Fatal("ожидалась ошибка обхода")
	}
	if len(o.scans) != 1 {
		t.Fatalf("Scanned вызван при ошибке обхода: %d", len(o.scans))
	}

	// отписка
	src.SetObserver(nil)
	os.MkdirAll(dir, 0o755)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(o.scans) != 1 {
		t.Fatal("отписанный наблюдатель вызван")
	}
}

func TestObserverDeleted(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	writeZip(t, dir, "series/gone.zip", file{"1.png", img})
	src := NewSource(removeFS{storage.NewFS(dir), dir}, search.NewMemIndex())
	o := &recObserver{src: src}
	src.SetObserver(o)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := src.Delete(model.LocalKey("missing.zip"), true); err == nil {
		t.Fatal("удаление несуществующего файла должно завершиться ошибкой")
	}
	if len(o.deleted) != 0 {
		t.Fatalf("Deleted при неудачном удалении: %v", o.deleted)
	}
	if err := src.Delete(model.LocalKey("series/gone.zip"), true); err != nil {
		t.Fatal(err)
	}
	if len(o.deleted) != 1 || o.deleted[0] != "series/gone.zip" {
		t.Fatalf("Deleted: %v", o.deleted)
	}
}

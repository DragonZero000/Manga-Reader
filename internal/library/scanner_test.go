package library

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// countOpens считает открытия архивов на время теста.
func countOpens(t *testing.T) *atomic.Int32 {
	var n atomic.Int32
	openHook = func() { n.Add(1) }
	t.Cleanup(func() { openHook = nil })
	return &n
}

func keys(gs []model.Gallery) []string {
	var out []string
	for _, g := range gs {
		out = append(out, g.Key.String())
	}
	return out
}

func TestScanFindsArchives(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	copyExample(t, dir, "example.zip")
	writeZip(t, dir, "series/vol1.ZIP", file{"1.png", img})
	writeFile(t, dir, "notes.txt", []byte("x"))
	writeZip(t, dir, ".hidden.zip", file{"1.png", img})
	writeZip(t, dir, ".trash/old.zip", file{"1.png", img})

	res, err := NewScanner(storage.NewFS(dir)).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, k := range keys(res.Galleries) {
		got[k] = true
	}
	if len(got) != 2 || !got["local:example.zip"] || !got["local:series/vol1.ZIP"] {
		t.Fatalf("галереи: %v", keys(res.Galleries))
	}
	if len(res.Added) != 2 {
		t.Fatalf("added=%d", len(res.Added))
	}
	// посторонний файл — ошибка, скрытые — пропущены
	if len(res.Errors) != 1 || res.Errors[0].RelPath != "notes.txt" {
		t.Fatalf("errors=%v", res.Errors)
	}
	var ue *UnsupportedError
	if !errors.As(res.Errors[0].Err, &ue) || ue.Kind != KindText {
		t.Fatalf("notes.txt: %v", res.Errors[0].Err)
	}
	if got := res.Errors[0].Err.Error(); got != "Текстовый файл — поддерживаются только zip-архивы" {
		t.Errorf("причина: %q", got)
	}
}

func TestScanEmptyAndMissing(t *testing.T) {
	res, err := NewScanner(storage.NewFS(t.TempDir())).Scan(context.Background())
	if err != nil || len(res.Galleries) != 0 {
		t.Fatalf("пустая папка: %v, %v", res.Galleries, err)
	}
	_, err = NewScanner(storage.NewFS(filepath.Join(t.TempDir(), "нет"))).Scan(context.Background())
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("несуществующая папка: ожидалась ErrUnavailable, получено %v", err)
	}
	// источник без папки (Android до выбора)
	if _, err := NewSource(nil, nil).Scan(context.Background()); !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("без хранилища: %v", err)
	}
}

func TestScanBrokenArchivesDoNotStop(t *testing.T) {
	dir := t.TempDir()
	copyExample(t, dir, "example.zip")
	broken := writeFile(t, dir, "broken.zip", []byte{0x00, 0x01, 0x02, 0xff, 0xfe})
	writeZip(t, dir, "meta-only.zip", file{"meta.json", []byte(`{}`)})

	s := NewScanner(storage.NewFS(dir))
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Galleries) != 1 || len(res.Errors) != 2 || len(res.NewErrors) != 2 {
		t.Fatalf("galleries=%v errors=%v", keys(res.Galleries), res.Errors)
	}
	if !errors.Is(res.Errors[0].Err, ErrNotZip) || res.Errors[0].RelPath != "broken.zip" {
		t.Errorf("ошибка broken.zip: %+v", res.Errors[0])
	}
	if info := stat(t, broken); res.Errors[0].Size != info.Size() || !res.Errors[0].ModTime.Equal(info.ModTime()) {
		t.Errorf("размер и время broken.zip: %+v", res.Errors[0])
	}
	if !errors.Is(res.Errors[1].Err, ErrNoImages) {
		t.Errorf("ошибка meta-only.zip: %+v", res.Errors[1])
	}

	// повторный скан: ошибки известны, но не «новые»
	res, _ = s.Scan(context.Background())
	if len(res.Errors) != 2 || len(res.NewErrors) != 0 {
		t.Fatalf("повторный скан: errors=%d new=%d", len(res.Errors), len(res.NewErrors))
	}
}

func TestScanIncremental(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	a := writeZip(t, dir, "a.zip", file{"1.png", img})
	writeZip(t, dir, "c.zip", file{"1.png", img})
	opens := countOpens(t)
	s := NewScanner(storage.NewFS(dir))

	if _, err := s.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := opens.Load(); n != 2 {
		t.Fatalf("первый скан: открытий %d", n)
	}

	res, _ := s.Scan(context.Background())
	if n := opens.Load(); n != 2 {
		t.Fatalf("без изменений архивы открывались повторно: %d", n)
	}
	if len(res.Galleries) != 2 || len(res.Added)+len(res.Changed)+len(res.Removed) != 0 {
		t.Fatalf("повторный скан: %+v", res)
	}

	if err := os.Remove(a); err != nil {
		t.Fatal(err)
	}
	writeZip(t, dir, "b.zip", file{"1.png", img})
	c := filepath.Join(dir, "c.zip")
	setMtime(t, c, time.Now().Add(time.Hour)) // «изменён»

	res, _ = s.Scan(context.Background())
	if got := keys(res.Galleries); len(got) != 2 || got[0] != "local:c.zip" || got[1] != "local:b.zip" {
		t.Fatalf("галереи: %v", got)
	}
	if len(res.Added) != 1 || res.Added[0].Key.ID != "b.zip" {
		t.Errorf("added: %v", keys(res.Added))
	}
	if len(res.Changed) != 1 || res.Changed[0].Key.ID != "c.zip" {
		t.Errorf("changed: %v", keys(res.Changed))
	}
	if len(res.Removed) != 1 || res.Removed[0].ID != "a.zip" {
		t.Errorf("removed: %v", res.Removed)
	}
	if n := opens.Load(); n != 4 {
		t.Errorf("открыты должны быть только b.zip и c.zip: всего %d", n)
	}
}

func TestScanOrder(t *testing.T) {
	dir := t.TempDir()
	img := pngBytes(t, 2, 2)
	base := time.Now().Add(-time.Hour)
	setMtime(t, writeZip(t, dir, "old.zip", file{"1.png", img}), base)
	setMtime(t, writeZip(t, dir, "new.zip", file{"1.png", img}), base.Add(time.Minute))
	setMtime(t, writeZip(t, dir, "v10.zip", file{"1.png", img}), base)
	setMtime(t, writeZip(t, dir, "v2.zip", file{"1.png", img}), base)

	res, err := NewScanner(storage.NewFS(dir)).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"local:new.zip", "local:old.zip", "local:v2.zip", "local:v10.zip"}
	got := keys(res.Galleries)
	for i := range want {
		if i >= len(got) || got[i] != want[i] {
			t.Fatalf("порядок %v, ожидался %v", got, want)
		}
	}
}

// recIndex записывает вызовы индекса.
type recIndex struct {
	search.NopIndex
	mu       sync.Mutex
	upserted []model.Key
	removed  []model.Key
}

func (r *recIndex) Upsert(_ context.Context, g model.Gallery) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upserted = append(r.upserted, g.Key)
	return nil
}

func (r *recIndex) Remove(_ context.Context, k model.Key) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.removed = append(r.removed, k)
	return errors.New("ошибка индекса не должна прерывать скан")
}

func TestSourceScanAndOpenPage(t *testing.T) {
	dir := t.TempDir()
	p := copyExample(t, dir, "example.zip")
	idx := &recIndex{}
	src := NewDirSource(dir, idx)

	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	k := model.LocalKey("example.zip")
	g, ok := src.Get(k)
	if !ok || len(src.Galleries()) != 1 {
		t.Fatal("галерея не найдена")
	}
	if len(idx.upserted) != 1 || idx.upserted[0] != k {
		t.Fatalf("Upsert: %v", idx.upserted)
	}

	rc, err := src.OpenPage(k, g.Pages[0].Name)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(rc)
	rc.Close()
	want := readFromZip(t, p, "example/1.jpg")
	if !bytes.Equal(got, want) || len(got) == 0 {
		t.Fatalf("байты страницы не совпадают: %d vs %d", len(got), len(want))
	}

	if _, err := src.OpenPage(k, "example/99.jpg"); err == nil {
		t.Error("неизвестная страница: ожидалась ошибка")
	}
	if _, err := src.OpenPage(model.LocalKey("nope.zip"), "1.jpg"); err == nil {
		t.Error("неизвестная галерея: ожидалась ошибка")
	}

	// архив не заблокирован: его можно удалить (важно для Windows)
	if err := os.Remove(p); err != nil {
		t.Fatalf("архив заблокирован: %v", err)
	}
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(src.Galleries()) != 0 || len(idx.removed) != 1 {
		t.Fatalf("после удаления: %v, removed %v", keys(src.Galleries()), idx.removed)
	}
}

func readFromZip(t *testing.T, zipPath, name string) []byte {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	f, err := zr.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f)
	return data
}

func TestSourceSetStorage(t *testing.T) {
	a, b := t.TempDir(), t.TempDir()
	copyExample(t, a, "example.zip")
	writeZip(t, b, "other.zip", file{"1.png", pngBytes(t, 2, 2)})
	idx := &recIndex{}
	src := NewDirSource(a, idx)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	src.SetStorage(storage.NewFS(b))
	if len(src.Galleries()) != 0 || len(idx.removed) != 1 || idx.removed[0].ID != "example.zip" {
		t.Fatalf("после смены папки: %v, removed %v", keys(src.Galleries()), idx.removed)
	}
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := keys(src.Galleries()); len(got) != 1 || got[0] != "local:other.zip" || src.Root() != b {
		t.Fatalf("новая папка: %v root=%q", got, src.Root())
	}
}

func TestSourceConcurrent(t *testing.T) {
	dir := t.TempDir()
	copyExample(t, dir, "example.zip")
	src := NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	k := model.LocalKey("example.zip")

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			page := []string{"example/1.jpg", "example/2.jpg"}[i%2]
			rc, err := src.OpenPage(k, page)
			if err != nil {
				t.Error(err)
				return
			}
			io.Copy(io.Discard, rc)
			rc.Close()
			src.Galleries()
			src.Scan(context.Background()) // параллельный скан допустим: вернёт ErrScanInProgress
		}(i)
	}
	wg.Wait()
}

func TestSourceScanInProgress(t *testing.T) {
	src := NewDirSource(t.TempDir(), nil)
	src.scanMu.Lock() // имитация идущего сканирования
	_, err := src.Scan(context.Background())
	src.scanMu.Unlock()
	if !errors.Is(err, ErrScanInProgress) {
		t.Fatalf("ожидалась ErrScanInProgress, получено %v", err)
	}
	if _, err := NewDirSource("", nil).Scan(context.Background()); err == nil {
		t.Fatal("пустой путь: ожидалась ошибка")
	}
}

package library

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// userTags — наложение в памяти по rel; как user.db, переносит данные по
// отпечатку при сверке (Observer.Scanned).
type userTags struct {
	mu    sync.Mutex
	byRel map[string][]model.Tag // rel → свои теги
	fps   map[string]string      // rel → отпечаток
	calls int
}

func (u *userTags) Apply(gs []model.Gallery) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	for i := range gs {
		gs[i].Custom = append([]model.Tag(nil), u.byRel[gs[i].Key.ID]...)
		gs[i].Hidden = nil
	}
	return nil
}

func (u *userTags) set(rel, fp string, tags ...model.Tag) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.byRel[rel], u.fps[rel] = tags, fp
}

// Scanned: пропавший файл с тем же отпечатком, что у нового, переносится.
func (u *userTags) Scanned(res ScanResult) {
	u.mu.Lock()
	defer u.mu.Unlock()
	present := map[string]string{}
	for _, g := range res.Galleries {
		present[g.Key.ID] = g.Fingerprint
	}
	for rel, fp := range u.fps {
		if _, ok := present[rel]; ok {
			continue
		}
		for newRel, newFP := range present {
			if _, known := u.fps[newRel]; !known && newFP == fp {
				u.byRel[newRel], u.fps[newRel] = u.byRel[rel], fp
				delete(u.byRel, rel)
				delete(u.fps, rel)
			}
		}
	}
}

func (u *userTags) Deleted(string) {}

func newUserTags() *userTags {
	return &userTags{byRel: map[string][]model.Tag{}, fps: map[string]string{}}
}

func find(t *testing.T, idx search.Index, query string) []string {
	t.Helper()
	q, err := search.Parse(query)
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := idx.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, k := range keys {
		out = append(out, k.ID)
	}
	return out
}

// Перенесённый по отпечатку файл индексируется со своими тегами в том же скане.
func TestOverlayFollowsMoveInSameScan(t *testing.T) {
	dir := t.TempDir()
	a := copyExample(t, dir, "a.zip")
	idx := search.NewMemIndex()
	src := NewSource(storage.NewFS(dir), idx)
	u := newUserTags()
	src.SetObserver(u)
	src.SetOverlay(u)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, _ := src.Get(model.LocalKey("a.zip"))
	u.set("a.zip", g.Fingerprint, model.NewTag("tag", "my-fav"))
	if _, err := src.Refresh(g.Key); err != nil {
		t.Fatal(err)
	}
	if got := find(t, idx, "custom-tag:my-fav"); !reflect.DeepEqual(got, []string{"a.zip"}) {
		t.Fatalf("после Refresh: %v", got)
	}

	if err := os.Rename(a, filepath.Join(dir, "b.zip")); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := find(t, idx, "custom-tag:my-fav"); !reflect.DeepEqual(got, []string{"b.zip"}) {
		t.Fatalf("после переименования: %v", got)
	}
	b, ok := src.Get(model.LocalKey("b.zip"))
	if !ok || len(b.Custom) != 1 || b.Custom[0] != model.NewTag("tag", "my-fav") {
		t.Fatalf("галерея после переименования: %+v", b.Custom)
	}
}

// LoadCatalog применяет наложение к галереям из хранилища, Reindex — к
// индексу.
func TestOverlayLoadCatalogAndReindex(t *testing.T) {
	dir := t.TempDir()
	copyExample(t, dir, "a.zip")
	store := newMemStore()
	idx := search.NewMemIndex()
	src := NewSourceWithStore(storage.NewFS(dir), idx, store)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}

	u := newUserTags()
	u.set("a.zip", "", model.NewTag("character", "alice"))
	src2 := NewSourceWithStore(storage.NewFS(dir), idx, store)
	src2.SetOverlay(u)
	src2.LoadCatalog()
	if g, _ := src2.Get(model.LocalKey("a.zip")); len(g.Custom) != 1 {
		t.Fatalf("LoadCatalog без наложения: %+v", g.Custom)
	}
	if got := find(t, idx, "custom-tag:alice"); len(got) != 0 {
		t.Fatalf("LoadCatalog изменил индекс: %v", got)
	}
	if err := src2.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := find(t, idx, "custom-tag:alice"); len(got) != 1 {
		t.Fatalf("после Reindex: %v", got)
	}

	// наложение пропало (пользовательские данные заменены) — Reindex убирает его
	src2.SetOverlay(nil)
	if err := src2.Reindex(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := find(t, idx, "custom-tag:alice"); len(got) != 0 {
		t.Fatalf("после Reindex без наложения: %v", got)
	}
	if g, _ := src2.Get(model.LocalKey("a.zip")); len(g.Custom) != 0 {
		t.Fatalf("галерея после Reindex без наложения: %+v", g.Custom)
	}
}

// blockFS задерживает обход папки до закрытия release.
type blockFS struct {
	*storage.FS
	started chan struct{}
	release chan struct{}
}

func (b blockFS) Walk(ctx context.Context, fn func(storage.Entry) error) error {
	select {
	case b.started <- struct{}{}:
	default:
	}
	<-b.release
	return b.FS.Walk(ctx, fn)
}

// Refresh во время сканирования ждёт его и применяется после него.
func TestRefreshWaitsForScan(t *testing.T) {
	dir := t.TempDir()
	copyExample(t, dir, "a.zip")
	idx := search.NewMemIndex()
	release := make(chan struct{})
	close(release)
	fs := blockFS{storage.NewFS(dir), make(chan struct{}, 1), release}
	src := NewSource(fs, idx)
	u := newUserTags()
	src.SetOverlay(u)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-fs.started

	// второе сканирование: файл изменён, обход задержан
	release2 := make(chan struct{})
	src.stMu.Lock()
	fs2 := blockFS{fs.FS, make(chan struct{}, 1), release2}
	src.st, src.scanner = fs2, NewScanner(fs2)
	src.stMu.Unlock()
	setMtime(t, filepath.Join(dir, "a.zip"), time.Now().Add(time.Hour))

	scanDone := make(chan error, 1)
	go func() { _, err := src.Scan(context.Background()); scanDone <- err }()
	<-fs2.started

	u.set("a.zip", "", model.NewTag("tag", "late"))
	refreshed := make(chan error, 1)
	go func() { _, err := src.Refresh(model.LocalKey("a.zip")); refreshed <- err }()
	select {
	case <-refreshed:
		t.Fatal("Refresh не дождался сканирования")
	case <-time.After(50 * time.Millisecond):
	}
	close(release2)
	if err := <-scanDone; err != nil {
		t.Fatal(err)
	}
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if got := find(t, idx, "custom-tag:late"); !reflect.DeepEqual(got, []string{"a.zip"}) {
		t.Fatalf("после сканирования и Refresh: %v", got)
	}
	if g, _ := src.Get(model.LocalKey("a.zip")); len(g.Custom) != 1 {
		t.Fatalf("галерея: %+v", g.Custom)
	}
}

// gateOverlay задерживает Apply до закрытия release.
type gateOverlay struct {
	started chan struct{}
	release chan struct{}
}

func (g gateOverlay) Apply([]model.Gallery) error {
	g.started <- struct{}{}
	<-g.release
	return nil
}

// Сканирование во время Reindex ждёт его, а не отменяется.
func TestScanWaitsForReindex(t *testing.T) {
	dir := t.TempDir()
	copyExample(t, dir, "a.zip")
	src := NewSource(storage.NewFS(dir), search.NewMemIndex())
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	gate := gateOverlay{make(chan struct{}, 1), make(chan struct{})}
	src.SetOverlay(gate)
	reindexed := make(chan error, 1)
	go func() { reindexed <- src.Reindex(context.Background()) }()
	<-gate.started

	scanned := make(chan error, 1)
	go func() { _, err := src.Scan(context.Background()); scanned <- err }()
	select {
	case err := <-scanned:
		t.Fatalf("сканирование не дождалось Reindex: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(gate.release)
	if err := <-reindexed; err != nil {
		t.Fatal(err)
	}
	<-gate.started // Apply сканирования
	if err := <-scanned; err != nil {
		t.Fatalf("сканирование после Reindex: %v", err)
	}
}

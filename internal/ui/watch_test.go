package ui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// busyFS — папка, часть файлов которой «занята другим процессом».
type busyFS struct {
	storage.Storage
	mu   sync.Mutex
	busy map[string]bool
}

func (b *busyFS) set(rel string, on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.busy[rel] = on
}

func (b *busyFS) Open(rel string) (storage.File, error) {
	b.mu.Lock()
	busy := b.busy[rel]
	b.mu.Unlock()
	if busy {
		return nil, errors.Join(storage.ErrBusy, errors.New("sharing violation"))
	}
	return b.Storage.Open(rel)
}

// libShell — оболочка над хранилищем st; UI-функции копятся в очереди.
type libShell struct {
	*Shell
	src *library.Source
	q   chan func()
}

func newLibShell(t *testing.T, st storage.Storage) *libShell {
	t.Helper()
	a := test.NewTempApp(t)
	src := library.NewSource(st, nil)
	s := NewShell(a, app.NewForTest(src, search.NopIndex{}, storage.NewMemSettings()))
	l := &libShell{Shell: s, src: src, q: make(chan func(), 256)}
	do := func(f func()) { l.q <- f }
	s.library.SetDispatcher(do)
	s.Search().SetDispatcher(do)
	s.Toast.do = func(func()) {}
	return l
}

// runUntil выполняет UI-функции из очереди, пока cond не станет истинным.
func (l *libShell) runUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		select {
		case f := <-l.q:
			f()
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func TestRequestScanDuringScan(t *testing.T) {
	dir := t.TempDir()
	st := &countFS{Storage: storage.NewFS(dir)}
	l := newLibShell(t, st)

	l.library.Refresh()     // идёт скан (результат ещё в очереди)
	l.library.RequestScan() // событие во время скана
	l.library.RequestScan() // и ещё одно — объединяется
	if err := os.WriteFile(filepath.Join(dir, "late.zip"), exampleZip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	l.runUntil(t, "late.zip в библиотеке", func() bool {
		_, ok := l.src.Get(model.LocalKey("late.zip"))
		return ok
	})
	time.Sleep(100 * time.Millisecond)
	for len(l.q) > 0 {
		(<-l.q)()
	}
	if n := st.walks(); n != 2 {
		t.Fatalf("сканирований %d, ожидалось 2 (исходное и одно дополнительное)", n)
	}
	if st.maxParallel() != 1 {
		t.Fatalf("одновременных сканирований: %d", st.maxParallel())
	}
}

func TestBusyFileRetries(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "g-4.zip"), exampleZip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &busyFS{Storage: storage.NewFS(dir), busy: map[string]bool{"g-4.zip": true}}
	l := newLibShell(t, st)
	l.library.SetBusyRetryDelay(20 * time.Millisecond)

	// антивирус держит файл недолго: ошибки нет, галерея появляется
	l.library.Refresh()
	l.runUntil(t, "первый скан", func() bool { return !l.library.Scanning() })
	if l.svc.Problems.Unseen() != 0 {
		t.Fatal("занятый файл не должен сразу становиться ошибкой")
	}
	st.set("g-4.zip", false)
	l.runUntil(t, "галерея после освобождения", func() bool {
		_, ok := l.src.Get(model.LocalKey("g-4.zip"))
		return ok
	})
	if l.svc.Problems.Unseen() != 0 {
		t.Fatal("после освобождения ошибок быть не должно")
	}
}

func TestBusyFileBecomesError(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "locked.zip"), exampleZip(t), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &busyFS{Storage: storage.NewFS(dir), busy: map[string]bool{"locked.zip": true}}
	l := newLibShell(t, st)
	l.library.SetBusyRetryDelay(10 * time.Millisecond)

	l.library.Refresh()
	l.runUntil(t, "ошибка «Файл занят»", func() bool {
		items := l.svc.Problems.Items()
		return len(items) == 1 && items[0].Reason == "Файл занят другой программой"
	})
	// повторы прекращаются
	time.Sleep(100 * time.Millisecond)
	for len(l.q) > 0 {
		(<-l.q)()
	}
	if l.library.Scanning() {
		t.Fatal("после исчерпания повторов сканирование не должно продолжаться")
	}
}

// countFS считает обходы папки и их одновременность.
type countFS struct {
	storage.Storage
	mu       sync.Mutex
	n, cur   int
	parallel int
}

func (c *countFS) walks() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

func (c *countFS) maxParallel() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.parallel
}

func exampleZip(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (c *countFS) Walk(ctx context.Context, fn func(storage.Entry) error) error {
	c.mu.Lock()
	c.n++
	c.cur++
	c.parallel = max(c.parallel, c.cur)
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		c.cur--
		c.mu.Unlock()
	}()
	return c.Storage.Walk(ctx, fn)
}

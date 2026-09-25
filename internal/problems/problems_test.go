package problems

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mangareader/internal/library"
	"mangareader/internal/storage"
)

var t0 = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func se(rel string, size int64, mod time.Time) library.ScanError {
	return library.ScanError{RelPath: rel, Size: size, ModTime: mod, Err: errors.New("причина " + rel)}
}

func paths(items []Item) string {
	var out []string
	for _, it := range items {
		out = append(out, it.RelPath)
	}
	return strings.Join(out, ",")
}

func TestSyncNoDuplicatesAndOrder(t *testing.T) {
	tr := New(storage.NewMemSettings())
	errs := []library.ScanError{se("a.jpg", 1, t0), se("b.mp4", 2, t0.Add(time.Hour)), se("v10.txt", 3, t0), se("v2.txt", 3, t0)}
	if n := tr.Sync(errs); n != 4 {
		t.Fatalf("новых: %d", n)
	}
	for i := 0; i < 2; i++ {
		if n := tr.Sync(errs); n != 0 {
			t.Fatalf("повторный Sync: новых %d", n)
		}
	}
	if got := paths(tr.Items()); got != "b.mp4,a.jpg,v2.txt,v10.txt" {
		t.Fatalf("порядок: %s", got)
	}
	if tr.Unseen() != 4 || tr.Items()[0].Reason != "причина b.mp4" {
		t.Fatalf("unseen=%d items=%+v", tr.Unseen(), tr.Items())
	}
}

func TestMarkAllSeenAndRemoval(t *testing.T) {
	tr := New(storage.NewMemSettings())
	tr.Sync([]library.ScanError{se("a.jpg", 1, t0), se("b.jpg", 1, t0)})
	tr.MarkAllSeen()
	if tr.Unseen() != 0 || !tr.Items()[0].Seen {
		t.Fatalf("после просмотра: %+v", tr.Items())
	}
	// файл удалён или исправлен — запись исчезает
	tr.Sync([]library.ScanError{se("b.jpg", 1, t0)})
	if got := paths(tr.Items()); got != "b.jpg" {
		t.Fatalf("после удаления: %s", got)
	}
	tr.Sync(nil)
	if len(tr.Items()) != 0 || tr.Unseen() != 0 {
		t.Fatal("пустой список ожидался")
	}
}

func TestRestart(t *testing.T) {
	st := storage.NewMemSettings()
	tr := New(st)
	tr.Sync([]library.ScanError{se("seen.jpg", 1, t0), se("unseen.jpg", 1, t0)})
	tr.MarkAllSeen()
	tr.Sync([]library.ScanError{se("seen.jpg", 1, t0), se("unseen.jpg", 1, t0), se("later.jpg", 1, t0)})

	// перезапуск: новый трекер над теми же настройками
	tr = New(st)
	n := tr.Sync([]library.ScanError{se("seen.jpg", 1, t0), se("unseen.jpg", 1, t0), se("later.jpg", 1, t0)})
	if n != 0 {
		t.Fatalf("после перезапуска известные ошибки считаются новыми: %d", n)
	}
	if tr.Unseen() != 1 {
		t.Fatalf("непросмотренных %d, ожидалась 1 (later.jpg)", tr.Unseen())
	}
	for _, it := range tr.Items() {
		if it.Seen != (it.RelPath != "later.jpg") {
			t.Errorf("%s: seen=%v", it.RelPath, it.Seen)
		}
	}
}

func TestChangedFileIsNew(t *testing.T) {
	tr := New(storage.NewMemSettings())
	tr.Sync([]library.ScanError{se("g-1.zip", 100, t0)})
	tr.MarkAllSeen()
	n := tr.Sync([]library.ScanError{se("g-1.zip", 200, t0.Add(time.Minute))})
	if n != 1 || tr.Unseen() != 1 || len(tr.Items()) != 1 {
		t.Fatalf("перекачанный файл: новых %d, unseen %d, items %d", n, tr.Unseen(), len(tr.Items()))
	}
}

// countSettings считает записи настроек.
type countSettings struct {
	*storage.MemSettings
	mu     sync.Mutex
	writes int
}

func (c *countSettings) SetString(k, v string) {
	c.mu.Lock()
	c.writes++
	c.mu.Unlock()
	c.MemSettings.SetString(k, v)
}

func TestStatePrunedAndWrittenOnlyOnChange(t *testing.T) {
	st := &countSettings{MemSettings: storage.NewMemSettings()}
	tr := New(st)
	errs := []library.ScanError{se("a.jpg", 1, t0)}
	tr.Sync(errs)
	w := st.writes
	tr.Sync(errs)
	tr.Sync(errs)
	if st.writes != w {
		t.Fatalf("запись без изменений: %d → %d", w, st.writes)
	}
	tr.Sync(nil)
	if got := st.String(KeyState, ""); got != "{}" {
		t.Fatalf("состояние не обрезано: %s", got)
	}
}

func TestCorruptState(t *testing.T) {
	st := storage.NewMemSettings()
	st.SetString(KeyState, "не json")
	tr := New(st)
	if n := tr.Sync([]library.ScanError{se("a.jpg", 1, t0)}); n != 1 {
		t.Fatalf("новых %d", n)
	}
}

func TestConcurrent(t *testing.T) {
	tr := New(storage.NewMemSettings())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.Sync([]library.ScanError{se("a.jpg", 1, t0)})
			tr.MarkAllSeen()
			tr.Items()
			tr.Unseen()
		}()
	}
	wg.Wait()
}

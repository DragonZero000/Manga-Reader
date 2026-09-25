package library

import (
	"context"
	"errors"
	"sync"
	"testing"

	"mangareader/internal/storage"
)

// busyStorage — папка, в которой часть файлов «занята другим процессом».
type busyStorage struct {
	storage.Storage
	mu   sync.Mutex
	busy map[string]bool
}

func (b *busyStorage) setBusy(rel string, on bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.busy[rel] = on
}

func (b *busyStorage) Open(rel string) (storage.File, error) {
	b.mu.Lock()
	busy := b.busy[rel]
	b.mu.Unlock()
	if busy {
		return nil, errors.Join(storage.ErrBusy, errors.New("sharing violation"))
	}
	return b.Storage.Open(rel)
}

func TestScanBusyFile(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "g-4.zip", file{"1.png", pngBytes(t, 2, 2)})
	st := &busyStorage{Storage: storage.NewFS(dir), busy: map[string]bool{"g-4.zip": true}}
	src := NewSource(st, nil)

	// занят: ни галереи, ни ошибки, файл в Busy
	res, err := src.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Galleries)+len(res.Errors) != 0 || len(res.Busy) != 1 {
		t.Fatalf("занятый файл: galleries=%v errors=%v busy=%v", keys(res.Galleries), res.Errors, res.Busy)
	}

	// освободился без изменения размера и времени — всё равно проверяется заново
	st.setBusy("g-4.zip", false)
	res, _ = src.Scan(context.Background())
	if got := keys(res.Galleries); len(got) != 1 || len(res.Busy) != 0 || len(res.Added) != 1 {
		t.Fatalf("после освобождения: %v busy=%v", got, res.Busy)
	}
}

func TestScanBusyAsError(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "locked.zip", file{"1.png", pngBytes(t, 2, 2)})
	st := &busyStorage{Storage: storage.NewFS(dir), busy: map[string]bool{"locked.zip": true}}
	src := NewSource(st, nil)
	src.SetBusyAsError(true)

	res, _ := src.Scan(context.Background())
	if len(res.Errors) != 1 || res.Errors[0].Err.Error() != "Файл занят другой программой" || len(res.Busy) != 1 {
		t.Fatalf("errors=%v busy=%v", res.Errors, res.Busy)
	}
	if len(res.NewErrors) != 1 {
		t.Fatalf("новых ошибок %d", len(res.NewErrors))
	}
	// всё ещё занят: ошибка остаётся, но новой не считается
	res, _ = src.Scan(context.Background())
	if len(res.Errors) != 1 || len(res.NewErrors) != 0 {
		t.Fatalf("повторно: errors=%v new=%v", res.Errors, res.NewErrors)
	}
	// освободился — ошибка уходит, галерея появляется (файл не менялся)
	st.setBusy("locked.zip", false)
	src.SetBusyAsError(false)
	res, _ = src.Scan(context.Background())
	if len(res.Errors) != 0 || len(res.Galleries) != 1 {
		t.Fatalf("после освобождения: errors=%v galleries=%v", res.Errors, keys(res.Galleries))
	}
}

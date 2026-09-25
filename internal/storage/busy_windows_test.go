package storage

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestOpenBusyFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "locked.zip")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// открыт другим «процессом» без совместного доступа
	name, _ := syscall.UTF16PtrFromString(p)
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, 0, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = NewFS(dir).Open("locked.zip")
	syscall.CloseHandle(h)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("ожидалась ErrBusy, получено %v", err)
	}
	f, err := NewFS(dir).Open("locked.zip")
	if err != nil {
		t.Fatalf("после освобождения: %v", err)
	}
	f.Close()
}

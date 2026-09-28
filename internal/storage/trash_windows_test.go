package storage

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestFSDeletePermanent(t *testing.T) {
	root := t.TempDir()
	write(t, root, "series/vol 1, part 2.zip", "x")
	st := NewFS(root)
	if err := st.Delete("series/vol 1, part 2.zip", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "series", "vol 1, part 2.zip")); !os.IsNotExist(err) {
		t.Fatalf("файл не удалён: %v", err)
	}
	if err := st.Delete("series/vol 1, part 2.zip", true); err == nil {
		t.Fatal("удаление отсутствующего файла должно давать ошибку")
	}
}

// Путь не выходит за пределы папки библиотеки.
func TestFSDeleteOutside(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "lib")
	write(t, parent, "other.zip", "x")
	write(t, root, "a.zip", "x")
	st := NewFS(root)
	for _, rel := range []string{"../other.zip", `..\other.zip`, "lib/../../other.zip", filepath.Join(parent, "other.zip"), "C:other.zip", ""} {
		if err := st.Delete(rel, true); err == nil {
			t.Errorf("%q: ожидалась ошибка", rel)
		}
		if _, err := st.CanTrash(rel); err == nil {
			t.Errorf("CanTrash %q: ожидалась ошибка", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(parent, "other.zip")); err != nil {
		t.Fatalf("файл вне папки затронут: %v", err)
	}
}

// Файл, открытый другим процессом без совместного доступа, не удаляется ни
// безвозвратно, ни в корзину.
func TestFSDeleteBusy(t *testing.T) {
	root := t.TempDir()
	write(t, root, "locked.zip", "x")
	p := filepath.Join(root, "locked.zip")
	name, _ := syscall.UTF16PtrFromString(p)
	h, err := syscall.CreateFile(name, syscall.GENERIC_READ, syscall.FILE_SHARE_READ, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	st := NewFS(root)
	for _, permanent := range []bool{true, false} {
		if err := st.Delete("locked.zip", permanent); !errors.Is(err, ErrBusy) {
			t.Errorf("permanent=%v: ожидалась ErrBusy, получено %v", permanent, err)
		}
	}
	syscall.CloseHandle(h)
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("занятый файл удалён: %v", err)
	}
	if err := st.Delete("locked.zip", true); err != nil {
		t.Fatalf("после освобождения: %v", err)
	}
}

// На локальном диске временной папки опрос корзины проходит без ошибок
// (есть ли на нём корзина — зависит от машины).
func TestFSCanTrash(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.zip", "x")
	ok, err := NewFS(root).CanTrash("a.zip")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("корзина для %s: %v", root, ok)
	var _ Deleter = NewFS(root)
}

// Перемещает файл в корзину Windows — только вручную:
// MANGAREADER_TRASH=<папка> go test -run TestTrashManual.
func TestTrashManual(t *testing.T) {
	root := os.Getenv("MANGAREADER_TRASH")
	if root == "" {
		t.Skip("кладёт файл в корзину Windows")
	}
	write(t, root, "mangareader trash check.zip", "x")
	st := NewFS(root)
	ok, err := st.CanTrash("mangareader trash check.zip")
	if err != nil || !ok {
		t.Fatalf("CanTrash: %v %v", ok, err)
	}
	if err := st.Delete("mangareader trash check.zip", false); err != nil {
		t.Fatal(err)
	}
}

package thumbs

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func TestDirStoreLoadSave(t *testing.T) {
	s := NewDirStore(filepath.Join(t.TempDir(), "covers"))
	if _, ok := s.Load("local:a.zip@1", 10, 20); ok {
		t.Fatal("пустое хранилище вернуло обложку")
	}
	if n, err := s.Size(); err != nil || n != 0 {
		t.Fatalf("Size до создания папки = %d, %v", n, err)
	}
	s.Save("local:a.zip@1", "a.zip", 10, 20, []byte("jpeg-a"))
	if data, ok := s.Load("local:a.zip@1", 10, 20); !ok || string(data) != "jpeg-a" {
		t.Fatalf("Load = %q, %v", data, ok)
	}
	if _, ok := s.Load("local:a.zip@1", 11, 20); ok {
		t.Fatal("обложка другого размера области использована")
	}
	// другой размер заменяет прежний файл ключа
	s.Save("local:a.zip@1", "a.zip", 11, 20, []byte("jpeg-a2"))
	if _, ok := s.Load("local:a.zip@1", 10, 20); ok {
		t.Fatal("файл прежнего размера не удалён")
	}
	if n, _ := s.Size(); n != int64(len("jpeg-a2")) {
		t.Fatalf("Size = %d, want %d", n, len("jpeg-a2"))
	}
}

func TestDirStorePruneClear(t *testing.T) {
	dir := t.TempDir()
	s := NewDirStore(dir)
	s.Save("local:a.zip@1", "a.zip", 10, 20, []byte("aaaa"))
	s.Save("local:b.zip@1", "b.zip", 10, 20, []byte("bb"))
	s.Save("local:c.zip@1", "c.zip", 10, 20, []byte("c"))
	os.WriteFile(filepath.Join(dir, ".tmp-leftover"), []byte("x"), 0o644)

	// b удалён, c изменён (новое время изменения — другой ключ)
	if err := s.Prune([]string{"local:a.zip@1", "local:c.zip@2"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Load("local:a.zip@1", 10, 20); !ok {
		t.Fatal("обложка существующего архива удалена")
	}
	for _, k := range []string{"local:b.zip@1", "local:c.zip@1"} {
		if _, ok := s.Load(k, 10, 20); ok {
			t.Fatalf("обложка %s не удалена", k)
		}
	}
	if n, _ := s.Size(); n != 4 {
		t.Fatalf("Size после Prune = %d, want 4 (остатки временных файлов тоже удаляются)", n)
	}
	if err := s.Clear(); err != nil {
		t.Fatal(err)
	}
	if n, _ := s.Size(); n != 0 {
		t.Fatalf("Size после Clear = %d", n)
	}
}

func TestDirStoreCache(t *testing.T) {
	// миниатюра из Cache сохраняется в DirStore и читается следующим кэшем без декодирования
	store := NewDirStore(t.TempDir())
	var calls atomic.Int32
	open := fakeOpen(pngOf(t, 40, 60), &calls, 0)
	g := gallery("a.zip")
	c1 := New(open, 0, 1)
	c1.SetStore(store)
	if r := load(c1, g); r.err != nil {
		t.Fatal(r.err)
	}
	if n, _ := store.Size(); n == 0 {
		t.Fatal("миниатюра не сохранена")
	}
	c2 := New(open, 0, 1)
	c2.SetStore(store)
	if r := load(c2, g); r.err != nil || calls.Load() != 1 || c2.storeHits.Load() != 1 {
		t.Fatalf("err=%v, открытий %d, попаданий %d", r.err, calls.Load(), c2.storeHits.Load())
	}
}

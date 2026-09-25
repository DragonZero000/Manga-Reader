package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func write(t *testing.T, root, rel, data string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFSWalkAndOpen(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.zip", "12345")
	write(t, root, "series/vol1.zip", "xy")
	st := NewFS(root)
	if st.Name() != root {
		t.Fatalf("Name = %q", st.Name())
	}
	var files []string
	sizes := map[string]int64{}
	err := st.Walk(context.Background(), func(e Entry) error {
		if !e.IsDir {
			files = append(files, e.RelPath)
			sizes[e.RelPath] = e.Size
			if e.ModTime.IsZero() {
				t.Errorf("нет времени у %s", e.RelPath)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	if len(files) != 2 || files[0] != "a.zip" || files[1] != "series/vol1.zip" || sizes["a.zip"] != 5 {
		t.Fatalf("элементы %v %v", files, sizes)
	}

	f, err := st.Open("series/vol1.zip")
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1)
	if _, err := f.ReadAt(buf, 1); err != nil || buf[0] != 'y' || f.Size() != 2 {
		t.Fatalf("ReadAt: %q %v size=%d", buf, err, f.Size())
	}
	f.Close()

	for _, bad := range []string{"../x", "series/../../x", "", "series"} {
		if _, err := st.Open(bad); err == nil {
			t.Errorf("Open(%q): ожидалась ошибка", bad)
		}
	}
}

func TestFSUnavailable(t *testing.T) {
	for _, st := range []*FS{NewFS(""), NewFS(filepath.Join(t.TempDir(), "нет"))} {
		err := st.Walk(context.Background(), func(Entry) error { return nil })
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("Walk(%q): %v", st.Name(), err)
		}
	}
}

func TestFileSettings(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	s := NewFileSettings(p)
	if got := s.String("reader.mode", "paged"); got != "paged" {
		t.Fatalf("по умолчанию: %q", got)
	}
	s.SetString("reader.mode", "strip")
	if got := NewFileSettings(p).String("reader.mode", "paged"); got != "strip" {
		t.Fatalf("после перезапуска: %q", got)
	}
	data, _ := os.ReadFile(p)
	if want := `"reader.mode": "strip"`; !strings.Contains(string(data), want) {
		t.Fatalf("файл: %s", data)
	}
	// временных файлов не осталось
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("в папке %d файлов", len(entries))
	}
}

func TestFileSettingsCorrupted(t *testing.T) {
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFileSettings(p)
	if got := s.String("reader.mode", "paged"); got != "paged" {
		t.Fatalf("повреждённый файл: %q", got)
	}
	s.SetString("reader.mode", "strip") // перезаписывает
	if got := NewFileSettings(p).String("reader.mode", ""); got != "strip" {
		t.Fatalf("после перезаписи: %q", got)
	}
}

func TestMemSettings(t *testing.T) {
	s := NewMemSettings()
	s.SetString("k", "v")
	if s.String("k", "") != "v" || s.String("x", "d") != "d" {
		t.Fatal("MemSettings")
	}
}

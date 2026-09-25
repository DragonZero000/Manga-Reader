package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppDirFor(t *testing.T) {
	wd := func() (string, error) { return filepath.FromSlash("/work/project"), nil }
	cases := []struct {
		exe, want string
	}{
		{"/apps/MangaReader/mangareader.exe", "/apps/MangaReader"},
		{"/tmp/go-build123/b001/exe/mangareader", "/work/project"}, // go run
		{"/home/u/.cache/go-build/x/mangareader", "/work/project"}, // go run с другим кэшем
		{"/tmp/other/mangareader", "/tmp/other"},                   // распакован во временную папку — это не go run
		{"/apps/go-builder/mangareader", "/work/project"},          // осторожно: префикс сегмента
		{"/apps/my-go-build-tools/mangareader", "/apps/my-go-build-tools"},
	}
	for _, c := range cases {
		got, err := appDirFor(filepath.FromSlash(c.exe), wd)
		if err != nil || got != filepath.FromSlash(c.want) {
			t.Errorf("appDirFor(%q) = %q, %v; want %q", c.exe, got, err, c.want)
		}
	}
}

func TestAppDirTest(t *testing.T) {
	// тестовый бинарник лежит во временном каталоге сборки — AppDir = рабочая папка
	dir, err := AppDir()
	if err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if dir != wd {
		t.Fatalf("AppDir = %q, рабочая папка %q", dir, wd)
	}
}

func TestEnsureDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a", LibraryDirName)
	if err := EnsureDir(dir); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		t.Fatalf("папка не создана: %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "file")
	os.WriteFile(blocker, nil, 0o644)
	if err := EnsureDir(filepath.Join(blocker, "manga")); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}

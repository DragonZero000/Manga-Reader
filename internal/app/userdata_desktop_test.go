//go:build !android

package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/catalog"
	"mangareader/internal/model"
	"mangareader/internal/paths"
	"mangareader/internal/userdata"
)

// Портативная папка перенесена в другое место: записи пользовательских
// данных находятся по тому же ключу папки; переименованный архив
// сохраняет запись.
func TestPortableUserDataSurvivesMove(t *testing.T) {
	base := t.TempDir()
	dir1 := filepath.Join(base, "old place", "MangaReader")
	lib := filepath.Join(dir1, paths.LibraryDirName)
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "a.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	s := newPortable("test", dir1)
	if s.UserData == nil || s.UserDataBackup != "" || s.UserDataKey() != desktopLibraryKey {
		t.Fatalf("пользовательские данные: %v, копия %q, ключ %q", s.UserData, s.UserDataBackup, s.UserDataKey())
	}
	if _, err := s.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, ok := s.Library.Get(model.LocalKey("a.zip"))
	if !ok {
		t.Fatal("галерея не найдена")
	}
	uid, err := s.UserData.Ensure(s.UserDataKey(), "a.zip", g.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	for _, name := range []string{userdata.FileName, catalog.FileName} {
		if _, err := os.Stat(filepath.Join(dir1, name)); err != nil {
			t.Errorf("%s не рядом с приложением: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(lib, name)); err == nil {
			t.Errorf("%s в папке библиотеки", name)
		}
	}

	dir2 := filepath.Join(base, "new place", "MangaReader")
	if err := os.MkdirAll(filepath.Dir(dir2), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir1, dir2); err != nil {
		t.Fatal(err)
	}
	s2 := newPortable("test", dir2)
	defer s2.Close()
	if _, err := s2.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s2.UserData.Lookup(s2.UserDataKey(), "a.zip"); err != nil || !ok || got != uid {
		t.Fatalf("после переноса папки: uid %d/%d, %v, %v", got, uid, ok, err)
	}

	// архив переименован при работающем приложении — запись переходит к нему
	lib2 := filepath.Join(dir2, paths.LibraryDirName)
	if err := os.MkdirAll(filepath.Join(lib2, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(lib2, "a.zip"), filepath.Join(lib2, "sub", "b.zip")); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := s2.UserData.Lookup(s2.UserDataKey(), "sub/b.zip"); err != nil || !ok || got != uid {
		t.Fatalf("после переименования: uid %d/%d, %v, %v", got, uid, ok, err)
	}
}

// Повреждённая база при запуске откладывается в копию, сервисы сообщают о ней.
func TestPortableUserDataRecovery(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, userdata.FileName), make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newPortable("test", dir)
	defer s.Close()
	if s.UserData == nil || s.UserDataBackup == "" {
		t.Fatalf("восстановление: %v, копия %q", s.UserData, s.UserDataBackup)
	}
	if _, err := os.Stat(s.UserDataBackup); err != nil {
		t.Fatal(err)
	}
}

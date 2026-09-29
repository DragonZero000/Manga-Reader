package userdata

import (
	"bytes"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openTest(t *testing.T, path string) *Store {
	t.Helper()
	s, rec, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Backup != "" {
		t.Fatalf("неожиданное восстановление: %+v", rec)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func userVersion(t *testing.T, s *Store) int {
	t.Helper()
	var v int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestOpenNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := openTest(t, path)
	if v := userVersion(t, s); v != len(migrations) {
		t.Fatalf("user_version %d, ожидалось %d", v, len(migrations))
	}
	var mode string
	s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode)
	if mode != "wal" {
		t.Errorf("journal_mode %q", mode)
	}
	var fk int
	s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&fk)
	if fk != 1 {
		t.Error("foreign_keys выключены")
	}
	if _, ok, err := s.Lookup("root", "a.zip"); err != nil || ok {
		t.Fatalf("новая база не пуста: %v %v", ok, err)
	}
}

func TestReopenKeepsRecords(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := openTest(t, path)
	uid, err := s.Ensure("root", "a.zip", "fp-a")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	s2 := openTest(t, path)
	got, ok, err := s2.Lookup("root", "a.zip")
	if err != nil || !ok || got != uid {
		t.Fatalf("после повторного открытия: uid %d/%d, ok %v, %v", got, uid, ok, err)
	}
}

func TestOpenGarbageMovesToBackup(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	garbage := []byte(strings.Repeat("это не база данных, а мусор ", 40))
	if err := os.WriteFile(path, garbage, 0o644); err != nil {
		t.Fatal(err)
	}
	s, rec, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if rec.Backup == "" || rec.Cause == nil {
		t.Fatalf("восстановление не выполнено: %+v", rec)
	}
	if dir2, name := filepath.Split(rec.Backup); filepath.Clean(dir2) != dir || !strings.HasPrefix(name, FileName+".broken-") ||
		len(name) != len(FileName+".broken-20060102-150405") {
		t.Fatalf("имя резервной копии %q", rec.Backup)
	}
	if data, err := os.ReadFile(rec.Backup); err != nil || !bytes.Equal(data, garbage) {
		t.Fatalf("резервная копия: %v", err)
	}
	if v := userVersion(t, s); v != len(migrations) {
		t.Fatalf("новая база: user_version %d", v)
	}
	if _, err := s.Ensure("root", "a.zip", "fp"); err != nil {
		t.Fatalf("новая база не пишется: %v", err)
	}
}

// Резервная копия забирает и журнал; имя не совпадает с уже существующей копией.
func TestBackupFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	now := time.Date(2026, 9, 30, 14, 25, 1, 0, time.Local)
	for _, name := range []string{FileName, FileName + "-wal", FileName + "-shm", FileName + ".broken-20260930-142501"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	backup, err := backupFiles(path, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := path + ".broken-20260930-142501-2"; backup != want {
		t.Fatalf("копия %q, ожидалось %q", backup, want)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if data, err := os.ReadFile(backup + suffix); err != nil || string(data) != FileName+suffix {
			t.Errorf("%s: %q %v", suffix, data, err)
		}
		if _, err := os.Stat(path + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s остался на месте: %v", suffix, err)
		}
	}
}

func TestOpenNewerSchemaUntouched(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := openTest(t, path)
	if _, err := s.Ensure("root", "a.zip", "fp"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := Open(path); !errors.Is(err, ErrNewerSchema) {
		t.Fatalf("ожидалась ErrNewerSchema: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("файл базы изменён")
	}
	if m, _ := filepath.Glob(path + ".broken-*"); len(m) != 0 {
		t.Fatalf("создана резервная копия: %v", m)
	}
}

func TestMigrationOverExistingData(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := openTest(t, path)
	uid, err := s.Ensure("root", "a.zip", "fp-a")
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	v2 := append(append([]migration(nil), migrations...), func(tx *sql.Tx) error {
		return execAll(tx,
			`CREATE TABLE notes(uid INTEGER NOT NULL REFERENCES works(uid) ON DELETE CASCADE, text TEXT)`)
	})
	s2, err := open(path, v2)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	if v := userVersion(t, s2); v != len(v2) {
		t.Fatalf("user_version %d, ожидалось %d", v, len(v2))
	}
	if got, ok, err := s2.Lookup("root", "a.zip"); err != nil || !ok || got != uid {
		t.Fatalf("запись после миграции: %d %v %v", got, ok, err)
	}
	// ON DELETE CASCADE: удаление записи удаляет её данные
	if _, err := s2.db.Exec(`INSERT INTO notes(uid, text) VALUES(?, 'x')`, uid); err != nil {
		t.Fatal(err)
	}
	if _, err := s2.db.Exec(`DELETE FROM works WHERE uid = ?`, uid); err != nil {
		t.Fatal(err)
	}
	var n int
	s2.db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&n)
	if n != 0 {
		t.Fatalf("данные удалённой записи остались: %d", n)
	}
}

// Неудачная миграция не применяется частично.
func TestMigrationFailureRollsBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	openTest(t, path).Close()

	bad := append(append([]migration(nil), migrations...),
		func(tx *sql.Tx) error { return execAll(tx, `CREATE TABLE a(x)`) },
		func(tx *sql.Tx) error { return errors.New("сбой") },
	)
	if _, err := open(path, bad); err == nil {
		t.Fatal("ожидалась ошибка")
	}
	s := openTest(t, path)
	if v := userVersion(t, s); v != len(migrations) {
		t.Fatalf("user_version %d", v)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'a'`).Scan(&n)
	if n != 0 {
		t.Fatal("часть миграции применена")
	}
}

func TestEnsureLookup(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	uid, err := s.Ensure("root", "a.zip", "fp-a")
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.Ensure("root", "a.zip", "")
	if err != nil || again != uid {
		t.Fatalf("повторный Ensure: %d/%d %v", again, uid, err)
	}
	if w := get(t, s, "root", "a.zip"); w.fp != "fp-a" {
		t.Fatalf("пустой отпечаток затёр прежний: %+v", w)
	}
	other, err := s.Ensure("other root", "a.zip", "fp-a")
	if err != nil || other == uid {
		t.Fatalf("запись другой папки: %d %v", other, err)
	}
	if _, ok, _ := s.Lookup("root", "b.zip"); ok {
		t.Fatal("Lookup нашёл несуществующую запись")
	}
}

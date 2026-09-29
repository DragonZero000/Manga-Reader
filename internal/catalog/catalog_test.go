//go:build sqlite_fts5

package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/search"
)

func openTest(t *testing.T, path, root string) *Catalog {
	t.Helper()
	c, err := Open(path, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func count(t *testing.T, c *Catalog, table string) int {
	t.Helper()
	var n int
	if err := c.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOpenNew(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c := openTest(t, path, "root")
	if !c.Fresh() {
		t.Fatal("новый каталог должен быть Fresh")
	}
	var ver int
	c.db.QueryRow(`PRAGMA user_version`).Scan(&ver)
	if ver != userVersion() {
		t.Fatalf("user_version %d", ver)
	}
	if _, err := c.db.Exec(`INSERT INTO links(rel, url) VALUES('a.zip', 'https://a')`); err != nil {
		t.Fatal(err)
	}
	c.Close()

	// тот же файл, та же папка — данные на месте, не Fresh
	c2 := openTest(t, path, "root")
	if c2.Fresh() || count(t, c2, "links") != 1 {
		t.Fatalf("повторное открытие: fresh=%v, links=%d", c2.Fresh(), count(t, c2, "links"))
	}
}

func TestOpenGarbageRecreates(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte("это не база данных, а мусор ............................................................................"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := openTest(t, path, "root")
	if !c.Fresh() || count(t, c, "files") != 0 {
		t.Fatal("повреждённый файл должен пересоздаваться")
	}
}

func TestOpenOldVersionRecreates(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c := openTest(t, path, "root")
	c.db.Exec(`INSERT INTO links(rel, url) VALUES('a.zip', 'https://a')`)
	c.db.Exec(`PRAGMA user_version = 5`)
	c.Close()

	c2 := openTest(t, path, "root")
	if !c2.Fresh() || count(t, c2, "links") != 0 {
		t.Fatalf("старая версия: fresh=%v, links=%d", c2.Fresh(), count(t, c2, "links"))
	}
}

// Каталог предыдущей версии схемы (теги без происхождения) пересоздаётся.
func TestOpenPreviousSchemaRecreates(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c := openTest(t, path, "root")
	c.db.Exec(`INSERT INTO files(rel, size, mtime, err_kind, err_text) VALUES('a.txt', 1, 1, '', 'не удалось открыть')`)
	c.db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, (schemaVersion-1)*100+search.NormVersion))
	c.Close()

	c2 := openTest(t, path, "root")
	if !c2.Fresh() || count(t, c2, "files") != 0 {
		t.Fatalf("предыдущая версия: fresh=%v, files=%d", c2.Fresh(), count(t, c2, "files"))
	}
}

func TestOpenOtherRootClears(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c := openTest(t, path, "root A")
	c.db.Exec(`INSERT INTO files(rel, size, mtime) VALUES('a.zip', 1, 1)`)
	c.Close()

	c2 := openTest(t, path, "root B")
	if !c2.Fresh() || count(t, c2, "files") != 0 {
		t.Fatalf("другая папка: fresh=%v, files=%d", c2.Fresh(), count(t, c2, "files"))
	}
	c2.db.Exec(`INSERT INTO files(rel, size, mtime) VALUES('b.zip', 1, 1)`)
	if err := c2.Reset("root C"); err != nil {
		t.Fatal(err)
	}
	if count(t, c2, "files") != 0 {
		t.Fatal("Reset должен очищать данные")
	}
}

// Эпоха наложения хранится между открытиями и сбрасывается сменой папки.
func TestOverlayEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	c := openTest(t, path, "root")
	if e, err := c.OverlayEpoch(); err != nil || e != "" {
		t.Fatalf("новый каталог: %q %v", e, err)
	}
	if err := c.SetOverlayEpoch("e1"); err != nil {
		t.Fatal(err)
	}
	c.Close()
	c = openTest(t, path, "root")
	if e, _ := c.OverlayEpoch(); e != "e1" {
		t.Fatalf("после открытия: %q", e)
	}
	if err := c.Reset("root 2"); err != nil {
		t.Fatal(err)
	}
	if e, _ := c.OverlayEpoch(); e != "" {
		t.Fatalf("после смены папки: %q", e)
	}
}

func TestNaturalCollation(t *testing.T) {
	c := openTest(t, filepath.Join(t.TempDir(), FileName), "root")
	rows, err := c.db.Query(`SELECT column1 FROM (VALUES ('b10'), ('b2'), ('a')) ORDER BY column1 COLLATE NATSORT`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s string
		rows.Scan(&s)
		got = append(got, s)
	}
	if len(got) != 3 || got[0] != "a" || got[1] != "b2" || got[2] != "b10" {
		t.Fatalf("натуральный порядок: %v", got)
	}
}

//go:build !android

package app

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/catalog"
	"mangareader/internal/model"
	"mangareader/internal/paths"
	"mangareader/internal/userdata"
)

// Архив с повторяющимися именами записей, разобранный прежней версией
// (ревизия правил 0, повтор — отдельная страница, другой отпечаток): после
// перепроверки у записи user.db прежний uid и новый отпечаток.
func TestRecheckUpdatesUserDataFingerprint(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, paths.LibraryDirName)
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for i, name := range []string{"1.png", "1.png", "2.png"} {
		var img bytes.Buffer
		if err := png.Encode(&img, image.NewGray(image.Rect(0, 0, i+2, i+2))); err != nil {
			t.Fatal(err)
		}
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(img.Bytes())
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "dup.zip"), buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	key := model.LocalKey("dup.zip")

	s := newPortable("test", dir)
	s.bg.Wait()
	if _, err := s.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, ok := s.Library.Get(key)
	if !ok || len(g.Pages) != 2 {
		t.Fatalf("галерея: %v, страницы %v", ok, g.Pages)
	}
	uid, err := s.UserData.Ensure(s.UserDataKey(), "dup.zip", g.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()

	// состояние после прежней версии: в каталоге повтор — отдельная страница,
	// ревизии нет; в user.db — прежний отпечаток
	exec := func(file string, fn func(db *sql.DB) error) {
		t.Helper()
		db, err := sql.Open("sqlite3", filepath.Join(dir, file))
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if err := fn(db); err != nil {
			t.Fatal(err)
		}
	}
	exec(catalog.FileName, func(db *sql.DB) error {
		var blob []byte
		if err := db.QueryRow(`SELECT gallery FROM files WHERE rel = 'dup.zip'`).Scan(&blob); err != nil {
			return err
		}
		var old model.Gallery
		if err := json.Unmarshal(blob, &old); err != nil {
			return err
		}
		old.Pages = append(old.Pages, model.Page{Name: "1.png"})
		old.SortPages()
		old.Fingerprint = "old"
		if blob, err = json.Marshal(old); err != nil {
			return err
		}
		if _, err := db.Exec(`UPDATE files SET gallery = ? WHERE rel = 'dup.zip'`, blob); err != nil {
			return err
		}
		_, err := db.Exec(`DELETE FROM meta WHERE key = 'parse_rev'`)
		return err
	})
	exec(userdata.FileName, func(db *sql.DB) error {
		_, err := db.Exec(`UPDATE works SET fingerprint = 'old' WHERE uid = ?`, uid)
		return err
	})

	s2 := newPortable("test", dir)
	defer s2.Close()
	s2.bg.Wait()
	if cached, _ := s2.Library.Get(key); len(cached.Pages) != 3 {
		t.Fatalf("до перепроверки из каталога: страниц %d", len(cached.Pages))
	}
	if _, err := s2.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g2, _ := s2.Library.Get(key)
	if len(g2.Pages) != 2 || g2.Fingerprint != g.Fingerprint {
		t.Fatalf("после перепроверки: страниц %d, отпечаток %q (ожидался %q)", len(g2.Pages), g2.Fingerprint, g.Fingerprint)
	}
	got, ok, err := s2.UserData.Lookup(s2.UserDataKey(), "dup.zip")
	if err != nil || !ok || got != uid {
		t.Fatalf("запись: uid %d/%d, %v, %v", got, uid, ok, err)
	}
	exec(userdata.FileName, func(db *sql.DB) error {
		var fp string
		if err := db.QueryRow(`SELECT fingerprint FROM works WHERE uid = ?`, uid).Scan(&fp); err != nil {
			return err
		}
		if fp != g.Fingerprint {
			t.Errorf("отпечаток записи %q, ожидался %q", fp, g.Fingerprint)
		}
		return nil
	})
}

package userdata

import (
	"path/filepath"
	"testing"
	"time"
)

// Миграция v2 → v3: теги сохранены, прогресса нет, таблица работает.
func TestMigrationV3KeepsTags(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s2, err := open(path, migrations[:2])
	if err != nil {
		t.Fatal(err)
	}
	uid, _ := s2.Ensure("root", "a.zip", "fp-a")
	if err := s2.AddCustom(uid, tag("tag", "x")); err != nil {
		t.Fatal(err)
	}
	s2.Close()

	s := openTest(t, path)
	if v := userVersion(t, s); v != 3 || len(migrations) != 3 {
		t.Fatalf("user_version %d", v)
	}
	if o, err := s.Overlay(uid); err != nil || len(o.Custom) != 1 {
		t.Fatalf("теги после миграции: %+v %v", o, err)
	}
	if all, err := s.AllProgress("root"); err != nil || len(all) != 0 {
		t.Fatalf("прогресс после миграции: %+v %v", all, err)
	}
	if err := s.SetProgress(uid, Progress{Page: "01.png", Index: 0, Total: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestProgressOperations(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	a, _ := s.Ensure("root", "a.zip", "fp-a")
	b, _ := s.Ensure("other", "b.zip", "fp-b")
	now := time.Unix(100, 0)
	want := Progress{Page: "ch05/012.jpg", Index: 41, Total: 87, Updated: now}
	if err := s.SetProgress(a, want); err != nil {
		t.Fatal(err)
	}
	if err := s.SetProgress(b, Progress{Page: "1.png", Total: 3, Updated: now}); err != nil {
		t.Fatal(err)
	}
	all, err := s.AllProgress("root")
	if err != nil || len(all) != 1 || all["a.zip"] != want {
		t.Fatalf("AllProgress: %+v %v", all, err)
	}

	// перезапись
	want = Progress{Page: "ch10/020.jpg", Index: 86, Total: 87, Finished: true, Updated: now.Add(time.Second)}
	if err := s.SetProgress(a, want); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.AllProgress("root"); all["a.zip"] != want {
		t.Fatalf("после перезаписи: %+v", all)
	}

	if err := s.ClearProgress(a); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.AllProgress("root"); len(all) != 0 {
		t.Fatalf("после сброса: %+v", all)
	}
}

// Перенос записи по отпечатку переносит прогресс; удаление записи-сироты
// по сроку удаляет и прогресс.
func TestProgressMoveAndCascade(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	a, _ := s.Ensure("root", "a.zip", "fp-a")
	c, _ := s.Ensure("root", "c.zip", "fp-c")
	for _, uid := range []int64{a, c} {
		if err := s.SetProgress(uid, Progress{Page: "2.png", Index: 1, Total: 2}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	// a.zip переименован в b.zip, c.zip пропал
	if err := s.Reconcile("root", map[string]string{"b.zip": "fp-a"}, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	all, _ := s.AllProgress("root")
	if _, ok := all["b.zip"]; !ok || len(all) != 1 {
		t.Fatalf("после переноса: %+v", all)
	}
	if err := s.Reconcile("root", map[string]string{"b.zip": "fp-a"}, now.Add(48*time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM progress WHERE uid = ?`, c).Scan(&n)
	if n != 0 {
		t.Fatal("прогресс удалённой записи остался")
	}
}

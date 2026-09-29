package userdata

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"mangareader/internal/model"
)

func tag(typ, name string) model.Tag { return model.NewTag(typ, name) }

// Миграция v1 → v2 сохраняет записи произведений.
func TestMigrationV2KeepsWorks(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s1, err := open(path, migrations[:1])
	if err != nil {
		t.Fatal(err)
	}
	uid, err := s1.Ensure("root", "a.zip", "fp-a")
	if err != nil {
		t.Fatal(err)
	}
	s1.Close()

	s := openTest(t, path)
	if v := userVersion(t, s); v != len(migrations) {
		t.Fatalf("user_version %d", v)
	}
	if got, ok, err := s.Lookup("root", "a.zip"); err != nil || !ok || got != uid {
		t.Fatalf("запись после миграции: %d %v %v", got, ok, err)
	}
	if err := s.AddCustom(uid, tag("tag", "x")); err != nil {
		t.Fatalf("user_tags после миграции: %v", err)
	}
	if e, err := s.Epoch(); err != nil || len(e) != 32 {
		t.Fatalf("эпоха после миграции: %q %v", e, err)
	}
}

func TestTagOperations(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	uid, err := s.Ensure("root", "a.zip", "fp-a")
	if err != nil {
		t.Fatal(err)
	}
	for _, tg := range []model.Tag{tag("tag", "b"), tag("character", "Alice"), tag("tag", "a")} {
		if err := s.AddCustom(uid, tg); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.AddCustom(uid, tag("Character", " alice ")); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("дубликат своего: %v", err)
	}
	for _, tg := range []model.Tag{tag("tag", "tag 3"), tag("tag", "tag 3"), tag("language", "japanese")} {
		if err := s.Hide(uid, tg); err != nil {
			t.Fatal(err)
		}
	}
	o, err := s.Overlay(uid)
	if err != nil {
		t.Fatal(err)
	}
	want := Overlay{
		Custom: []model.Tag{tag("tag", "b"), tag("character", "alice"), tag("tag", "a")},
		Hidden: []model.Tag{tag("tag", "tag 3"), tag("language", "japanese")},
	}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("Overlay = %+v, want %+v", o, want)
	}

	// удаление и повторное добавление — в конец
	if err := s.RemoveCustom(uid, tag("tag", "b")); err != nil {
		t.Fatal(err)
	}
	if err := s.AddCustom(uid, tag("tag", "b")); err != nil {
		t.Fatal(err)
	}
	if err := s.Unhide(uid, tag("tag", "tag 3")); err != nil {
		t.Fatal(err)
	}
	o, _ = s.Overlay(uid)
	want = Overlay{
		Custom: []model.Tag{tag("character", "alice"), tag("tag", "a"), tag("tag", "b")},
		Hidden: []model.Tag{tag("language", "japanese")},
	}
	if !reflect.DeepEqual(o, want) {
		t.Fatalf("после удаления: %+v, want %+v", o, want)
	}

	all, err := s.Overlays("root")
	if err != nil || !reflect.DeepEqual(all, map[string]Overlay{"a.zip": want}) {
		t.Fatalf("Overlays = %+v %v", all, err)
	}

	if err := s.Reset(uid); err != nil {
		t.Fatal(err)
	}
	if o, _ := s.Overlay(uid); !o.Empty() {
		t.Fatalf("после сброса: %+v", o)
	}
}

// Overlays не возвращает сирот и записи других папок; удаление сироты
// сверкой удаляет её теги.
func TestOverlaysOrphansAndCascade(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	a, _ := s.Ensure("root", "a.zip", "fp-a")
	b, _ := s.Ensure("root", "b.zip", "fp-b")
	c, _ := s.Ensure("other", "c.zip", "fp-c")
	for _, uid := range []int64{a, b, c} {
		if err := s.AddCustom(uid, tag("tag", "x")); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := s.Reconcile("root", map[string]string{"a.zip": "fp-a"}, now, 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	all, err := s.Overlays("root")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := all["b.zip"]; ok || len(all) != 1 {
		t.Fatalf("Overlays с сиротой: %+v", all)
	}

	// срок истёк — запись и её теги удалены
	if err := s.Reconcile("root", map[string]string{"a.zip": "fp-a"}, now.Add(48*time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM user_tags WHERE uid = ?`, b).Scan(&n)
	if n != 0 {
		t.Fatalf("теги удалённой записи остались: %d", n)
	}
	s.db.QueryRow(`SELECT COUNT(*) FROM user_tags`).Scan(&n)
	if n != 2 {
		t.Fatalf("всего тегов %d, ожидалось 2", n)
	}
}

// Перенос по отпечатку сохраняет теги под новым путём.
func TestOverlayFollowsMove(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	uid, _ := s.Ensure("root", "a.zip", "fp-a")
	if err := s.AddCustom(uid, tag("tag", "my-fav")); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile("root", map[string]string{"b.zip": "fp-a"}, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	all, _ := s.Overlays("root")
	if o := all["b.zip"]; len(o.Custom) != 1 || o.Custom[0] != tag("tag", "my-fav") {
		t.Fatalf("Overlays после переноса: %+v", all)
	}
}

// Эпоха стабильна между открытиями и новая у пересозданной базы.
func TestEpoch(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	s := openTest(t, path)
	e1, err := s.Epoch()
	if err != nil || e1 == "" {
		t.Fatalf("эпоха: %q %v", e1, err)
	}
	s.Close()
	s = openTest(t, path)
	if e2, _ := s.Epoch(); e2 != e1 {
		t.Fatalf("эпоха изменилась: %q → %q", e1, e2)
	}
	s.Close()

	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(path + suffix)
	}
	s = openTest(t, path)
	if e3, _ := s.Epoch(); e3 == e1 || e3 == "" {
		t.Fatalf("эпоха новой базы %q, прежняя %q", e3, e1)
	}
}

//go:build !android

package app

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"mangareader/internal/model"
	"mangareader/internal/paths"
	"mangareader/internal/search"
	"mangareader/internal/userdata"
)

// portableWithExample — портативная папка с testdata/example.zip в
// библиотеке, отсканированная.
func portableWithExample(t *testing.T) (dir string, s *Services) {
	t.Helper()
	dir = t.TempDir()
	lib := filepath.Join(dir, paths.LibraryDirName)
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lib, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	s = newPortable("test", dir)
	s.bg.Wait()
	if _, err := s.Library.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	return dir, s
}

var exampleKey = model.LocalKey("example.zip")

func find(t *testing.T, idx search.Index, query string) []string {
	t.Helper()
	q, err := search.Parse(query)
	if err != nil {
		t.Fatal(err)
	}
	keys, _, err := idx.Search(context.Background(), q)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, k := range keys {
		out = append(out, k.ID)
	}
	return out
}

func edit(t *testing.T, s *Services, op TagOp) model.Gallery {
	t.Helper()
	g, err := s.EditTags(exampleKey, op)
	if err != nil {
		t.Fatalf("EditTags(%+v): %v", op, err)
	}
	return g
}

func TestEditTags(t *testing.T) {
	_, s := portableWithExample(t)
	defer s.Close()
	if !s.TagsAvailable() {
		t.Fatal("TagsAvailable() == false")
	}
	ex := []string{"example.zip"}

	g := edit(t, s, TagOp{TagAdd, model.NewTag("character", "Alice")})
	if !reflect.DeepEqual(g.Custom, []model.Tag{model.NewTag("character", "alice")}) {
		t.Fatalf("свои теги: %v", g.Custom)
	}
	if got, _ := s.Library.Get(exampleKey); len(got.Custom) != 1 {
		t.Fatalf("галерея источника: %v", got.Custom)
	}
	if got := find(t, s.Index, "character:alice"); !reflect.DeepEqual(got, ex) {
		t.Fatalf("поиск своего тега: %v", got)
	}

	// то же имя другого типа добавляется
	edit(t, s, TagOp{TagAdd, model.NewTag("tag", "japanese")})

	g = edit(t, s, TagOp{TagHide, model.NewTag("tag", "tag 3")})
	if !reflect.DeepEqual(g.Hidden, []model.Tag{model.NewTag("tag", "tag 3")}) {
		t.Fatalf("скрытые: %v", g.Hidden)
	}
	if got := find(t, s.Index, `hidden-tag:"tag 3"`); !reflect.DeepEqual(got, ex) {
		t.Fatalf("поиск скрытого: %v", got)
	}

	// дубликаты: оригинальный, скрытый, свой
	for _, tg := range []model.Tag{
		model.NewTag("tag", "Tag 1"),
		model.NewTag("tag", "tag 3"),
		model.NewTag("character", " ALICE "),
	} {
		if _, err := s.EditTags(exampleKey, TagOp{TagAdd, tg}); !errors.Is(err, ErrTagExists) {
			t.Errorf("дубликат %v: %v", tg, err)
		}
	}
	if got, _ := s.Library.Get(exampleKey); !got.IsHidden(model.NewTag("tag", "tag 3")) {
		t.Fatal("дубликат скрытого изменил его")
	}

	// недопустимые имена
	for _, name := range []string{"   ", `a"b`, strings.Repeat("я", MaxTagName+1)} {
		if _, err := s.EditTags(exampleKey, TagOp{TagAdd, model.NewTag("tag", name)}); !errors.Is(err, ErrTagInvalid) {
			t.Errorf("имя %q: %v", name, err)
		}
	}
	if _, err := s.EditTags(exampleKey, TagOp{TagAdd, model.NewTag("tag", strings.Repeat("я", MaxTagName))}); err != nil {
		t.Errorf("имя из %d символов: %v", MaxTagName, err)
	}

	g = edit(t, s, TagOp{TagUnhide, model.NewTag("tag", "tag 3")})
	if len(g.Hidden) != 0 {
		t.Fatalf("после возврата: %v", g.Hidden)
	}
	g = edit(t, s, TagOp{TagRemove, model.NewTag("tag", "japanese")})
	if len(g.Custom) != 2 {
		t.Fatalf("после удаления: %v", g.Custom)
	}

	edit(t, s, TagOp{TagHide, model.NewTag("tag", "tag 2")})
	g = edit(t, s, TagOp{Kind: TagReset})
	if len(g.Custom) != 0 || len(g.Hidden) != 0 {
		t.Fatalf("после сброса: %+v %+v", g.Custom, g.Hidden)
	}
	if got := find(t, s.Index, "custom-tag:alice"); len(got) != 0 {
		t.Fatalf("поиск после сброса: %v", got)
	}
}

func TestTagsUnavailableWithoutUserData(t *testing.T) {
	dir := t.TempDir()
	// база новее приложения — пользовательские данные недоступны
	s0 := newPortable("test", dir)
	if _, err := s0.UserData.Ensure("x", "y", ""); err != nil {
		t.Fatal(err)
	}
	s0.Close()
	db, err := sql.Open("sqlite3", filepath.Join(dir, userdata.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	s := newPortable("test", dir)
	defer s.Close()
	if s.TagsAvailable() {
		t.Fatal("TagsAvailable() == true без пользовательских данных")
	}
	if _, err := s.EditTags(exampleKey, TagOp{TagAdd, model.NewTag("tag", "x")}); !errors.Is(err, ErrUserDataUnavailable) {
		t.Fatalf("EditTags без данных: %v", err)
	}
}

// Свои теги сохраняются после перезапуска; замена user.db убирает их из
// индекса (переиндексация по эпохе).
func TestOverlayEpochReindex(t *testing.T) {
	dir, s := portableWithExample(t)
	edit(t, s, TagOp{TagAdd, model.NewTag("character", "alice")})
	edit(t, s, TagOp{TagHide, model.NewTag("tag", "tag 3")})
	s.Close()

	// обычный перезапуск: наложение на месте, переиндексации нет
	s = newPortable("test", dir)
	if s.stopBg != nil {
		t.Error("переиндексация при неизменных пользовательских данных")
	}
	s.bg.Wait()
	if g, ok := s.Library.Get(exampleKey); !ok || len(g.Custom) != 1 || len(g.Hidden) != 1 {
		t.Fatalf("галерея из каталога: %+v %+v", g.Custom, g.Hidden)
	}
	if got := find(t, s.Index, "custom-tag:alice"); len(got) != 1 {
		t.Fatalf("после перезапуска: %v", got)
	}
	s.Close()

	// user.db заменена новой базой
	for _, suffix := range []string{"", "-wal", "-shm"} {
		os.Remove(filepath.Join(dir, userdata.FileName+suffix))
	}
	s = newPortable("test", dir)
	defer s.Close()
	s.bg.Wait()
	if got := find(t, s.Index, "custom-tag:alice"); len(got) != 0 {
		t.Fatalf("старые свои теги в индексе: %v", got)
	}
	if got := find(t, s.Index, `tag:"tag 3"`); len(got) != 1 {
		t.Fatalf("скрытый тег не вернулся в индекс: %v", got)
	}
	if g, _ := s.Library.Get(exampleKey); len(g.Custom) != 0 || len(g.Hidden) != 0 {
		t.Fatalf("галерея: %+v %+v", g.Custom, g.Hidden)
	}
	want, _ := s.UserData.Epoch()
	if got, _ := s.Catalog.OverlayEpoch(); got != want {
		t.Fatalf("эпоха каталога %q, ожидалась %q", got, want)
	}
}

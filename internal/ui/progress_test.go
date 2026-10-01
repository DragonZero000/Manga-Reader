package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
	"mangareader/internal/userdata"
)

// Позиция чтения через оболочку: «Продолжить» после чтения, продолжение
// на сохранённой странице, отметка «дочитано» и сброс из меню.
func TestShellReadingProgress(t *testing.T) {
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store, _, err := userdata.Open(filepath.Join(t.TempDir(), userdata.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	src := library.NewDirSource(dir, nil)
	svc := app.NewForTestWithUserData(src, search.NopIndex{}, storage.NewMemSettings(), store)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := NewShell(a, svc)
	drop := func(func()) {}
	s.do = drop
	s.Details.SetDispatcher(drop)
	s.Reader.SetDispatcher(drop)

	g, _ := src.Get(model.LocalKey("example.zip"))
	s.Details.Open(g)
	if s.Details.ContinueButton().Visible() {
		t.Fatal("«Продолжить» у непрочитанного произведения")
	}

	// «Читать», листание, закрытие — «Продолжить · стр. 2»
	s.Reader.Open(s.Details.Gallery())
	s.Reader.TypedKey(fyne.KeyRight)
	s.typedKey(&fyne.KeyEvent{Name: fyne.KeyEscape})
	btn := s.Details.ContinueButton()
	if !btn.Visible() || btn.Text != i18n.T("details.continue", "Page", 2) {
		t.Fatalf("после чтения: видна %v, текст %q", btn.Visible(), btn.Text)
	}

	// «Продолжить» открывает читалку на сохранённой странице
	test.Tap(btn)
	if !s.Reader.Visible() || s.Reader.Page() != 1 {
		t.Fatalf("«Продолжить»: открыта %v, страница %d", s.Reader.Visible(), s.Reader.Page())
	}

	// дочитывание: «вперёд» на последней странице и подтверждение после паузы
	s.Reader.TypedKey(fyne.KeyRight)
	time.Sleep(500 * time.Millisecond)
	s.Reader.TypedKey(fyne.KeyRight)
	if s.Reader.Visible() {
		t.Fatal("читалка не закрылась после подтверждения")
	}
	if btn := s.Details.ContinueButton(); btn.Visible() {
		t.Fatalf("дочитанное без новых страниц: %q", btn.Text)
	}
	if !svc.Progress.Has(g) {
		t.Fatal("отметка «дочитано» не сохранена")
	}

	// сброс из меню
	s.resetProgress(g)
	if svc.Progress.Has(g) || s.actions.HasProgress(g) {
		t.Fatal("прогресс после сброса")
	}
	if !svc.Progress.Flush(time.Second) {
		t.Fatal("очередь записи не успела")
	}
	svc.Progress.Reload()
	if svc.Progress.Has(g) {
		t.Fatal("сброс не записан в базу")
	}
}

// Просмотр через «Читать» без листания не затирает сохранённую позицию.
func TestShellPeekKeepsProgress(t *testing.T) {
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	store, _, err := userdata.Open(filepath.Join(t.TempDir(), userdata.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	src := library.NewDirSource(dir, nil)
	svc := app.NewForTestWithUserData(src, search.NopIndex{}, storage.NewMemSettings(), store)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := NewShell(a, svc)
	drop := func(func()) {}
	s.do = drop
	s.Details.SetDispatcher(drop)
	s.Reader.SetDispatcher(drop)

	g, _ := src.Get(model.LocalKey("example.zip"))
	svc.Progress.Save(g, 1)
	s.Details.Open(g)
	s.Reader.Open(g)
	s.Reader.Close()
	if page, ok, _ := svc.Progress.Resume(g); !ok || page != 1 {
		t.Fatalf("после просмотра: %d %v", page, ok)
	}
}

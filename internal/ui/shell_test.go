package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// Esc закрывает слои по цепочке: читалка → страница произведения → ничего.
func TestBackChain(t *testing.T) {
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	src := library.NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := app.NewForTest(src, search.NopIndex{}, storage.NewMemSettings())
	s := NewShell(a, svc)
	// фоновые результаты загрузок не применяем: проверяется только навигация
	drop := func(func()) {}
	s.Details.SetDispatcher(drop)
	s.Reader.SetDispatcher(drop)

	g, _ := src.Get(model.LocalKey("example.zip"))
	s.Details.Open(g)
	s.Reader.Open(s.Details.Gallery()) // как по кнопке «Читать»
	if !s.Reader.Visible() || !s.Details.Visible() {
		t.Fatal("должны быть открыты оба слоя")
	}

	esc := &fyne.KeyEvent{Name: fyne.KeyEscape}
	s.typedKey(esc)
	if s.Reader.Visible() || !s.Details.Visible() {
		t.Fatalf("первый Esc: reader=%v details=%v", s.Reader.Visible(), s.Details.Visible())
	}
	s.typedKey(esc)
	if s.Details.Visible() {
		t.Fatal("второй Esc должен закрыть страницу произведения")
	}
	s.typedKey(esc) // без слоёв — ничего не происходит
	s.typedKey(&fyne.KeyEvent{Name: "Back"})
}

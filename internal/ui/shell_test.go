package ui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
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

// «Домой» в читалке закрывает читалку и страницу произведения; остаётся
// вкладка, с которой пользователь пришёл.
func TestReaderHome(t *testing.T) {
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
	s := NewShell(a, app.NewForTest(src, search.NopIndex{}, storage.NewMemSettings()))
	drop := func(func()) {}
	s.Details.SetDispatcher(drop)
	s.Reader.SetDispatcher(drop)

	s.Tabs.Select(s.searchTab)
	g, _ := src.Get(model.LocalKey("example.zip"))
	s.Details.Open(g)
	s.Reader.Open(s.Details.Gallery())
	test.Tap(s.Reader.HomeButton())
	if s.Reader.Visible() || s.Details.Visible() {
		t.Fatalf("после «Домой»: reader=%v details=%v", s.Reader.Visible(), s.Details.Visible())
	}
	if s.Tabs.Selected() != s.searchTab {
		t.Fatalf("выбрана вкладка %q, ждали «Поиск»", s.Tabs.Selected().Text)
	}
}

// Сочетания масштаба действуют только в открытой читалке.
func TestZoomShortcutsOnlyInReader(t *testing.T) {
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
	s := NewShell(a, app.NewForTest(src, search.NopIndex{}, storage.NewMemSettings()))
	q := make(chan func(), 1024)
	s.Reader.SetDispatcher(func(f func()) { q <- f })
	ctrlEq := &desktop.CustomShortcut{KeyName: fyne.KeyEqual, Modifier: fyne.KeyModifierShortcutDefault}
	// тестовый canvas Fyne не вызывает зарегистрированные сочетания —
	// вызываем обработчик, который регистрирует оболочка
	press := func() { s.zoomShortcut(ctrlEq) }

	press() // читалка закрыта — ничего
	g, _ := src.Get(model.LocalKey("example.zip"))
	s.Reader.Open(g)
	deadline := time.Now().Add(3 * time.Second)
	for s.Reader.Scale() == 1 && time.Now().Before(deadline) {
		for len(q) > 0 {
			(<-q)()
		}
		press() // до загрузки страницы масштаб не меняется
		time.Sleep(10 * time.Millisecond)
	}
	if s.Reader.Scale() != 1.25 {
		t.Fatalf("Ctrl+=: масштаб %v", s.Reader.Scale())
	}
}

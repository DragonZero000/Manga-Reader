package ui

import (
	"context"
	"fmt"
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

// newLayersShell — оболочка над n копиями example.zip (g00.zip…), библиотека
// показана сеткой; фоновые результаты загрузок не применяются.
func newLayersShell(t *testing.T, n int) (*Shell, *library.Source) {
	t.Helper()
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("g%02d.zip", i)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := search.NewMemIndex()
	src := library.NewDirSource(dir, idx)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	s := NewShell(a, app.NewForTest(src, idx, storage.NewMemSettings()))
	drop := func(func()) {}
	s.Details.SetDispatcher(drop)
	s.Reader.SetDispatcher(drop)
	s.Search().SetDispatcher(drop)
	s.Library().ShowCached(library.ScanResult{Galleries: src.Galleries()})
	s.Window.Resize(fyne.NewSize(960, 640))
	return s, src
}

// lowerCardPos — точка у нижнего края окна над карточкой первой колонки
// сетки библиотеки (абсолютная). На странице произведения, прокрученной до
// конца, здесь подписи сведений — свободное место без кнопок.
func lowerCardPos(s *Shell) fyne.Position {
	grid := s.Library().GridWidget()
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(grid)
	return fyne.NewPos(pos.X+40, s.Window.Canvas().Size().Height-20)
}

// Нажатие на свободное место страницы произведения над карточкой сетки не
// открывает другую галерею: вкладки под страницей скрыты.
func TestDetailsTapDoesNotReachGrid(t *testing.T) {
	s, src := newLayersShell(t, 30)
	pos := lowerCardPos(s)
	c := s.Window.Canvas()

	// без слоёв нажатие открывает карточку
	test.TapCanvas(c, pos)
	if !s.Details.Visible() {
		t.Fatal("нажатие на карточку должно открыть страницу произведения")
	}
	under := s.Details.Gallery().Key
	s.Details.Close()

	gs := s.Library().Galleries()
	other := gs[len(gs)-1].Key // последняя карточка — не на экране
	if other == under {
		t.Fatalf("под точкой — %v", under)
	}
	g, _ := src.Get(other)
	s.Details.Open(g)
	s.Details.Scroller().ScrollToOffset(1e4) // конец страницы: сведения
	test.TapCanvas(c, pos)
	if !s.Details.Visible() || s.Details.Gallery().Key != other {
		t.Fatalf("открыта %v, ждали %v", s.Details.Gallery().Key, other)
	}
}

// Слои, закрытые сверху, скрыты; после закрытия верхних слоёв вкладки
// показываются с прежней прокруткой и текстом поиска.
func TestLayersHideCovered(t *testing.T) {
	s, src := newLayersShell(t, 60)
	s.Search().TypeText("japan")
	grid := s.Library().Scroller()
	grid.ScrollToOffset(300)
	off := grid.ScrollOffset()
	if off == 0 {
		t.Fatal("сетка не прокрутилась")
	}

	g, _ := src.Get(model.LocalKey("g05.zip"))
	s.Details.Open(g)
	if s.Tabs.Visible() || !s.Details.Layer().Visible() {
		t.Fatalf("страница: вкладки %v, страница %v", s.Tabs.Visible(), s.Details.Layer().Visible())
	}
	s.Reader.Open(g)
	if s.Tabs.Visible() || s.Details.Layer().Visible() || !s.Reader.Layer().Visible() {
		t.Fatalf("читалка: вкладки %v, страница %v", s.Tabs.Visible(), s.Details.Layer().Visible())
	}
	s.Reader.Close()
	if s.Tabs.Visible() || !s.Details.Layer().Visible() || !s.Details.Visible() {
		t.Fatalf("после читалки: вкладки %v, страница %v", s.Tabs.Visible(), s.Details.Layer().Visible())
	}
	s.Details.Close()
	if !s.Tabs.Visible() || s.Details.Layer().Visible() {
		t.Fatalf("после страницы: вкладки %v, страница %v", s.Tabs.Visible(), s.Details.Layer().Visible())
	}
	if s.Tabs.SelectedIndex() != 0 || grid.ScrollOffset() != off || s.Search().Query() != "japan" {
		t.Fatalf("вкладка %d, смещение %v (было %v), поиск %q", s.Tabs.SelectedIndex(), grid.ScrollOffset(), off, s.Search().Query())
	}

	// «Домой» и поиск по тегу закрывают оба слоя и показывают вкладки
	s.Details.Open(g)
	s.Reader.Open(g)
	test.Tap(s.Reader.HomeButton())
	if !s.Tabs.Visible() || s.Details.Layer().Visible() || s.Reader.Layer().Visible() {
		t.Fatal("после «Домой» должны быть видны только вкладки")
	}
	s.Details.Open(g)
	s.Reader.Open(g)
	s.SearchFor("tag:x")
	if !s.Tabs.Visible() || s.Tabs.Selected() != s.searchTab || s.Details.Layer().Visible() {
		t.Fatal("поиск по тегу: должны быть видны вкладки «Поиск»")
	}
}

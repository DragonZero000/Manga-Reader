package screens

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/model"
	"mangareader/internal/storage"
	"mangareader/internal/thumbs"
)

// setMobile подменяет платформу на время теста.
func setMobile(t *testing.T, mobile bool) {
	old := isMobile
	isMobile = func() bool { return mobile }
	t.Cleanup(func() { isMobile = old })
}

// gridWindow — сетка из n галерей в окне размера size.
func gridWindow(t *testing.T, m *GridMetrics, n int, size fyne.Size) (*galleryGrid, fyne.Window) {
	t.Helper()
	th := thumbs.New(func(model.Key, string) (io.ReadCloser, error) { return nil, io.EOF }, 1<<20, 1)
	g := newGalleryGrid(th, m, &GalleryActions{Open: func(model.Gallery) {}})
	g.do = func(func()) {}
	for i := range n {
		g.items = append(g.items, galleryOf(fmt.Sprintf("%d.zip", i)))
	}
	w := fyne.CurrentApp().NewWindow("t")
	w.SetContent(g.Widget())
	w.Resize(size)
	return g, w
}

// На телефоне в портретной ориентации ровно N колонок на всю ширину; в
// альбомной — карточки того же размера, колонок больше.
func TestGridColumnsMobile(t *testing.T) {
	setMobile(t, true)
	for _, width := range []float32{360, 412, 480} {
		for _, n := range []int{2, 3, 4} {
			t.Run(fmt.Sprintf("%v-%d", width, n), func(t *testing.T) {
				test.NewTempApp(t)
				st := storage.NewMemSettings()
				app.SetGridColumns(st, n)
				m := NewGridMetrics(st)
				g, w := gridWindow(t, m, 30, fyne.NewSize(width, width*2))

				if c := g.grid.ColumnCount(); c != n {
					t.Fatalf("портрет: колонок %d, карточка %v, сетка %v", c, m.Cover(), g.grid.Size())
				}
				// справа нет пустой полосы шире отступа
				pad := float32(4)
				used := float32(n)*(m.Cover().Width+pad) - pad
				if gap := g.grid.Size().Width - used; gap < 0 || gap > pad {
					t.Fatalf("портрет: остаток %v", gap)
				}
				if min := g.grid.MinSize(); min.Width != m.Cover().Width {
					t.Fatalf("ячейка %v, обложка %v", min, m.Cover())
				}

				portrait := m.Cover()
				w.Resize(fyne.NewSize(width*2, width))
				if m.Cover() != portrait {
					t.Fatalf("альбом: карточка %v, в портрете %v", m.Cover(), portrait)
				}
				if c := g.grid.ColumnCount(); c <= n {
					t.Fatalf("альбом: колонок %d", c)
				}
				w.Resize(fyne.NewSize(width, width*2))
				if c := g.grid.ColumnCount(); c != n || m.Cover() != portrait {
					t.Fatalf("снова портрет: колонок %d, карточка %v", c, m.Cover())
				}
			})
		}
	}
}

// На разных телефонах при одной настройке — одно число колонок, на
// широком карточки шире.
func TestGridColumnsPhones(t *testing.T) {
	setMobile(t, true)
	test.NewTempApp(t)
	st := storage.NewMemSettings()
	app.SetGridColumns(st, 4)
	narrow := NewGridMetrics(st)
	gn, _ := gridWindow(t, narrow, 10, fyne.NewSize(360, 740))
	wide := NewGridMetrics(st)
	gw, _ := gridWindow(t, wide, 10, fyne.NewSize(412, 900))
	if gn.grid.ColumnCount() != 4 || gw.grid.ColumnCount() != 4 {
		t.Fatalf("колонок %d и %d", gn.grid.ColumnCount(), gw.grid.ColumnCount())
	}
	if wide.Cover().Width <= narrow.Cover().Width {
		t.Fatalf("карточки %v и %v", narrow.Cover(), wide.Cover())
	}
}

// ПК: размер карточек из настройки, колонки — по ширине окна.
func TestGridSizeDesktop(t *testing.T) {
	setMobile(t, false)
	test.NewTempApp(t)
	st := storage.NewMemSettings()
	m := NewGridMetrics(st)
	if m.Cover() != fyne.NewSize(160, 226) {
		t.Fatalf("по умолчанию %v", m.Cover())
	}
	g, w := gridWindow(t, m, 30, fyne.NewSize(900, 600))
	medium := g.grid.ColumnCount()

	app.SetGridSize(st, app.GridSizeLarge)
	m.Reload()
	if m.Cover().Width != 200 {
		t.Fatalf("крупные: %v", m.Cover())
	}
	if c := g.grid.ColumnCount(); c >= medium {
		t.Fatalf("крупные: колонок %d, было %d", c, medium)
	}
	app.SetGridSize(st, app.GridSizeSmall)
	m.Reload()
	if c := g.grid.ColumnCount(); c <= medium || m.Cover().Width != 130 {
		t.Fatalf("маленькие: колонок %d, карточка %v", c, m.Cover())
	}
	// колонки по-прежнему по ширине окна
	w.Resize(fyne.NewSize(1400, 600))
	if c := g.grid.ColumnCount(); c <= medium {
		t.Fatalf("шире окно: колонок %d", c)
	}
}

// Смена «Карточек в ряду» применяется сразу к обеим сеткам.
func TestGridReloadBothGrids(t *testing.T) {
	setMobile(t, true)
	test.NewTempApp(t)
	st := storage.NewMemSettings()
	m := NewGridMetrics(st)
	lib, _ := gridWindow(t, m, 30, fyne.NewSize(412, 900))
	found, _ := gridWindow(t, m, 30, fyne.NewSize(412, 900))
	if lib.grid.ColumnCount() != 3 || found.grid.ColumnCount() != 3 {
		t.Fatalf("по умолчанию: %d и %d", lib.grid.ColumnCount(), found.grid.ColumnCount())
	}
	for _, n := range []int{4, 2} {
		app.SetGridColumns(st, n)
		m.Reload()
		if lib.grid.ColumnCount() != n || found.grid.ColumnCount() != n {
			t.Fatalf("после выбора %d: %d и %d", n, lib.grid.ColumnCount(), found.grid.ColumnCount())
		}
	}
}

// Повторная раскладка того же размера не перестраивает сетку.
func TestGridFitStable(t *testing.T) {
	setMobile(t, true)
	test.NewTempApp(t)
	m := NewGridMetrics(storage.NewMemSettings())
	gridWindow(t, m, 10, fyne.NewSize(412, 900))
	if m.fit(m.avail) || m.fit(m.avail+0.2) {
		t.Fatal("тот же размер изменил метрики")
	}
}

// После смены размера карточки не показывают старую миниатюру: до загрузки
// под новую область — заглушка.
func TestGridReloadShowsPlaceholder(t *testing.T) {
	setMobile(t, true)
	test.NewTempApp(t)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 40, 60))); err != nil {
		t.Fatal(err)
	}
	st := storage.NewMemSettings()
	m := NewGridMetrics(st)
	th := thumbs.New(func(model.Key, string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}, 1<<20, 1)
	g := newGalleryGrid(th, m, &GalleryActions{Open: func(model.Gallery) {}})
	q := make(chan func(), 64)
	g.do = func(f func()) { q <- f }
	g.items = []model.Gallery{galleryOf("a.zip")}
	w := fyne.CurrentApp().NewWindow("t")
	w.SetContent(g.Widget())
	w.Resize(fyne.NewSize(412, 900))

	card := visibleCard(t, g)
	deadline := time.Now().Add(3 * time.Second)
	for !card.cover.Visible() && time.Now().Before(deadline) {
		select {
		case f := <-q:
			f()
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if !card.cover.Visible() {
		t.Fatal("обложка не загрузилась")
	}

	app.SetGridColumns(st, 2)
	m.Reload()
	card = visibleCard(t, g)
	if card.cover.Visible() || !card.icon.Visible() {
		t.Fatal("после смены размера видна прежняя миниатюра")
	}
}

// visibleCard — единственная карточка на экране. Дерево обходится без
// раскладки: test.LaidOutObjects перераскладывает GridWrap и создаёт новые карточки.
func visibleCard(t *testing.T, g *galleryGrid) *galleryCard {
	t.Helper()
	var found []*galleryCard
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch o := o.(type) {
		case *galleryCard:
			found = append(found, o)
		case *fyne.Container:
			for _, c := range o.Objects {
				walk(c)
			}
		case fyne.Widget:
			for _, c := range test.WidgetRenderer(o).Objects() {
				walk(c)
			}
		}
	}
	walk(g.grid)
	if len(found) != 1 {
		t.Fatalf("карточек на экране: %d", len(found))
	}
	return found[0]
}

package screens

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"io"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/model"
	"mangareader/internal/storage"
	"mangareader/internal/thumbs"
)

// coverGrid — сетка из n галерей в окне size; миниатюры всех галерей уже в
// кэше, часы пула подменены (clock).
type coverGrid struct {
	g     *galleryGrid
	clock time.Time
}

func newCoverGrid(t *testing.T, n int, size fyne.Size) *coverGrid {
	t.Helper()
	a := test.NewTempApp(t)
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 40, 60))); err != nil {
		t.Fatal(err)
	}
	th := thumbs.New(func(model.Key, string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
	}, 64<<20, 2)
	g := newGalleryGrid(th, NewGridMetrics(storage.NewMemSettings()), &GalleryActions{Open: func(model.Gallery) {}})
	g.do = func(f func()) { f() } // миниатюры уже в кэше: колбэков загрузки нет
	cg := &coverGrid{g: g, clock: time.Unix(1e6, 0)}
	g.covers.pool.now = func() time.Time { return cg.clock }
	g.covers.startAnim = func() {} // такты — только drain
	w := a.NewWindow("t")
	w.SetContent(g.Widget())
	w.Resize(size)
	if !g.setThumbBox() {
		t.Fatal("сетка не на экране")
	}
	items := make([]model.Gallery, n)
	done := make(chan struct{}, n)
	for i := range items {
		items[i] = galleryOf(fmt.Sprintf("%03d.zip", i))
		th.Load(items[i], func(image.Image, error) { done <- struct{}{} })
	}
	for range items {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("миниатюры не загрузились")
		}
	}
	g.SetItems(items)
	return cg
}

// drain выполняет такты с шагом dt, пока очередь не опустеет; время по часам пула.
func (cg *coverGrid) drain(t *testing.T, dt time.Duration) time.Duration {
	t.Helper()
	start := cg.clock
	for i := 0; len(cg.g.covers.queue) > 0; i++ {
		if i > 100000 {
			t.Fatal("очередь не опустела")
		}
		cg.clock = cg.clock.Add(dt)
		cg.g.covers.step(cg.clock)
	}
	return cg.clock.Sub(start)
}

// shownCards — видимые карточки с обложкой и без неё.
func (cg *coverGrid) cards(t *testing.T) []*galleryCard {
	t.Helper()
	var found []*galleryCard
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if !o.Visible() {
			return
		}
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
	walk(cg.g.grid)
	return found
}

// Время показа одинаковой очереди обложек не зависит от частоты тактов.
func TestCoverBudgetIndependentOfFrameRate(t *testing.T) {
	size := fyne.NewSize(960, 1400)
	measure := func(dt time.Duration) (time.Duration, int) {
		cg := newCoverGrid(t, 60, size)
		queued := len(cg.g.covers.queue)
		return cg.drain(t, dt), queued
	}
	fast, n1 := measure(2 * time.Millisecond)
	slow, n2 := measure(16700 * time.Microsecond)
	if n1 < 20 || n1 != n2 {
		t.Fatalf("в очереди %d и %d обложек, нужно ≥ 20 и поровну", n1, n2)
	}
	if d := fast - slow; d > 17*time.Millisecond || d < -17*time.Millisecond {
		t.Fatalf("500 Гц: %v, 60 Гц: %v", fast, slow)
	}
	want := time.Duration(float64(n1) / coverRateDesktop * float64(time.Second))
	if fast < want-20*time.Millisecond || fast > want+20*time.Millisecond {
		t.Fatalf("очередь из %d обложек показана за %v, ждали ~%v", n1, fast, want)
	}
}

// Пауза между тактами не даёт залп: за такт — не больше rate × 100 мс.
func TestCoverBudgetCapsPause(t *testing.T) {
	cg := newCoverGrid(t, 60, fyne.NewSize(960, 1400))
	p := cg.g.covers
	before := p.uploads
	cg.clock = cg.clock.Add(5 * time.Second)
	p.step(cg.clock)
	if got := p.uploads - before; got > int(coverRateDesktop*coverMaxStep.Seconds())+1 {
		t.Fatalf("после паузы показано %d обложек за такт", got)
	}
}

// Очередь — построчно по направлению прокрутки, внутри строки слева направо.
func TestCoverQueueOrder(t *testing.T) {
	cg := newCoverGrid(t, 60, fyne.NewSize(960, 1400))
	p := cg.g.covers
	cols := cg.g.grid.ColumnCount()
	if cols < 3 {
		t.Fatalf("колонок %d", cols)
	}
	ids := []int{2*cols + 1, 0, cols + 2, 2 * cols, cols, 1}
	order := func(dir int) []int {
		cg.g.dir = dir
		p.queue = nil
		for _, id := range ids {
			p.queue = append(p.queue, &galleryCard{id: id})
		}
		p.sortQueue()
		out := make([]int, len(p.queue))
		for i, c := range p.queue {
			out[i] = c.id
		}
		p.queue = nil
		return out
	}
	if got, want := order(1), []int{0, 1, cols, cols + 2, 2 * cols, 2*cols + 1}; !slices.Equal(got, want) {
		t.Fatalf("вниз: %v, ждали %v", got, want)
	}
	if got, want := order(-1), []int{2 * cols, 2*cols + 1, cols, cols + 2, 0, 1}; !slices.Equal(got, want) {
		t.Fatalf("вверх: %v, ждали %v", got, want)
	}
}

// Обновление сетки без изменений не трогает обложки: те же объекты, без
// новых загрузок и очереди.
func TestGridRefreshKeepsCovers(t *testing.T) {
	cg := newCoverGrid(t, 30, fyne.NewSize(960, 700))
	cg.drain(t, 16*time.Millisecond)
	cards := cg.cards(t)
	shown := map[*galleryCard]*pooledImage{}
	for _, c := range cards {
		if c.shown == nil {
			t.Fatalf("карточка %d без обложки", c.id)
		}
		shown[c] = c.shown
	}
	uploads := cg.g.covers.uploads
	cg.g.grid.Refresh()
	cg.g.SetItems(cg.g.Items())
	if len(cg.g.covers.queue) != 0 || cg.g.covers.uploads != uploads {
		t.Fatalf("очередь %d, загрузок %d (было %d)", len(cg.g.covers.queue), cg.g.covers.uploads, uploads)
	}
	for c, e := range shown {
		if c.shown != e || c.image().Image == nil {
			t.Fatalf("карточка %d сменила объект обложки", c.id)
		}
	}
}

// Возврат прокрутки: обложки верхнего экрана — сразу, без очереди и новых
// загрузок; через coverWarm без показа объект остывает.
func TestCoverScrollBackWarm(t *testing.T) {
	cg := newCoverGrid(t, 90, fyne.NewSize(960, 700))
	g := cg.g
	cg.drain(t, 16*time.Millisecond)
	g.ScrollToOffset(g.rowHeight() * float32(g.visibleRows()))
	if len(g.covers.queue) == 0 {
		t.Fatal("новый экран должен ждать очереди")
	}
	cg.drain(t, 16*time.Millisecond)
	uploads := g.covers.uploads

	cg.clock = cg.clock.Add(time.Second)
	g.ScrollToOffset(0)
	if len(g.covers.queue) != 0 || g.covers.uploads != uploads {
		t.Fatalf("возврат: очередь %d, загрузок %d (было %d)", len(g.covers.queue), g.covers.uploads, uploads)
	}
	for _, c := range cg.cards(t) {
		if c.image() == nil {
			t.Fatalf("карточка %d без обложки после возврата", c.id)
		}
	}

}

// Объект пула, не видный на экране дольше coverWarm, остыл: обложка ждёт
// очереди; тёплый ставится сразу.
func TestCoverColdObjectQueued(t *testing.T) {
	cg := newCoverGrid(t, 0, fyne.NewSize(960, 700))
	p := cg.g.covers
	img := image.NewRGBA(image.Rect(0, 0, 4, 6))
	e := p.pool.add("k", img)
	e.seen = cg.clock

	warm := newGalleryCard(cg.g.metrics)
	cg.clock = cg.clock.Add(coverWarm - time.Second)
	p.present(warm, "k", img, nil)
	if warm.shown != e || len(p.queue) != 0 {
		t.Fatal("тёплый объект должен встать сразу")
	}
	warm.showLoading() // карточка ушла с экрана: объект снят сейчас

	cold := newGalleryCard(cg.g.metrics)
	cg.clock = cg.clock.Add(coverWarm + time.Second)
	p.present(cold, "k", img, nil)
	if cold.shown != nil || len(p.queue) != 1 {
		t.Fatal("остывший объект должен ждать очереди")
	}
}

// Окно предзагрузки: в направлении прокрутки — два экрана, в обратном —
// один; сначала по направлению, от ближней карточки.
func TestPrefetchWindow(t *testing.T) {
	// 3 колонки, 2 строки на экран, видимы карточки 12…17
	down := prefetchWindow(12, 18, 3, 2, 100, 1)
	want := []int{18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 11, 10, 9, 8, 7, 6}
	if !slices.Equal(down, want) {
		t.Fatalf("вниз: %v", down)
	}
	up := prefetchWindow(12, 18, 3, 2, 100, -1)
	want = []int{11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1, 0, 18, 19, 20, 21, 22, 23}
	if !slices.Equal(up, want) {
		t.Fatalf("вверх: %v", up)
	}
	if got := prefetchWindow(0, 6, 3, 2, 8, 1); !slices.Equal(got, []int{6, 7}) {
		t.Fatalf("у края: %v", got)
	}
}

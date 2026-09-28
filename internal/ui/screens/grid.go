package screens

import (
	"image"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/model"
	"mangareader/internal/thumbs"
)

// galleryGrid — сетка карточек галерей (обложка + название) с фоновой
// загрузкой миниатюр. Общая для библиотеки и поиска. Меняется только в UI-потоке.
type galleryGrid struct {
	thumbs  *thumbs.Cache
	metrics *GridMetrics
	onOpen  func(model.Gallery)
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	items []model.Gallery
	grid  *widget.GridWrap
	// box — GridWrap в контейнере, который подгоняет размер карточек под
	// ширину сетки перед раскладкой.
	box *fyne.Container
}

func newGalleryGrid(th *thumbs.Cache, m *GridMetrics, onOpen func(model.Gallery)) *galleryGrid {
	g := &galleryGrid{thumbs: th, metrics: m, onOpen: onOpen, do: fyne.Do}
	m.grids = append(m.grids, g)
	g.grid = widget.NewGridWrap(
		func() int { return len(g.items) },
		func() fyne.CanvasObject { return newGalleryCard(g.metrics) },
		func(id widget.GridWrapItemID, o fyne.CanvasObject) { g.updateCard(id, o.(*galleryCard)) },
	)
	g.grid.OnSelected = func(id widget.GridWrapItemID) {
		g.grid.UnselectAll()
		if id >= 0 && id < len(g.items) {
			g.onOpen(g.items[id])
		}
	}
	g.box = container.New(gridLayout{g}, g.grid)
	return g
}

// Widget — объект сетки для размещения на экране.
func (g *galleryGrid) Widget() fyne.CanvasObject { return g.box }

// gridLayout раскладывает GridWrap на всю площадь, перед этим подгоняя
// размер карточек под ширину сетки (телефон).
type gridLayout struct{ g *galleryGrid }

func (l gridLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	l.g.fit(size.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(size)
}

func (l gridLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// fit подгоняет ширину карточки на телефоне под ширину сетки width. Ширина
// считается по короткой стороне экрана: в альбомной ориентации карточки
// того же размера, что в портретной, а колонок больше.
func (g *galleryGrid) fit(width float32) {
	if !g.metrics.mobile || width <= 0 {
		return
	}
	c := g.canvas()
	if c == nil {
		return
	}
	size := c.Size()
	_, area := c.InteractiveArea()
	// поля вокруг сетки (отступы экрана) — те же в любой ориентации
	avail := min(size.Width, size.Height) - (area.Width - width)
	if g.metrics.fit(avail) {
		g.metrics.changed()
	}
}

// canvas — холст сетки; до первой отрисовки — холст окна приложения (на
// телефоне оно одно). nil — окна ещё нет.
func (g *galleryGrid) canvas() fyne.Canvas {
	if c := fyne.CurrentApp().Driver().CanvasForObject(g.grid); c != nil {
		return c
	}
	if ws := fyne.CurrentApp().Driver().AllWindows(); len(ws) > 0 {
		return ws[0].Canvas()
	}
	return nil
}

// apply перестраивает сетку под новый размер карточек: GridWrap пересчитывает
// размер ячейки и число колонок, карточки заново запрашивают обложки под
// новую область (до загрузки — заглушка).
func (g *galleryGrid) apply() {
	g.setThumbBox()
	g.grid.Refresh()
	g.grid.Resize(g.grid.Size()) // сбрасывает кэш числа колонок GridWrap
}

// SetItems заменяет содержимое сетки и прокручивает её в начало.
func (g *galleryGrid) SetItems(items []model.Gallery) {
	g.items = items
	g.grid.Refresh()
}

// Items — текущие галереи сетки.
func (g *galleryGrid) Items() []model.Gallery { return g.items }

// updateCard заполняет переиспользуемую карточку данными галереи.
func (g *galleryGrid) updateCard(id widget.GridWrapItemID, c *galleryCard) {
	if id < 0 || id >= len(g.items) {
		return
	}
	gal := g.items[id]
	c.title.SetText(gal.Title)

	key := thumbs.CacheKey(gal)
	if c.cancel != nil && c.thumbKey != key {
		c.cancel() // карточка теперь для другой галереи — прежняя обложка не нужна
		c.cancel = nil
	}
	c.thumbKey = key
	if !g.setThumbBox() {
		c.showLoading() // размер экрана ещё неизвестен — обложка при следующем обновлении
		return
	}
	if img, err, ok := g.thumbs.Cached(gal); ok {
		c.showCover(img, err)
		return
	}
	if c.cancel != nil {
		return // эта обложка уже грузится для карточки
	}
	c.showLoading()
	c.cancel = g.thumbs.Load(gal, func(img image.Image, err error) {
		g.do(func() {
			// карточка могла быть переиспользована под другую галерею
			if c.thumbKey == key {
				c.cancel = nil
				c.showCover(img, err)
			}
		})
	})
}

// setThumbBox задаёт кэшу миниатюр область обложки карточки в физических
// пикселях экрана. false — сетка ещё не на экране и масштаб неизвестен.
func (g *galleryGrid) setThumbBox() bool {
	c := fyne.CurrentApp().Driver().CanvasForObject(g.grid)
	if c == nil {
		return false
	}
	scale, cover := c.Scale(), g.metrics.Cover()
	g.thumbs.SetBox(int(math.Ceil(float64(cover.Width*scale))), int(math.Ceil(float64(cover.Height*scale))))
	return true
}

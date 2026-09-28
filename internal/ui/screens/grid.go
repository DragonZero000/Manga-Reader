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
	actions *GalleryActions
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	items []model.Gallery
	grid  *widget.GridWrap
	// box — GridWrap в контейнере, который подгоняет размер карточек под
	// ширину сетки перед раскладкой.
	box *fyne.Container
}

// newGalleryGrid создаёт сетку; нажатие на карточку — actions.Open, меню
// карточки («⋮», правый клик, долгое нажатие) — actions.CardMenu.
func newGalleryGrid(th *thumbs.Cache, m *GridMetrics, actions *GalleryActions) *galleryGrid {
	g := &galleryGrid{thumbs: th, metrics: m, actions: actions, do: fyne.Do}
	m.grids = append(m.grids, g)
	g.grid = widget.NewGridWrap(
		func() int { return len(g.items) },
		func() fyne.CanvasObject { return g.newCard() },
		func(id widget.GridWrapItemID, o fyne.CanvasObject) { g.updateCard(id, o.(*galleryCard)) },
	)
	// выбор с клавиатуры (ПК); нажатия мышью и пальцем обрабатывает карточка
	g.grid.OnSelected = func(id widget.GridWrapItemID) {
		g.grid.UnselectAll()
		g.open(id)
	}
	g.box = container.New(gridLayout{g}, g.grid)
	return g
}

func (g *galleryGrid) newCard() *galleryCard {
	c := newGalleryCard(g.metrics)
	c.onTap = g.open
	c.onMenu = g.showMenu
	return c
}

// item — галерея на позиции id; false — позиции уже нет.
func (g *galleryGrid) item(id widget.GridWrapItemID) (model.Gallery, bool) {
	if id < 0 || id >= len(g.items) {
		return model.Gallery{}, false
	}
	return g.items[id], true
}

func (g *galleryGrid) open(id widget.GridWrapItemID) {
	if it, ok := g.item(id); ok && g.actions.Open != nil {
		g.actions.Open(it)
	}
}

// showMenu показывает меню галереи id: в точке pos или, если pos == nil,
// под объектом anchor.
func (g *galleryGrid) showMenu(id widget.GridWrapItemID, anchor fyne.CanvasObject, pos *fyne.Position) {
	it, ok := g.item(id)
	if !ok {
		return
	}
	m := g.actions.CardMenu(it)
	if pos == nil {
		ShowMenuBelow(m, anchor)
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(anchor); c != nil && len(m.Items) > 0 {
		showMenu(m, c, *pos)
	}
}

// Remove убирает галерею с ключом k из сетки (после удаления файла);
// false — её в сетке нет.
func (g *galleryGrid) Remove(k model.Key) bool {
	for i, it := range g.items {
		if it.Key == k {
			items := make([]model.Gallery, 0, len(g.items)-1)
			g.items = append(append(items, g.items[:i]...), g.items[i+1:]...)
			g.grid.Refresh()
			return true
		}
	}
	return false
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
	c.id = id
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

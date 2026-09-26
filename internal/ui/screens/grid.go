package screens

import (
	"image"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/model"
	"mangareader/internal/thumbs"
)

// galleryGrid — сетка карточек галерей (обложка + название) с фоновой
// загрузкой миниатюр. Общая для библиотеки и поиска. Меняется только в UI-потоке.
type galleryGrid struct {
	thumbs    *thumbs.Cache
	coverSize fyne.Size
	onOpen    func(model.Gallery)
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	items []model.Gallery
	grid  *widget.GridWrap
}

func newGalleryGrid(th *thumbs.Cache, onOpen func(model.Gallery)) *galleryGrid {
	g := &galleryGrid{thumbs: th, onOpen: onOpen, do: fyne.Do, coverSize: coverSizeDesktop}
	if fyne.CurrentDevice().IsMobile() {
		g.coverSize = coverSizeMobile
	}
	g.grid = widget.NewGridWrap(
		func() int { return len(g.items) },
		func() fyne.CanvasObject { return newGalleryCard(g.coverSize) },
		func(id widget.GridWrapItemID, o fyne.CanvasObject) { g.updateCard(id, o.(*galleryCard)) },
	)
	g.grid.OnSelected = func(id widget.GridWrapItemID) {
		g.grid.UnselectAll()
		if id >= 0 && id < len(g.items) {
			g.onOpen(g.items[id])
		}
	}
	return g
}

// Widget — объект сетки для размещения на экране.
func (g *galleryGrid) Widget() *widget.GridWrap { return g.grid }

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
	scale := c.Scale()
	g.thumbs.SetBox(int(math.Ceil(float64(g.coverSize.Width*scale))), int(math.Ceil(float64(g.coverSize.Height*scale))))
	return true
}

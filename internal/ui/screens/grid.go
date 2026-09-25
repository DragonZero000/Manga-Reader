package screens

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/model"
	"mangareader/internal/thumbs"
)

// galleryGrid — сетка карточек галерей (обложка + название) с фоновой
// загрузкой миниатюр. Общая для библиотеки и поиска. Меняется только в UI-потоке.
type galleryGrid struct {
	thumbs *thumbs.Cache
	onOpen func(model.Gallery)
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	items []model.Gallery
	grid  *widget.GridWrap
}

func newGalleryGrid(th *thumbs.Cache, onOpen func(model.Gallery)) *galleryGrid {
	g := &galleryGrid{thumbs: th, onOpen: onOpen, do: fyne.Do}
	coverSize := coverSizeDesktop
	if fyne.CurrentDevice().IsMobile() {
		coverSize = coverSizeMobile
	}
	g.grid = widget.NewGridWrap(
		func() int { return len(g.items) },
		func() fyne.CanvasObject { return newGalleryCard(coverSize) },
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
	c.thumbKey = key
	if img, err, ok := g.thumbs.Cached(gal); ok {
		c.showCover(img, err)
		return
	}
	c.showLoading()
	g.thumbs.Load(gal, func(img image.Image, err error) {
		g.do(func() {
			// карточка могла быть переиспользована под другую галерею
			if c.thumbKey == key {
				c.showCover(img, err)
			}
		})
	})
}

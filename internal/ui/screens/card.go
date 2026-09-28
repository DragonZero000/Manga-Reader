package screens

import (
	"image"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// galleryCard — карточка галереи: обложка и название в две строки.
// Виджет переиспользуется сеткой для разных галерей.
type galleryCard struct {
	widget.BaseWidget

	cover *canvas.Image
	bg    *canvas.Rectangle
	icon  *widget.Icon // заглушка на время загрузки или значок ошибки
	title *widget.Label

	// thumbKey — миниатюра, которую карточка ждёт сейчас. Меняется только
	// в UI-потоке; устаревшие результаты загрузки отбрасываются.
	thumbKey string
	// cancel отменяет загрузку обложки thumbKey (nil — не грузится).
	cancel func()

	content fyne.CanvasObject
}

func newGalleryCard(m *GridMetrics) *galleryCard {
	c := &galleryCard{
		cover: canvas.NewImageFromImage(nil),
		bg:    canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		icon:  widget.NewIcon(theme.FileImageIcon()),
		title: widget.NewLabel(""),
	}
	c.cover.FillMode = canvas.ImageFillContain
	c.cover.ScaleMode = canvas.ImageScaleFastest // миниатюра уже по размеру карточки
	c.bg.CornerRadius = theme.InputRadiusSize()

	c.title.Wrapping = fyne.TextWrapWord
	c.title.Truncation = fyne.TextTruncateEllipsis
	c.title.Alignment = fyne.TextAlignCenter
	// две строки текста: высота строки × 2 + внутренние отступы надписи
	titleHeight := c.title.MinSize().Height*2 - theme.InnerPadding()

	iconBox := container.NewCenter(container.New(layout.NewGridWrapLayout(fyne.NewSquareSize(48)), c.icon))
	c.content = container.New(&cardLayout{metrics: m, titleHeight: titleHeight},
		container.NewStack(c.bg, iconBox, c.cover), c.title)
	c.ExtendBaseWidget(c)
	return c
}

// cardLayout — обложка сверху, под ней название. Размер карточки берётся
// из метрик сетки при каждом вызове: смена плотности не пересоздаёт карточки.
type cardLayout struct {
	metrics     *GridMetrics
	titleHeight float32
}

func (l *cardLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	cover := l.metrics.Cover()
	return fyne.NewSize(cover.Width, cover.Height+theme.Padding()+l.titleHeight)
}

// Layout: objs[0] — обложка, objs[1] — название.
func (l *cardLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	coverHeight := size.Height - theme.Padding() - l.titleHeight
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(size.Width, coverHeight))
	objs[1].Move(fyne.NewPos(0, coverHeight+theme.Padding()))
	objs[1].Resize(fyne.NewSize(size.Width, l.titleHeight))
}

func (c *galleryCard) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(c.content)
}

// showLoading показывает заглушку до загрузки обложки.
func (c *galleryCard) showLoading() {
	c.cover.Image = nil
	c.cover.Hide()
	c.icon.SetResource(theme.FileImageIcon())
	c.icon.Show()
}

// showCover показывает миниатюру или значок ошибки.
func (c *galleryCard) showCover(img image.Image, err error) {
	if err != nil || img == nil {
		c.cover.Image = nil
		c.cover.Hide()
		c.icon.SetResource(theme.ErrorIcon())
		c.icon.Show()
		return
	}
	c.icon.Hide()
	c.cover.Image = img
	c.cover.Show()
	c.cover.Refresh()
}

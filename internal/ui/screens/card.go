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

// Размеры карточки в сетке.
var (
	coverSizeDesktop = fyne.NewSize(160, 226)
	coverSizeMobile  = fyne.NewSize(128, 181)
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

func newGalleryCard(coverSize fyne.Size) *galleryCard {
	c := &galleryCard{
		cover: canvas.NewImageFromImage(nil),
		bg:    canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		icon:  widget.NewIcon(theme.FileImageIcon()),
		title: widget.NewLabel(""),
	}
	c.cover.FillMode = canvas.ImageFillContain
	c.cover.ScaleMode = canvas.ImageScaleFastest // миниатюра уже по размеру карточки
	c.cover.SetMinSize(coverSize)
	c.bg.CornerRadius = theme.InputRadiusSize()

	c.title.Wrapping = fyne.TextWrapWord
	c.title.Truncation = fyne.TextTruncateEllipsis
	c.title.Alignment = fyne.TextAlignCenter
	// две строки текста: высота строки × 2 + внутренние отступы надписи
	titleHeight := c.title.MinSize().Height*2 - theme.InnerPadding()
	titleBox := container.New(layout.NewGridWrapLayout(fyne.NewSize(coverSize.Width, titleHeight)), c.title)

	iconBox := container.NewCenter(container.New(layout.NewGridWrapLayout(fyne.NewSquareSize(48)), c.icon))
	c.content = container.NewBorder(nil, titleBox, nil, nil,
		container.NewStack(c.bg, iconBox, c.cover))
	c.ExtendBaseWidget(c)
	return c
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

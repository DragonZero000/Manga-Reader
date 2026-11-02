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

// galleryCard — карточка галереи: обложка с кнопкой «⋮» и название в две
// строки. Виджет переиспользуется сеткой для разных галерей.
//
// Нажатие карточка обрабатывает сама: Fyne отдаёт касание самому глубокому
// объекту, который принимает обычное или вторичное нажатие, поэтому
// карточка с меню по вторичному нажатию должна принимать и обычное.
type galleryCard struct {
	widget.BaseWidget

	// slot — место обложки: объект изображения из пула сетки (shown) или пусто.
	slot  *fyne.Container
	shown *pooledImage
	bg    *canvas.Rectangle
	icon  *widget.Icon // заглушка на время загрузки или значок ошибки
	title *widget.Label
	menu  *MenuButton

	// id — позиция галереи карточки в сетке; галерея берётся из сетки в
	// момент нажатия (карточка переиспользуется).
	id widget.GridWrapItemID
	// onTap — обычное нажатие, onMenu — меню в точке pos (nil — под «⋮»).
	onTap  func(id widget.GridWrapItemID)
	onMenu func(id widget.GridWrapItemID, anchor fyne.CanvasObject, pos *fyne.Position)

	// thumbKey — миниатюра, которую карточка ждёт сейчас. Меняется только
	// в UI-потоке; устаревшие результаты загрузки отбрасываются.
	thumbKey string
	// cancel отменяет загрузку обложки thumbKey (nil — не грузится).
	cancel func()
	// want — ключ пула обложки, которую карточка показывает или ждёт;
	// ready — готовая миниатюра, пока карточка в очереди показа (nil — не в очереди).
	want  string
	ready image.Image
	// iconRes — значок заглушки, показанный сейчас.
	iconRes fyne.Resource

	content fyne.CanvasObject
}

func newGalleryCard(m *GridMetrics) *galleryCard {
	c := &galleryCard{
		slot:    container.NewStack(),
		bg:      canvas.NewRectangle(theme.Color(theme.ColorNameInputBackground)),
		icon:    widget.NewIcon(theme.FileImageIcon()),
		iconRes: theme.FileImageIcon(),
		title:   widget.NewLabel(""),
	}
	c.bg.CornerRadius = theme.InputRadiusSize()

	c.title.Wrapping = fyne.TextWrapWord
	c.title.Truncation = fyne.TextTruncateEllipsis
	c.title.Alignment = fyne.TextAlignCenter
	// две строки текста: высота строки × 2 + внутренние отступы надписи
	titleHeight := c.title.MinSize().Height*2 - theme.InnerPadding()

	c.menu = NewMenuButton(func() {
		if c.onMenu != nil {
			c.onMenu(c.id, c.menu, nil)
		}
	})
	iconBox := container.NewCenter(container.New(layout.NewGridWrapLayout(fyne.NewSquareSize(48)), c.icon))
	corner := container.New(topRightLayout{}, c.menu)
	c.content = container.New(&cardLayout{metrics: m, titleHeight: titleHeight},
		container.NewStack(c.bg, iconBox, c.slot, corner), c.title)
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

// Tapped — обычное нажатие: страница произведения.
func (c *galleryCard) Tapped(*fyne.PointEvent) {
	if c.onTap != nil {
		c.onTap(c.id)
	}
}

// TappedSecondary — правый клик (ПК) или долгое нажатие (телефон): меню в
// точке нажатия.
func (c *galleryCard) TappedSecondary(ev *fyne.PointEvent) {
	if c.onMenu != nil {
		pos := ev.AbsolutePosition
		c.onMenu(c.id, c, &pos)
	}
}

// setTitle меняет название, только если оно другое.
func (c *galleryCard) setTitle(text string) {
	if c.title.Text != text {
		c.title.SetText(text)
	}
}

// showLoading показывает заглушку до загрузки обложки.
func (c *galleryCard) showLoading() { c.showIcon(theme.FileImageIcon()) }

// showError показывает значок ошибки вместо обложки.
func (c *galleryCard) showError() {
	c.want = ""
	c.showIcon(theme.ErrorIcon())
}

func (c *galleryCard) showIcon(res fyne.Resource) {
	c.detach()
	if c.iconRes != res {
		c.iconRes = res
		c.icon.SetResource(res)
	}
	if !c.icon.Visible() {
		c.icon.Show()
	}
}

// setImage показывает объект пула e. Объект не обновляется (Refresh
// освободил бы его текстуру), обновляется только место обложки.
func (c *galleryCard) setImage(e *pooledImage) {
	if c.shown == e {
		return
	}
	c.detach()
	c.shown, e.owner = e, c
	c.slot.Objects = []fyne.CanvasObject{e.img}
	c.slot.Layout.Layout(c.slot.Objects, c.slot.Size())
	if c.icon.Visible() {
		c.icon.Hide()
	}
	canvas.Refresh(c.slot)
}

// detach снимает с карточки объект пула (если он есть).
func (c *galleryCard) detach() {
	e := c.shown
	if e == nil {
		return
	}
	c.shown, e.owner = nil, nil
	e.seen = e.pool.now()
	c.slot.Objects = nil
	canvas.Refresh(c.slot)
}

// image — изображение обложки на карточке (nil — заглушка или ошибка).
func (c *galleryCard) image() *canvas.Image {
	if c.shown == nil {
		return nil
	}
	return c.shown.img
}

package screens

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
)

// GalleryActions — действия меню галереи (карточка, страница произведения).
// nil — пункта нет: например, «Показать в папке» вне Windows или
// «Удалить…», если хранилище не умеет удалять.
type GalleryActions struct {
	Open         func(model.Gallery) // страница произведения
	Read         func(model.Gallery) // читалка с первой страницы
	OpenURL      func(model.Gallery) // ссылка на произведение; пункт — только если ссылка есть
	CopyTitle    func(model.Gallery)
	ShowInFolder func(model.Gallery)
	Delete       func(model.Gallery)
	// ResetProgress сбрасывает позицию чтения; пункт — только если
	// HasProgress(g) (оба nil — пункта нет).
	ResetProgress func(model.Gallery)
	HasProgress   func(model.Gallery) bool
}

// CardMenu — меню карточки: «Открыть», «Читать», «Открыть в браузере»,
// «Скопировать название», «Показать в папке», разделитель, «Удалить…».
func (a *GalleryActions) CardMenu(g model.Gallery) *fyne.Menu {
	var items []*fyne.MenuItem
	items = a.add(items, g, "menu.open", theme.VisibilityIcon(), a.Open)
	items = a.add(items, g, "menu.read", theme.MediaPlayIcon(), a.Read)
	if g.SourceURL != "" {
		items = a.add(items, g, "menu.open_in_browser", theme.ComputerIcon(), a.OpenURL)
	}
	return fyne.NewMenu("", a.tail(items, g)...)
}

// DetailsMenu — меню страницы произведения: только действия, которых нет на
// самой странице.
func (a *GalleryActions) DetailsMenu(g model.Gallery) *fyne.Menu {
	return fyne.NewMenu("", a.tail(nil, g)...)
}

// tail — общие пункты: копирование, папка, сброс прогресса, разделитель,
// удаление.
func (a *GalleryActions) tail(items []*fyne.MenuItem, g model.Gallery) []*fyne.MenuItem {
	items = a.add(items, g, "menu.copy_title", theme.ContentCopyIcon(), a.CopyTitle)
	items = a.add(items, g, "menu.show_in_folder", theme.FolderOpenIcon(), a.ShowInFolder)
	if a.HasProgress != nil && a.HasProgress(g) {
		items = a.add(items, g, "menu.reset_progress", theme.HistoryIcon(), a.ResetProgress)
	}
	if a.Delete != nil {
		if len(items) > 0 {
			items = append(items, fyne.NewMenuItemSeparator())
		}
		items = a.add(items, g, "menu.delete", theme.DeleteIcon(), a.Delete)
	}
	return items
}

func (a *GalleryActions) add(items []*fyne.MenuItem, g model.Gallery, key string, icon fyne.Resource, fn func(model.Gallery)) []*fyne.MenuItem {
	if fn == nil {
		return items
	}
	it := fyne.NewMenuItem(i18n.T(key), func() { fn(g) })
	it.Icon = icon
	return append(items, it)
}

// showMenu показывает всплывающее меню в точке pos холста c; подменяется в тестах.
var showMenu = func(m *fyne.Menu, c fyne.Canvas, pos fyne.Position) {
	widget.ShowPopUpMenuAtPosition(m, c, pos)
}

// ShowMenuBelow показывает меню под объектом o (кнопка «⋮»).
func ShowMenuBelow(m *fyne.Menu, o fyne.CanvasObject) {
	d := fyne.CurrentApp().Driver()
	c := d.CanvasForObject(o)
	if c == nil || len(m.Items) == 0 {
		return
	}
	showMenu(m, c, d.AbsolutePositionForObject(o).AddXY(0, o.Size().Height))
}

// Размеры кнопки «⋮»: видимый круг и область нажатия (не меньше 40 dp —
// удобно попасть пальцем даже при 4 карточках в ряд).
const (
	menuCircleSize = 28
	menuTapSize    = 40
	menuIconSize   = 18
	// menuCircleAlpha — непрозрачность круга: сквозь него видна обложка.
	menuCircleAlpha = 0x99
)

// MenuButton — кнопка «⋮»: значок на полупрозрачном круге в центре
// прозрачной области нажатия.
type MenuButton struct {
	widget.BaseWidget
	OnTapped func()
}

// NewMenuButton создаёт кнопку «⋮».
func NewMenuButton(tapped func()) *MenuButton {
	b := &MenuButton{OnTapped: tapped}
	b.ExtendBaseWidget(b)
	return b
}

func (b *MenuButton) Tapped(*fyne.PointEvent) {
	if b.OnTapped != nil {
		b.OnTapped()
	}
}

func (b *MenuButton) MinSize() fyne.Size { return fyne.NewSquareSize(menuTapSize) }

func (b *MenuButton) CreateRenderer() fyne.WidgetRenderer {
	r := &menuButtonRenderer{circle: canvas.NewCircle(color.Transparent),
		icon: canvas.NewImageFromResource(theme.MoreVerticalIcon())}
	r.icon.FillMode = canvas.ImageFillContain
	r.Refresh()
	return r
}

type menuButtonRenderer struct {
	circle *canvas.Circle
	icon   *canvas.Image
}

func (r *menuButtonRenderer) Layout(size fyne.Size) {
	center := fyne.NewPos(size.Width/2, size.Height/2)
	r.circle.Move(center.SubtractXY(menuCircleSize/2, menuCircleSize/2))
	r.circle.Resize(fyne.NewSquareSize(menuCircleSize))
	r.icon.Move(center.SubtractXY(menuIconSize/2, menuIconSize/2))
	r.icon.Resize(fyne.NewSquareSize(menuIconSize))
}

func (r *menuButtonRenderer) MinSize() fyne.Size { return fyne.NewSquareSize(menuTapSize) }

// Refresh берёт цвета из текущей темы: фон — полупрозрачный цвет фона,
// значок — цвета текста.
func (r *menuButtonRenderer) Refresh() {
	bg := color.NRGBAModel.Convert(theme.Color(theme.ColorNameBackground)).(color.NRGBA)
	bg.A = menuCircleAlpha
	r.circle.FillColor = bg
	r.circle.Refresh()
	r.icon.Resource = theme.MoreVerticalIcon()
	r.icon.Refresh()
}

func (r *menuButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.circle, r.icon}
}

func (r *menuButtonRenderer) Destroy() {}

// topRightLayout — единственный объект в правом верхнем углу в размере MinSize.
type topRightLayout struct{}

func (topRightLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		s := o.MinSize()
		o.Resize(s)
		o.Move(fyne.NewPos(size.Width-s.Width, 0))
	}
}

func (topRightLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.Size{} }

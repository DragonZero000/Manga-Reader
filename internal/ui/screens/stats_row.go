package screens

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
)

// Размеры строки статистики.
const (
	statBarHeight = 4
	// statLeadSize — ширина стрелки раскрытия слева от подписи.
	statLeadSize = 32
	// statIndent — отступ строк месяцев внутри года.
	statIndent = 24
)

// rowData — данные строки статистики: подпись, значение, полоска (доля
// 0–1 от наибольшего в блоке) и действия.
type rowData struct {
	Label string
	Value string
	// Frac — длина полоски; NoBar — строка без полоски (сводка).
	Frac  float32
	NoBar bool
	// Tap — нажатие (nil — строка не нажимается).
	Tap func()
	// Menu — меню правого клика или долгого нажатия (nil — нет меню).
	Menu func() *fyne.Menu
	// Lead — стрелка раскрытия слева (nil — нет): значок и действие.
	LeadIcon fyne.Resource
	Lead     func()
	// Indent — строка вложена (месяц внутри года).
	Indent bool
}

// statRow — строка блока: подпись слева, значение справа, под ними полоска.
// Виджет переиспользуется: set меняет данные без пересоздания объектов.
//
// Нажатие строка обрабатывает сама: Fyne отдаёт касание самому глубокому
// объекту, который принимает обычное или вторичное нажатие.
type statRow struct {
	widget.BaseWidget
	data rowData

	label *widget.Label
	value *widget.Label
	track *canvas.Rectangle
	fill  *canvas.Rectangle
	lead  *statLead
	hover *canvas.Rectangle

	hovered bool
	// menuOpen — открыто меню строки: строка подсвечена, пока его не закроют.
	menuOpen bool
}

var (
	_ fyne.Tappable          = (*statRow)(nil)
	_ fyne.SecondaryTappable = (*statRow)(nil)
	_ desktop.Hoverable      = (*statRow)(nil)
	_ desktop.Cursorable     = (*statRow)(nil)
)

func newStatRow() *statRow {
	r := &statRow{
		label: widget.NewLabel(""),
		value: widget.NewLabel(""),
		track: canvas.NewRectangle(color.Transparent),
		fill:  canvas.NewRectangle(color.Transparent),
		hover: canvas.NewRectangle(color.Transparent),
		lead:  newStatLead(),
	}
	r.label.Truncation = fyne.TextTruncateEllipsis
	r.value.Alignment = fyne.TextAlignTrailing
	r.lead.Hide()
	r.ExtendBaseWidget(r)
	return r
}

// set показывает данные d. Объекты обновляются, только если что-то изменилось.
func (r *statRow) set(d rowData) {
	r.data = d
	if r.label.Text != d.Label {
		r.label.SetText(d.Label)
	}
	if r.value.Text != d.Value {
		r.value.SetText(d.Value)
	}
	r.lead.set(d.LeadIcon, d.Lead)
	r.Refresh()
}

func (r *statRow) CreateRenderer() fyne.WidgetRenderer {
	return &statRowRenderer{r: r}
}

func (r *statRow) Tapped(*fyne.PointEvent) {
	if r.data.Tap != nil {
		r.data.Tap()
	}
}

// TappedSecondary — правый клик (ПК) или долгое нажатие (телефон): меню.
func (r *statRow) TappedSecondary(ev *fyne.PointEvent) {
	if r.data.Menu == nil {
		return
	}
	m := r.data.Menu()
	if m == nil || len(m.Items) == 0 {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(r); c != nil {
		r.menuOpen = true
		r.Refresh()
		showRowMenu(m, c, ev.AbsolutePosition, func() {
			r.menuOpen = false
			r.Refresh()
		})
	}
}

func (r *statRow) MouseIn(*desktop.MouseEvent) {
	if r.data.Tap == nil {
		return
	}
	r.hovered = true
	r.Refresh()
}

func (r *statRow) MouseMoved(*desktop.MouseEvent) {}

func (r *statRow) MouseOut() {
	if r.hovered {
		r.hovered = false
		r.Refresh()
	}
}

func (r *statRow) Cursor() desktop.Cursor {
	if r.data.Tap != nil {
		return desktop.PointerCursor
	}
	return desktop.DefaultCursor
}

// statRowRenderer — раскладка: стрелка, подпись, значение справа, полоска.
type statRowRenderer struct {
	r *statRow
}

func (sr *statRowRenderer) Objects() []fyne.CanvasObject {
	r := sr.r
	return []fyne.CanvasObject{r.hover, r.track, r.fill, r.label, r.value, r.lead}
}

func (sr *statRowRenderer) Destroy() {}

func (sr *statRowRenderer) textHeight() float32 {
	return sr.r.label.MinSize().Height
}

func (sr *statRowRenderer) MinSize() fyne.Size {
	r := sr.r
	h := sr.textHeight()
	if !r.data.NoBar {
		h += statBarHeight + theme.Padding()
	}
	w := r.value.MinSize().Width + theme.Padding()*8
	return fyne.NewSize(w, h)
}

func (sr *statRowRenderer) Layout(size fyne.Size) {
	r := sr.r
	x := float32(0)
	if r.data.Indent {
		x = statIndent
	}
	r.hover.Move(fyne.NewPos(0, 0))
	r.hover.Resize(size)
	if r.data.LeadIcon != nil {
		r.lead.Move(fyne.NewPos(x, 0))
		r.lead.Resize(fyne.NewSize(statLeadSize, sr.textHeight()))
		x += statLeadSize
	}
	th := sr.textHeight()
	vw := r.value.MinSize().Width
	r.value.Move(fyne.NewPos(size.Width-vw, 0))
	r.value.Resize(fyne.NewSize(vw, th))
	r.label.Move(fyne.NewPos(x, 0))
	r.label.Resize(fyne.NewSize(max(0, size.Width-vw-x), th))

	barW := max(0, size.Width-x-theme.Padding())
	y := th
	r.track.Move(fyne.NewPos(x+theme.Padding(), y))
	r.track.Resize(fyne.NewSize(barW, statBarHeight))
	r.fill.Move(fyne.NewPos(x+theme.Padding(), y))
	r.fill.Resize(fyne.NewSize(barW*max(0, min(1, r.data.Frac)), statBarHeight))
}

func (sr *statRowRenderer) Refresh() {
	r := sr.r
	if r.hovered || r.menuOpen {
		r.hover.FillColor = theme.Color(theme.ColorNameHover)
	} else {
		r.hover.FillColor = color.Transparent
	}
	if r.data.NoBar {
		r.track.Hide()
		r.fill.Hide()
	} else {
		r.track.Show()
		// полоска нулевой длины со скруглением рисуется штрихом — скрываем
		if r.data.Frac > 0 {
			r.fill.Show()
		} else {
			r.fill.Hide()
		}
		r.track.FillColor = theme.Color(theme.ColorNameInputBackground)
		r.fill.FillColor = theme.Color(theme.ColorNamePrimary)
		r.track.CornerRadius = statBarHeight / 2
		r.fill.CornerRadius = statBarHeight / 2
	}
	if r.data.LeadIcon != nil {
		r.lead.Show()
	} else {
		r.lead.Hide()
	}
	sr.Layout(r.Size())
	r.hover.Refresh()
	r.track.Refresh()
	r.fill.Refresh()
	r.label.Refresh()
	r.value.Refresh()
	r.lead.Refresh()
}

// statLead — стрелка раскрытия года: значок и своё нажатие (не нажатие строки).
type statLead struct {
	widget.BaseWidget
	icon *widget.Icon
	tap  func()
}

func newStatLead() *statLead {
	l := &statLead{icon: widget.NewIcon(theme.MenuDropDownIcon())}
	l.ExtendBaseWidget(l)
	return l
}

func (l *statLead) set(res fyne.Resource, tap func()) {
	l.tap = tap
	if res != nil {
		l.icon.SetResource(res)
	}
}

func (l *statLead) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(l.icon)
}

func (l *statLead) Tapped(*fyne.PointEvent) {
	if l.tap != nil {
		l.tap()
	}
}

func (l *statLead) MinSize() fyne.Size { return fyne.NewSize(statLeadSize, statLeadSize) }

// rowList — список строк одного блока в вертикальном контейнере: строки
// создаются по мере надобности и переиспользуются по индексу, лишние
// скрываются. Пустой список показывает «нет данных».
type rowList struct {
	box   *fyne.Container
	empty *widget.Label
	rows  []*statRow
}

func newRowList() *rowList {
	empty := widget.NewLabel(i18n.T("stats.no_data"))
	empty.Importance = widget.LowImportance
	empty.Hide()
	return &rowList{box: container.NewVBox(empty), empty: empty}
}

// Set показывает строки data.
func (l *rowList) Set(data []rowData) {
	if len(data) == 0 {
		l.empty.Show()
	} else {
		l.empty.Hide()
	}
	for len(l.rows) < len(data) {
		r := newStatRow()
		l.rows = append(l.rows, r)
		l.box.Objects = append(l.box.Objects, r)
	}
	for i, r := range l.rows {
		if i < len(data) {
			r.set(data[i])
			if !r.Visible() {
				r.Show()
			}
		} else if r.Visible() {
			r.Hide()
		}
	}
	l.box.Refresh()
}

// Len — число показанных строк.
func (l *rowList) Len() int {
	n := 0
	for _, r := range l.rows {
		if r.Visible() {
			n++
		}
	}
	return n
}

// Row — i-я показанная строка (для тестов).
func (l *rowList) Row(i int) *statRow { return l.rows[i] }

// showRowMenu показывает меню строки в точке pos холста c и вызывает
// closed, когда меню закрыто (выбран пункт или нажатие мимо); подменяется в
// тестах.
var showRowMenu = func(m *fyne.Menu, c fyne.Canvas, pos fyne.Position, closed func()) {
	p := widget.NewPopUpMenu(m, c)
	if p == nil {
		closed()
		return
	}
	p.OnDismiss = func() {
		p.Hide()
		closed()
	}
	p.ShowAtPosition(pos)
}

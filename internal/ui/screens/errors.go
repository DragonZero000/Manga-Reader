package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/problems"
	"mangareader/internal/ui/details"
)

// Errors — экран ошибок библиотеки: файлы, которые приложение не может
// открыть. Список берётся из problems.Tracker; меняется только в UI-потоке.
type Errors struct {
	tracker *problems.Tracker

	list    *fyne.Container
	scroll  *container.Scroll
	empty   fyne.CanvasObject
	content fyne.CanvasObject
}

func NewErrors(tracker *problems.Tracker) *Errors {
	e := &Errors{tracker: tracker}
	e.list = container.NewVBox()
	e.scroll = container.NewVScroll(e.list)

	title := widget.NewLabelWithStyle("Ошибок нет", fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	hint := widget.NewLabel("Все файлы в папке библиотеки открываются.")
	hint.Alignment = fyne.TextAlignCenter
	hint.Wrapping = fyne.TextWrapWord
	e.empty = container.NewVBox(layout.NewSpacer(), title, hint, layout.NewSpacer())

	e.content = container.NewStack(e.empty, e.scroll)
	e.Refresh()
	return e
}

func (e *Errors) Content() fyne.CanvasObject { return e.content }

// Refresh перестраивает список по текущему состоянию Tracker. Ошибок обычно
// немного, поэтому строки создаются заново, а не переиспользуются.
func (e *Errors) Refresh() {
	items := e.tracker.Items()
	rows := make([]fyne.CanvasObject, 0, len(items))
	for i, it := range items {
		if i > 0 {
			rows = append(rows, widget.NewSeparator())
		}
		rows = append(rows, newErrorRow(it))
	}
	e.list.Objects = rows
	e.list.Refresh()
	if len(items) == 0 {
		e.scroll.Hide()
		e.empty.Show()
	} else {
		e.empty.Hide()
		e.scroll.Show()
	}
}

// Rows — число записей в списке (для тестов).
func (e *Errors) Rows() int {
	n := 0
	for _, o := range e.list.Objects {
		if _, ok := o.(*errorRow); ok {
			n++
		}
	}
	return n
}

// errorRow — строка ошибки: маркер непросмотренной, путь, причина, время.
type errorRow struct {
	widget.BaseWidget
	item problems.Item
}

func newErrorRow(it problems.Item) *errorRow {
	r := &errorRow{item: it}
	r.ExtendBaseWidget(r)
	return r
}

func (r *errorRow) CreateRenderer() fyne.WidgetRenderer {
	it := r.item
	path := widget.NewLabelWithStyle(it.RelPath, fyne.TextAlignLeading, fyne.TextStyle{Bold: !it.Seen})
	path.Wrapping = fyne.TextWrapBreak

	reason := widget.NewLabel(it.Reason)
	reason.Wrapping = fyne.TextWrapWord
	reason.Importance = widget.DangerImportance

	when := widget.NewLabel(details.FormatDateTime(it.ModTime))
	when.Importance = widget.LowImportance

	// маркер занимает место и у просмотренных, чтобы строки не «прыгали»
	dot := canvas.NewCircle(theme.Color(theme.ColorNamePrimary))
	if it.Seen {
		dot.Hide()
	}
	marker := container.NewCenter(container.NewGridWrap(fyne.NewSquareSize(theme.Padding()*2.5), dot))

	body := container.NewVBox(path, reason, when)
	return widget.NewSimpleRenderer(container.NewBorder(nil, nil, container.NewPadded(marker), nil, body))
}

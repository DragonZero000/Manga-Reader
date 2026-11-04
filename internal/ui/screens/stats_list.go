package screens

import (
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/stats"
)

// expandRows — высота раскрытого списка в строках (меньше — по числу строк).
const expandRows = 10

// expandList — строки блока: первые stats.TopLimit, «Показать все»
// раскрывает полный список (widget.List) с полем «Найти в списке». Высота
// раскрытого списка — min(n, expandRows) строк; она считается при раскрытии
// и новых данных, а не при поиске, чтобы блок не прыгал.
type expandList struct {
	list    *rowList
	more    *widget.Button
	find    *widget.Entry
	full    *widget.List
	fullH   *canvas.Rectangle
	fullBox *fyne.Container
	less    *widget.Button
	box     *fyne.Container

	expanded bool
	n        int
	row      func(i int) rowData
	name     func(i int) string
	shown    []int // индексы строк раскрытого списка после «Найти в списке»
}

func newExpandList() *expandList {
	l := &expandList{list: newRowList()}
	l.more = widget.NewButton(i18n.T("stats.show_all"), func() { l.setExpanded(true) })
	l.more.Importance = widget.LowImportance
	l.find = widget.NewEntry()
	l.find.SetPlaceHolder(i18n.T("stats.find"))
	l.find.OnChanged = func(string) { l.refilter() }
	l.full = widget.NewList(
		func() int { return len(l.shown) },
		func() fyne.CanvasObject { return newStatRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			if i < len(l.shown) {
				o.(*statRow).set(l.row(l.shown[i]))
			}
		})
	l.less = widget.NewButton(i18n.T("stats.show_less", "N", stats.TopLimit), func() { l.setExpanded(false) })
	l.less.Importance = widget.LowImportance
	l.fullH = canvas.NewRectangle(nil)
	l.fullBox = container.NewBorder(l.find, l.less, nil, nil, container.NewStack(l.fullH, l.full))
	l.fullBox.Hide()
	l.box = container.NewVBox(l.list.box, l.more, l.fullBox)
	return l
}

// Set показывает n строк: row строит строку, name — текст для «Найти в списке».
func (l *expandList) Set(n int, row func(int) rowData, name func(int) string) {
	l.n, l.row, l.name = n, row, name
	data := make([]rowData, min(n, stats.TopLimit))
	for i := range data {
		data[i] = row(i)
	}
	l.list.Set(data)
	if n <= stats.TopLimit {
		l.expanded = false
	}
	l.resizeFull()
	l.layout()
	if l.expanded {
		l.refilter()
	}
}

// resizeFull задаёт высоту раскрытого списка: min(n, expandRows) строк с
// промежутками widget.List между ними.
func (l *expandList) resizeFull() {
	rows := float32(min(l.n, expandRows))
	h := newStatRow().MinSize().Height*rows + theme.Padding()*max(0, rows-1)
	l.fullH.SetMinSize(fyne.NewSize(0, h))
}

// setExpanded раскрывает или сворачивает полный список.
func (l *expandList) setExpanded(on bool) {
	l.expanded = on
	if on {
		l.find.SetText("")
	}
	l.layout()
	if on {
		l.refilter()
	}
}

// layout показывает нужные объекты: первые строки или раскрытый список.
func (l *expandList) layout() {
	long := l.n > stats.TopLimit
	if l.expanded && long {
		l.list.box.Hide()
		l.more.Hide()
		l.fullBox.Show()
	} else {
		l.fullBox.Hide()
		l.list.box.Show()
		if long {
			l.more.Show()
		} else {
			l.more.Hide()
		}
	}
	l.box.Refresh()
}

// refilter оставляет в раскрытом списке строки, чей текст содержит текст
// поля (после той же нормализации, что у поиска).
func (l *expandList) refilter() {
	want := search.Normalize(strings.TrimSpace(l.find.Text))
	l.shown = l.shown[:0]
	for i := 0; i < l.n; i++ {
		if want == "" || strings.Contains(search.Normalize(l.name(i)), want) {
			l.shown = append(l.shown, i)
		}
	}
	l.full.Refresh()
}

// orderButton — кнопка порядка: стрелка вниз — по убыванию, вверх — по
// возрастанию.
func orderButton(tap func()) *widget.Button {
	b := widget.NewButtonWithIcon("", theme.MoveDownIcon(), tap)
	b.Importance = widget.LowImportance
	return b
}

func setOrderIcon(b *widget.Button, asc bool) {
	icon := theme.MoveDownIcon()
	if asc {
		icon = theme.MoveUpIcon()
	}
	if b.Icon != icon {
		b.SetIcon(icon)
	}
}

// valueMax — наибольшее значение для полосок: при порядке по возрастанию
// оно в конце списка.
func valueMax(first, last int64) int64 { return max(first, last) }

// --- Топ тегов ---

// topWidget — топ тегов: заголовок с кнопкой порядка (и переключателем
// режима у топов типов) и строки expandList.
type topWidget struct {
	s  *Stats
	id stats.BlockID // блок исключений и порядка: тип тега, custom или hidden

	box   *fyne.Container
	mode  *widget.Select
	order *widget.Button
	rows  *expandList

	top stats.Top
	// fullName — подпись строки целиком «тип:имя» (топы своих и скрытых)
	fullName bool
	// filling — переключатель меняется программно
	filling bool
}

var modeOrder = []stats.Mode{stats.ModeWorks, stats.ModePages, stats.ModeSize}

func modeLabel(m stats.Mode) string { return i18n.T("stats.mode." + string(m)) }

func newTopWidget(s *Stats, id stats.BlockID, title string, withMode, fullName bool) *topWidget {
	t := &topWidget{s: s, id: id, fullName: fullName, rows: newExpandList()}
	t.order = orderButton(func() { s.toggleOrder(id) })
	right := fyne.CanvasObject(t.order)
	if withMode {
		var opts []string
		for _, m := range modeOrder {
			opts = append(opts, modeLabel(m))
		}
		t.mode = widget.NewSelect(opts, func(sel string) {
			if t.filling {
				return
			}
			for _, m := range modeOrder {
				if modeLabel(m) == sel {
					s.setMode(id, m)
				}
			}
		})
		fitSelect(t.mode)
		right = container.NewHBox(t.order, t.mode)
	}
	t.box = blockBox(title, right, t.rows.box)
	return t
}

func (t *topWidget) Object() fyne.CanvasObject { return t.box }

// rowData — строка топа: имя, значение режима и доля; меню исключения.
func (t *topWidget) rowData(r stats.TagRow) rowData {
	var mx int64
	if n := len(t.top.Rows); n > 0 {
		mx = valueMax(t.top.Rows[0].Value, t.top.Rows[n-1].Value)
	}
	name := r.Tag.Name
	if t.fullName {
		name = r.Tag.String()
	}
	value := i18n.Int(r.Value)
	if t.top.Mode == stats.ModeSize {
		value = statSize(r.Value)
	}
	tag := r.Tag
	return rowData{
		Label: name,
		Value: value + " (" + percent(r.Share) + ")",
		Frac:  fracOf(r.Value, mx),
		Tap:   t.s.tapRef(r.Ref),
		Menu:  func() *fyne.Menu { return t.s.excludeMenu(t.id, tag) },
	}
}

// Set показывает топ top.
func (t *topWidget) Set(top stats.Top) {
	t.top = top
	if t.mode != nil {
		t.filling = true
		t.mode.SetSelected(modeLabel(top.Mode))
		t.filling = false
	}
	setOrderIcon(t.order, t.s.ascending[t.id])
	t.rows.Set(len(top.Rows),
		func(i int) rowData { return t.rowData(t.top.Rows[i]) },
		func(i int) string { return t.top.Rows[i].Tag.Name })
}

// Rows — число строк короткого топа (для тестов).
func (t *topWidget) Rows() int { return t.rows.list.Len() }

// --- Самые большие произведения ---

// largestWidget — список произведений выборки по объёму или страницам:
// заголовок с кнопкой порядка и переключателем, строки expandList.
type largestWidget struct {
	s     *Stats
	box   *fyne.Container
	by    *widget.Select
	order *widget.Button
	rows  *expandList

	items   []stats.Large
	byPages bool
	filling bool
}

func newLargestWidget(s *Stats) *largestWidget {
	w := &largestWidget{s: s, rows: newExpandList()}
	w.order = orderButton(func() { s.toggleOrder(stats.BlockLargest) })
	sizeL, pagesL := modeLabel(stats.ModeSize), modeLabel(stats.ModePages)
	w.by = widget.NewSelect([]string{sizeL, pagesL}, func(sel string) {
		if w.filling {
			return
		}
		m := stats.ModeSize
		if sel == pagesL {
			m = stats.ModePages
		}
		s.setLargestBy(m)
	})
	fitSelect(w.by)
	head := container.NewBorder(nil, nil, nil, container.NewHBox(w.order, w.by), titleLabel(i18n.T("stats.largest")))
	w.box = container.NewVBox(head, w.rows.box)
	return w
}

func (w *largestWidget) value(l stats.Large) int64 {
	if w.byPages {
		return int64(l.Pages)
	}
	return l.Size
}

// Set показывает список блока объёма.
func (w *largestWidget) Set(b *stats.SizeBlock) {
	w.items, w.byPages = b.Largest, b.By == stats.ModePages
	w.filling = true
	w.by.SetSelected(modeLabel(b.By))
	w.filling = false
	setOrderIcon(w.order, w.s.ascending[stats.BlockLargest])
	var mx int64
	if n := len(w.items); n > 0 {
		mx = valueMax(w.value(w.items[0]), w.value(w.items[n-1]))
	}
	w.rows.Set(len(w.items), func(i int) rowData {
		l := w.items[i]
		v := w.value(l)
		text := statSize(v)
		if w.byPages {
			text = i18n.Int(v)
		}
		key := l.Key
		return rowData{Label: l.Title, Value: text, Frac: fracOf(v, mx), Tap: func() { w.s.openGallery(key) }}
	}, func(i int) string { return w.items[i].Title })
	if len(w.items) == 0 {
		w.box.Hide()
	} else {
		w.box.Show()
	}
}

// statSize — размер на экране статистики: две цифры после разделителя.
func statSize(n int64) string { return i18n.SizeDigits(n, 2) }

// tagLabel — подпись исключённого тега: «тип:имя — Блок».
func tagLabel(tag model.Tag, id stats.BlockID) string {
	return i18n.T("stats.chip", "Tag", tag.String(), "Block", blockTitle(id))
}

// fitSelect задаёт переключателю ширину по самому длинному варианту: Fyne
// считает ширину Select по тексту-заглушке, и выбранный вариант длиннее
// неё («Произведения») обрезается.
func fitSelect(sel *widget.Select) {
	longest, width := "", float32(0)
	for _, o := range sel.Options {
		if w := fyne.MeasureText(o, theme.TextSize(), fyne.TextStyle{}).Width; w > width {
			longest, width = o, w
		}
	}
	sel.PlaceHolder = longest
}

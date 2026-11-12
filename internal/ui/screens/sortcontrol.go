package screens

import (
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/search"
)

// Ключи перевода названий полей сортировки.
var sortFieldKeys = map[search.Field]string{
	search.FieldTitle:    "sort.title",
	search.FieldPages:    "sort.pages",
	search.FieldSize:     "sort.size",
	search.FieldAdded:    "sort.added",
	search.FieldUploaded: "sort.uploaded",
}

// SortState — сортировка вкладки. Пока Custom == false, действует стартовая
// сортировка из настроек; ручной выбор ставит Custom и живёт только в памяти.
type SortState struct {
	Sort   search.Sort
	Custom bool
}

// sortControl — кнопка «Поле ↓» с меню выбора сортировки над сеткой.
// Используется только в UI-потоке.
type sortControl struct {
	btn   *widget.Button
	state SortState
	// start возвращает стартовую сортировку из настроек.
	start func() search.Sort
	// changed вызывается после смены действующей сортировки.
	changed func(search.Sort)
}

func newSortControl(start func() search.Sort, changed func(search.Sort)) *sortControl {
	c := &sortControl{start: start, changed: changed}
	c.btn = widget.NewButton("", c.showMenu)
	c.update()
	return c
}

// Sort — действующая сортировка вкладки.
func (c *sortControl) Sort() search.Sort {
	if c.state.Custom {
		return c.state.Sort
	}
	return c.start()
}

// Highlighted сообщает, отличается ли сортировка вкладки от стартовой.
func (c *sortControl) Highlighted() bool { return c.Sort() != c.start() }

// Label — подпись кнопки, например «Размер ↓».
func (c *sortControl) Label() string { return sortLabel(c.Sort()) }

func sortLabel(s search.Sort) string {
	arrow := "↑"
	if s.Desc {
		arrow = "↓"
	}
	return i18n.T(sortFieldKeys[s.Field]) + " " + arrow
}

// Select выбирает поле: новое получает направление по умолчанию, текущее
// меняет направление на обратное.
func (c *sortControl) Select(f search.Field) {
	cur := c.Sort()
	next := search.Sort{Field: f, Desc: app.DefaultSortDirection(f)}
	if f == cur.Field {
		next.Desc = !cur.Desc
	}
	c.state = SortState{Sort: next, Custom: true}
	c.apply()
}

// Reset возвращает стартовую сортировку.
func (c *sortControl) Reset() {
	c.state = SortState{}
	c.apply()
}

// StartChanged обновляет вид после смены стартовой сортировки в настройках.
// Возвращает true, если действующая сортировка вкладки изменилась.
func (c *sortControl) StartChanged() bool {
	c.update()
	return !c.state.Custom
}

func (c *sortControl) apply() {
	c.update()
	if c.changed != nil {
		c.changed(c.Sort())
	}
}

// update обновляет подпись и подсветку кнопки.
func (c *sortControl) update() {
	c.btn.SetText(c.Label())
	if c.Highlighted() {
		c.btn.Importance = widget.HighImportance
	} else {
		c.btn.Importance = widget.MediumImportance
	}
	c.btn.Refresh()
}

// Menu — меню: пять полей (текущее отмечено) и «По умолчанию».
func (c *sortControl) Menu() *fyne.Menu {
	cur := c.Sort()
	var items []*fyne.MenuItem
	for _, f := range app.SortFields {
		it := fyne.NewMenuItem(i18n.T(sortFieldKeys[f]), func() { c.Select(f) })
		it.Checked = f == cur.Field
		items = append(items, it)
	}
	items = append(items, fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem(i18n.T("sort.default"), c.Reset))
	return fyne.NewMenu("", items...)
}

func (c *sortControl) showMenu() { ShowMenuBelow(c.Menu(), c.btn) }

// sortedCopy возвращает копию items в порядке s: списки сервисов не меняются.
func sortedCopy(items []model.Gallery, s search.Sort) []model.Gallery {
	out := slices.Clone(items)
	search.SortGalleries(out, s)
	return out
}

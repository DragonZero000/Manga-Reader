package screens

import (
	"context"
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
)

// searchDelay — задержка поиска после последнего изменения текста.
const searchDelay = 300 * time.Millisecond

// helpExamples — справка по синтаксису при пустом запросе.
var helpExamples = [][2]string{
	{"school", "часть слова в названии, тегах, сканлейторе, имени файла, ID"},
	{`"english name"`, "фраза целиком"},
	{`tag:"tag 1"`, "точный тег любого типа"},
	{`artist:"artist 1"`, "тег типа: artist, group, parody, character, language, category"},
	{`-tag:yuri`, "исключить произведения с тегом"},
	{"pages:>20", "число страниц: > < >= <= 10..50"},
	{"uploaded:2024", "дата загрузки: 2024, >2024-06, 2024-01..2024-03"},
	{"size:>10mb", "размер файла: k, mb, gb"},
	{"favorites:>100  id:535147", "избранное и ID"},
	{"school pages:>20 -tag:yuri", "всё вместе: условия объединяются «И»"},
}

// Search — экран поиска: поле запроса, статус, результаты сеткой или справка.
// Состояние меняется только в UI-потоке.
type Search struct {
	index search.Index
	src   *library.Source
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	entry   *widget.Entry
	status  *widget.Label
	grid    *galleryGrid
	help    fyne.CanvasObject
	content fyne.CanvasObject

	seq   int         // номер последнего запуска; старые результаты отбрасываются
	timer *time.Timer // задержка поиска при вводе
}

// NewSearch создаёт экран; open вызывается при нажатии на результат.
func NewSearch(svc *app.Services, open func(model.Gallery)) *Search {
	s := &Search{index: svc.Index, src: svc.Library, do: fyne.Do}

	s.entry = widget.NewEntry()
	s.entry.SetPlaceHolder(`Слово, "фраза", tag:"…", pages:>20…`)
	s.entry.OnChanged = func(string) { s.schedule() }
	s.entry.OnSubmitted = func(string) { s.Run() }

	s.status = widget.NewLabel("")
	s.status.Wrapping = fyne.TextWrapWord
	s.grid = newGalleryGrid(svc.Thumbs, open)
	s.help = newSearchHelp()

	button := widget.NewButtonWithIcon("", theme.SearchIcon(), s.Run)
	top := container.NewVBox(container.NewBorder(nil, nil, nil, button, s.entry), s.status)
	s.content = container.NewBorder(top, nil, nil, nil, container.NewStack(s.help, s.grid.Widget()))
	s.showHelp()
	return s
}

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (s *Search) SetDispatcher(do func(func())) {
	s.do = do
	s.grid.do = do
}

func (s *Search) Content() fyne.CanvasObject { return s.content }

// SetQuery заполняет поле и сразу выполняет поиск.
func (s *Search) SetQuery(text string) {
	s.entry.SetText(text) // вызовет schedule — таймер отменяется в Run
	s.Run()
}

// Rerun повторяет текущий запрос (после сканирования, при открытии вкладки).
func (s *Search) Rerun() { s.Run() }

// schedule откладывает поиск до паузы в вводе.
func (s *Search) schedule() {
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(searchDelay, func() { s.do(s.Run) })
}

// Run выполняет текущий запрос: разбор — сразу, поиск — в фоне.
func (s *Search) Run() {
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.seq++
	seq := s.seq
	text := s.entry.Text
	if strings.TrimSpace(text) == "" {
		s.showHelp()
		return
	}
	q, err := search.Parse(text)
	if err != nil {
		// прежние результаты остаются до исправления запроса
		s.setStatus("Ошибка в запросе: "+err.Error(), true)
		return
	}
	go func() {
		keys, total, err := s.index.Search(context.Background(), q)
		items := make([]model.Gallery, 0, len(keys))
		for _, k := range keys {
			if g, ok := s.src.Get(k); ok {
				items = append(items, g)
			}
		}
		s.do(func() {
			if seq != s.seq {
				return // запрос уже изменился
			}
			s.apply(items, total, err)
		})
	}()
}

func (s *Search) apply(items []model.Gallery, total int, err error) {
	if err != nil {
		s.setStatus("Ошибка поиска: "+err.Error(), true)
		return
	}
	s.help.Hide()
	s.grid.Widget().Show()
	s.grid.SetItems(items)
	if total == 0 {
		s.setStatus("Ничего не найдено", false)
	} else {
		s.setStatus(fmt.Sprintf("Найдено: %d", total), false)
	}
}

func (s *Search) showHelp() {
	s.setStatus("", false)
	s.grid.SetItems(nil)
	s.grid.Widget().Hide()
	s.help.Show()
	s.help.Refresh() // Container.Show сам не перерисовывает
}

func (s *Search) setStatus(text string, isErr bool) {
	s.status.Importance = widget.MediumImportance
	if isErr {
		s.status.Importance = widget.DangerImportance
	}
	s.status.SetText(text)
	if text == "" {
		s.status.Hide()
	} else {
		s.status.Show()
	}
}

// newSearchHelp — справка: пример запроса и что он ищет.
func newSearchHelp() fyne.CanvasObject {
	form := container.New(layout.NewFormLayout())
	for _, ex := range helpExamples {
		q := widget.NewLabelWithStyle(ex[0], fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
		d := widget.NewLabel(ex[1])
		d.Wrapping = fyne.TextWrapWord
		form.Add(q)
		form.Add(d)
	}
	title := widget.NewLabelWithStyle("Как искать", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	return container.NewVScroll(container.NewVBox(title, form))
}

// --- для тестов ---

// StatusText — текст строки статуса.
func (s *Search) StatusText() string { return s.status.Text }

// HelpVisible сообщает, показана ли справка.
func (s *Search) HelpVisible() bool { return s.help.Visible() }

// Results — галереи в сетке результатов.
func (s *Search) Results() []model.Gallery { return s.grid.Items() }

// Query — текст поля запроса.
func (s *Search) Query() string { return s.entry.Text }

// TypeText заменяет текст поля так, как при вводе: поиск — после паузы.
func (s *Search) TypeText(text string) { s.entry.SetText(text) }

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

// searchDelay — задержка поиска в режиме «При вводе» после последнего
// изменения текста.
const searchDelay = 500 * time.Millisecond

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
// Запрос выполняется по паузе в вводе (app.SearchModeDynamic) или по кнопке
// (app.SearchModeSubmit). Состояние меняется только в UI-потоке.
type Search struct {
	index search.Index
	src   *library.Source
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	entry    *widget.Entry
	clearBtn *widget.Button
	runBtn   *widget.Button
	status   *widget.Label
	grid     *galleryGrid
	help     fyne.CanvasObject
	content  fyne.CanvasObject

	mode  string
	delay time.Duration
	// last — текст последнего выполненного запроса («» — запросов не было);
	// его повторяет Rerun.
	last string
	// helpDone — запрос уже выполнялся, справка больше не показывается.
	helpDone bool
	// filling — поле меняется программно: OnChanged не запускает таймер.
	filling bool

	seq      int         // номер последнего запуска; старые результаты отбрасываются
	timer    *time.Timer // задержка поиска при вводе
	timerGen int         // номер ожидания; устаревшие срабатывания таймера отбрасываются
}

// NewSearch создаёт экран; open вызывается при нажатии на результат.
func NewSearch(svc *app.Services, open func(model.Gallery)) *Search {
	s := &Search{index: svc.Index, src: svc.Library, do: fyne.Do, delay: searchDelay}

	s.entry = widget.NewEntry()
	s.entry.SetPlaceHolder(`Слово, "фраза", tag:"…", pages:>20…`)
	s.entry.OnChanged = func(string) { s.changed() }
	s.entry.OnSubmitted = func(string) { s.submitted() }

	s.status = widget.NewLabel("")
	s.status.Wrapping = fyne.TextWrapWord
	s.grid = newGalleryGrid(svc.Thumbs, open)
	s.help = newSearchHelp()

	s.clearBtn = widget.NewButtonWithIcon("", theme.ContentClearIcon(), s.Clear)
	s.runBtn = widget.NewButtonWithIcon("", theme.SearchIcon(), func() { s.execute(s.entry.Text) })
	buttons := container.NewHBox(s.clearBtn, s.runBtn)
	top := container.NewVBox(container.NewBorder(nil, nil, nil, buttons, s.entry), s.status)
	s.content = container.NewBorder(top, nil, nil, nil, container.NewStack(s.help, s.grid.Widget()))
	s.setStatus("", false)
	s.grid.Widget().Hide()
	s.SetMode(app.SearchMode(svc.Settings))
	return s
}

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (s *Search) SetDispatcher(do func(func())) {
	s.do = do
	s.grid.do = do
}

// SetDelay задаёт задержку режима «При вводе» (для тестов).
func (s *Search) SetDelay(d time.Duration) { s.delay = d }

func (s *Search) Content() fyne.CanvasObject { return s.content }

// SetMode переключает режим поиска: отменяет ожидающий запрос, показывает
// кнопку поиска только в режиме «По кнопке».
func (s *Search) SetMode(mode string) {
	s.cancelTimer()
	s.mode = mode
	if mode == app.SearchModeSubmit {
		s.runBtn.Show()
	} else {
		s.runBtn.Hide()
	}
}

// SetQuery заполняет поле и сразу выполняет поиск в любом режиме.
func (s *Search) SetQuery(text string) {
	s.setText(text)
	s.execute(text)
}

// Clear очищает поле и отменяет ожидающий запрос; результаты остаются.
func (s *Search) Clear() {
	s.setText("")
}

// Rerun повторяет последний выполненный запрос (после сканирования, при
// открытии вкладки); до первого запроса остаётся справка.
func (s *Search) Rerun() { s.execute(s.last) }

// setText меняет поле без запуска таймера и отменяет ожидание.
func (s *Search) setText(text string) {
	s.cancelTimer()
	s.filling = true
	s.entry.SetText(text)
	s.filling = false
}

// changed — текст поля изменился при вводе.
func (s *Search) changed() {
	if s.filling || s.mode != app.SearchModeDynamic {
		return
	}
	s.schedule()
}

// submitted — Enter в поле: в режиме «При вводе» только снимает фокус
// (закрывает клавиатуру), таймер продолжает идти.
func (s *Search) submitted() {
	if s.mode == app.SearchModeSubmit {
		s.execute(s.entry.Text)
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(s.entry); c != nil {
		c.Unfocus()
	}
}

// schedule (пере)запускает ожидание паузы в вводе.
func (s *Search) schedule() {
	s.cancelTimer()
	gen := s.timerGen
	s.timer = time.AfterFunc(s.delay, func() {
		s.do(func() {
			// Stop не отменяет срабатывание, уже поставленное в очередь
			if gen == s.timerGen {
				s.execute(s.entry.Text)
			}
		})
	})
}

func (s *Search) cancelTimer() {
	s.timerGen++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// execute выполняет запрос: разбор — сразу, поиск — в фоне. Пустой текст
// не выполняется — на экране остаётся прежнее состояние.
func (s *Search) execute(text string) {
	s.cancelTimer()
	if strings.TrimSpace(text) == "" {
		return
	}
	s.last = text
	s.seq++
	seq := s.seq
	if !s.helpDone {
		s.helpDone = true
		s.help.Hide()
		s.grid.Widget().Show()
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
				return // выполнен более новый запрос
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
	s.grid.SetItems(items)
	if total == 0 {
		s.setStatus("Ничего не найдено", false)
	} else {
		s.setStatus(fmt.Sprintf("Найдено: %d", total), false)
	}
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

// TypeText заменяет текст поля так, как при вводе.
func (s *Search) TypeText(text string) { s.entry.SetText(text) }

// Submit — нажатие Enter в поле.
func (s *Search) Submit() { s.entry.OnSubmitted(s.entry.Text) }

// PressSearch — нажатие кнопки поиска.
func (s *Search) PressSearch() { s.runBtn.OnTapped() }

// Seq — число выполненных запросов.
func (s *Search) Seq() int { return s.seq }

// SearchButtonVisible сообщает, показана ли кнопка поиска.
func (s *Search) SearchButtonVisible() bool { return s.runBtn.Visible() }

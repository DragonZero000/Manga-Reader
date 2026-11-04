package screens

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/stats"
	"mangareader/internal/storage"
)

// statsDelay — задержка пересчёта после последнего изменения фильтра.
const statsDelay = 500 * time.Millisecond

// Stats — экран статистики: панель (фильтр, исключённые теги, «Настроить»)
// и блоки. Подсчёт идёт в фоне (app.StatsService); экран показывает прежние
// цифры, пока считается новый результат. Состояние меняется только в
// UI-потоке.
type Stats struct {
	settings storage.Settings
	service  *app.StatsService
	src      statsSource
	win      fyne.Window
	// searchFor открывает «Поиск» с запросом; open — страницу произведения.
	searchFor func(string)
	open      func(model.Gallery)
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do    func(func())
	delay time.Duration

	filter string // текст фильтра в поле
	shown  string // фильтр, по которому посчитаны показанные цифры
	reqFlt string // фильтр последнего запроса
	// excluded — постоянные исключения (настройки), temp — временные (до
	// перезапуска); в подсчёт идёт их объединение.
	excluded  map[stats.BlockID]map[string]bool
	temp      map[stats.BlockID]map[string]bool
	disabled  map[stats.BlockID]bool
	modes     map[stats.BlockID]stats.Mode
	ascending map[stats.BlockID]bool
	largestBy stats.Mode
	// sumHidden — скрытые поля Сводки.
	sumHidden map[string]bool
	// chipsOpen — список временных исключений раскрыт (при > maxChips).
	chipsOpen bool
	chipBtns  []*widget.Button // кнопки ✕ показанных чипов (для тестов)

	entry    *widget.Entry
	clearBtn *widget.Button
	chips    *fyne.Container
	counting *widget.Label
	message  *widget.Label // ошибка фильтра или пустое состояние
	reset    *widget.Button
	hint     *widget.Label // «все блоки выключены»
	listBox  *fyne.Container
	scroll   *container.Scroll
	content  fyne.CanvasObject

	blocks []blockEntry
	// rev — номер изменения библиотеки (nil — Rerun считает всегда).
	rev      func() int
	lastRev  int
	computed bool
	// dirty — настройки изменены после последнего подсчёта.
	dirty   bool
	reqSeq  uint64
	filling bool
	result  stats.Result
	library int // число произведений библиотеки в показанном результате

	// lastConfig — открытый диалог «Настроить» (для тестов)
	lastConfig *statsConfig

	timer    *time.Timer
	timerGen int
}

type blockEntry struct {
	id    stats.BlockID
	block statsBlock
}

// NewStats создаёт экран. searchFor открывает «Поиск» с запросом, open —
// страницу произведения.
func NewStats(svc *app.Services, win fyne.Window, searchFor func(string), open func(model.Gallery)) *Stats {
	return newStats(svc.Settings, svc.Library, svc.Index, win, searchFor, open)
}

// statsSource — список галерей библиотеки (library.Source).
type statsSource interface {
	Galleries() []model.Gallery
	Get(model.Key) (model.Gallery, bool)
}

func newStats(settings storage.Settings, src statsSource, idx search.Index, win fyne.Window, searchFor func(string), open func(model.Gallery)) *Stats {
	s := &Stats{
		settings:  settings,
		service:   app.NewStatsService(src, idx),
		src:       src,
		win:       win,
		searchFor: searchFor,
		open:      open,
		do:        fyne.Do,
		delay:     statsDelay,
		filter:    app.StatsFilter(settings),
		excluded:  app.StatsExcluded(settings),
		temp:      map[stats.BlockID]map[string]bool{},
		disabled:  app.StatsDisabled(settings),
		modes:     app.StatsModes(settings),
		ascending: app.StatsAscending(settings),
		largestBy: app.StatsLargestBy(settings),
		sumHidden: app.StatsSummaryHidden(settings),
	}
	s.entry = widget.NewEntry()
	s.entry.SetPlaceHolder(i18n.T("stats.filter.placeholder"))
	s.entry.OnChanged = func(string) { s.filterChanged() }
	s.entry.OnSubmitted = func(string) { s.runFilter() }
	s.filling = true
	s.entry.SetText(s.filter)
	s.filling = false
	s.clearBtn = widget.NewButtonWithIcon("", theme.ContentClearIcon(), s.ClearFilter)
	cfg := widget.NewButtonWithIcon(i18n.T("stats.configure"), theme.SettingsIcon(), s.showConfig)
	s.chips = container.NewVBox()
	s.counting = widget.NewLabel(i18n.T("stats.counting"))
	s.counting.Importance = widget.LowImportance
	s.counting.Hide()

	s.message = widget.NewLabel("")
	s.message.Wrapping = fyne.TextWrapWord
	s.message.Alignment = fyne.TextAlignCenter
	s.message.Hide()
	s.reset = widget.NewButton(i18n.T("stats.reset_filter"), s.ClearFilter)
	s.reset.Hide()
	s.hint = widget.NewLabel(i18n.T("stats.all_hidden"))
	s.hint.Wrapping = fyne.TextWrapWord
	s.hint.Alignment = fyne.TextAlignCenter
	s.hint.Hide()

	s.blocks = []blockEntry{{stats.BlockSummary, newSummaryBlock(s)}}
	for _, typ := range stats.TypeBlocks {
		id := stats.BlockID(typ)
		s.blocks = append(s.blocks, blockEntry{id, newTypeBlock(s, id)})
	}
	s.blocks = append(s.blocks,
		blockEntry{stats.BlockPages, newPagesBlock(s)},
		blockEntry{stats.BlockSize, newSizeBlock(s)},
		blockEntry{stats.BlockUploaded, newUploadedBlock(s)},
		blockEntry{stats.BlockGrowth, newGrowthBlock(s)},
		blockEntry{stats.BlockYours, newYoursBlock(s)},
		blockEntry{stats.BlockQuality, newQualityBlock(s)},
	)
	objs := []fyne.CanvasObject{s.message, s.reset, s.hint}
	for _, b := range s.blocks {
		o := b.block.Object()
		o.Hide() // до первого результата блоков нет
		objs = append(objs, o)
	}
	s.listBox = container.NewVBox(objs...)
	s.scroll = container.NewVScroll(container.NewPadded(s.listBox))

	top := container.NewVBox(
		container.NewBorder(nil, nil, nil, container.NewHBox(s.clearBtn, cfg), s.entry),
		s.chips, s.counting)
	s.content = container.NewBorder(top, nil, nil, nil, WithWheel(ScrollOf(s.scroll), s.scroll))
	s.rebuildChips()
	return s
}

// Content — содержимое вкладки.
func (s *Stats) Content() fyne.CanvasObject { return s.content }

// SetDispatcher задаёт функцию выполнения в UI-потоке (для тестов).
func (s *Stats) SetDispatcher(do func(func())) { s.do = do }

// SetDelay задаёт задержку пересчёта после ввода фильтра (для тестов).
func (s *Stats) SetDelay(d time.Duration) { s.delay = d }

// SetRevision задаёт номер изменения библиотеки: Rerun пересчитывает
// статистику, только если номер изменился с прошлого подсчёта.
func (s *Stats) SetRevision(rev func() int) { s.rev = rev }

// Rerun пересчитывает статистику, если её ещё не считали, библиотека
// изменилась или менялись настройки; иначе цифры прежние. Вызывается при
// открытии вкладки и при изменении библиотеки, пока вкладка открыта.
func (s *Stats) Rerun() {
	if s.computed && !s.dirty && (s.rev == nil || s.rev() == s.lastRev) {
		return
	}
	s.recompute()
}

// recompute запускает подсчёт в фоне; прежние цифры остаются до результата.
func (s *Stats) recompute() {
	s.cancelTimer()
	s.computed, s.dirty = true, false
	if s.rev != nil {
		s.lastRev = s.rev()
	}
	s.reqFlt = s.filter
	opt := s.options()
	s.setCounting(true)
	s.reqSeq = s.service.Request(s.filter, opt, func(r app.StatsResponse) {
		s.do(func() { s.apply(r) })
	})
}

// options — настройки подсчёта из состояния экрана и границ из настроек.
func (s *Stats) options() stats.Options {
	pages, size := app.StatsBounds(s.settings)
	disabled := map[stats.BlockID]bool{}
	for _, id := range stats.AllBlocks() {
		disabled[id] = s.blockOff(id)
	}
	excluded := map[stats.BlockID]map[string]bool{}
	for _, src := range []map[stats.BlockID]map[string]bool{s.excluded, s.temp} {
		for id, set := range src {
			for k := range set {
				if excluded[id] == nil {
					excluded[id] = map[string]bool{}
				}
				excluded[id][k] = true
			}
		}
	}
	return stats.Options{
		Disabled:   disabled,
		Excluded:   excluded,
		Modes:      s.modes,
		Ascending:  s.ascending,
		LargestBy:  s.largestBy,
		PageBounds: pages,
		SizeBounds: size,
	}
}

// blockOff сообщает, выключен ли блок: галочкой или (Сводка) скрытием всех
// её полей.
func (s *Stats) blockOff(id stats.BlockID) bool {
	if s.disabled[id] {
		return true
	}
	if id == stats.BlockSummary {
		for _, f := range stats.SummaryFields() {
			if !s.sumHidden[f] {
				return false
			}
		}
		return true
	}
	return false
}

func (s *Stats) setCounting(on bool) {
	if on {
		s.counting.Show()
	} else {
		s.counting.Hide()
	}
}

// apply показывает результат подсчёта; результат не последнего запроса
// отбрасывается.
func (s *Stats) apply(r app.StatsResponse) {
	if r.Seq != s.reqSeq {
		return
	}
	s.setCounting(false)
	s.library = r.Library
	if r.Err != nil {
		s.showMessage(i18n.T("search.query_error", "Error", ErrorText(r.Err)), true, false)
		s.setBlocksVisible(false)
		return
	}
	s.result, s.shown = r.Result, s.reqFlt
	switch {
	case r.Library == 0:
		s.showMessage(i18n.T("stats.empty.library"), false, false)
		s.setBlocksVisible(false)
	case r.Result.Works == 0:
		s.showMessage(i18n.T("stats.empty.filter"), false, true)
		s.setBlocksVisible(false)
	default:
		s.message.Hide()
		s.reset.Hide()
		s.setBlocksVisible(true)
		s.updateBlocks()
	}
}

// showMessage показывает текст вместо блоков: ошибку фильтра, пустую
// библиотеку или «ничего не найдено» (с кнопкой сброса фильтра).
func (s *Stats) showMessage(text string, isErr, withReset bool) {
	s.message.Importance = widget.MediumImportance
	if isErr {
		s.message.Importance = widget.DangerImportance
	}
	s.message.SetText(text)
	s.message.Show()
	if withReset {
		s.reset.Show()
	} else {
		s.reset.Hide()
	}
}

// setBlocksVisible показывает или скрывает все блоки (пустые состояния).
func (s *Stats) setBlocksVisible(on bool) {
	any := false
	for _, b := range s.blocks {
		o := b.block.Object()
		if on && !s.blockOff(b.id) {
			o.Show()
			any = true
		} else {
			o.Hide()
		}
	}
	if on && !any {
		s.hint.Show()
	} else {
		s.hint.Hide()
	}
	s.listBox.Refresh()
}

// updateBlocks передаёт результат включённым блокам.
func (s *Stats) updateBlocks() {
	for _, b := range s.blocks {
		if !s.blockOff(b.id) {
			b.block.Update(&s.result)
		}
	}
}

// --- Фильтр ---

// Filter — текст фильтра в поле.
func (s *Stats) Filter() string { return s.filter }

// SetFilter заполняет поле, сохраняет фильтр и сразу пересчитывает.
func (s *Stats) SetFilter(text string) {
	s.cancelTimer()
	s.filling = true
	s.entry.SetText(text)
	s.filling = false
	s.filter = text
	app.SetStatsFilter(s.settings, text)
	s.recompute()
}

// ClearFilter очищает фильтр.
func (s *Stats) ClearFilter() { s.SetFilter("") }

// filterChanged — текст поля изменился при вводе: пересчёт после паузы.
func (s *Stats) filterChanged() {
	if s.filling {
		return
	}
	s.cancelTimer()
	gen := s.timerGen
	s.timer = time.AfterFunc(s.delay, func() {
		s.do(func() {
			if gen == s.timerGen {
				s.runFilter()
			}
		})
	})
}

// runFilter применяет текст поля (по паузе ввода или Enter).
func (s *Stats) runFilter() {
	s.SetFilter(s.entry.Text)
}

func (s *Stats) cancelTimer() {
	s.timerGen++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
}

// --- Действия строк ---

// goSearch открывает «Поиск» с условием строки и фильтром статистики.
func (s *Stats) goSearch(ref stats.RowRef) {
	if q := stats.Query(ref, s.shown); q != "" && s.searchFor != nil {
		s.searchFor(q)
	}
}

// openGallery открывает страницу произведения k.
func (s *Stats) openGallery(k model.Key) {
	if g, ok := s.src.Get(k); ok && s.open != nil {
		s.open(g)
	}
}

// setMode задаёт режим топа и пересчитывает.
func (s *Stats) setMode(id stats.BlockID, m stats.Mode) {
	if s.modes[id] == m {
		return
	}
	s.modes[id] = m
	app.SetStatsModes(s.settings, s.modes)
	s.recompute()
}

// --- для тестов ---

// Result — последний показанный результат.
func (s *Stats) Result() stats.Result { return s.result }

// Counting сообщает, виден ли индикатор подсчёта.
func (s *Stats) Counting() bool { return s.counting.Visible() }

// MessageText — текст пустого состояния или ошибки фильтра («» — нет).
func (s *Stats) MessageText() string {
	if !s.message.Visible() {
		return ""
	}
	return s.message.Text
}

// ResetButton — кнопка сброса фильтра (видна при пустой выборке).
func (s *Stats) ResetButton() *widget.Button { return s.reset }

// BlockVisible сообщает, показан ли блок id.
func (s *Stats) BlockVisible(id stats.BlockID) bool {
	for _, b := range s.blocks {
		if b.id == id {
			return b.block.Object().Visible()
		}
	}
	return false
}

// Block — блок по id (для тестов).
func (s *Stats) Block(id stats.BlockID) statsBlock {
	for _, b := range s.blocks {
		if b.id == id {
			return b.block
		}
	}
	return nil
}

// Scroller — прокрутка экрана (для замера и тестов).
func (s *Stats) Scroller() ScrollTarget { return ScrollOf(s.scroll) }

// HintVisible сообщает, показана ли подсказка «все блоки выключены».
func (s *Stats) HintVisible() bool { return s.hint.Visible() }

// Seq — номер последнего запроса подсчёта.
func (s *Stats) Seq() uint64 { return s.reqSeq }

// topOf — виджет топа блока id: типы тегов и «Ваши теги» (ExcludeCustom,
// ExcludeHidden).
func (s *Stats) topOf(id stats.BlockID) *topWidget {
	switch b := s.Block(id).(type) {
	case *typeBlock:
		return b.top
	}
	if yb, ok := s.Block(stats.BlockYours).(*yoursBlock); ok {
		switch id {
		case stats.ExcludeCustom:
			return yb.custom
		case stats.ExcludeHidden:
			return yb.hidden
		}
	}
	return nil
}

// TagRows — подписи показанных строк топа id (для тестов).
func (s *Stats) TagRows(id stats.BlockID) []string {
	l := s.topOf(id).rows.list
	var out []string
	for i := 0; i < l.Len(); i++ {
		out = append(out, l.Row(i).label.Text)
	}
	return out
}

// TapTagRow нажимает i-ю строку топа id.
func (s *Stats) TapTagRow(id stats.BlockID, i int) { s.topOf(id).rows.list.Row(i).Tapped(nil) }

// ExcludeTagRow выбирает «Скрыть» в меню i-й строки топа id;
// ExcludeTagRowForever — «Скрыть навсегда».
func (s *Stats) ExcludeTagRow(id stats.BlockID, i int) {
	s.topOf(id).rows.list.Row(i).data.Menu().Items[0].Action()
}
func (s *Stats) ExcludeTagRowForever(id stats.BlockID, i int) {
	s.topOf(id).rows.list.Row(i).data.Menu().Items[1].Action()
}

// ShowAll нажимает «Показать все» в топе id; FindInList вводит текст в поле
// «Найти в списке»; FullRows — подписи строк раскрытого списка.
func (s *Stats) ShowAll(id stats.BlockID) { s.topOf(id).rows.more.OnTapped() }
func (s *Stats) FindInList(id stats.BlockID, text string) {
	s.topOf(id).rows.find.SetText(text)
}
func (s *Stats) FullRows(id stats.BlockID) []string {
	l := s.topOf(id).rows
	var out []string
	for _, i := range l.shown {
		out = append(out, l.name(i))
	}
	return out
}

// ShowAllVisible сообщает, показана ли кнопка «Показать все» у топа id.
func (s *Stats) ShowAllVisible(id stats.BlockID) bool { return s.topOf(id).rows.more.Visible() }

// TapOrder нажимает кнопку порядка топа id.
func (s *Stats) TapOrder(id stats.BlockID) { s.topOf(id).order.OnTapped() }

// RowLabels — подписи и значения показанных строк блока: pages, size,
// uploaded, growth, quality (для тестов).
func (s *Stats) RowLabels(id stats.BlockID) [][2]string {
	var l *rowList
	switch b := s.Block(id).(type) {
	case *pagesBlock:
		l = b.list
	case *sizeBlock:
		l = b.list
	case *uploadedBlock:
		l = b.list
	case *growthBlock:
		l = b.list
	case *qualityBlock:
		l = b.list
	case *summaryBlock:
		l = b.list
	}
	var out [][2]string
	for i := 0; l != nil && i < len(l.rows); i++ {
		if r := l.rows[i]; r.Visible() {
			out = append(out, [2]string{r.label.Text, r.value.Text})
		}
	}
	return out
}

// TapRow нажимает i-ю показанную строку блока (см. RowLabels); для строки
// года роста нажатие стрелки раскрытия — ToggleGrowthYear.
func (s *Stats) TapRow(id stats.BlockID, i int) {
	switch b := s.Block(id).(type) {
	case *pagesBlock:
		b.list.Row(i).Tapped(nil)
	case *sizeBlock:
		b.list.Row(i).Tapped(nil)
	case *uploadedBlock:
		b.list.Row(i).Tapped(nil)
	case *growthBlock:
		b.list.Row(i).Tapped(nil)
	case *qualityBlock:
		b.list.Row(i).Tapped(nil)
	}
}

// largest — список самых больших в блоке объёма.
func (s *Stats) largest() *largestWidget { return s.Block(stats.BlockSize).(*sizeBlock).largest }

// TapLargest нажимает i-е из самых больших произведений.
func (s *Stats) TapLargest(i int) { s.largest().rows.list.Row(i).Tapped(nil) }

// LargestRows — подписи и значения показанных строк самых больших.
func (s *Stats) LargestRows() [][2]string {
	l := s.largest().rows.list
	var out [][2]string
	for i := 0; i < l.Len(); i++ {
		out = append(out, [2]string{l.Row(i).label.Text, l.Row(i).value.Text})
	}
	return out
}

// SelectLargestBy выбирает «Объём» или «Страницы» у самых больших;
// TapLargestOrder нажимает их кнопку порядка.
func (s *Stats) SelectLargestBy(m stats.Mode) { s.largest().by.SetSelected(modeLabel(m)) }
func (s *Stats) TapLargestOrder()             { s.largest().order.OnTapped() }

// ToggleGrowthYear раскрывает или сворачивает год в блоке роста (нажатие
// стрелки у i-й строки).
func (s *Stats) ToggleGrowthYear(i int) {
	s.Block(stats.BlockGrowth).(*growthBlock).list.Row(i).data.Lead()
}

// SetMode выбирает режим топа типа id так, как переключатель блока.
func (s *Stats) SelectMode(id stats.BlockID, m stats.Mode) {
	s.topOf(id).mode.SetSelected(modeLabel(m))
}

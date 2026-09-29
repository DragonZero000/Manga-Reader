package screens

import (
	"errors"
	"log"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/display"
	"mangareader/internal/i18n"
)

// Settings — экран настроек: язык, папка библиотеки, поиск, браузер и
// сведения о версии.
type Settings struct {
	svc *app.Services
	// language — выбор языка интерфейса; languageCodes — код варианта
	// (i18n.Auto или код языка) по индексу в language.Options
	language      *widget.RadioGroup
	languageCodes []string
	// languageHint — «Язык изменится после перезапуска» на выбранном языке
	languageHint *widget.Label
	// searchMode — выбор режима поиска
	searchMode *widget.RadioGroup
	// randomMode — выбор режима кнопки 🎲
	randomMode *widget.RadioGroup
	// grid — «Карточек в ряду» (телефон) или «Размер карточек» (ПК)
	grid *widget.RadioGroup
	// max60 — флажок «Ограничить 60 Гц» (nil — раздела «Экран» нет)
	max60 *widget.Check
	// retention — срок хранения данных удалённых произведений; retentionDays —
	// поле своего числа дней, retentionErr — ошибка в нём
	retention     *widget.Select
	retentionDays *widget.Entry
	retentionErr  *widget.Label
	path          *widget.Label
	copyBtn       *widget.Button
	browser       *browserCard // nil — встроенного браузера нет
	// mobileBrowser — раздел «Браузер» на Android (nil — нет)
	mobileBrowser *widget.Card
	content       fyne.CanvasObject
}

// NewSettings создаёт экран; choose открывает выбор папки (Android, иначе nil),
// onSearchMode получает новый режим поиска, onRandomMode — новый режим
// кнопки 🎲, onGrid вызывается после смены плотности сетки. win — окно для
// диалогов.
func NewSettings(a fyne.App, win fyne.Window, svc *app.Services, notify func(string), choose func(), onSearchMode, onRandomMode func(string), onGrid func()) *Settings {
	s := &Settings{svc: svc}
	s.path = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	s.path.Wrapping = fyne.TextWrapBreak

	items := []fyne.CanvasObject{s.path}
	if svc.CanChooseFolder {
		change := widget.NewButtonWithIcon(i18n.T("settings.library.change"), theme.FolderOpenIcon(), choose)
		items = append(items, change, cardNote(i18n.T("settings.library.hint")))
	} else {
		s.copyBtn = widget.NewButton(i18n.T("settings.library.copy"), func() {
			a.Clipboard().SetContent(svc.Library.Root())
			notify(i18n.T("settings.library.copied"))
		})
		items = append(items, s.copyBtn)
	}
	if svc.LibraryErr != nil {
		errLabel := cardNote(i18n.T("settings.library.error", "Error", ErrorText(svc.LibraryErr)))
		errLabel.Importance = widget.DangerImportance
		items = append(items, errLabel)
	}

	libraryCard := widget.NewCard(i18n.T("settings.library.title"), "", container.NewVBox(items...))
	aboutCard := widget.NewCard(i18n.T("settings.about.title"), "", widget.NewLabel("MangaReader "+svc.Version))
	cards := container.NewVBox(s.newLanguageCard(), libraryCard, s.newSearchCard(onSearchMode),
		s.newRandomCard(onRandomMode), s.newGridCard(onGrid), s.newRetentionCard())
	if displaySupported {
		cards.Add(s.newDisplayCard())
	}
	if svc.Browser != nil {
		s.browser = newBrowserCard(svc, win, notify)
		cards.Add(s.browser.card)
	}
	if svc.MobileBrowser {
		s.mobileBrowser = newMobileBrowserCard(svc, win, notify)
		cards.Add(s.mobileBrowser)
	}
	cards.Add(aboutCard)
	s.content = container.NewVScroll(cards)
	s.Update()
	return s
}

// cardNote — пояснение раздела настроек с переносом по словам. Подзаголовок
// widget.Card не переносится: длинный растягивает весь экран настроек шире
// телефона.
func cardNote(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// newLanguageCard — раздел «Язык / Language»: «Системный» и языки из файлов
// переводов, каждый — на самом этом языке. Выбор применяется при следующем
// запуске; до перезапуска показывается подсказка на выбранном языке.
func (s *Settings) newLanguageCard() *widget.Card {
	st := s.svc.Settings
	labels := []string{i18n.T("settings.language.system")}
	s.languageCodes = []string{i18n.Auto}
	for _, l := range i18n.Available() {
		labels = append(labels, l.Name)
		s.languageCodes = append(s.languageCodes, l.Code)
	}
	started := app.UILanguage(st)
	s.languageHint = cardNote("")
	s.languageHint.Hide()
	s.language = widget.NewRadioGroup(labels, nil)
	s.language.Required = true
	s.language.SetSelected(labels[s.languageIndex(started)])
	s.language.OnChanged = func(v string) {
		code := i18n.Auto
		for i, l := range labels {
			if l == v {
				code = s.languageCodes[i]
			}
		}
		app.SetUILanguage(st, code)
		if code == started {
			s.languageHint.Hide()
			return
		}
		lang := i18n.Resolve(code, i18n.SystemLocale())
		s.languageHint.SetText(i18n.TIn(lang, "settings.language.restart"))
		s.languageHint.Show()
	}
	return widget.NewCard(i18n.T("settings.language.title"), "", container.NewVBox(s.language, s.languageHint))
}

// languageIndex — вариант выбора для кода языка (неизвестный — «Системный»).
func (s *Settings) languageIndex(code string) int {
	for i, c := range s.languageCodes {
		if c == code {
			return i
		}
	}
	return 0
}

// Ключи перевода подписей режимов поиска.
var searchModeKeys = map[string]string{
	app.SearchModeDynamic: "settings.search.dynamic",
	app.SearchModeSubmit:  "settings.search.submit",
}

// newSearchCard — раздел «Поиск»: режим запуска запроса.
func (s *Settings) newSearchCard(onMode func(string)) *widget.Card {
	dynamic, submit := i18n.T(searchModeKeys[app.SearchModeDynamic]), i18n.T(searchModeKeys[app.SearchModeSubmit])
	s.searchMode = widget.NewRadioGroup([]string{dynamic, submit}, nil)
	s.searchMode.Required = true
	s.searchMode.SetSelected(i18n.T(searchModeKeys[app.SearchMode(s.svc.Settings)]))
	s.searchMode.OnChanged = func(v string) {
		mode := app.SearchModeDynamic
		if v == submit {
			mode = app.SearchModeSubmit
		}
		s.svc.Settings.SetString(app.KeySearchMode, mode)
		if onMode != nil {
			onMode(mode)
		}
	}
	return widget.NewCard(i18n.T("settings.search.title"), "",
		container.NewVBox(s.searchMode, cardNote(i18n.T("settings.search.hint"))))
}

// Ключи перевода подписей режимов кнопки 🎲.
var randomModeKeys = map[string]string{
	app.RandomModeRepeat:   "settings.random.repeat",
	app.RandomModeNoRepeat: "settings.random.norepeat",
}

// newRandomCard — раздел «Случайный выбор»: режим кнопки 🎲.
func (s *Settings) newRandomCard(onMode func(string)) *widget.Card {
	repeat, noRepeat := i18n.T(randomModeKeys[app.RandomModeRepeat]), i18n.T(randomModeKeys[app.RandomModeNoRepeat])
	s.randomMode = widget.NewRadioGroup([]string{repeat, noRepeat}, nil)
	s.randomMode.Required = true
	s.randomMode.SetSelected(i18n.T(randomModeKeys[app.RandomMode(s.svc.Settings)]))
	s.randomMode.OnChanged = func(v string) {
		mode := app.RandomModeRepeat
		if v == noRepeat {
			mode = app.RandomModeNoRepeat
		}
		app.SetRandomMode(s.svc.Settings, mode)
		if onMode != nil {
			onMode(mode)
		}
	}
	return widget.NewCard(i18n.T("settings.random.title"), "",
		container.NewVBox(s.randomMode, cardNote(i18n.T("settings.random.hint"))))
}

// Ключи перевода подписей настройки «Размер карточек» (ПК).
var gridSizeKeys = map[string]string{
	app.GridSizeSmall:  "settings.grid.small",
	app.GridSizeMedium: "settings.grid.medium",
	app.GridSizeLarge:  "settings.grid.large",
}

// newGridCard — раздел «Сетка»: «Карточек в ряду» на телефоне, «Размер
// карточек» на ПК. Настройка общая для библиотеки и поиска.
func (s *Settings) newGridCard(onChange func()) *widget.Card {
	st := s.svc.Settings
	var title, hint string
	if isMobile() {
		title, hint = i18n.T("settings.grid.columns"), i18n.T("settings.grid.columns_hint")
		s.grid = widget.NewRadioGroup([]string{"2", "3", "4"}, nil)
		s.grid.SetSelected(strconv.Itoa(app.GridColumns(st)))
		s.grid.OnChanged = func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				app.SetGridColumns(st, n)
			}
			if onChange != nil {
				onChange()
			}
		}
	} else {
		title, hint = i18n.T("settings.grid.size"), i18n.T("settings.grid.size_hint")
		sizes := []string{app.GridSizeSmall, app.GridSizeMedium, app.GridSizeLarge}
		labels := make([]string, len(sizes))
		for i, v := range sizes {
			labels[i] = i18n.T(gridSizeKeys[v])
		}
		s.grid = widget.NewRadioGroup(labels, nil)
		s.grid.SetSelected(i18n.T(gridSizeKeys[app.GridSize(st)]))
		s.grid.OnChanged = func(label string) {
			for i, v := range sizes {
				if labels[i] == label {
					app.SetGridSize(st, v)
				}
			}
			if onChange != nil {
				onChange()
			}
		}
	}
	s.grid.Horizontal = true
	s.grid.Required = true
	return widget.NewCard(i18n.T("settings.grid.title"), "",
		container.NewVBox(widget.NewLabel(title), s.grid, cardNote(hint)))
}

// SelectGrid выбирает плотность сетки так, как пользователь: подпись
// варианта («3» или «Крупные»); для тестов.
func (s *Settings) SelectGrid(label string) { s.grid.SetSelected(label) }

// GridLabel — выбранная плотность сетки (подпись); для тестов.
func (s *Settings) GridLabel() string { return s.grid.Selected }

// retentionPresets — готовые варианты срока хранения: дни и ключ подписи.
var retentionPresets = []struct {
	days int
	key  string
}{
	{1, "settings.userdata.day"},
	{7, "settings.userdata.week"},
	{30, "settings.userdata.month"},
	{90, "settings.userdata.months3"},
	{365, "settings.userdata.year"},
	{0, "settings.userdata.forever"},
}

// errRetentionDays — недопустимое своё число дней.
var errRetentionDays = errors.New("invalid number of days")

// parseRetentionDays — своё число дней: целое от 1 до app.MaxRetentionDays.
func parseRetentionDays(text string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n < 1 || n > app.MaxRetentionDays {
		return 0, errRetentionDays
	}
	return n, nil
}

// newRetentionCard — раздел «Данные удалённых произведений»: сколько хранить
// данные произведения, файл которого не найден. Готовые варианты и «Своё
// число дней…» с полем ввода; сохраняется только допустимое число.
func (s *Settings) newRetentionCard() *widget.Card {
	st := s.svc.Settings
	custom := i18n.T("settings.userdata.custom")
	labels := make([]string, 0, len(retentionPresets)+1)
	for _, p := range retentionPresets {
		labels = append(labels, i18n.T(p.key))
	}
	labels = append(labels, custom)

	s.retentionErr = cardNote(i18n.T("settings.userdata.days_invalid", "Max", app.MaxRetentionDays))
	s.retentionErr.Importance = widget.DangerImportance
	s.retentionErr.Hide()
	s.retentionDays = widget.NewEntry()
	s.retentionDays.SetPlaceHolder(i18n.T("settings.userdata.days_placeholder"))
	s.retentionDays.Validator = func(text string) error {
		_, err := parseRetentionDays(text)
		return err
	}
	s.retentionDays.OnChanged = func(text string) {
		n, err := parseRetentionDays(text)
		if err != nil {
			s.retentionErr.Show()
			return
		}
		s.retentionErr.Hide()
		app.SetRetentionDays(st, n)
	}

	s.retention = widget.NewSelect(labels, nil)
	days := app.RetentionDays(st)
	selected := custom
	for i, p := range retentionPresets {
		if p.days == days {
			selected = labels[i]
		}
	}
	s.retention.SetSelected(selected)
	if selected == custom {
		s.retentionDays.SetText(strconv.Itoa(days))
	} else {
		s.retentionDays.Hide()
	}
	s.retention.OnChanged = func(v string) {
		if v == custom {
			if d := app.RetentionDays(st); d > 0 {
				s.retentionDays.SetText(strconv.Itoa(d))
			}
			s.retentionDays.Show()
			return
		}
		s.retentionDays.Hide()
		s.retentionErr.Hide()
		for i, p := range retentionPresets {
			if labels[i] == v {
				app.SetRetentionDays(st, p.days)
			}
		}
	}
	return widget.NewCard(i18n.T("settings.userdata.title"), "", container.NewVBox(
		cardNote(i18n.T("settings.userdata.keep")), s.retention, s.retentionDays, s.retentionErr,
		cardNote(i18n.T("settings.userdata.hint"))))
}

// SelectRetention выбирает срок хранения так, как пользователь: подпись
// варианта (для тестов).
func (s *Settings) SelectRetention(label string) { s.retention.SetSelected(label) }

// RetentionLabel — выбранный вариант срока хранения (подпись); для тестов.
func (s *Settings) RetentionLabel() string { return s.retention.Selected }

// RetentionDaysEntry — поле своего числа дней; для тестов.
func (s *Settings) RetentionDaysEntry() *widget.Entry { return s.retentionDays }

// RetentionError — ошибка своего числа дней («» — нет); для тестов.
func (s *Settings) RetentionError() string {
	if !s.retentionErr.Visible() {
		return ""
	}
	return s.retentionErr.Text
}

// Частота экрана; подменяются в тестах.
var (
	displaySupported = display.Supported
	setMax60         = display.SetMax60
)

// newDisplayCard — раздел «Экран» (Android): ограничение частоты 60 Гц.
func (s *Settings) newDisplayCard() *widget.Card {
	s.max60 = widget.NewCheck(i18n.T("settings.display.max60"), nil)
	s.max60.SetChecked(app.DisplayMax60(s.svc.Settings))
	s.max60.OnChanged = func(on bool) {
		app.SetDisplayMax60(s.svc.Settings, on)
		if err := setMax60(on); err != nil {
			log.Printf("display refresh rate: %v", err)
		}
	}
	return widget.NewCard(i18n.T("settings.display.title"), "",
		container.NewVBox(s.max60, cardNote(i18n.T("settings.display.hint"))))
}

// DisplayCheck — флажок «Ограничить 60 Гц» (nil — раздела нет); для тестов.
func (s *Settings) DisplayCheck() *widget.Check { return s.max60 }

// SelectSearchMode выбирает режим поиска так, как пользователь (для тестов).
func (s *Settings) SelectSearchMode(mode string) {
	s.searchMode.SetSelected(i18n.T(searchModeKeys[mode]))
}

// SelectRandomMode выбирает режим кнопки 🎲 так, как пользователь (для тестов).
func (s *Settings) SelectRandomMode(mode string) {
	s.randomMode.SetSelected(i18n.T(randomModeKeys[mode]))
}

// RandomModeLabel — выбранный режим кнопки 🎲 (подпись); для тестов.
func (s *Settings) RandomModeLabel() string { return s.randomMode.Selected }

// SearchModeLabel — выбранный режим поиска (подпись); для тестов.
func (s *Settings) SearchModeLabel() string { return s.searchMode.Selected }

// SelectLanguage выбирает язык так, как пользователь: код языка или
// i18n.Auto (для тестов).
func (s *Settings) SelectLanguage(code string) {
	s.language.SetSelected(s.language.Options[s.languageIndex(code)])
}

// LanguageHint — подсказка о перезапуске («» — скрыта); для тестов.
func (s *Settings) LanguageHint() string {
	if !s.languageHint.Visible() {
		return ""
	}
	return s.languageHint.Text
}

func (s *Settings) Content() fyne.CanvasObject { return s.content }

// SetBrowserRunning отражает, открыт ли встроенный браузер. Вызывать из UI-потока.
func (s *Settings) SetBrowserRunning(running bool) {
	if s.browser != nil {
		s.browser.SetRunning(running)
	}
}

// BrowserCard — раздел «Браузер» (nil — нет); для тестов.
func (s *Settings) BrowserCard() fyne.CanvasObject {
	switch {
	case s.browser != nil:
		return s.browser.card
	case s.mobileBrowser != nil:
		return s.mobileBrowser
	}
	return nil
}

// Update показывает текущую папку (после её смены).
func (s *Settings) Update() {
	root := s.svc.Library.Root()
	if root == "" {
		root = i18n.T("settings.library.not_selected")
	}
	s.path.SetText(root)
	if s.copyBtn != nil && s.svc.Library.Root() == "" {
		s.copyBtn.Disable()
	}
}

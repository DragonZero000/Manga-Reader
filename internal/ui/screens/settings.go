package screens

import (
	"log"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/display"
)

// Settings — экран настроек: папка библиотеки, поиск, браузер и сведения о версии.
type Settings struct {
	svc *app.Services
	// searchMode — выбор режима поиска
	searchMode *widget.RadioGroup
	// max60 — флажок «Ограничить 60 Гц» (nil — раздела «Экран» нет)
	max60   *widget.Check
	path    *widget.Label
	copyBtn *widget.Button
	browser *browserCard // nil — встроенного браузера нет
	// mobileBrowser — раздел «Браузер» на Android (nil — нет)
	mobileBrowser *widget.Card
	content       fyne.CanvasObject
}

// NewSettings создаёт экран; choose открывает выбор папки (Android, иначе nil),
// onSearchMode получает новый режим поиска. win — окно для диалогов.
func NewSettings(a fyne.App, win fyne.Window, svc *app.Services, notify func(string), choose func(), onSearchMode func(string)) *Settings {
	s := &Settings{svc: svc}
	s.path = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Monospace: true})
	s.path.Wrapping = fyne.TextWrapBreak

	items := []fyne.CanvasObject{s.path}
	if svc.CanChooseFolder {
		change := widget.NewButtonWithIcon("Изменить папку", theme.FolderOpenIcon(), choose)
		hint := widget.NewLabel("Выберите или создайте папку внутри Download, например Download/manga, " +
			"и складывайте архивы туда. Корень Download Android выбрать не даёт.")
		hint.Wrapping = fyne.TextWrapWord
		items = append(items, change, hint)
	} else {
		s.copyBtn = widget.NewButton("Скопировать путь", func() {
			a.Clipboard().SetContent(svc.Library.Root())
			notify("Путь скопирован")
		})
		items = append(items, s.copyBtn)
	}
	if svc.LibraryErr != nil {
		errLabel := widget.NewLabel("Ошибка: " + svc.LibraryErr.Error())
		errLabel.Wrapping = fyne.TextWrapWord
		errLabel.Importance = widget.DangerImportance
		items = append(items, errLabel)
	}

	libraryCard := widget.NewCard("Папка библиотеки", "", container.NewVBox(items...))
	aboutCard := widget.NewCard("О приложении", "", widget.NewLabel("MangaReader "+svc.Version))
	cards := container.NewVBox(libraryCard, s.newSearchCard(onSearchMode))
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

// Подписи режимов поиска.
var searchModeLabels = map[string]string{
	app.SearchModeDynamic: "При вводе",
	app.SearchModeSubmit:  "По кнопке",
}

// newSearchCard — раздел «Поиск»: режим запуска запроса.
func (s *Settings) newSearchCard(onMode func(string)) *widget.Card {
	dynamic, submit := searchModeLabels[app.SearchModeDynamic], searchModeLabels[app.SearchModeSubmit]
	s.searchMode = widget.NewRadioGroup([]string{dynamic, submit}, nil)
	s.searchMode.Required = true
	s.searchMode.SetSelected(searchModeLabels[app.SearchMode(s.svc.Settings)])
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
	hint := widget.NewLabel("При вводе — поиск через 0,5 с после ввода; по кнопке — по кнопке поиска или Enter.")
	hint.Wrapping = fyne.TextWrapWord
	return widget.NewCard("Поиск", "", container.NewVBox(s.searchMode, hint))
}

// Частота экрана; подменяются в тестах.
var (
	displaySupported = display.Supported
	setMax60         = display.SetMax60
)

// newDisplayCard — раздел «Экран» (Android): ограничение частоты 60 Гц.
func (s *Settings) newDisplayCard() *widget.Card {
	s.max60 = widget.NewCheck("Ограничить 60 Гц", nil)
	s.max60.SetChecked(app.DisplayMax60(s.svc.Settings))
	s.max60.OnChanged = func(on bool) {
		app.SetDisplayMax60(s.svc.Settings, on)
		if err := setMax60(on); err != nil {
			log.Printf("частота экрана: %v", err)
		}
	}
	hint := widget.NewLabel("Прокрутка ровнее: приложение рисует 60 кадров в секунду, " +
		"а на экране 120 Гц они показываются неравномерно.")
	hint.Wrapping = fyne.TextWrapWord
	return widget.NewCard("Экран", "", container.NewVBox(s.max60, hint))
}

// DisplayCheck — флажок «Ограничить 60 Гц» (nil — раздела нет); для тестов.
func (s *Settings) DisplayCheck() *widget.Check { return s.max60 }

// SelectSearchMode выбирает режим поиска так, как пользователь (для тестов).
func (s *Settings) SelectSearchMode(mode string) { s.searchMode.SetSelected(searchModeLabels[mode]) }

// SearchModeLabel — выбранный режим поиска (подпись); для тестов.
func (s *Settings) SearchModeLabel() string { return s.searchMode.Selected }

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
		root = "Папка не выбрана"
	}
	s.path.SetText(root)
	if s.copyBtn != nil && s.svc.Library.Root() == "" {
		s.copyBtn.Disable()
	}
}

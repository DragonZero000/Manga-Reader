package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/browser"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
)

// browserCard — раздел «Браузер» экрана настроек (Windows). Меняется только
// в UI-потоке; действия с браузером (запуск, закрытие) — в горутинах.
type browserCard struct {
	svc    *app.Services
	win    fyne.Window
	notify func(string)

	home     *widget.Entry
	homeErr  *widget.Label
	restart  *widget.Label // «Изменения вступят в силу после перезапуска браузера»
	running  bool
	changed  bool // настройки менялись, пока браузер открыт
	card     *widget.Card
	clearBtn map[string]*widget.Button
}

func newBrowserCard(svc *app.Services, win fyne.Window, notify func(string)) *browserCard {
	c := &browserCard{svc: svc, win: win, notify: notify, clearBtn: map[string]*widget.Button{}}
	st := svc.Settings

	c.home = widget.NewEntry()
	c.home.SetPlaceHolder(i18n.T("browser.home_placeholder_desktop"))
	c.home.SetText(st.String(app.KeyBrowserHome, ""))
	c.homeErr = widget.NewLabel(i18n.T("browser.home_invalid"))
	c.homeErr.Importance = widget.DangerImportance
	c.homeErr.Hide()
	c.home.OnChanged = c.setHome

	c.restart = widget.NewLabel(i18n.T("browser.restart"))
	c.restart.Wrapping = fyne.TextWrapWord
	c.restart.Importance = widget.WarningImportance
	c.restart.Hide()

	// поисковик выбирается в настройках самого Firefox (без корпоративных политик
	// приложение не может задать его) и хранится в профиле
	search := widget.NewButtonWithIcon(i18n.T("browser.search"), theme.SearchIcon(), func() { c.open("about:preferences#search") })
	addons := widget.NewButtonWithIcon(i18n.T("browser.addons"), theme.SettingsIcon(), func() { c.open("about:addons") })
	c.clearBtn[browser.ClearCookies] = widget.NewButtonWithIcon(i18n.T("browser.clear_cookies"), theme.DeleteIcon(),
		func() {
			c.confirmClear(browser.ClearCookies, i18n.T("browser.clear_cookies.confirm"), i18n.T("browser.clear_cookies.text_desktop"))
		})
	c.clearBtn[browser.ClearHistory] = widget.NewButtonWithIcon(i18n.T("browser.clear_history"), theme.DeleteIcon(),
		func() {
			c.confirmClear(browser.ClearHistory, i18n.T("browser.clear_history.confirm"), i18n.T("browser.clear_history.text_desktop"))
		})

	form := widget.NewForm(
		widget.NewFormItem(i18n.T("browser.home"), container.NewVBox(c.home, c.homeErr)),
	)
	c.card = widget.NewCard(i18n.T("browser.title"), "", container.NewVBox(
		cardNote(i18n.T("browser.note")),
		form, c.restart, container.NewGridWithColumns(2, search, addons),
		c.clearBtn[browser.ClearCookies], c.clearBtn[browser.ClearHistory],
	))
	return c
}

// setHome сохраняет домашнюю страницу, если адрес корректен (или пуст).
func (c *browserCard) setHome(text string) {
	if text != "" && model.WebURL(text) == "" {
		c.homeErr.Show()
		return
	}
	c.homeErr.Hide()
	c.svc.Settings.SetString(app.KeyBrowserHome, model.WebURL(text))
	c.settingsChanged()
}

func (c *browserCard) settingsChanged() {
	if c.running {
		c.changed = true
		c.restart.Show()
	}
}

// SetRunning отражает состояние браузера (открыт/закрыт).
func (c *browserCard) SetRunning(running bool) {
	c.running = running
	if !running {
		c.changed = false
	}
	if c.changed {
		c.restart.Show()
	} else {
		c.restart.Hide()
	}
}

// open открывает адрес во встроенном браузере (в фоне: запуск может ждать).
func (c *browserCard) open(url string) {
	b := c.svc.Browser
	go func() {
		if err := b.Open(url); err != nil {
			c.notify(ErrorText(err))
		}
	}()
}

// confirmClear запрашивает очистку kind; Firefox выполнит её при следующем
// открытии. Открытый браузер предлагается закрыть.
func (c *browserCard) confirmClear(kind, title, text string) {
	Confirm(title, text, i18n.T("browser.clear"), func(ok bool) {
		if !ok {
			return
		}
		app.RequestBrowserClear(c.svc.Settings, kind)
		if !c.running {
			c.notify(i18n.T("browser.clear_later"))
			return
		}
		Confirm(i18n.T("browser.close.title"), i18n.T("browser.close.text"), i18n.T("browser.close"), func(close bool) {
			if close {
				go c.svc.Browser.Close()
			}
			c.notify(i18n.T("browser.clear_later"))
		}, c.win)
	}, c.win)
}

package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/model"
)

// Положение панели браузера — ключи перевода подписей в настройках.
var toolbarKeys = map[string]string{
	mobilebrowser.ToolbarBottom: "browser.toolbar.bottom",
	mobilebrowser.ToolbarTop:    "browser.toolbar.top",
}

// newMobileBrowserCard — раздел «Браузер» на Android: домашняя страница,
// поисковик, положение панели, очистка данных. Настройки передаются
// браузеру при каждом открытии.
func newMobileBrowserCard(svc *app.Services, win fyne.Window, notify func(string)) *widget.Card {
	st := svc.Settings

	home := widget.NewEntry()
	home.SetPlaceHolder(i18n.T("browser.home_placeholder_mobile"))
	home.SetText(st.String(app.KeyBrowserHome, ""))
	homeErr := widget.NewLabel(i18n.T("browser.home_invalid"))
	homeErr.Importance = widget.DangerImportance
	homeErr.Hide()
	home.OnChanged = func(text string) {
		if text != "" && model.WebURL(text) == "" {
			homeErr.Show()
			return
		}
		homeErr.Hide()
		st.SetString(app.KeyBrowserHome, model.WebURL(text))
	}

	var engines []string
	for _, e := range mobilebrowser.Engines {
		engines = append(engines, e.Name)
	}
	search := widget.NewSelect(engines, func(v string) { st.SetString(app.KeyBrowserSearch, v) })
	cur := st.String(app.KeyBrowserSearch, "")
	if cur == "" {
		cur = engines[0]
	}
	search.SetSelected(cur)

	toolbarLabels := map[string]string{}
	for k, key := range toolbarKeys {
		toolbarLabels[k] = i18n.T(key)
	}
	toolbar := widget.NewSelect([]string{toolbarLabels[mobilebrowser.ToolbarBottom], toolbarLabels[mobilebrowser.ToolbarTop]}, func(v string) {
		for k, label := range toolbarLabels {
			if label == v {
				st.SetString(app.KeyBrowserToolbar, k)
			}
		}
	})
	toolbar.SetSelected(toolbarLabels[app.MobileBrowserSettings(st).Toolbar])

	clear := func(kind, title, text string) func() {
		return func() {
			Confirm(title, text, i18n.T("browser.clear"), func(ok bool) {
				if !ok {
					return
				}
				if err := mobilebrowser.Clear([]string{kind}); err != nil {
					notify(i18n.T("browser.clear_failed", "Error", ErrorText(err)))
				}
			}, win)
		}
	}
	clearCookies := widget.NewButtonWithIcon(i18n.T("browser.clear_cookies"), theme.DeleteIcon(), clear(mobilebrowser.ClearCookies,
		i18n.T("browser.clear_cookies.confirm"), i18n.T("browser.clear_cookies.text_mobile")))
	clearHistory := widget.NewButtonWithIcon(i18n.T("browser.clear_history"), theme.DeleteIcon(), clear(mobilebrowser.ClearHistory,
		i18n.T("browser.clear_history.confirm"), i18n.T("browser.clear_history.text_mobile")))

	form := widget.NewForm(
		widget.NewFormItem(i18n.T("browser.home"), container.NewVBox(home, homeErr)),
		widget.NewFormItem(i18n.T("browser.search"), search),
		widget.NewFormItem(i18n.T("browser.toolbar"), toolbar),
	)
	return widget.NewCard(i18n.T("browser.title"), "", container.NewVBox(
		cardNote(i18n.T("browser.note")), form, clearCookies, clearHistory))
}

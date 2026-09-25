package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/model"
)

// Положение панели браузера — подписи в настройках.
var toolbarLabels = map[string]string{
	mobilebrowser.ToolbarBottom: "Снизу",
	mobilebrowser.ToolbarTop:    "Сверху",
}

// newMobileBrowserCard — раздел «Браузер» на Android: домашняя страница,
// поисковик, положение панели, очистка данных. Настройки передаются
// браузеру при каждом открытии.
func newMobileBrowserCard(svc *app.Services, win fyne.Window, notify func(string)) *widget.Card {
	st := svc.Settings

	home := widget.NewEntry()
	home.SetPlaceHolder("Пустая вкладка")
	home.SetText(st.String(app.KeyBrowserHome, ""))
	homeErr := widget.NewLabel("Нужен адрес, начинающийся с http:// или https://")
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
			Confirm(title, text, "Очистить", func(ok bool) {
				if !ok {
					return
				}
				if err := mobilebrowser.Clear([]string{kind}); err != nil {
					notify("Не удалось очистить: " + err.Error())
				}
			}, win)
		}
	}
	clearCookies := widget.NewButtonWithIcon("Очистить cookies и данные сайтов", theme.DeleteIcon(), clear(mobilebrowser.ClearCookies,
		"Очистить cookies и данные сайтов?", "Вы выйдете из аккаунтов на сайтах. Закладки и расширения сохранятся."))
	clearHistory := widget.NewButtonWithIcon("Очистить историю", theme.DeleteIcon(), clear(mobilebrowser.ClearHistory,
		"Очистить историю?", "Будут удалены история вкладок и список загрузок. Закладки, расширения и cookies сохранятся."))

	form := widget.NewForm(
		widget.NewFormItem("Домашняя страница", container.NewVBox(home, homeErr)),
		widget.NewFormItem("Поисковик", search),
		widget.NewFormItem("Панель браузера", toolbar),
	)
	return widget.NewCard("Браузер", "Встроенный Firefox; загрузки сохраняются в папку библиотеки",
		container.NewVBox(form, clearCookies, clearHistory))
}

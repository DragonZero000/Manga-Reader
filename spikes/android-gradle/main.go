// Спайк S-G1: Fyne-приложение, собранное Gradle вместо fyne package.
// Проверяет: запуск GoNativeActivity из своего APK, метаданные без
// fyne_metadata_init.go, Preferences, экранную клавиатуру, выбор папки (SAF),
// кнопку «Назад».
package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

// version подставляется при сборке (-X main.version).
var version = "dev"

func init() {
	// вместо fyne_metadata_init.go, который генерирует fyne package
	app.SetMetadata(fyne.AppMetadata{ID: "io.github.mangareader.spike.gradle", Name: "Gradle spike", Version: version, Build: 1})
}

func main() {
	a := app.NewWithID("io.github.mangareader.spike.gradle")
	w := a.NewWindow("Gradle spike")

	runs := a.Preferences().IntWithFallback("runs", 0) + 1
	a.Preferences().SetInt("runs", runs)
	meta := a.Metadata()
	info := widget.NewLabel(fmt.Sprintf("ID=%s версия=%s запусков=%d", meta.ID, meta.Version, runs))
	info.Wrapping = fyne.TextWrapBreak

	folder := widget.NewLabel("папка не выбрана")
	folder.Wrapping = fyne.TextWrapBreak
	choose := widget.NewButton("Выбрать папку", func() {
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			switch {
			case err != nil:
				folder.SetText("ошибка: " + err.Error())
			case u == nil:
				folder.SetText("отменено")
			default:
				items, lerr := u.List()
				folder.SetText(fmt.Sprintf("%s\nэлементов: %d %v", u.String(), len(items), lerr))
			}
		}, w)
	})
	entry := widget.NewEntry()
	entry.SetPlaceHolder("поле ввода (клавиатура)")
	back := widget.NewLabel("«Назад»: —")
	w.Canvas().SetOnTypedKey(func(ev *fyne.KeyEvent) { back.SetText("«Назад»: " + string(ev.Name)) })

	w.SetContent(container.NewVBox(info, entry, choose, folder, back))
	w.ShowAndRun()
}

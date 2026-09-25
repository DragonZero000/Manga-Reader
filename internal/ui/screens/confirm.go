package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// Confirm — диалог подтверждения с русскими подписями кнопок: confirm —
// действие («Очистить»), вторая кнопка — «Отмена». dialog.ShowConfirm
// подписывает кнопки по языку системы, а на Android Fyne его не определяет
// и показывает «Yes/No».
func Confirm(title, text, confirm string, cb func(bool), win fyne.Window) {
	d := dialog.NewConfirm(title, text, cb, win)
	d.SetConfirmText(confirm)
	d.SetDismissText("Отмена")
	d.Show()
}

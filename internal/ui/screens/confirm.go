package screens

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"mangareader/internal/i18n"
)

// Confirm — диалог подтверждения с подписями кнопок на языке интерфейса:
// confirm — действие («Очистить»), вторая кнопка — «Отмена». dialog.ShowConfirm
// подписывает кнопки «Yes/No» по языку Fyne, а не действием.
func Confirm(title, text, confirm string, cb func(bool), win fyne.Window) {
	d := dialog.NewConfirm(title, text, cb, win)
	d.SetConfirmText(confirm)
	d.SetDismissText(i18n.T("dialog.cancel"))
	d.Show()
}

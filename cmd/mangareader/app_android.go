//go:build android

package main

import (
	"runtime/debug"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"

	"mangareader/internal/paths"
)

// mobileMemoryLimit — мягкий лимит кучи Go на телефонах. В открытой читалке
// живая куча около 220 МБ (кэш страниц, миниатюры, пул обложек): при лимите
// ниже неё сборщик работает почти непрерывно — тысячи пауз за минуту.
const mobileMemoryLimit = 384 << 20

// newFyneApp — Android: приложение с ID (метаданные — metadata_android.go).
func newFyneApp() fyne.App {
	a := fyneapp.NewWithID(paths.AppID)
	// мягкий лимит кучи: сборщик мусора работает активнее, а не ждёт
	// двукратного роста (декодированные страницы занимают десятки МБ)
	debug.SetMemoryLimit(mobileMemoryLimit)
	if version == "" {
		version = a.Metadata().Version
	}
	return a
}

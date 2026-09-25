//go:build !android

package main

import (
	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"
)

// newFyneApp — ПК: приложение без ID. С пустым ID Fyne не создаёт хранилище
// настроек и не пишет preferences.json в %APPDATA% (портативный режим).
// Метаданные задаются кодом: заданное имя отключает поиск FyneApp.toml.
func newFyneApp() fyne.App {
	if version == "" {
		version = "dev"
	}
	fyneapp.SetMetadata(fyne.AppMetadata{
		Name:       "MangaReader",
		Version:    version,
		Build:      1,
		Migrations: map[string]bool{"fyneDo": true},
	})
	return fyneapp.New()
}

package main

import (
	"log"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/ui"
)

// version задаётся при сборке: -ldflags "-X main.version=...".
// Если не задана, на Android берётся из метаданных приложения, на ПК — "dev".
var version string

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	a := newFyneApp()
	log.Printf("MangaReader %s", version)
	svc := app.New(version, a)
	// язык — до создания экранов: подписи берутся при их построении
	sys := i18n.DetectSystemLocale()
	i18n.Start(app.UILanguage(svc.Settings), sys)
	if err := i18n.ApplyToFyne(sys); err != nil {
		log.Printf("i18n: %v", err)
	}
	log.Printf("language: %s (setting %s, system %s)", i18n.Lang(), app.UILanguage(svc.Settings), sys)
	shell := ui.NewShell(a, svc)
	shell.Window.ShowAndRun()
}

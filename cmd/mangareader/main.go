package main

import (
	"log"

	"mangareader/internal/app"
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
	shell := ui.NewShell(a, svc)
	shell.Window.ShowAndRun()
}

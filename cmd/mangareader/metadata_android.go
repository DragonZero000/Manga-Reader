//go:build android

package main

import (
	"strconv"

	"fyne.io/fyne/v2"
	fyneapp "fyne.io/fyne/v2/app"

	"mangareader/internal/paths"
)

// build — номер сборки: -ldflags "-X main.build=...".
var build string

// Метаданные приложения: раньше их подставлял fyne package
// (fyne_metadata_init.go), при сборке через Gradle — этот файл.
// Версия и номер сборки передаются Makefile из FyneApp.toml.
func init() {
	n, _ := strconv.Atoi(build)
	fyneapp.SetMetadata(fyne.AppMetadata{
		ID:      paths.AppID,
		Name:    "MangaReader",
		Version: version,
		Build:   n,
	})
}

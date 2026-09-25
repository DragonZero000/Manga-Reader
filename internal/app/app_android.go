//go:build android

package app

import (
	"fmt"
	"path/filepath"

	"fyne.io/fyne/v2"

	"mangareader/internal/library"
	"mangareader/internal/storage"
)

// New — Android: папка библиотеки выбирается пользователем (SAF), выбор
// хранится в настройках Fyne (внутреннее хранилище приложения).
func New(version string, a fyne.App) *Services {
	settings := storage.NewPrefSettings(a.Preferences())
	// адреса страниц скачанных файлов — в личной папке приложения
	links := library.LoadLinks(filepath.Join(a.Storage().RootURI().Path(), "links.json"))
	var st storage.Storage
	if tree := settings.String(KeyLibraryTree, ""); tree != "" {
		st = library.WithLinks(storage.NewSAF(tree), links)
	}
	s := newServices(version, st, settings)
	s.CanChooseFolder = true
	s.MobileBrowser = true
	s.Links = links
	return s
}

// NeedsWriteAccess — к папке библиотеки есть доступ только на чтение (выбрана
// до появления браузера): для загрузок её нужно выбрать заново.
func (s *Services) NeedsWriteAccess() bool {
	tree := s.Settings.String(KeyLibraryTree, "")
	return tree != "" && !storage.HasWriteAccess(tree)
}

// SetFolder запоминает выбранную папку (tree-URI из системного выбора):
// делает разрешение постоянным и переключает библиотеку на неё.
// Вызывать сразу после выбора, пока действует временный доступ.
func (s *Services) SetFolder(tree string) error {
	if err := storage.TakePersistable(tree); err != nil {
		return fmt.Errorf("не удалось сохранить доступ к папке: %w", err)
	}
	s.Settings.SetString(KeyLibraryTree, tree)
	s.Library.SetStorage(library.WithLinks(storage.NewSAF(tree), s.Links))
	return nil
}

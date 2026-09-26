//go:build android

package app

import (
	"fmt"
	"log"
	"path/filepath"

	"fyne.io/fyne/v2"

	"mangareader/internal/catalog"
	"mangareader/internal/library"
	"mangareader/internal/storage"
)

// New — Android: папка библиотеки выбирается пользователем (SAF), выбор
// хранится в настройках Fyne (внутреннее хранилище приложения).
func New(version string, a fyne.App) *Services {
	settings := storage.NewPrefSettings(a.Preferences())
	dir := a.Storage().RootURI().Path() // личная папка приложения
	tree := settings.String(KeyLibraryTree, "")
	cat := openCatalog(filepath.Join(dir, catalog.FileName), tree)
	// адреса страниц скачанных файлов: links.json и копия в каталоге —
	// каждая восстанавливает другую
	links := library.LoadLinksWithBackup(filepath.Join(dir, "links.json"), linkBackup(cat))
	if cat != nil && cat.Fresh() {
		if err := cat.ImportLinks(links.All()); err != nil {
			log.Printf("каталог: перенос ссылок: %v", err)
		}
	}
	var st storage.Storage
	if tree != "" {
		st = library.WithLinks(storage.NewSAF(tree), links)
	}
	// телефон: меньше памяти и ядер под миниатюры, чтобы не мешать UI
	s := newServices(version, st, settings, thumbsConfig{limit: 48 << 20, workers: 2}, cat)
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
	if s.Catalog != nil {
		// каталог — одной папки: прежние галереи, обложки и ссылки не нужны
		if err := s.Catalog.Reset(tree); err != nil {
			log.Printf("каталог: смена папки: %v", err)
		}
	}
	s.Library.SetStorage(library.WithLinks(storage.NewSAF(tree), s.Links))
	return nil
}

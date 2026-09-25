//go:build !android

package app

import (
	"log"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"

	"mangareader/internal/browser"
	"mangareader/internal/library"
	"mangareader/internal/paths"
	"mangareader/internal/storage"
)

// New — портативный режим ПК: всё в папке приложения (архивы в manga,
// настройки в settings.json). Вне папки приложения ничего не пишется.
func New(version string, _ fyne.App) *Services {
	dir, err := paths.AppDir()
	if err != nil {
		log.Printf("папка приложения: %v", err)
	}
	lib := filepath.Join(dir, paths.LibraryDirName)
	settings := storage.NewFileSettings(filepath.Join(dir, paths.SettingsFileName))
	s := newServices(version, storage.NewFS(lib), settings)
	if err := paths.EnsureDir(lib); err != nil {
		s.LibraryErr = err
		log.Printf("папка библиотеки: %v", err)
		return s
	}
	// без наблюдения работают сканирование при запуске и кнопка «Обновить»
	if w, err := library.NewWatcher(lib); err != nil {
		log.Printf("папка библиотеки: %v", err)
	} else {
		s.Watcher = w
	}
	if runtime.GOOS == "windows" {
		s.Browser = newBrowser(dir, lib, settings)
	}
	return s
}

// newBrowser — встроенный браузер: Firefox и профиль в папке browser рядом
// с приложением, загрузки — в папку библиотеки.
func newBrowser(dir, lib string, settings storage.Settings) *browser.Browser {
	ff := filepath.Join(dir, "browser", "firefox")
	b := browser.New(browser.Options{
		FirefoxDir:   ff,
		ProfileDir:   filepath.Join(dir, "browser", "profile"),
		DownloadDir:  lib,
		Prefs:        func() (string, []string) { return BrowserPrefs(settings) },
		ClearApplied: func() { settings.SetString(KeyBrowserClear, "") },
	})
	// уборка записей реестра, если браузер в прошлый раз не закрылся штатно
	go func() {
		if b.Available() && !b.Running() {
			if n, err := browser.CleanRegistry(ff, nil); err != nil {
				log.Printf("браузер: уборка реестра: %v", err)
			} else if n > 0 {
				log.Printf("браузер: удалено записей реестра после прошлого запуска: %d", n)
			}
		}
	}()
	return b
}

// NeedsWriteAccess: на ПК папка библиотеки доступна на запись всегда.
func (s *Services) NeedsWriteAccess() bool { return false }

// SetFolder на ПК не поддерживается: папка фиксирована.
func (s *Services) SetFolder(string) error { return ErrChooseNotSupported }

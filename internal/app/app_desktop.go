//go:build !android

package app

import (
	"log"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2"

	"mangareader/internal/browser"
	"mangareader/internal/catalog"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/paths"
	"mangareader/internal/storage"
	"mangareader/internal/userdata"
)

// desktopLibraryKey — ключ папки библиотеки ПК в пользовательских данных:
// папка всегда <папка приложения>/manga, и перенос портативной папки не
// должен отвязывать данные от произведений.
const desktopLibraryKey = "app:manga"

// New — портативный режим ПК: всё в папке приложения (архивы в manga,
// настройки в settings.json). Вне папки приложения ничего не пишется.
func New(version string, _ fyne.App) *Services {
	dir, err := paths.AppDir()
	if err != nil {
		log.Printf("app folder: %v", err)
	}
	s := newPortable(version, dir)
	lib := filepath.Join(dir, paths.LibraryDirName)
	if err := paths.EnsureDir(lib); err != nil {
		s.LibraryErr = err
		log.Printf("library folder: %v", err)
		return s
	}
	// без наблюдения работают сканирование при запуске и кнопка «Обновить»
	if w, err := library.NewWatcher(lib); err != nil {
		log.Printf("library folder: %v", err)
	} else {
		s.Watcher = w
	}
	if runtime.GOOS == "windows" {
		s.Browser = newBrowser(dir, lib, s.Settings)
	}
	return s
}

// newPortable — сервисы над папкой приложения dir: настройки, каталог
// и пользовательские данные рядом с исполняемым файлом, архивы в manga.
func newPortable(version, dir string) *Services {
	lib := filepath.Join(dir, paths.LibraryDirName)
	settings := storage.NewFileSettings(filepath.Join(dir, paths.SettingsFileName))
	// каталог и пользовательские данные — рядом с settings.json (портативно)
	cat := openCatalog(filepath.Join(dir, catalog.FileName), lib)
	ud, backup := openUserData(filepath.Join(dir, userdata.FileName))
	s := newServices(version, storage.NewFS(lib), settings, thumbsConfig{limit: 64 << 20}, cat)
	s.attachUserData(ud, backup, desktopLibraryKey)
	// до окна и до первого сканирования: чтение каталога без открытия архивов
	s.loadCatalog()
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
		Lang:         i18n.Lang, // язык задаётся после создания сервисов
		Prefs:        func() (string, []string) { return BrowserPrefs(settings) },
		ClearApplied: func() { settings.SetString(KeyBrowserClear, "") },
	})
	// уборка записей реестра, если браузер в прошлый раз не закрылся штатно
	go func() {
		if b.Available() && !b.Running() {
			if n, err := browser.CleanRegistry(ff, nil); err != nil {
				log.Printf("browser: registry cleanup: %v", err)
			} else if n > 0 {
				log.Printf("browser: registry entries removed after the previous run: %d", n)
			}
		}
	}()
	return b
}

// NeedsWriteAccess: на ПК папка библиотеки доступна на запись всегда.
func (s *Services) NeedsWriteAccess() bool { return false }

// SetFolder на ПК не поддерживается: папка фиксирована.
func (s *Services) SetFolder(string) error { return ErrChooseNotSupported }

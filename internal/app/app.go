// Package app собирает зависимости приложения. Пакет не работает с виджетами:
// UI получает готовые сервисы через Services.
package app

import (
	"errors"
	"log"
	"strings"

	"mangareader/internal/browser"
	"mangareader/internal/library"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/problems"
	"mangareader/internal/search"
	"mangareader/internal/storage"
	"mangareader/internal/thumbs"
)

// KeyLibraryTree — настройка с деревом SAF выбранной папки (Android).
const KeyLibraryTree = "library.tree"

// ErrChooseNotSupported — выбор папки на этой платформе не поддерживается.
var ErrChooseNotSupported = errors.New("выбор папки не поддерживается")

// Services — зависимости, передаваемые в UI.
type Services struct {
	Version string

	// LibraryErr — ошибка подготовки папки библиотеки (ПК); приложение продолжает работу.
	LibraryErr error
	// CanChooseFolder — папку выбирает пользователь (Android).
	CanChooseFolder bool

	Settings storage.Settings
	Index    search.Index
	Library  *library.Source
	Thumbs   *thumbs.Cache
	// Problems — ошибочные файлы библиотеки и их статус просмотра.
	Problems *problems.Tracker
	// Watcher — наблюдение за папкой библиотеки (ПК); nil — недоступно.
	Watcher *library.Watcher
	// Browser — встроенный браузер (Windows); nil — нет на этой платформе.
	Browser *browser.Browser
	// MobileBrowser — встроенный браузер Android (GeckoView) есть.
	MobileBrowser bool
	// Links — адреса страниц скачанных файлов (Android); nil — нет.
	Links *library.Links
}

// Настройки встроенного браузера.
const (
	KeyBrowserHome = "browser.home"
	// KeyBrowserSearch, KeyBrowserToolbar — поисковик (имя из
	// mobilebrowser.Engines) и положение панели (Android).
	KeyBrowserSearch  = "browser.search"
	KeyBrowserToolbar = "browser.toolbar"
	// KeyBrowserClear — запрошенные очистки через запятую (browser.ClearCookies,
	// browser.ClearHistory); выполняются при следующем запуске браузера.
	KeyBrowserClear = "browser.clear"
)

// Режим запуска поиска.
const (
	KeySearchMode = "search.mode"
	// SearchModeDynamic — запрос через паузу после ввода (по умолчанию).
	SearchModeDynamic = "dynamic"
	// SearchModeSubmit — запрос только по кнопке или Enter.
	SearchModeSubmit = "submit"
)

// SearchMode — режим поиска из Settings; неизвестное значение — SearchModeDynamic.
func SearchMode(s storage.Settings) string {
	if s.String(KeySearchMode, "") == SearchModeSubmit {
		return SearchModeSubmit
	}
	return SearchModeDynamic
}

// BrowserPrefs — настройки браузера из Settings (для browser.Options.Prefs).
// Поисковик выбирается в настройках самого Firefox и хранится в профиле.
func BrowserPrefs(s storage.Settings) (home string, clear []string) {
	for _, c := range strings.Split(s.String(KeyBrowserClear, ""), ",") {
		if c != "" {
			clear = append(clear, c)
		}
	}
	return s.String(KeyBrowserHome, ""), clear
}

// RequestBrowserClear добавляет очистку c к запрошенным.
func RequestBrowserClear(s storage.Settings, c string) {
	_, clear := BrowserPrefs(s)
	for _, x := range clear {
		if x == c {
			return
		}
	}
	s.SetString(KeyBrowserClear, strings.Join(append(clear, c), ","))
}

// newServices собирает общие сервисы вокруг хранилища (nil — папка не выбрана).
func newServices(version string, st storage.Storage, settings storage.Settings) *Services {
	s := &Services{Version: version, Settings: settings, Index: search.NewMemIndex(), Problems: problems.New(settings)}
	s.Library = library.NewSource(st, s.Index)
	s.Thumbs = thumbs.New(s.Library.OpenPage, thumbs.DefaultCapacity, 0)
	if root := s.Library.Root(); root != "" {
		log.Printf("папка библиотеки: %s", root)
	} else {
		log.Printf("папка библиотеки не выбрана")
	}
	return s
}

// NewForTest — сервисы над готовым источником (для тестов UI).
func NewForTest(src *library.Source, idx search.Index, settings storage.Settings) *Services {
	return &Services{Version: "test", Settings: settings, Index: idx, Library: src,
		Thumbs: thumbs.New(src.OpenPage, 10, 1), Problems: problems.New(settings)}
}

// MobileBrowserSettings — настройки браузера Android из Settings.
func MobileBrowserSettings(s storage.Settings) mobilebrowser.Settings {
	toolbar := s.String(KeyBrowserToolbar, mobilebrowser.ToolbarBottom)
	if toolbar != mobilebrowser.ToolbarTop {
		toolbar = mobilebrowser.ToolbarBottom
	}
	return mobilebrowser.Settings{
		Home:    s.String(KeyBrowserHome, ""),
		Search:  mobilebrowser.EngineTemplate(s.String(KeyBrowserSearch, "")),
		Toolbar: toolbar,
	}
}

// OpenMobileBrowser открывает встроенный браузер Android: url — в новой
// вкладке («» — текущая вкладка).
func (s *Services) OpenMobileBrowser(url string) error {
	return mobilebrowser.Open(url, s.Settings.String(KeyLibraryTree, ""), MobileBrowserSettings(s.Settings))
}

// OnDownloaded — встроенный браузер Android скачал файл rel со страницы
// page: адрес запоминается, файл будет разобран заново. Вызывается из
// потока загрузки; ждёт идущего сканирования.
func (s *Services) OnDownloaded(rel, page string) {
	if s.Links != nil {
		if err := s.Links.Set(rel, page); err != nil {
			log.Printf("браузер: %v", err)
		}
	}
	s.Library.Invalidate(rel)
}

// PruneLinks удаляет адреса файлов, которых больше нет в папке библиотеки.
func (s *Services) PruneLinks() {
	if s.Links == nil {
		return
	}
	exists := map[string]bool{}
	for _, g := range s.Library.Galleries() {
		exists[g.Key.ID] = true
	}
	for _, it := range s.Problems.Items() {
		exists[it.RelPath] = true
	}
	if err := s.Links.Prune(func(rel string) bool { return exists[rel] }); err != nil {
		log.Printf("ссылки: %v", err)
	}
}

// FolderChosen — папка выбрана (на ПК — всегда).
func (s *Services) FolderChosen() bool { return s.Library.Root() != "" }

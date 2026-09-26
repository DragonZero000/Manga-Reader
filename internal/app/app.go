// Package app собирает зависимости приложения. Пакет не работает с виджетами:
// UI получает готовые сервисы через Services.
package app

import (
	"errors"
	"log"
	"strings"

	"mangareader/internal/browser"
	"mangareader/internal/catalog"
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
	// Catalog — каталог библиотеки (library.db); nil — недоступен, всё в памяти.
	Catalog *catalog.Catalog
	// Cached — библиотека из каталога на момент запуска (до сканирования).
	Cached  library.ScanResult
	Index   search.Index
	Library *library.Source
	Thumbs  *thumbs.Cache
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

// KeyDisplayMax60 — ограничить частоту экрана 60 Гц (Android): «1» или «0».
const KeyDisplayMax60 = "display.max60"

// DisplayMax60 — ограничивать ли частоту экрана 60 Гц (по умолчанию да).
func DisplayMax60(s storage.Settings) bool {
	return s.String(KeyDisplayMax60, "1") != "0"
}

// SetDisplayMax60 сохраняет настройку частоты экрана.
func SetDisplayMax60(s storage.Settings, on bool) {
	v := "0"
	if on {
		v = "1"
	}
	s.SetString(KeyDisplayMax60, v)
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

// thumbsConfig — объём кэша миниатюр и число декодеров (0 — по умолчанию).
type thumbsConfig struct {
	limit   int64
	workers int
}

// openCatalog открывает каталог библиотеки path для папки root; nil — не
// удалось (приложение работает без каталога, всё в памяти).
func openCatalog(path, root string) *catalog.Catalog {
	c, err := catalog.Open(path, root)
	if err != nil {
		log.Printf("каталог недоступен, библиотека — только в памяти: %v", err)
		return nil
	}
	if c.Fresh() {
		log.Printf("каталог %s создан, библиотека будет просканирована целиком", path)
	}
	return c
}

// linkBackup — каталог как вторая копия ссылок (nil — нет каталога).
func linkBackup(c *catalog.Catalog) library.LinkBackup {
	if c == nil {
		return nil
	}
	return c
}

// newServices собирает общие сервисы вокруг хранилища (nil — папка не
// выбрана) и каталога (nil — без каталога: индекс и результаты в памяти).
func newServices(version string, st storage.Storage, settings storage.Settings, tc thumbsConfig, cat *catalog.Catalog) *Services {
	s := &Services{Version: version, Settings: settings, Catalog: cat, Problems: problems.New(settings)}
	if cat != nil {
		s.Index = cat.Index()
		s.Library = library.NewSourceWithStore(st, s.Index, cat.ScanStore())
	} else {
		s.Index = search.NewMemIndex()
		s.Library = library.NewSource(st, s.Index)
	}
	s.Thumbs = thumbs.New(s.Library.OpenPage, tc.limit, tc.workers)
	if cat != nil {
		s.Thumbs.SetStore(cat.Covers())
		// до окна и до первого сканирования: чтение каталога без открытия архивов
		s.Cached = s.Library.LoadCatalog()
	}
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
		Thumbs: thumbs.New(src.OpenPage, 1<<20, 1), Problems: problems.New(settings)}
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

// Close закрывает каталог (при выходе из приложения).
func (s *Services) Close() {
	if s.Catalog != nil {
		if err := s.Catalog.Close(); err != nil {
			log.Printf("каталог: %v", err)
		}
	}
}

// FolderChosen — папка выбрана (на ПК — всегда).
func (s *Services) FolderChosen() bool { return s.Library.Root() != "" }

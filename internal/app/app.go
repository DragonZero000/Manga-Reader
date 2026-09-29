// Package app собирает зависимости приложения. Пакет не работает с виджетами:
// UI получает готовые сервисы через Services.
package app

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"mangareader/internal/browser"
	"mangareader/internal/catalog"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/problems"
	"mangareader/internal/search"
	"mangareader/internal/storage"
	"mangareader/internal/thumbs"
	"mangareader/internal/userdata"
)

// KeyLibraryTree — настройка с деревом SAF выбранной папки (Android).
const KeyLibraryTree = "library.tree"

// ErrChooseNotSupported — выбор папки на этой платформе не поддерживается.
var ErrChooseNotSupported = errors.New("folder selection is not supported")

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
	// UserData — пользовательские данные о произведениях (user.db); nil —
	// недоступны (ошибка открытия или база новее приложения).
	UserData *userdata.Store
	// UserDataBackup — резервная копия повреждённой базы, созданная при
	// запуске; «» — база была в порядке.
	UserDataBackup string

	userObs *userDataObserver // nil — без пользовательских данных
	// bg — фоновые задачи сервисов (переиндексация); stopBg их отменяет
	bg     sync.WaitGroup
	stopBg context.CancelFunc
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

// Режим кнопки «Случайное».
const (
	KeyRandomMode = "random.mode"
	// RandomModeRepeat — каждый раз из всего набора (по умолчанию).
	RandomModeRepeat = "repeat"
	// RandomModeNoRepeat — без повторов, пока не выпадут все галереи набора.
	RandomModeNoRepeat = "norepeat"
)

// RandomMode — режим случайного выбора из Settings; неизвестное значение —
// RandomModeRepeat.
func RandomMode(s storage.Settings) string {
	if s.String(KeyRandomMode, "") == RandomModeNoRepeat {
		return RandomModeNoRepeat
	}
	return RandomModeRepeat
}

// SetRandomMode сохраняет режим случайного выбора.
func SetRandomMode(s storage.Settings, mode string) {
	s.SetString(KeyRandomMode, mode)
}

// KeyUILanguage — язык интерфейса: код языка из i18n.Available или
// i18n.Auto (как в системе, по умолчанию). Применяется при запуске.
const KeyUILanguage = "ui.language"

// UILanguage — выбранный язык интерфейса: код языка или i18n.Auto.
func UILanguage(s storage.Settings) string {
	return s.String(KeyUILanguage, i18n.Auto)
}

// SetUILanguage сохраняет язык интерфейса (код языка или i18n.Auto).
func SetUILanguage(s storage.Settings, lang string) {
	s.SetString(KeyUILanguage, lang)
}

// Плотность сетки карточек.
const (
	// KeyGridColumns — карточек в ряду на телефоне: «2», «3» или «4».
	KeyGridColumns = "grid.columns"
	// KeyGridSize — размер карточек на ПК: GridSizeSmall, GridSizeMedium, GridSizeLarge.
	KeyGridSize = "grid.size"

	GridSizeSmall  = "s"
	GridSizeMedium = "m" // по умолчанию
	GridSizeLarge  = "l"
)

// GridColumns — карточек в ряду на телефоне (2–4); неизвестное значение — 3.
func GridColumns(s storage.Settings) int {
	switch s.String(KeyGridColumns, "") {
	case "2":
		return 2
	case "4":
		return 4
	}
	return 3
}

// SetGridColumns сохраняет число карточек в ряду.
func SetGridColumns(s storage.Settings, n int) {
	s.SetString(KeyGridColumns, strconv.Itoa(n))
}

// GridSize — размер карточек на ПК; неизвестное значение — GridSizeMedium.
func GridSize(s storage.Settings) string {
	switch v := s.String(KeyGridSize, ""); v {
	case GridSizeSmall, GridSizeLarge:
		return v
	}
	return GridSizeMedium
}

// SetGridSize сохраняет размер карточек.
func SetGridSize(s storage.Settings, size string) {
	s.SetString(KeyGridSize, size)
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

// KeyRetention — сколько дней хранить данные произведений, файлы которых не
// найдены (сирот): число строкой, «0» — бессрочно.
const KeyRetention = "userdata.retention"

// Срок хранения сирот в днях.
const (
	DefaultRetentionDays = 30
	MaxRetentionDays     = 36500
)

// RetentionDays — срок хранения сирот в днях (0 — бессрочно); недопустимое
// значение — DefaultRetentionDays.
func RetentionDays(s storage.Settings) int {
	n, err := strconv.Atoi(s.String(KeyRetention, ""))
	if err != nil || n < 0 || n > MaxRetentionDays {
		return DefaultRetentionDays
	}
	return n
}

// Retention — срок хранения сирот (0 — бессрочно).
func Retention(s storage.Settings) time.Duration {
	return time.Duration(RetentionDays(s)) * 24 * time.Hour
}

// SetRetentionDays сохраняет срок хранения сирот в днях (0 — бессрочно).
func SetRetentionDays(s storage.Settings, days int) {
	s.SetString(KeyRetention, strconv.Itoa(days))
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
		log.Printf("catalog unavailable, library kept in memory only: %v", err)
		return nil
	}
	if c.Fresh() {
		log.Printf("catalog %s created, the library will be scanned in full", path)
	}
	return c
}

// openUserData открывает пользовательские данные path; nil — недоступны
// (приложение работает без них). backup — резервная копия повреждённой базы.
func openUserData(path string) (store *userdata.Store, backup string) {
	store, rec, err := userdata.Open(path)
	if err != nil {
		log.Printf("user data unavailable: %v", err)
		return nil, ""
	}
	if rec.Backup != "" {
		log.Printf("user data %s is damaged (%v): moved to %s, a new one is created", path, rec.Cause, rec.Backup)
	}
	return store, rec.Backup
}

// userDataObserver сверяет пользовательские данные папки с каждым успешным
// сканированием и помечает сиротами удалённые через приложение. Вызывается
// в горутинах сканирования и удаления.
type userDataObserver struct {
	store    *userdata.Store
	settings storage.Settings

	mu   sync.Mutex
	root string // ключ папки библиотеки
}

func (o *userDataObserver) key() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.root
}

func (o *userDataObserver) setKey(root string) {
	o.mu.Lock()
	o.root = root
	o.mu.Unlock()
}

// Scanned: найденные файлы — галереи с отпечатком, а также ошибочные и
// занятые (файл есть, но не разобран — отпечаток пустой).
func (o *userDataObserver) Scanned(res library.ScanResult) {
	files := make(map[string]string, len(res.Galleries)+len(res.Errors)+len(res.Busy))
	for _, g := range res.Galleries {
		files[g.Key.ID] = g.Fingerprint
	}
	for _, e := range res.Errors {
		files[e.RelPath] = ""
	}
	for _, rel := range res.Busy {
		if _, ok := files[rel]; !ok {
			files[rel] = ""
		}
	}
	if err := o.store.Reconcile(o.key(), files, time.Now(), Retention(o.settings)); err != nil {
		log.Printf("user data: reconciling: %v", err)
	}
}

func (o *userDataObserver) Deleted(rel string) {
	if err := o.store.MarkOrphan(o.key(), rel, time.Now()); err != nil {
		log.Printf("user data: %s: %v", rel, err)
	}
}

// attachUserData подключает пользовательские данные к библиотеке папки
// с ключом root (store == nil — без пользовательских данных).
func (s *Services) attachUserData(store *userdata.Store, backup, root string) {
	s.UserData, s.UserDataBackup = store, backup
	if store == nil {
		return
	}
	s.userObs = &userDataObserver{store: store, settings: s.Settings, root: root}
	s.Library.SetObserver(s.userObs)
	s.Library.SetOverlay(userDataOverlay{s.userObs})
}

// UserDataKey — ключ текущей папки библиотеки в пользовательских данных
// («» — пользовательских данных нет).
func (s *Services) UserDataKey() string {
	if s.userObs == nil {
		return ""
	}
	return s.userObs.key()
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
	}
	if root := s.Library.Root(); root != "" {
		log.Printf("library folder: %s", root)
	} else {
		log.Printf("library folder is not selected")
	}
	return s
}

// NewForTest — сервисы над готовым источником (для тестов UI).
func NewForTest(src *library.Source, idx search.Index, settings storage.Settings) *Services {
	return &Services{Version: "test", Settings: settings, Index: idx, Library: src,
		Thumbs: thumbs.New(src.OpenPage, 1<<20, 1), Problems: problems.New(settings)}
}

// NewForTestWithUserData — NewForTest с пользовательскими данными store
// (ключ папки «test»; для тестов UI).
func NewForTestWithUserData(src *library.Source, idx search.Index, settings storage.Settings, store *userdata.Store) *Services {
	s := NewForTest(src, idx, settings)
	s.attachUserData(store, "", "test")
	return s
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
		Lang:    i18n.Lang(),
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
			log.Printf("browser: %v", err)
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
		log.Printf("links: %v", err)
	}
}

// Close закрывает каталог и пользовательские данные (при выходе из приложения).
func (s *Services) Close() {
	if s.stopBg != nil {
		s.stopBg()
	}
	s.bg.Wait()
	if s.Catalog != nil {
		if err := s.Catalog.Close(); err != nil {
			log.Printf("catalog: %v", err)
		}
	}
	if s.UserData != nil {
		s.Library.SetObserver(nil)
		if err := s.UserData.Close(); err != nil {
			log.Printf("user data: %v", err)
		}
	}
}

// FolderChosen — папка выбрана (на ПК — всегда).
func (s *Services) FolderChosen() bool { return s.Library.Root() != "" }

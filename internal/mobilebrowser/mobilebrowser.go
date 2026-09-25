// Package mobilebrowser — мост к встроенному браузеру Android (GeckoView,
// Kotlin-часть в android/app/src/main/kotlin/…/browser): открыть браузер,
// очистить данные, получить сообщения о загрузках. На других платформах
// браузера нет (ErrUnsupported). Пакет не зависит от виджетов.
package mobilebrowser

import (
	"encoding/json"
	"errors"
	"sync"
)

// ErrUnsupported — встроенный браузер Android на этой платформе недоступен.
var ErrUnsupported = errors.New("встроенный браузер Android недоступен")

// Engine — поисковик: имя и шаблон адреса (%s — запрос).
type Engine struct {
	Name, Template string
}

// Engines — поисковики на выбор в настройках.
var Engines = []Engine{
	{"Google", "https://www.google.com/search?q=%s"},
	{"DuckDuckGo", "https://duckduckgo.com/?q=%s"},
	{"Bing", "https://www.bing.com/search?q=%s"},
	{"Startpage", "https://www.startpage.com/do/search?q=%s"},
	{"Ecosia", "https://www.ecosia.org/search?q=%s"},
}

// EngineTemplate — шаблон поисковика по имени (неизвестное — первый).
func EngineTemplate(name string) string {
	for _, e := range Engines {
		if e.Name == name {
			return e.Template
		}
	}
	return Engines[0].Template
}

// Положение панели браузера.
const (
	ToolbarBottom = "bottom"
	ToolbarTop    = "top"
)

// Очистки данных.
const (
	ClearCookies = "cookies"
	ClearHistory = "history"
)

// Settings — настройки, которые передаются браузеру при каждом открытии.
type Settings struct {
	Home    string `json:"home"`    // «» — пустая вкладка
	Search  string `json:"search"`  // шаблон поисковика
	Toolbar string `json:"toolbar"` // ToolbarBottom или ToolbarTop
}

func (s Settings) json() string {
	data, _ := json.Marshal(s)
	return string(data)
}

var (
	mu         sync.Mutex
	onDownload func(rel, page string)
)

// SetDownloadHandler задаёт обработчик сообщений о загрузках: rel — путь
// файла относительно папки библиотеки, page — адрес страницы. Вызывается из
// потока загрузки Android — не из UI-потока.
func SetDownloadHandler(f func(rel, page string)) {
	mu.Lock()
	onDownload = f
	mu.Unlock()
}

func downloaded(rel, page string) {
	mu.Lock()
	f := onDownload
	mu.Unlock()
	if f != nil {
		f(rel, page)
	}
}

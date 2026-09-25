package model

import (
	"path"
	"sort"
	"strings"
	"time"
)

// Page — страница произведения. Открытие данных страницы — задача источника.
type Page struct {
	Name string // имя файла внутри архива, например "example/1.jpg"
}

// FileInfo — сведения о файле произведения в папке библиотеки.
type FileInfo struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// Gallery — единая модель произведения независимо от источника.
type Gallery struct {
	Key        Key
	ExternalID int64 // ID на сайте; 0 — неизвестен
	Title      string
	AltTitle   string
	Scanlator  string
	Tags       []Tag
	Uploaded   time.Time
	NumPages   int // из метаданных; реальное число — len(Pages)
	Favorites  int // снимок на момент скачивания
	Pages      []Page
	File       FileInfo
	// SourceURL — ссылка на произведение (http/https): из meta.json, иначе
	// адрес страницы, с которой файл скачан; «» — неизвестна.
	SourceURL string
}

// AddTag добавляет нормализованный тег, если такого ещё нет. Пустые теги пропускаются.
func (g *Gallery) AddTag(t Tag) {
	t = NewTag(t.Type, t.Name)
	if t.Name == "" {
		return
	}
	for _, existing := range g.Tags {
		if existing == t {
			return
		}
	}
	g.Tags = append(g.Tags, t)
}

// HasTag сообщает, есть ли у галереи тег.
func (g *Gallery) HasTag(t Tag) bool {
	t = NewTag(t.Type, t.Name)
	for _, existing := range g.Tags {
		if existing == t {
			return true
		}
	}
	return false
}

// SortPages упорядочивает страницы по имени файла в натуральном порядке.
func (g *Gallery) SortPages() {
	sort.SliceStable(g.Pages, func(i, j int) bool {
		return NaturalLess(g.Pages[i].Name, g.Pages[j].Name)
	})
}

// Cover возвращает обложку — первую страницу. false, если страниц нет.
func (g *Gallery) Cover() (Page, bool) {
	if len(g.Pages) == 0 {
		return Page{}, false
	}
	return g.Pages[0], true
}

// ChooseTitle выбирает основное и альтернативное название:
// английское → японское → имя файла без расширения.
func ChooseTitle(english, japanese, filename string) (title, alt string) {
	english = strings.TrimSpace(english)
	japanese = strings.TrimSpace(japanese)
	switch {
	case english != "":
		return english, japanese
	case japanese != "":
		return japanese, ""
	default:
		base := path.Base(strings.ReplaceAll(filename, `\`, "/"))
		return strings.TrimSuffix(base, path.Ext(base)), ""
	}
}

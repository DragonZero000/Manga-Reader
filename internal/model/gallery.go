package model

import (
	"encoding/json"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// Page — страница произведения. Открытие данных страницы — задача источника.
type Page struct {
	// Name — имя записи внутри архива как есть, например "example/1.jpg";
	// может быть не в UTF-8 (Shift-JIS, CP866 без флага UTF-8 в zip).
	Name string
}

// pageJSON — страница в JSON (кэш сканера в каталоге). encoding/json
// заменил бы байты не в UTF-8 на U+FFFD, и разные имена слились бы в одно,
// поэтому такое имя хранится в NameRaw (base64); имя в UTF-8 — в Name, как
// прежде.
type pageJSON struct {
	Name    string `json:",omitempty"`
	NameRaw []byte `json:",omitempty"`
}

func (p Page) MarshalJSON() ([]byte, error) {
	if utf8.ValidString(p.Name) {
		return json.Marshal(pageJSON{Name: p.Name})
	}
	return json.Marshal(pageJSON{NameRaw: []byte(p.Name)})
}

func (p *Page) UnmarshalJSON(b []byte) error {
	var v pageJSON
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	p.Name = v.Name
	if v.NameRaw != nil {
		p.Name = string(v.NameRaw)
	}
	return nil
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
	// Fingerprint — отпечаток содержимого: SHA-256 (hex) списка страниц из
	// оглавления архива (путь, CRC32, размер). Не зависит от имени файла и
	// meta.json; по нему узнаётся переименованный или перемещённый архив.
	Fingerprint string `json:",omitempty"`
	// Custom и Hidden — наложение пользователя из user.db: свои теги в
	// порядке добавления и скрытые оригинальные. В кэш сканера не пишутся.
	Custom []Tag `json:"-"`
	Hidden []Tag `json:"-"`
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

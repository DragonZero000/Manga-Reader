package library

import (
	"bytes"
	"encoding/xml"
	"net/url"
	"strings"
	"time"

	"mangareader/internal/model"
)

// comicInfo — ComicInfo.xml (Anansi Project) внутри архива. Поля Manga и
// AgeRating не читаются: направление чтения и рейтинг не используются.
type comicInfo struct {
	Title           string `xml:"Title"`
	Series          string `xml:"Series"`
	Number          string `xml:"Number"`
	AlternateSeries string `xml:"AlternateSeries"`
	Tags            string `xml:"Tags"`
	Characters      string `xml:"Characters"`
	LanguageISO     string `xml:"LanguageISO"`
	Format          string `xml:"Format"`
	Writer          string `xml:"Writer"`
	Penciller       string `xml:"Penciller"`
	Publisher       string `xml:"Publisher"`
	Year            int    `xml:"Year"`
	Month           int    `xml:"Month"`
	Day             int    `xml:"Day"`
	PageCount       int    `xml:"PageCount"`
	Web             string `xml:"Web"`
}

// parseComicInfo разбирает ComicInfo.xml в UTF-8 (с BOM и без).
func parseComicInfo(data []byte) (*comicInfo, error) {
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var ci comicInfo
	if err := xml.Unmarshal(data, &ci); err != nil {
		return nil, err
	}
	return &ci, nil
}

// isComicInfoName — имя файла ComicInfo.xml без учёта регистра.
func isComicInfoName(base string) bool { return strings.EqualFold(base, "ComicInfo.xml") }

// languageNames — коды ISO 639-1 → название языка, как в тегах language.
var languageNames = map[string]string{
	"en": "english", "ja": "japanese", "zh": "chinese", "ko": "korean",
	"ru": "russian", "fr": "french", "de": "german", "es": "spanish",
	"it": "italian", "pt": "portuguese", "pl": "polish", "tr": "turkish",
	"uk": "ukrainian", "vi": "vietnamese", "th": "thai", "id": "indonesian",
	"ar": "arabic", "nl": "dutch", "cs": "czech", "hu": "hungarian",
}

// splitList делит значение по запятым: пробелы по краям и пустые значения
// отбрасываются.
func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// title — название: Title, иначе Series с Number.
func (c *comicInfo) title() string {
	if t := strings.TrimSpace(c.Title); t != "" {
		return t
	}
	s := strings.TrimSpace(c.Series)
	if s == "" {
		return ""
	}
	if n := strings.TrimSpace(c.Number); n != "" {
		return s + " " + n
	}
	return s
}

// link — адрес работы: корректный http(s) с непустым путём (главная страница
// сайта не считается ссылкой на работу).
func (c *comicInfo) link() string {
	u := model.WebURL(c.Web)
	if u == "" {
		return ""
	}
	pu, err := url.Parse(u)
	if err != nil || pu.Path == "" || pu.Path == "/" {
		return ""
	}
	return u
}

// date — дата загрузки в UTC; без года не задаётся, без месяца или дня — 1.
func (c *comicInfo) date() time.Time {
	if c.Year <= 0 || c.Year > 9999 {
		return time.Time{}
	}
	m, d := c.Month, c.Day
	if m < 1 || m > 12 {
		m = 1
	}
	if d < 1 || d > 31 {
		d = 1
	}
	return time.Date(c.Year, time.Month(m), d, 0, 0, 0, 0, time.UTC)
}

// applyComicInfo дополняет галерею полями ComicInfo.xml: непустые значения
// не заменяются (сначала применяется meta.json), теги добавляются без
// повторов. titled — название уже получено из метаданных, а не из имени файла.
func applyComicInfo(g *model.Gallery, c *comicInfo, titled bool) {
	alt := strings.TrimSpace(c.AlternateSeries)
	if t := c.title(); t != "" && !titled {
		g.Title, g.AltTitle = t, ""
		if alt != t {
			g.AltTitle = alt
		}
	} else if titled && g.AltTitle == "" && alt != g.Title {
		g.AltTitle = alt
	}
	if g.SourceURL == "" {
		g.SourceURL = c.link()
	}
	if g.Uploaded.IsZero() {
		g.Uploaded = c.date()
	}
	if g.NumPages == 0 && c.PageCount > 0 {
		g.NumPages = c.PageCount
	}
	add := func(typ string, values ...string) {
		for _, v := range values {
			g.AddTag(model.NewTag(typ, v))
		}
	}
	add(model.TagTypeTag, splitList(c.Tags)...)
	add(model.TagTypeCharacter, splitList(c.Characters)...)
	if code := strings.TrimSpace(c.LanguageISO); code != "" {
		name, ok := languageNames[strings.ToLower(code)]
		if !ok {
			name = code
		}
		add(model.TagTypeLanguage, name)
	}
	add(model.TagTypeCategory, strings.TrimSpace(c.Format))
	add(model.TagTypeArtist, splitList(c.Writer)...)
	add(model.TagTypeArtist, splitList(c.Penciller)...)
	add(model.TagTypeGroup, strings.TrimSpace(c.Publisher))
}

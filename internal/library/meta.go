package library

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"mangareader/internal/model"
)

// metaJSON — формат meta.json внутри архива (выгрузка с сайта).
type metaJSON struct {
	ID    flexInt `json:"id"`
	Title struct {
		English  string `json:"english"`
		Japanese string `json:"japanese"`
		Pretty   string `json:"pretty"`
	} `json:"title"`
	UploadDate   flexInt `json:"upload_date"`
	NumPages     flexInt `json:"num_pages"`
	NumFavorites flexInt `json:"num_favorites"`
	Scanlator    string  `json:"scanlator"`
	Tags         []struct {
		Type string `json:"type"`
		Name string `json:"name"`
	} `json:"tags"`

	// Ссылка на произведение: имя поля в выгрузках разное, берётся первое
	// корректное в порядке linkFields.
	URL        json.RawMessage `json:"url"`
	Source     json.RawMessage `json:"source"`
	Link       json.RawMessage `json:"link"`
	SourceURL  json.RawMessage `json:"source_url"`
	GalleryURL json.RawMessage `json:"gallery_url"`
}

// link возвращает первую ссылку http(s) из полей url, source, link,
// source_url, gallery_url. Поля другого типа пропускаются.
func (m *metaJSON) link() string {
	for _, raw := range []json.RawMessage{m.URL, m.Source, m.Link, m.SourceURL, m.GalleryURL} {
		var s string
		if len(raw) == 0 || json.Unmarshal(raw, &s) != nil {
			continue
		}
		if u := model.WebURL(s); u != "" {
			return u
		}
	}
	return ""
}

// flexInt принимает число или строку с числом ("535147").
type flexInt int64

func (f *flexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if bytes.Equal(b, []byte("null")) {
		*f = 0
		return nil
	}
	if len(b) > 0 && b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			*f = 0
			return nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return fmt.Errorf("число в строке: %w", err)
		}
		*f = flexInt(n)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	v, err := n.Int64()
	if err != nil {
		fl, ferr := n.Float64()
		if ferr != nil {
			return err
		}
		v = int64(fl)
	}
	*f = flexInt(v)
	return nil
}

func parseMeta(data []byte) (*metaJSON, error) {
	var m metaJSON
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// applyMeta переносит метаданные в галерею. m может быть nil — тогда
// название берётся из имени файла.
func applyMeta(g *model.Gallery, m *metaJSON, filename string) {
	if m == nil {
		g.Title, g.AltTitle = model.ChooseTitle("", "", filename)
		return
	}
	g.Title, g.AltTitle = model.ChooseTitle(m.Title.English, m.Title.Japanese, filename)
	g.ExternalID = int64(m.ID)
	if m.UploadDate > 0 {
		g.Uploaded = time.Unix(int64(m.UploadDate), 0).UTC()
	}
	g.NumPages = int(m.NumPages)
	g.Favorites = int(m.NumFavorites)
	g.Scanlator = m.Scanlator
	g.SourceURL = m.link()
	for _, t := range m.Tags {
		g.AddTag(model.NewTag(t.Type, t.Name))
	}
}

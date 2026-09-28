package ui

import (
	"path/filepath"
	"testing"
	"time"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/browser"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// visibleTexts собирает подписи объектов дерева: надписи, кнопки, подсказки
// полей, варианты переключателей и списков, заголовки разделов и вкладок.
func visibleTexts(o fyne.CanvasObject, out *[]string) {
	add := func(s ...string) { *out = append(*out, s...) }
	switch w := o.(type) {
	case *fyne.Container:
		for _, c := range w.Objects {
			visibleTexts(c, out)
		}
	case *container.Scroll:
		visibleTexts(w.Content, out)
	case *container.AppTabs:
		for _, it := range w.Items {
			add(it.Text)
			visibleTexts(it.Content, out)
		}
	case *widget.Card:
		add(w.Title, w.Subtitle)
		if w.Content != nil {
			visibleTexts(w.Content, out)
		}
	case *widget.Form:
		for _, it := range w.Items {
			add(it.Text, it.HintText)
			visibleTexts(it.Widget, out)
		}
	case *widget.Label:
		add(w.Text)
	case *widget.Button:
		add(w.Text)
	case *widget.Entry:
		add(w.PlaceHolder)
	case *widget.Check:
		add(w.Text)
	case *widget.RadioGroup:
		add(w.Options...)
	case *widget.Select:
		add(w.Options...)
		add(w.PlaceHolder)
	case fyne.Widget:
		// составные виджеты приложения (карточки, обложка): их части
		for _, c := range test.WidgetRenderer(w).Objects() {
			visibleTexts(c, out)
		}
	}
}

// На English в интерфейсе нет русского текста: вкладки, библиотека, поиск
// со справкой, ошибки, настройки со всеми разделами (включая браузер Windows
// и Android), страница произведения и читалка. Исключения — сам выбор языка
// («Русский», «Язык / Language»).
func TestShellInEnglish(t *testing.T) {
	i18n.Init("en")
	t.Cleanup(func() { i18n.Init("ru") })

	a := test.NewTempApp(t)
	dir := t.TempDir()
	svc := app.NewForTest(library.NewDirSource(dir, nil), search.NopIndex{}, storage.NewMemSettings())
	svc.CanChooseFolder = true
	svc.MobileBrowser = true
	svc.Browser = browser.New(browser.Options{FirefoxDir: filepath.Join(dir, "none"), ProfileDir: dir, DownloadDir: dir})
	s := NewShell(a, svc)
	s.Details.Open(model.Gallery{
		Key: model.LocalKey("a.zip"), Title: "english name", AltTitle: "japanese name", NumPages: 3,
		Pages: []model.Page{{Name: "1.jpg"}}, ExternalID: 5, Favorites: 12345, Scanlator: "group",
		Uploaded: time.Date(2024, 3, 5, 0, 0, 0, 0, time.UTC), SourceURL: "https://example.org/g/5/",
		Tags: []model.Tag{model.NewTag(model.TagTypeArtist, "artist 1"), model.NewTag("circle", "x")},
		File: model.FileInfo{Path: "a.zip", Size: 1 << 20, ModTime: time.Now()},
	})

	var texts []string
	visibleTexts(s.Window.Content(), &texts)
	if len(texts) < 50 {
		t.Fatalf("собрано подписей: %d — обход дерева не дошёл до экранов", len(texts))
	}
	allowed := map[string]bool{"Русский": true, "Язык / Language": true}
	for _, txt := range texts {
		if allowed[txt] {
			continue
		}
		for _, r := range txt {
			if unicode.Is(unicode.Cyrillic, r) {
				t.Errorf("русский текст на English: %q", txt)
				break
			}
		}
	}
}

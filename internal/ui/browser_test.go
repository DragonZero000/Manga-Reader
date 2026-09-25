package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/browser"
	"mangareader/internal/library"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// findButton ищет кнопку с текстом в дереве объектов.
func findButton(o fyne.CanvasObject, text string) *widget.Button {
	switch w := o.(type) {
	case *widget.Button:
		if w.Text == text {
			return w
		}
	case *fyne.Container:
		for _, c := range w.Objects {
			if b := findButton(c, text); b != nil {
				return b
			}
		}
	case *widget.Card:
		return findButton(w.Content, text)
	case *container.Scroll:
		return findButton(w.Content, text)
	}
	return nil
}

func TestBrowserUIWithoutFirefox(t *testing.T) {
	a := test.NewTempApp(t)
	dir := t.TempDir()
	svc := app.NewForTest(library.NewDirSource(dir, nil), search.NopIndex{}, storage.NewMemSettings())
	svc.Browser = browser.New(browser.Options{FirefoxDir: filepath.Join(dir, "нет"), ProfileDir: dir, DownloadDir: dir})
	s := NewShell(a, svc)
	if s.settings.BrowserCard() == nil {
		t.Fatal("на Windows с браузером должен быть раздел «Браузер»")
	}
	btn := findButton(s.library.Content(), "Браузер")
	if btn == nil {
		t.Fatal("нет кнопки «Браузер» на панели библиотеки")
	}
	// Firefox не найден: ошибка с путём (Open в горутине → toast)
	err := svc.Browser.Open("")
	if err == nil || !strings.Contains(err.Error(), "Браузер не найден") || !strings.Contains(err.Error(), "firefox.exe") {
		t.Fatalf("ошибка: %v", err)
	}
}

func TestNoBrowserSection(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), search.NopIndex{}, storage.NewMemSettings())
	s := NewShell(a, svc)
	if s.settings.BrowserCard() != nil || findButton(s.library.Content(), "Браузер") != nil {
		t.Fatal("без встроенного браузера (Android) раздела и кнопки быть не должно")
	}
}

func TestMobileBrowserUI(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), search.NopIndex{}, storage.NewMemSettings())
	svc.MobileBrowser = true
	s := NewShell(a, svc)
	if s.settings.BrowserCard() == nil {
		t.Fatal("на Android должен быть раздел «Браузер»")
	}
	if findButton(s.settings.BrowserCard(), "Очистить историю") == nil {
		t.Error("нет кнопки очистки истории")
	}
	if findButton(s.library.Content(), "Браузер") == nil {
		t.Fatal("нет кнопки «Браузер» на панели библиотеки")
	}
}

package app

import (
	"strings"
	"testing"

	"mangareader/internal/browser"
	"mangareader/internal/storage"
)

func TestBrowserPrefs(t *testing.T) {
	st := storage.NewMemSettings()
	if h, c := BrowserPrefs(st); h != "" || c != nil {
		t.Fatalf("по умолчанию: %q %v", h, c)
	}
	st.SetString(KeyBrowserHome, "https://example.org")
	RequestBrowserClear(st, browser.ClearHistory)
	RequestBrowserClear(st, browser.ClearCookies)
	RequestBrowserClear(st, browser.ClearHistory) // повтор не дублируется
	h, c := BrowserPrefs(st)
	if h != "https://example.org" || strings.Join(c, ",") != "history,cookies" {
		t.Fatalf("настройки: %q %v", h, c)
	}
}

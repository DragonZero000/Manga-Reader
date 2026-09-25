package details

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/model"
)

func TestOpenInBrowserButton(t *testing.T) {
	f := setup(t, map[string][]byte{"example.zip": exampleData(t)})
	g, _ := f.src.Get(model.LocalKey("example.zip"))
	var opened []string
	f.d.SetOpenURL(func(u string) { opened = append(opened, u) })

	// ссылки нет — кнопки нет
	f.d.Open(g)
	if f.d.OpenInBrowserButton() != nil || contains(texts(f.d.body), "Открыть в браузере") {
		t.Fatal("без ссылки кнопки быть не должно")
	}
	f.d.Close()

	g.SourceURL = "https://site.example/g/535147/"
	f.d.Open(g)
	btn := f.d.OpenInBrowserButton()
	if btn == nil || !contains(texts(f.d.body), "Открыть в браузере") {
		t.Fatal("со ссылкой кнопка должна быть")
	}
	test.Tap(btn)
	if len(opened) != 1 || opened[0] != g.SourceURL {
		t.Fatalf("открыто: %v", opened)
	}

	// без обработчика (нет браузера) — кнопки нет даже со ссылкой
	f.d.Close()
	f.d.SetOpenURL(nil)
	f.d.Open(g)
	if f.d.OpenInBrowserButton() != nil {
		t.Fatal("без обработчика кнопки быть не должно")
	}
}

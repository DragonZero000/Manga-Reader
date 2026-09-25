package mobilebrowser

import (
	"strings"
	"testing"
)

func TestEngineTemplate(t *testing.T) {
	if got := EngineTemplate("DuckDuckGo"); got != "https://duckduckgo.com/?q=%s" {
		t.Errorf("DuckDuckGo: %s", got)
	}
	if got := EngineTemplate("нет"); got != Engines[0].Template {
		t.Errorf("неизвестный — первый: %s", got)
	}
	for _, e := range Engines {
		if !strings.Contains(e.Template, "%s") || !strings.HasPrefix(e.Template, "https://") {
			t.Errorf("%s: %s", e.Name, e.Template)
		}
	}
}

func TestSettingsJSON(t *testing.T) {
	got := Settings{Home: "https://a.example", Search: "https://s.example/?q=%s", Toolbar: ToolbarTop}.json()
	if got != `{"home":"https://a.example","search":"https://s.example/?q=%s","toolbar":"top"}` {
		t.Fatal(got)
	}
}

func TestDownloadHandler(t *testing.T) {
	var gotRel, gotPage string
	SetDownloadHandler(func(rel, page string) { gotRel, gotPage = rel, page })
	defer SetDownloadHandler(nil)
	downloaded("g-1.zip", "https://site.example/g/1/")
	if gotRel != "g-1.zip" || gotPage != "https://site.example/g/1/" {
		t.Fatalf("%q %q", gotRel, gotPage)
	}
}

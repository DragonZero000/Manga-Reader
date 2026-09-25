package browser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "перезаписать эталоны testdata/*.golden")

func golden(t *testing.T, name string, got []byte) {
	t.Helper()
	p := filepath.Join("testdata", name+".golden")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("%v (запустите go test -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("%s отличается от эталона:\n%s", name, got)
	}
}

var cfg = Config{
	FirefoxDir:  `C:\App\browser\firefox`,
	ProfileDir:  `C:\App\browser\profile`,
	DownloadDir: `C:\App\manga`,
}

func TestUserJS(t *testing.T) {
	js := string(UserJS(cfg))
	golden(t, "user-default", []byte(js))
	for _, want := range []string{
		`user_pref("browser.download.dir", "C:\\App\\manga");`,
		`user_pref("browser.download.folderList", 2);`,
		`user_pref("xpinstall.signatures.required", false);`,
		`user_pref("app.update.disabledForTesting", true);`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("нет %s", want)
		}
	}
	if strings.Contains(js, "browser.startup.homepage\"") || strings.Contains(js, "sanitize.pending") {
		t.Error("без домашней страницы и очистки эти настройки не пишутся")
	}

	c := cfg
	c.Home = "https://example.org"
	c.Clear = []string{ClearHistory, ClearCookies}
	js = string(UserJS(c))
	golden(t, "user-custom", []byte(js))
	for _, want := range []string{
		`user_pref("browser.startup.homepage", "https://example.org");`,
		`user_pref("browser.startup.page", 1);`,
		`user_pref("privacy.sanitize.pending", "[{\"id\":\"mangareader\",\"itemsToClear\":[\"cache\",\"cookies\",\"downloads\",\"formdata\",\"history\",\"offlineApps\"],\"options\":{}}]");`,
	} {
		if !strings.Contains(js, want) {
			t.Errorf("нет %s", want)
		}
	}
}

func TestToolbarLayout(t *testing.T) {
	var state struct {
		Placements map[string][]string `json:"placements"`
	}
	if err := json.Unmarshal([]byte(uiState), &state); err != nil {
		t.Fatal(err)
	}
	nav := strings.Join(state.Placements["nav-bar"], ",")
	for _, w := range []string{"alltabs-button", "new-tab-button", "bookmarks-menu-button", "downloads-button", extensionWidget} {
		if !strings.Contains(nav, w) {
			t.Errorf("в шапке нет %s: %s", w, nav)
		}
	}
}

func TestUserChrome(t *testing.T) {
	golden(t, "userChrome", UserChrome())
}

func TestExtension(t *testing.T) {
	data, err := Extension(51234, "abc")
	if err != nil {
		t.Fatal(err)
	}
	again, _ := Extension(51234, "abc")
	if !bytes.Equal(data, again) {
		t.Error("архив должен быть детерминированным")
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
	}
	for _, name := range []string{"manifest.json", "bg.js", "icon.svg", "config.js"} {
		if _, ok := files[name]; !ok {
			t.Errorf("в архиве нет %s", name)
		}
	}
	if !strings.Contains(files["config.js"], `const BRIDGE = {port: 51234, token: "abc"};`) {
		t.Errorf("config.js: %s", files["config.js"])
	}
	var m struct {
		BrowserSpecificSettings struct {
			Gecko struct{ ID string } `json:"gecko"`
		} `json:"browser_specific_settings"`
	}
	if err := json.Unmarshal([]byte(files["manifest.json"]), &m); err != nil || m.BrowserSpecificSettings.Gecko.ID != ExtensionID {
		t.Errorf("manifest: %v %+v", err, m)
	}
	if got := filepath.Base(ExtensionPath(`C:\p`)); got != "bridge@mangareader.app.xpi" {
		t.Errorf("имя файла расширения: %s", got)
	}
}

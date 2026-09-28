package browser

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// Интеграционный тест языка браузера с настоящим Firefox (открывает окна).
// Запуск: MANGAREADER_BROWSER_IT=<папка firefox> go test ./internal/browser -run IntegrationLocale -v
// MANGAREADER_BROWSER_PROFILE=<папка> — профиль (например, копия профиля
// прежней сборки для проверки перехода); по умолчанию — новый.
// MANGAREADER_BROWSER_PAUSE=<секунды> — пауза с открытой about:addons на
// каждом языке (для скриншота).
func TestIntegrationLocale(t *testing.T) {
	ffDir := os.Getenv("MANGAREADER_BROWSER_IT")
	if ffDir == "" {
		t.Skip("MANGAREADER_BROWSER_IT не задан")
	}
	ffDir, _ = filepath.Abs(ffDir)
	profile := os.Getenv("MANGAREADER_BROWSER_PROFILE")
	if profile == "" {
		profile = filepath.Join(t.TempDir(), "profile")
	}
	userAddons := addonIDs(t, profile, false)

	var mu sync.Mutex
	var acceptLang string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		acceptLang = r.Header.Get("Accept-Language")
		mu.Unlock()
		fmt.Fprint(w, `<!DOCTYPE html><title>lang</title>`)
	}))
	defer srv.Close()

	lang := "ru"
	b := New(Options{
		FirefoxDir:  ffDir,
		ProfileDir:  profile,
		DownloadDir: t.TempDir(),
		Lang:        func() string { return lang },
	})
	if b.Running() {
		t.Fatal("браузер из этой папки уже запущен")
	}
	// run открывает страницу сервера и возвращает Accept-Language её запроса
	run := func(name string) string {
		t.Helper()
		mu.Lock()
		acceptLang = ""
		mu.Unlock()
		if err := b.Open(srv.URL + "/" + name); err != nil {
			t.Fatal(err)
		}
		var got string
		waitUntil(t, "запрос страницы ("+name+")", func() bool {
			mu.Lock()
			defer mu.Unlock()
			got = acceptLang
			return got != ""
		})
		if pause := os.Getenv("MANGAREADER_BROWSER_PAUSE"); pause != "" {
			var s int
			fmt.Sscan(pause, &s)
			b.Open("about:addons")
			fmt.Printf("SCREENSHOT %s\n", name)
			time.Sleep(time.Duration(s) * time.Second)
		}
		b.Close()
		if b.Running() {
			t.Fatal("после Close браузер работает")
		}
		t.Logf("%s: Accept-Language %q", name, got)
		return got
	}

	if got := run("ru"); !strings.HasPrefix(got, "ru") {
		t.Errorf("русский: Accept-Language %q, ожидался ru…", got)
	}
	if active := addonIDs(t, profile, true); !active["langpack-ru@firefox.mozilla.org"] {
		t.Errorf("языковой пакет не активен: %v", active)
	}

	lang = "en"
	if got := run("en"); !strings.HasPrefix(got, "en-US") {
		t.Errorf("English: Accept-Language %q, ожидался en-US…", got)
	}

	// выбор языка страниц в настройках Firefox хранится в prefs.js
	// (Firefox закрыт) и не затирается user.js
	prefs := filepath.Join(profile, "prefs.js")
	data, err := os.ReadFile(prefs)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, []byte("user_pref(\"intl.accept_languages\", \"ru,en-us,en\");\n")...)
	if err := os.WriteFile(prefs, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := run("en-user"); !strings.HasPrefix(got, "ru") {
		t.Errorf("выбор пользователя: Accept-Language %q, ожидался ru…", got)
	}

	for id := range userAddons {
		if !addonIDs(t, profile, false)[id] {
			t.Errorf("пропало расширение %s", id)
		}
	}
}

// addonIDs — расширения профиля из extensions.json (onlyActive — только
// включённые); нет файла — пусто.
func addonIDs(t *testing.T, profile string, onlyActive bool) map[string]bool {
	t.Helper()
	ids := map[string]bool{}
	data, err := os.ReadFile(filepath.Join(profile, "extensions.json"))
	if err != nil {
		return ids
	}
	var m struct {
		Addons []struct {
			ID       string `json:"id"`
			Active   bool   `json:"active"`
			Location string `json:"location"`
		} `json:"addons"`
	}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, a := range m.Addons {
		if a.Location != "app-builtin" && a.Location != "app-system-defaults" && (!onlyActive || a.Active) {
			ids[a.ID] = true
		}
	}
	return ids
}

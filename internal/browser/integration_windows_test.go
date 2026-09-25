package browser

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// Интеграционный тест с настоящим Firefox: открывает окно на экране.
// Запуск: MANGAREADER_BROWSER_IT=<папка firefox> go test ./internal/browser -run Integration -v
// MANGAREADER_BROWSER_PAUSE=<секунды> — пауза с открытым окном (для скриншота).
func TestIntegration(t *testing.T) {
	ffDir := os.Getenv("MANGAREADER_BROWSER_IT")
	if ffDir == "" {
		t.Skip("MANGAREADER_BROWSER_IT не задан")
	}
	ffDir, _ = filepath.Abs(ffDir)
	tmp := t.TempDir()
	dl := filepath.Join(tmp, "manga")
	os.MkdirAll(dl, 0o755)

	zip, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/g/1/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Gallery one</title></head><body><a href="/g/2/" target="_blank">next</a></body></html>`)
	})
	// файл отдаёт другой источник (как API/CDN сайта) через перенаправление:
	// Referer обрезается до адреса сайта, адрес страницы знает только расширение
	var fileReferer string
	files := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/download" {
			http.Redirect(w, r, "/files/g-2.zip", http.StatusFound)
			return
		}
		fileReferer = r.Header.Get("Referer")
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="g-2.zip"`)
		w.Write(zip)
	}))
	defer files.Close()
	filesURL := strings.Replace(files.URL, "127.0.0.1", "localhost", 1)
	mux.HandleFunc("/g/2/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>Gallery two</title></head><body>
<a id="d" href="%s/api/download?id=2">download</a>
<script>setTimeout(function(){document.getElementById('d').click()},1500)</script></body></html>`, filesURL)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// policies.json прежних версий удаляется: иначе «управляется Вашей организацией»
	legacy := filepath.Join(ffDir, "distribution", "policies.json")
	os.MkdirAll(filepath.Dir(legacy), 0o755)
	os.WriteFile(legacy, []byte(`{"policies":{}}`), 0o644)

	var mu sync.Mutex
	var states []bool
	clear := []string{ClearHistory}
	applied := false
	b := New(Options{
		FirefoxDir:  ffDir,
		ProfileDir:  filepath.Join(tmp, "profile"),
		DownloadDir: dl,
		Prefs: func() (string, []string) {
			mu.Lock()
			defer mu.Unlock()
			return "", clear
		},
		ClearApplied: func() {
			mu.Lock()
			applied, clear = true, nil
			mu.Unlock()
		},
		OnState: func(r bool) { mu.Lock(); states = append(states, r); mu.Unlock() },
	})
	var downloaded []string
	b.SetHandlers(func(rel string) { mu.Lock(); downloaded = append(downloaded, rel); mu.Unlock() }, nil)
	if b.Running() {
		t.Fatal("браузер из этой папки уже запущен")
	}

	if err := b.Open(srv.URL + "/g/1/"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "окно браузера", func() bool { return len(findWindows(b.exe)) == 1 })
	if _, err := os.Stat(legacy); err == nil {
		t.Error("policies.json должен быть удалён")
	}
	js, _ := os.ReadFile(filepath.Join(tmp, "profile", "user.js"))
	mu.Lock()
	if !applied || !strings.Contains(string(js), "privacy.sanitize.pending") {
		t.Error("запрошенная очистка должна попасть в user.js один раз")
	}
	mu.Unlock()

	// второй Open при открытом браузере: адрес уходит в тот же экземпляр
	if err := b.Open(srv.URL + "/g/2/"); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "расширение сообщило о загрузке", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(downloaded) == 1
	})
	if downloaded[0] != "g-2.zip" {
		t.Errorf("OnDownload: %v", downloaded)
	}
	if fileReferer != srv.URL+"/" {
		t.Logf("Referer сервера файла: %q (ожидался обрезанный до адреса сайта)", fileReferer)
	}
	page, _ := os.ReadFile(filepath.Join(dl, "g-2.zip") + ":mangareader.source")
	if strings.TrimSpace(string(page)) != srv.URL+"/g/2/" {
		t.Errorf("адрес страницы: %q, ожидался %q", page, srv.URL+"/g/2/")
	}
	if n := len(findWindows(b.exe)); n != 1 {
		t.Errorf("окон браузера %d, ожидалось 1", n)
	}
	if pause := os.Getenv("MANGAREADER_BROWSER_PAUSE"); pause != "" {
		var s int
		fmt.Sscan(pause, &s)
		for _, h := range findWindows(b.exe) {
			fmt.Printf("HWND=%d\n", h)
		}
		time.Sleep(time.Duration(s) * time.Second)
	}

	b.Close()
	if b.Running() {
		t.Fatal("после Close браузер работает")
	}
	waitUntil(t, "OnState(false)", func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(states) >= 2 && states[0] && !states[len(states)-1]
	})
	if left := ourValues(ffDir); len(left) > 0 {
		t.Errorf("в реестре остались записи: %v", left)
	}
	// повторный запуск: очистка уже не пишется
	b.writeConfig()
	if js, _ := os.ReadFile(filepath.Join(tmp, "profile", "user.js")); strings.Contains(string(js), "privacy.sanitize.pending") {
		t.Error("очистка должна быть однократной")
	}
}

// ourValues — значения в HKCU\Software\Mozilla\Firefox с путём dir.
func ourValues(dir string) []string {
	var out []string
	var walk func(path string)
	walk = func(path string) {
		k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.READ)
		if err != nil {
			return
		}
		names, _ := k.ReadValueNames(-1)
		k.Close()
		for _, n := range names {
			if underDir(n, dir) {
				out = append(out, path+" :: "+n)
			}
		}
		for _, s := range subkeys(registry.CURRENT_USER, path) {
			walk(path + `\` + s)
		}
	}
	walk(mozillaKey)
	return out
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались: %s", what)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

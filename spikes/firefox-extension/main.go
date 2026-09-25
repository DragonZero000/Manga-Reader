// Спайк S5: встроенное расширение Firefox ESR (неподписанное, force_installed
// политикой), настройки через 3rdparty/managed storage, адрес страницы
// загрузки при обрезанном Referer, связь с приложением по 127.0.0.1.
//
// go run . <папка firefox> <рабочая папка>
package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	ff, work := os.Args[1], os.Args[2]
	os.RemoveAll(work)
	dl := filepath.Join(work, "manga")
	prof := filepath.Join(work, "profile")
	os.MkdirAll(dl, 0o755)
	os.MkdirAll(filepath.Join(prof, "chrome"), 0o755)


	// мост: приём сообщений расширения
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(err)
	port := ln.Addr().(*net.TCPAddr).Port
	token := "t0k3n"
	events := make(chan string, 16)
	go http.Serve(ln, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		events <- fmt.Sprintf("%s token=%v %s", r.URL.Path, r.Header.Get("X-MangaReader-Token") == token, body)
	}))

	// сайт: страница на 127.0.0.1:8765, файл на localhost:8766 (другой источник)
	zipData, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	go http.ListenAndServe("127.0.0.1:8765", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html><html><head><title>Gallery 535147</title></head><body>
<a id="d" href="http://localhost:8766/api/download?id=535147">download</a>
<script>setTimeout(function(){document.getElementById('d').click()},1500)</script></body></html>`)
	}))
	go http.ListenAndServe("127.0.0.1:8766", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/download" { // как API сайта: редирект на служебный адрес
			http.Redirect(w, r, "/files/g-535147.zip", http.StatusFound)
			return
		}
		fmt.Printf("сервер файла: Referer=%q\n", r.Header.Get("Referer"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="g-535147.zip"`)
		w.Write(zipData)
	}))

	// без policies.json: иначе Firefox показывает «Ваш браузер управляется Вашей организацией»
	os.Remove(filepath.Join(ff, "distribution", "policies.json"))
	// расширение — в папке extensions профиля; порт и токен зашиты в архив
	os.MkdirAll(filepath.Join(prof, "extensions"), 0o755)
	must(zipDirWith("ext", filepath.Join(prof, "extensions", "bridge@mangareader.app.xpi"),
		fmt.Sprintf("const BRIDGE = {port: %d, token: %q};\n", port, token)))
	dlJS, _ := json.Marshal(dl)
	must(os.WriteFile(filepath.Join(prof, "user.js"), []byte(`user_pref("xpinstall.signatures.required", false);
user_pref("extensions.autoDisableScopes", 0);
user_pref("extensions.enabledScopes", 15);
user_pref("toolkit.legacyUserProfileCustomizations.stylesheets", true);
user_pref("browser.preonboarding.enabled", false);
user_pref("browser.aboutwelcome.enabled", false);
user_pref("startup.homepage_welcome_url", "");
user_pref("browser.startup.homepage_override.mstone", "ignore");
user_pref("app.update.disabledForTesting", true);
user_pref("app.update.auto", false);
user_pref("browser.download.folderList", 2);
user_pref("browser.download.dir", `+string(dlJS)+`);
user_pref("browser.download.useDownloadDir", true);
user_pref("browser.download.always_ask_before_handling_new_types", false);
user_pref("datareporting.policy.dataSubmissionEnabled", false);
user_pref("toolkit.telemetry.enabled", false);
user_pref("app.shield.optoutstudies.enabled", false);
`), 0o644))

	cmd := exec.Command(filepath.Join(ff, "firefox.exe"), "-profile", prof, "http://127.0.0.1:8765/g/535147/")
	cmd.Env = append(os.Environ(), "MOZ_CRASHREPORTER_DISABLE=1")
	must(cmd.Start())
	fmt.Println("мост на порту", port)
	timeout := time.After(90 * time.Second)
	for {
		select {
		case e := <-events:
			fmt.Println("событие:", e)
		case <-timeout:
			zone, _ := os.ReadFile(filepath.Join(dl, "g-535147.zip") + ":Zone.Identifier")
			fmt.Printf("Zone.Identifier:\n%s\n", zone)
			return
		}
	}
}

func fileURL(p string) string { return "file:///" + filepath.ToSlash(p) }

func zipDirWith(dir, out, config string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		w, _ := zw.Create(e.Name())
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		w.Write(data)
	}
	w, _ := zw.Create("config.js")
	w.Write([]byte(config))
	return zw.Close()
}

func zipDir(dir, out string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		w, _ := zw.Create(e.Name())
		data, _ := os.ReadFile(filepath.Join(dir, e.Name()))
		w.Write(data)
	}
	return zw.Close()
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

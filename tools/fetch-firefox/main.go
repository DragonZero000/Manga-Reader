// Команда fetch-firefox готовит портативный Firefox ESR для встроенного
// браузера: скачивает закреплённую версию установщика (en-US) и языковые
// пакеты с archive.mozilla.org, сверяет SHA-256, распаковывает установщик без
// установки (/ExtractDir) в папку назначения и кладёт пакеты в её папку
// langpacks. Работает только на Windows.
//
//	go run ./tools/fetch-firefox -dst browser/firefox
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"mangareader/internal/browser"
)

// Закреплённая версия: обновляется вручную вместе со всеми контрольными
// суммами из https://archive.mozilla.org/pub/firefox/releases/<версия>/SHA256SUMS
// (строки win64/en-US/Firefox Setup <версия>.exe и win64/xpi/<язык>.xpi).
// Языковые пакеты работают только с той же версией ESR: их суммы обновляются
// вместе с версией, иначе Firefox отключит пакет и покажет English.
// При обновлении версии обновите THIRD_PARTY_NOTICES.md (версия и ссылка на исходники).
const (
	version = "153.3.0esr"
	locale  = browser.BaseLocale
	sha     = "be79723a8fb9f976c7434a5f7dfee0a868a04a7148bf169fb3dab1fef932ad59"
)

// langpacks — языковые пакеты для всех языков приложения, кроме English
// (код языка приложения совпадает с именем пакета win64/xpi/<язык>.xpi).
var langpacks = []struct{ lang, sha string }{
	{"ru", "8e99dc049f170b55b8e6f830ea70c55173b1b49845459048df834b5a5d60d49d"},
}

const baseURL = "https://archive.mozilla.org/pub/firefox/releases/" + version + "/win64/"

func main() {
	dst := flag.String("dst", filepath.Join("browser", "firefox"), "папка для firefox.exe")
	cache := flag.String("cache", filepath.Join("browser", ".cache"), "папка для скачанных файлов")
	flag.Parse()
	if runtime.GOOS != "windows" {
		log.Fatal("fetch-firefox работает только на Windows")
	}
	if err := run(*dst, *cache); err != nil {
		log.Fatal(err)
	}
}

// stamp — содержимое .version: версия, язык сборки и языковые пакеты; при
// любом их изменении папка собирается заново.
func stamp() string {
	s := []string{version, locale}
	for _, lp := range langpacks {
		s = append(s, lp.lang+":"+lp.sha)
	}
	return strings.Join(s, " ")
}

func run(dst, cache string) error {
	if _, err := os.Stat(filepath.Join(dst, "firefox.exe")); err == nil {
		if v, _ := os.ReadFile(filepath.Join(dst, ".version")); string(v) == stamp() {
			log.Printf("Firefox %s уже в %s", version, dst)
			return nil
		}
	}
	setup, err := download(cache, fmt.Sprintf("Firefox Setup %s.exe", version),
		baseURL+locale+"/Firefox%20Setup%20"+version+".exe", sha)
	if err != nil {
		return err
	}
	packs := make(map[string]string, len(langpacks)) // язык → проверенный файл
	for _, lp := range langpacks {
		p, err := download(cache, fmt.Sprintf("langpack-%s-%s.xpi", lp.lang, version), baseURL+"xpi/"+lp.lang+".xpi", lp.sha)
		if err != nil {
			return fmt.Errorf("языковой пакет %s: %w", lp.lang, err)
		}
		packs[lp.lang] = p
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dst), ".firefox-extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)

	abs, _ := filepath.Abs(tmp)
	log.Printf("распаковка в %s", abs)
	cmd := exec.Command(setup, "/ExtractDir="+abs)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("распаковка: %v %s", err, out)
	}
	core := filepath.Join(tmp, "core")
	if _, err := os.Stat(filepath.Join(core, "firefox.exe")); err != nil {
		return fmt.Errorf("после распаковки нет core/firefox.exe: %w", err)
	}
	if err := copyLangpacks(packs, browser.LangpacksDir(core)); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(core, ".version"), []byte(stamp()), 0o644); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.Rename(core, dst); err != nil {
		return err
	}
	log.Printf("Firefox %s (%s, пакеты: %d) готов: %s", version, locale, len(langpacks), dst)
	return nil
}

// copyLangpacks проверяет manifest.json каждого пакета (id — имя файла в
// профиле, язык — код пакета) и копирует его в dir как <язык>.xpi.
func copyLangpacks(packs map[string]string, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for lang, p := range packs {
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		lp, err := browser.ReadLangpack(data)
		if err != nil {
			return fmt.Errorf("языковой пакет %s: %w", lang, err)
		}
		if lp.Locale != lang {
			return fmt.Errorf("языковой пакет %s: в manifest.json язык %q", lang, lp.Locale)
		}
		log.Printf("языковой пакет %s: %s", lang, lp.ID)
		if err := os.WriteFile(filepath.Join(dir, lang+".xpi"), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// download возвращает путь к проверенному файлу name в cache (скачивает url,
// если в кэше нет файла с верной контрольной суммой want).
func download(cache, name, url, want string) (string, error) {
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(cache, name)
	if got := sum(p); got == want {
		return p, nil
	}
	log.Printf("скачивание %s", url)
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("скачивание: %s", resp.Status)
	}
	tmp := p + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if got := sum(tmp); got != want {
		os.Remove(tmp)
		return "", fmt.Errorf("контрольная сумма %s не совпала: %s, ожидалась %s", name, got, want)
	}
	return p, os.Rename(tmp, p)
}

// sum — SHA-256 файла в hex ("" — файла нет или он не читается).
func sum(p string) string {
	f, err := os.Open(p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Команда fetch-firefox готовит портативный Firefox ESR для встроенного
// браузера: скачивает закреплённую версию установщика с archive.mozilla.org,
// сверяет SHA-256 и распаковывает его без установки (/ExtractDir) в папку
// назначения. Работает только на Windows.
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
)

// Закреплённая версия: обновляется вручную вместе с контрольной суммой из
// https://archive.mozilla.org/pub/firefox/releases/<версия>/SHA256SUMS
// (строка win64/ru/Firefox Setup <версия>.exe).
// При обновлении версии обновите THIRD_PARTY_NOTICES.md (версия и ссылка на исходники).
const (
	version = "153.3.0esr"
	locale  = "ru"
	sha     = "4b9396459525e19ff5280a1f4f13223d6d08c0e27efe05527dada1572e734b8c"
)

func main() {
	dst := flag.String("dst", filepath.Join("browser", "firefox"), "папка для firefox.exe")
	cache := flag.String("cache", filepath.Join("browser", ".cache"), "папка для скачанного установщика")
	flag.Parse()
	if runtime.GOOS != "windows" {
		log.Fatal("fetch-firefox работает только на Windows")
	}
	if err := run(*dst, *cache); err != nil {
		log.Fatal(err)
	}
}

func run(dst, cache string) error {
	if _, err := os.Stat(filepath.Join(dst, "firefox.exe")); err == nil {
		if v, _ := os.ReadFile(filepath.Join(dst, ".version")); string(v) == version {
			log.Printf("Firefox %s уже в %s", version, dst)
			return nil
		}
	}
	setup, err := download(cache)
	if err != nil {
		return err
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
	if err := os.WriteFile(filepath.Join(core, ".version"), []byte(version), 0o644); err != nil {
		return err
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.Rename(core, dst); err != nil {
		return err
	}
	log.Printf("Firefox %s готов: %s", version, dst)
	return nil
}

// download возвращает путь к проверенному установщику (скачивает, если в
// кэше нет файла с верной контрольной суммой).
func download(cache string) (string, error) {
	if err := os.MkdirAll(cache, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(cache, fmt.Sprintf("Firefox Setup %s.exe", version))
	if ok, _ := verify(p); ok {
		return p, nil
	}
	url := fmt.Sprintf("https://archive.mozilla.org/pub/firefox/releases/%[1]s/win64/%[2]s/Firefox%%20Setup%%20%[1]s.exe", version, locale)
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
	if ok, got := verify(tmp); !ok {
		os.Remove(tmp)
		return "", fmt.Errorf("контрольная сумма не совпала: %s, ожидалась %s", got, sha)
	}
	return p, os.Rename(tmp, p)
}

func verify(p string) (bool, string) {
	f, err := os.Open(p)
	if err != nil {
		return false, ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return false, ""
	}
	got := hex.EncodeToString(h.Sum(nil))
	return got == sha, got
}

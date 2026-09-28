package browser

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
)

// BaseLocale — язык базовой сборки Firefox: интерфейс для English и для
// языков без языкового пакета; строки, которых нет в пакете, тоже на нём.
const BaseLocale = "en-US"

// LangpacksDir — папка языковых пакетов рядом с firefox.exe
// (<язык приложения>.xpi, кладёт tools/fetch-firefox).
func LangpacksDir(firefoxDir string) string {
	return filepath.Join(firefoxDir, "langpacks")
}

// Langpack — сведения из manifest.json языкового пакета Firefox.
type Langpack struct {
	ID     string // id расширения: langpack-ru@firefox.mozilla.org
	Locale string // язык интерфейса Firefox (langpack_id): ru
}

// ReadLangpack читает id и язык из manifest.json языкового пакета.
func ReadLangpack(data []byte) (Langpack, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Langpack{}, err
	}
	f, err := zr.Open("manifest.json")
	if err != nil {
		return Langpack{}, err
	}
	defer f.Close()
	var m struct {
		LangpackID              string `json:"langpack_id"`
		BrowserSpecificSettings struct {
			Gecko struct{ ID string } `json:"gecko"`
		} `json:"browser_specific_settings"`
	}
	if err := json.NewDecoder(f).Decode(&m); err != nil {
		return Langpack{}, fmt.Errorf("manifest.json: %w", err)
	}
	lp := Langpack{ID: m.BrowserSpecificSettings.Gecko.ID, Locale: m.LangpackID}
	if lp.ID == "" || lp.Locale == "" {
		return Langpack{}, fmt.Errorf("manifest.json: not a language pack (id %q, langpack_id %q)", lp.ID, lp.Locale)
	}
	return lp, nil
}

// InstallLangpack копирует языковой пакет языка приложения lang в папку
// extensions профиля (имя файла — id пакета, как у встроенного расширения)
// и возвращает язык интерфейса Firefox для intl.locale.requested. Для
// English и при отсутствии или ошибке пакета — BaseLocale. Пакеты других
// языков в профиле остаются: переключение языка не требует переустановки.
func InstallLangpack(firefoxDir, profileDir, lang string) string {
	if lang == "" || lang == "en" {
		return BaseLocale
	}
	data, err := os.ReadFile(filepath.Join(LangpacksDir(firefoxDir), lang+".xpi"))
	if err != nil {
		log.Printf("browser: no language pack for %q, using %s: %v", lang, BaseLocale, err)
		return BaseLocale
	}
	lp, err := ReadLangpack(data)
	if err != nil {
		log.Printf("browser: language pack %q: %v", lang, err)
		return BaseLocale
	}
	dst := filepath.Join(profileDir, "extensions", lp.ID+".xpi")
	// без перезаписи того же файла: Firefox переустанавливает изменённые
	if old, err := os.Open(dst); err == nil {
		same := sameContent(old, data)
		old.Close()
		if same {
			return lp.Locale
		}
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err == nil {
		err = os.WriteFile(dst, data, 0o644)
	}
	if err != nil {
		log.Printf("browser: installing language pack %q: %v", lang, err)
		return BaseLocale
	}
	return lp.Locale
}

func sameContent(r io.Reader, data []byte) bool {
	got, err := io.ReadAll(io.LimitReader(r, int64(len(data))+1))
	return err == nil && bytes.Equal(got, data)
}

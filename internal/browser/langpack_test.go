package browser

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// fakeLangpack — xpi с manifest.json языкового пакета.
func fakeLangpack(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(manifest))
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const ruManifest = `{"langpack_id": "ru", "browser_specific_settings": {"gecko": {"id": "langpack-ru@firefox.mozilla.org"}}}`

func TestReadLangpack(t *testing.T) {
	lp, err := ReadLangpack(fakeLangpack(t, ruManifest))
	if err != nil || lp != (Langpack{ID: "langpack-ru@firefox.mozilla.org", Locale: "ru"}) {
		t.Errorf("%+v %v", lp, err)
	}
	if _, err := ReadLangpack(fakeLangpack(t, `{"browser_specific_settings": {"gecko": {"id": "x@y"}}}`)); err == nil {
		t.Error("расширение без langpack_id — не языковой пакет")
	}
	if _, err := ReadLangpack([]byte("not a zip")); err == nil {
		t.Error("ожидалась ошибка для не-zip")
	}
}

func TestInstallLangpack(t *testing.T) {
	ff, profile := t.TempDir(), t.TempDir()
	dst := filepath.Join(profile, "extensions", "langpack-ru@firefox.mozilla.org.xpi")

	if got := InstallLangpack(ff, profile, "ru"); got != BaseLocale {
		t.Errorf("без пакета: %s, ожидался %s", got, BaseLocale)
	}

	data := fakeLangpack(t, ruManifest)
	os.MkdirAll(LangpacksDir(ff), 0o755)
	if err := os.WriteFile(filepath.Join(LangpacksDir(ff), "ru.xpi"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"en", ""} {
		if got := InstallLangpack(ff, profile, lang); got != BaseLocale {
			t.Errorf("%q: %s", lang, got)
		}
	}
	if _, err := os.Stat(dst); err == nil {
		t.Error("для English пакет не устанавливается")
	}

	if got := InstallLangpack(ff, profile, "ru"); got != "ru" {
		t.Errorf("ru: %s", got)
	}
	if b, err := os.ReadFile(dst); err != nil || !bytes.Equal(b, data) {
		t.Fatalf("пакет в профиле: %v", err)
	}
	// тот же пакет не перезаписывается (Firefox переустановил бы изменённый)
	old := time.Now().Add(-time.Hour).Truncate(time.Second)
	os.Chtimes(dst, old, old)
	InstallLangpack(ff, profile, "ru")
	if st, _ := os.Stat(dst); !st.ModTime().Equal(old) {
		t.Error("неизменённый пакет перезаписан")
	}

	// после переключения на English пакет остаётся в профиле
	InstallLangpack(ff, profile, "en")
	if _, err := os.Stat(dst); err != nil {
		t.Error("пакет удалён при переключении языка")
	}
}

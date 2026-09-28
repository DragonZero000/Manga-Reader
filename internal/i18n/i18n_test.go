package i18n

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"
	"time"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// use задаёт язык на время теста.
func use(t *testing.T, lang string) {
	t.Helper()
	old := Lang()
	Init(lang)
	t.Cleanup(func() { Init(old) })
}

func TestFallbackBeforeInit(t *testing.T) {
	if Lang() != Fallback {
		t.Fatalf("до Init язык %q, ожидался %q", Lang(), Fallback)
	}
	if got := T("tab.library"); got != "Library" {
		t.Errorf("T до Init = %q", got)
	}
}

func TestInitUnknownLanguage(t *testing.T) {
	use(t, "de")
	if Lang() != Fallback {
		t.Errorf("неизвестный язык: %q", Lang())
	}
}

func TestTranslate(t *testing.T) {
	use(t, "ru")
	if got := T("tab.library"); got != "Библиотека" {
		t.Errorf("T = %q", got)
	}
	if got := T("shell.open_link_failed", "Error", "x"); got != "Не удалось открыть ссылку: x" {
		t.Errorf("подстановка: %q", got)
	}
	if got := TIn("en", "tab.library"); got != "Library" {
		t.Errorf("TIn = %q", got)
	}
}

func TestPlural(t *testing.T) {
	use(t, "ru")
	for n, want := range map[int]string{
		1: "1 галерея", 3: "3 галереи", 5: "5 галерей", 11: "11 галерей", 21: "21 галерея", 22: "22 галереи", 0: "0 галерей",
	} {
		if got := N("library.count", n); got != want {
			t.Errorf("ru N(%d) = %q, ожидалось %q", n, got, want)
		}
	}
	Init("en")
	for n, want := range map[int]string{1: "1 gallery", 5: "5 galleries", 0: "0 galleries"} {
		if got := N("library.count", n); got != want {
			t.Errorf("en N(%d) = %q, ожидалось %q", n, got, want)
		}
	}
}

// Ключа нет в выбранном языке — английский текст; нет нигде — сам ключ.
func TestMissingKey(t *testing.T) {
	if err := bundle.AddMessages(language.English, &goi18n.Message{ID: "test.only_en", Other: "only english"}); err != nil {
		t.Fatal(err)
	}
	use(t, "ru")
	if got := T("test.only_en"); got != "only english" {
		t.Errorf("нет в ru: %q", got)
	}
	if got := T("test.nowhere"); got != "test.nowhere" {
		t.Errorf("нет нигде: %q", got)
	}
}

func TestResolve(t *testing.T) {
	cases := []struct{ setting, system, want string }{
		{Auto, "ru-RU", "ru"},
		{"", "ru_RU", "ru"},
		{Auto, "en-GB", "en"},
		{Auto, "de-DE", "en"},
		{Auto, "", "en"},
		{"en", "ru-RU", "en"},
		{"ru", "de-DE", "ru"},
		{"xx", "ru-RU", "en"},
	}
	for _, c := range cases {
		if got := Resolve(c.setting, c.system); got != c.want {
			t.Errorf("Resolve(%q, %q) = %q, ожидалось %q", c.setting, c.system, got, c.want)
		}
	}
}

func TestAvailable(t *testing.T) {
	got := map[string]string{}
	for _, l := range Available() {
		got[l.Code] = l.Name
	}
	if got["en"] != "English" || got["ru"] != "Русский" {
		t.Errorf("языки: %v", got)
	}
}

// Во всех файлах переводов одинаковый набор ключей, а у форм
// множественного числа есть форма other.
func TestLocalesComplete(t *testing.T) {
	keys := map[string]map[string]bool{}
	for _, c := range codes {
		data, err := localesFS.ReadFile("locales/" + c + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", c, err)
		}
		keys[c] = map[string]bool{}
		for k, v := range m {
			keys[c][k] = true
			if forms, ok := v.(map[string]any); ok {
				if _, ok := forms["other"]; !ok {
					t.Errorf("%s: %s: нет формы other", c, k)
				}
			}
		}
	}
	for _, a := range codes {
		for _, b := range codes {
			var missing []string
			for k := range keys[a] {
				if !keys[b][k] {
					missing = append(missing, k)
				}
			}
			sort.Strings(missing)
			if len(missing) > 0 {
				t.Errorf("в %s.json нет ключей из %s.json: %s", b, a, strings.Join(missing, ", "))
			}
		}
	}
}

func TestFormats(t *testing.T) {
	up := time.Date(2024, 3, 5, 23, 30, 0, 0, time.UTC)
	mod := time.Date(2024, 3, 5, 14, 7, 0, 0, time.Local)
	cases := []struct {
		lang                           string
		date, dateTime, size, big, num string
	}{
		{"ru", "05.03.2024", "05.03.2024 14:07", "1,5 МБ", "512 Б", "12 345"},
		{"en", "Mar 5, 2024", "Mar 5, 2024 14:07", "1.5 MB", "512 B", "12,345"},
	}
	for _, c := range cases {
		use(t, c.lang)
		if got := Date(up); got != c.date {
			t.Errorf("%s Date = %q", c.lang, got)
		}
		if got := DateTime(mod); got != c.dateTime {
			t.Errorf("%s DateTime = %q", c.lang, got)
		}
		if got := Size(1572864); got != c.size {
			t.Errorf("%s Size = %q", c.lang, got)
		}
		if got := Size(512); got != c.big {
			t.Errorf("%s Size(512) = %q", c.lang, got)
		}
		if got := Int(12345); got != c.num {
			t.Errorf("%s Int = %q", c.lang, got)
		}
	}
}

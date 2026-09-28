package archtest

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// androidStrings — ключи строк (string и plurals) файла ресурсов Android.
func androidStrings(t *testing.T, path string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var res struct {
		Strings []struct {
			Name string `xml:"name,attr"`
		} `xml:"string"`
		Plurals []struct {
			Name string `xml:"name,attr"`
		} `xml:"plurals"`
	}
	if err := xml.Unmarshal(data, &res); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	keys := map[string]bool{}
	for _, s := range res.Strings {
		keys[s.Name] = true
	}
	for _, p := range res.Plurals {
		keys[p.Name] = true
	}
	return keys
}

// Строки браузера Android переведены на все языки: в каждом values-<язык>
// тот же набор ключей, что в values (English).
func TestAndroidStringsComplete(t *testing.T) {
	res := filepath.Join("..", "..", "android", "app", "src", "main", "res")
	base := androidStrings(t, filepath.Join(res, "values", "strings.xml"))
	dirs, err := filepath.Glob(filepath.Join(res, "values-*", "strings.xml"))
	if err != nil || len(dirs) == 0 {
		t.Fatalf("нет переводов values-*: %v", err)
	}
	for _, p := range dirs {
		keys := androidStrings(t, p)
		var diff []string
		for k := range base {
			if !keys[k] {
				diff = append(diff, "нет "+k)
			}
		}
		for k := range keys {
			if !base[k] {
				diff = append(diff, "лишний "+k)
			}
		}
		sort.Strings(diff)
		if len(diff) > 0 {
			t.Errorf("%s: %s", p, strings.Join(diff, ", "))
		}
	}
}

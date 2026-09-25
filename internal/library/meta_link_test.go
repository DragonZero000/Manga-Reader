package library

import (
	"testing"
)

func TestMetaLink(t *testing.T) {
	tests := []struct {
		name, meta, want string
	}{
		{"url", `{"url": "https://site.example/g/535147/"}`, "https://site.example/g/535147/"},
		{"первое некорректно", `{"url": "не ссылка", "source": "https://site.example/g/1/"}`, "https://site.example/g/1/"},
		{"другой тип", `{"url": 123, "link": "http://a.example/x"}`, "http://a.example/x"},
		{"не http", `{"url": "ftp://a.example/x", "gallery_url": "javascript:alert(1)"}`, ""},
		{"source_url", `{"source_url": "  https://b.example/y  "}`, "https://b.example/y"},
		{"нет полей", `{"id": 1}`, ""},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		p := writeZip(t, dir, "a.zip", file{"1.png", pngBytes(t, 2, 2)}, file{"meta.json", []byte(tt.meta)})
		g, warnings, err := readArchivePath(t, p, "a.zip", stat(t, p))
		if err != nil || len(warnings) != 0 {
			t.Fatalf("%s: err=%v warnings=%v", tt.name, err, warnings)
		}
		if g.SourceURL != tt.want {
			t.Errorf("%s: %q, ожидалось %q", tt.name, g.SourceURL, tt.want)
		}
	}
}

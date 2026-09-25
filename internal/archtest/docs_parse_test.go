package archtest

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestFindLinks(t *testing.T) {
	cases := map[string][]string{
		"см. [сборку](building.md) и ![экран](../images/a.png)": {"building.md", "../images/a.png"},
		`<img src="docs/images/b.png" width="400">`:             {"docs/images/b.png"},
		`[x](a.md "заголовок")`:                                 {"a.md"},
		"`[не ссылка](nope.md)` и [ссылка](yes.md)":             {"yes.md"},
		"[сайт](https://example.com/x.md)":                      {"https://example.com/x.md"},
		"без ссылок":                                            nil,
	}
	for in, want := range cases {
		if got := findLinks(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %q, ожидалось %q", in, got, want)
		}
	}
}

func TestLocalPath(t *testing.T) {
	cases := []struct {
		in, want string
		ok       bool
	}{
		{"building.md", "building.md", true},
		{"building.md#ключ", "building.md", true},
		{"my%20file.md", "my file.md", true},
		{"/LICENSE", "/LICENSE", true},
		{"#раздел", "", false},
		{"https://github.com/x", "", false},
		{"mailto:a@b.c", "", false},
		{"//cdn.example.com/x.png", "", false},
	}
	for _, c := range cases {
		got, ok := localPath(c.in)
		if got != c.want || ok != c.ok {
			t.Errorf("%q: %q %v, ожидалось %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestLinksOutsideCode(t *testing.T) {
	lines := []string{
		"[a](a.md)",
		"```sh",
		"[b](b.md)",
		"```",
		"    [c](c.md)",
		"~~~",
		"[d](d.md)",
		"~~~",
		"[e](e.md)",
	}
	got := linksOutsideCode(lines)
	want := map[int][]string{1: {"a.md"}, 9: {"e.md"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%v, ожидалось %v", got, want)
	}
}

func TestBrokenLinks(t *testing.T) {
	root := t.TempDir()
	docs := filepath.Join(root, "docs", "ru")
	os.MkdirAll(docs, 0o755)
	os.WriteFile(filepath.Join(docs, "building.md"), nil, 0o644)
	os.WriteFile(filepath.Join(root, "LICENSE"), nil, 0o644)
	lines := []string{
		"[есть](building.md#раздел)",
		"[нет](architektura.md)",
		"[корень](/LICENSE) и [папка](../ru/)",
		"[внешняя](https://example.com/nope.md)",
	}
	got := brokenLinks(lines, docs, root)
	want := []string{"2: ссылка на несуществующий architektura.md"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%q, ожидалось %q", got, want)
	}
}

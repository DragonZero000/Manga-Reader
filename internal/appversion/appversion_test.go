package appversion

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCode(t *testing.T) {
	for s, want := range map[string]int{"0.1.0": 100, "0.2.0": 200, "1.2.3": 10203, "2099.99.99": 20999999} {
		v, err := Parse(s)
		if err != nil {
			t.Fatalf("%s: %v", s, err)
		}
		if v.Code() != want || v.String() != s {
			t.Errorf("%s: код %d, строка %q; ожидался код %d", s, v.Code(), v.String(), want)
		}
	}
}

func TestCodeOrder(t *testing.T) {
	a, _ := Parse("0.9.99")
	b, _ := Parse("0.10.0")
	if a.Code() != 999 || b.Code() != 1000 {
		t.Fatalf("0.9.99 → %d, 0.10.0 → %d", a.Code(), b.Code())
	}
}

func TestParseInvalid(t *testing.T) {
	for _, s := range []string{"", "0.2.0-beta", "1.100.0", "1.2.100", "2100.0.0", "01.2.3", "1.02.3", "1.2", "1.2.3.4", "v1.2.3", " 1.2.3"} {
		if _, err := Parse(s); err == nil {
			t.Errorf("%q: ожидалась ошибка", s)
		}
	}
}

func TestRead(t *testing.T) {
	p := filepath.Join(t.TempDir(), "FyneApp.toml")
	os.WriteFile(p, []byte("Website = \"x\"\n\n[Details]\n  Name = \"MangaReader\"\n  Version = \"0.2.1\"\n"), 0o644)
	v, err := Read(p)
	if err != nil || v != (Version{0, 2, 1}) {
		t.Fatalf("%v %v", v, err)
	}
	os.WriteFile(p, []byte("[Details]\n  Name = \"MangaReader\"\n"), 0o644)
	if _, err := Read(p); err == nil {
		t.Error("без Version — ошибка")
	}
	os.WriteFile(p, []byte("[Details]\n  Version = \"0.2.0-beta\"\n"), 0o644)
	if _, err := Read(p); err == nil {
		t.Error("неверный формат — ошибка")
	}
	if _, err := Read(filepath.Join(t.TempDir(), "нет.toml")); err == nil {
		t.Error("нет файла — ошибка")
	}
}

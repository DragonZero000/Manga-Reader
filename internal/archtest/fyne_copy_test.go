package archtest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// fyneModule — модуль Fyne, заменённый копией в third_party/fyne.
const fyneModule = "fyne.io/fyne/v2"

// goModFyne — версия Fyne в require и путь его replace из go.mod.
func goModFyne(goMod string) (version, replace string) {
	for _, line := range strings.Split(goMod, "\n") {
		f := strings.Fields(strings.TrimPrefix(strings.TrimSpace(line), "require "))
		switch {
		case len(f) >= 2 && f[0] == fyneModule && strings.HasPrefix(f[1], "v"):
			version = f[1]
		case len(f) >= 4 && f[0] == "replace" && f[1] == fyneModule && f[2] == "=>":
			replace = f[3]
		}
	}
	return version, replace
}

var patchesBase = regexp.MustCompile("(?m)^Основа: `" + regexp.QuoteMeta(fyneModule) + "` (v\\S+)")

// Fyne подключён из копии third_party/fyne той же версии, что в require:
// иначе после go get можно молча собрать исходный Fyne без правок.
func TestFyneCopy(t *testing.T) {
	root := repoRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	version, replace := goModFyne(string(data))
	if replace != "./third_party/fyne" {
		t.Fatalf("go.mod: replace %s => %q, нужно ./third_party/fyne", fyneModule, replace)
	}
	patches, err := os.ReadFile(filepath.Join(root, "third_party", "fyne", "PATCHES.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := patchesBase.FindSubmatch(patches)
	if m == nil {
		t.Fatal("third_party/fyne/PATCHES.md: нет строки «Основа: `fyne.io/fyne/v2` vX.Y.Z»")
	}
	if string(m[1]) != version {
		t.Errorf("go.mod: require %s %s, а копия в third_party/fyne — %s (PATCHES.md)", fyneModule, version, m[1])
	}
}

func TestGoModFyne(t *testing.T) {
	mod := "module m\n\nrequire (\n\tfyne.io/fyne/v2 v2.8.0\n\tx v1\n)\n\nreplace fyne.io/fyne/v2 => ./third_party/fyne\n"
	if v, r := goModFyne(mod); v != "v2.8.0" || r != "./third_party/fyne" {
		t.Errorf("%q %q", v, r)
	}
	if v, r := goModFyne("module m\nrequire fyne.io/fyne/v2 v2.9.0\n"); v != "v2.9.0" || r != "" {
		t.Errorf("%q %q", v, r)
	}
}

// К темпу кадров драйвера (driver/pacing) приложение обращается только через
// internal/ui/frameclock: откат на исходный Fyne — замена одного файла.
func TestPacingOnlyViaFrameclock(t *testing.T) {
	root := repoRoot(t)
	const pacingPkg = `"fyne.io/fyne/v2/driver/pacing"`
	for _, dir := range []string{"cmd", "internal", "tools"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
				return err
			}
			rel, _ := filepath.Rel(root, p)
			if filepath.ToSlash(filepath.Dir(rel)) == "internal/ui/frameclock" || filepath.ToSlash(rel) == "internal/archtest/fyne_copy_test.go" {
				return nil
			}
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), pacingPkg) {
				t.Errorf("%s импортирует driver/pacing — только через internal/ui/frameclock", filepath.ToSlash(rel))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

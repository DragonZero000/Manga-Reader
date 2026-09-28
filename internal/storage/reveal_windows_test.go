package storage

import (
	"os"
	"testing"
)

func TestRevealMissing(t *testing.T) {
	root := t.TempDir()
	if err := Reveal(root, "nope.zip"); !os.IsNotExist(err) {
		t.Fatalf("ожидалась «нет файла», получено %v", err)
	}
	if err := Reveal(root, "../x.zip"); err == nil {
		t.Fatal("путь за пределами папки")
	}
}

// Открывает Проводник — только вручную, в папке, которая остаётся после теста:
// MANGAREADER_REVEAL=<папка> go test -run TestRevealManual.
func TestRevealManual(t *testing.T) {
	root := os.Getenv("MANGAREADER_REVEAL")
	if root == "" {
		t.Skip("открывает окно Проводника")
	}
	write(t, root, "my series, part 1/vol 1, extra.zip", "x")
	if err := Reveal(root, "my series, part 1/vol 1, extra.zip"); err != nil {
		t.Fatal(err)
	}
	t.Logf("папка: %s", root)
}

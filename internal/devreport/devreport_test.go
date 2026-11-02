//go:build !android

package devreport

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSave(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "perf-1.txt", "отчёт"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, DirName, "perf-1.txt"))
	if err != nil || string(data) != "отчёт" {
		t.Fatalf("файл: %q %v", data, err)
	}
	if err := Share("s", "t"); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Share = %v", err)
	}
}

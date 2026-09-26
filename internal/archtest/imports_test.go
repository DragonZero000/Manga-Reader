// Package archtest проверяет архитектурные границы между пакетами и структуру
// документации (переводы, переключатель языка, ссылки).
package archtest

import (
	"os/exec"
	"strings"
	"testing"
)

// Сервисные пакеты не должны зависеть от виджетов Fyne.
func TestServicePackagesHaveNoWidgets(t *testing.T) {
	forbidden := []string{
		"fyne.io/fyne/v2/widget",
		"fyne.io/fyne/v2/container",
	}
	for _, pkg := range []string{
		"mangareader/internal/appversion",
		"mangareader/internal/paths",
		"mangareader/internal/model",
		"mangareader/internal/search",
		"mangareader/internal/app",
		"mangareader/internal/library",
		"mangareader/internal/thumbs",
		"mangareader/internal/pages",
		"mangareader/internal/storage",
		"mangareader/internal/problems",
		"mangareader/internal/browser",
		"mangareader/internal/mobilebrowser",
		"mangareader/internal/display",
		"mangareader/internal/catalog",
		"mangareader/internal/search/indextest",
	} {
		for _, goos := range []string{"windows", "android"} {
			cmd := exec.Command("go", "list", "-deps", "-tags", "sqlite_fts5", pkg)
			cmd.Env = append(cmd.Environ(), "GOOS="+goos, "CGO_ENABLED=1")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("go list %s (%s): %v\n%s", pkg, goos, err, out)
			}
			for _, dep := range strings.Fields(string(out)) {
				for _, f := range forbidden {
					if dep == f {
						t.Errorf("%s (GOOS=%s) зависит от %s", pkg, goos, f)
					}
				}
			}
		}
	}
}

package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// errorsShell — оболочка над папкой dir; сканирование и toast выполняются
// через очередь, которую разбирает scan.
type errorsShell struct {
	*Shell
	dir      string
	settings *storage.MemSettings
	scan     func()
}

func newErrorsShell(t *testing.T, dir string, settings *storage.MemSettings) *errorsShell {
	t.Helper()
	a := test.NewTempApp(t)
	src := library.NewDirSource(dir, nil)
	svc := app.NewForTest(src, search.NopIndex{}, settings)
	e := &errorsShell{Shell: NewShell(a, svc), dir: dir, settings: settings}

	q := make(chan func(), 64)
	do := func(f func()) { q <- f }
	e.library.SetDispatcher(do)
	e.Search().SetDispatcher(do)
	e.Toast.do = do
	e.scan = func() {
		e.library.Refresh()
		waitFor(t, func() bool { return len(q) > 0 })
		for len(q) > 0 {
			(<-q)()
		}
	}
	return e
}

// lastToast — текст последнего уведомления («» — не было после clearToast).
func (e *errorsShell) lastToast() string { return e.Toast.label.Text }

func (e *errorsShell) clearToast() { e.Toast.label.Text = "" }

func (e *errorsShell) badge() string {
	_, tab := e.Errors()
	return tab.Icon.Name()
}

func TestErrorsTab(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, data []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("page.jpg", jpegHeader)
	write("clip.txt", []byte("заметки"))
	write("dl.zip.part", []byte("PK")) // идёт загрузка — не ошибка

	e := newErrorsShell(t, dir, storage.NewMemSettings())
	screen, tab := e.Errors()
	e.scan()

	if screen.Rows() != 2 || e.svc.Problems.Unseen() != 2 {
		t.Fatalf("строк %d, непросмотренных %d", screen.Rows(), e.svc.Problems.Unseen())
	}
	if !strings.Contains(e.badge(), "-2-") {
		t.Fatalf("иконка вкладки без числа 2: %s", e.badge())
	}
	if got := e.lastToast(); got != "Новые ошибки: 2 — см. вкладку «Ошибки»" {
		t.Fatalf("toast: %q", got)
	}

	// повторный скан — те же ошибки, без нового toast
	e.clearToast()
	e.scan()
	if screen.Rows() != 2 || e.lastToast() != "" {
		t.Fatalf("повторный скан: строк %d, toast %q", screen.Rows(), e.lastToast())
	}

	// открытие вкладки — всё просмотрено, число исчезает
	e.Tabs.Select(tab)
	if e.svc.Problems.Unseen() != 0 || strings.HasPrefix(e.badge(), "badge-") {
		t.Fatalf("после открытия вкладки: unseen %d, иконка %s", e.svc.Problems.Unseen(), e.badge())
	}

	// новая ошибка при открытой вкладке сразу просмотрена
	write("new.mp4", []byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p', 'i', 's', 'o', 'm'})
	e.scan()
	if screen.Rows() != 3 || e.svc.Problems.Unseen() != 0 {
		t.Fatalf("при открытой вкладке: строк %d, unseen %d", screen.Rows(), e.svc.Problems.Unseen())
	}
}

func TestErrorsBadgeDisappearsWhenFileRemoved(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "page.jpg")
	if err := os.WriteFile(p, jpegHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	e := newErrorsShell(t, dir, storage.NewMemSettings())
	screen, _ := e.Errors()
	e.scan()
	if !strings.Contains(e.badge(), "-1-") {
		t.Fatalf("иконка: %s", e.badge())
	}
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
	e.scan()
	if screen.Rows() != 0 || strings.HasPrefix(e.badge(), "badge-") {
		t.Fatalf("после удаления: строк %d, иконка %s", screen.Rows(), e.badge())
	}
}

func TestErrorsFileKept(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "g-2.zip")
	html := []byte("<!DOCTYPE html><html>captcha</html>")
	if err := os.WriteFile(p, html, 0o644); err != nil {
		t.Fatal(err)
	}
	e := newErrorsShell(t, dir, storage.NewMemSettings())
	e.scan()
	data, err := os.ReadFile(p)
	if err != nil || string(data) != string(html) {
		t.Fatalf("файл должен остаться без изменений: %v", err)
	}
}

func TestErrorsSeenSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "page.jpg"), jpegHeader, 0o644); err != nil {
		t.Fatal(err)
	}
	settings := storage.NewMemSettings()
	e := newErrorsShell(t, dir, settings)
	_, tab := e.Errors()
	e.scan()
	e.Tabs.Select(tab)

	e = newErrorsShell(t, dir, settings) // «перезапуск»
	e.scan()
	if e.svc.Problems.Unseen() != 0 || e.lastToast() != "" || strings.HasPrefix(e.badge(), "badge-") {
		t.Fatalf("после перезапуска: unseen %d, toast %q, иконка %s", e.svc.Problems.Unseen(), e.lastToast(), e.badge())
	}
}

// jpegHeader — начало JPEG-файла (достаточно для определения типа).
var jpegHeader = []byte{0xff, 0xd8, 0xff, 0xe0, 0, 0x10, 'J', 'F', 'I', 'F', 0}

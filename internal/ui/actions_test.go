package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
	"mangareader/internal/ui/screens"
)

// permFS — папка без корзины: удаление только безвозвратное (тесты не
// трогают корзину Windows); fail — ошибка удаления.
type permFS struct {
	*storage.FS
	root string
	fail error
}

func (s *permFS) CanTrash(string) (bool, error) { return false, nil }

func (s *permFS) Delete(rel string, permanent bool) error {
	if s.fail != nil {
		return s.fail
	}
	if !permanent {
		return storage.ErrNoTrash
	}
	return os.Remove(filepath.Join(s.root, filepath.FromSlash(rel)))
}

// confirmCall — показанный диалог подтверждения.
type confirmCall struct {
	title, text, button string
	cb                  func(bool)
}

type deleteEnv struct {
	s        *Shell
	st       *permFS
	dir      string
	pump     func()
	confirms []confirmCall
	g        model.Gallery
}

// newDeleteShell — оболочка над папкой с example.zip и second.zip (без
// корзины); UI-функции копятся в очереди, диалоги подтверждения
// записываются; библиотека и поиск показывают обе галереи.
func newDeleteShell(t *testing.T) *deleteEnv {
	t.Helper()
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"example.zip", "second.zip"} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	e := &deleteEnv{dir: dir, st: &permFS{FS: storage.NewFS(dir), root: dir}}
	idx := search.NewMemIndex()
	src := library.NewSource(e.st, idx)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.s = NewShell(a, app.NewForTest(src, idx, storage.NewMemSettings()))
	q := make(chan func(), 4096)
	do := func(f func()) { q <- f }
	e.s.do = do
	e.s.Toast.do = do
	e.s.Details.SetDispatcher(func(func()) {})
	e.s.Reader.SetDispatcher(func(func()) {})
	e.s.Library().SetDispatcher(do)
	e.s.Search().SetDispatcher(do)
	e.s.confirm = func(title, text, button string, cb func(bool)) {
		e.confirms = append(e.confirms, confirmCall{title, text, button, cb})
	}
	e.pump = func() {
		for {
			select {
			case f := <-q:
				f()
			default:
				return
			}
		}
	}
	e.g, _ = src.Get(model.LocalKey("example.zip"))

	e.s.Library().Refresh()
	waitUI(t, e.pump, "библиотека", func() bool { return len(e.s.Library().Galleries()) == 2 })
	e.s.Search().SetQuery("english")
	waitUI(t, e.pump, "поиск", func() bool { return len(e.s.Search().Results()) == 2 })
	return e
}

func (e *deleteEnv) fileExists() bool {
	_, err := os.Stat(filepath.Join(e.dir, "example.zip"))
	return err == nil
}

func (e *deleteEnv) shown(k model.Key) (inLibrary, inSearch bool) {
	for _, g := range e.s.Library().Galleries() {
		inLibrary = inLibrary || g.Key == k
	}
	for _, g := range e.s.Search().Results() {
		inSearch = inSearch || g.Key == k
	}
	return
}

// waitConfirm запускает удаление и ждёт диалога подтверждения.
func (e *deleteEnv) waitConfirm(t *testing.T) confirmCall {
	t.Helper()
	n := len(e.confirms)
	e.s.Delete(e.g)
	waitUI(t, e.pump, "подтверждение", func() bool { return len(e.confirms) > n })
	return e.confirms[len(e.confirms)-1]
}

func TestDeleteInUse(t *testing.T) {
	e := newDeleteShell(t)
	e.s.Reader.Open(e.g)
	e.s.Delete(e.g)
	e.pump()
	if got := e.s.Toast.label.Text; got != i18n.T("actions.delete.in_use") {
		t.Fatalf("toast %q", got)
	}
	if len(e.confirms) != 0 || !e.fileExists() {
		t.Fatal("удаление открытого в читалке произведения не отклонено")
	}
	// страница произведения открытым произведением не считается
	e.s.Reader.Close()
	e.s.Details.Open(e.g)
	e.waitConfirm(t)
}

// Доступ к папке только на чтение (Android): вместо удаления — просьба
// выбрать папку заново; файл не трогается.
func TestDeleteNeedsWriteAccess(t *testing.T) {
	e := newDeleteShell(t)
	e.s.needsWriteAccess = func() bool { return true }
	e.s.Delete(e.g)
	e.pump()
	if len(e.confirms) != 1 || e.confirms[0].title != i18n.T("shell.write_access.title") ||
		e.confirms[0].button != i18n.T("folder.choose") {
		t.Fatalf("диалоги %+v", e.confirms)
	}
	e.confirms[0].cb(false)
	e.pump()
	if lib, found := e.shown(e.g.Key); !e.fileExists() || !lib || !found {
		t.Fatalf("без доступа на запись: файл %v, библиотека %v, поиск %v", e.fileExists(), lib, found)
	}
}

func TestDeleteCancel(t *testing.T) {
	e := newDeleteShell(t)
	c := e.waitConfirm(t)
	for _, want := range []string{e.g.Title, "example.zip", i18n.T("actions.delete.no_trash")} {
		if !strings.Contains(c.text, want) {
			t.Errorf("текст подтверждения %q без %q", c.text, want)
		}
	}
	if c.button != i18n.T("actions.delete.button") || c.title != i18n.T("actions.delete.title") {
		t.Errorf("диалог %q, кнопка %q", c.title, c.button)
	}
	c.cb(false)
	e.pump()
	lib, found := e.shown(e.g.Key)
	if !e.fileExists() || !lib || !found {
		t.Fatalf("после отмены: файл %v, библиотека %v, поиск %v", e.fileExists(), lib, found)
	}
}

func TestDeleteBusy(t *testing.T) {
	e := newDeleteShell(t)
	e.st.fail = storage.ErrBusy
	e.waitConfirm(t).cb(true)
	waitUI(t, e.pump, "toast", func() bool { return e.s.Toast.label.Text == i18n.T("error.busy") })
	if lib, found := e.shown(e.g.Key); !lib || !found || !e.fileExists() {
		t.Fatalf("после ошибки: библиотека %v, поиск %v", lib, found)
	}
}

func TestDeleteSuccess(t *testing.T) {
	e := newDeleteShell(t)
	e.waitConfirm(t).cb(true)
	waitUI(t, e.pump, "удаление", func() bool {
		lib, found := e.shown(e.g.Key)
		return !lib && !found
	})
	if e.fileExists() {
		t.Fatal("файл не удалён")
	}
	if got := e.s.Library().Count(); got != i18n.N("library.count", 1) {
		t.Errorf("счётчик библиотеки %q", got)
	}
	if got := e.s.Search().StatusText(); got != i18n.T("search.found", "Count", 1) {
		t.Errorf("статус поиска %q", got)
	}
	if e.s.Search().RandomButton().Disabled() {
		t.Error("🎲 поиска должна оставаться активной")
	}
	// сканирование после удаления: галереи нет, ошибок нет
	waitUI(t, e.pump, "сканирование", func() bool { return !e.s.Library().Scanning() })
	if _, ok := e.s.svc.Library.Get(e.g.Key); ok || len(e.s.svc.Problems.Items()) != 0 {
		t.Fatalf("после сканирования: ошибки %v", e.s.svc.Problems.Items())
	}
}

// Удаление со страницы произведения закрывает её.
func TestDeleteFromDetails(t *testing.T) {
	e := newDeleteShell(t)
	e.s.Details.Open(e.g)
	if !e.s.Details.MenuButton().Visible() {
		t.Fatal("нет «⋮» на странице произведения")
	}
	menu := e.s.actions.DetailsMenu(e.s.Details.Gallery())
	var del *fyne.MenuItem
	for _, it := range menu.Items {
		if it.Label == i18n.T("menu.delete") {
			del = it
		}
	}
	if del == nil {
		t.Fatal("в меню страницы нет «Удалить…»")
	}
	del.Action()
	waitUI(t, e.pump, "подтверждение", func() bool { return len(e.confirms) == 1 })
	e.confirms[0].cb(true)
	waitUI(t, e.pump, "закрытие страницы", func() bool { return !e.s.Details.Visible() })
	if lib, _ := e.shown(e.g.Key); lib {
		t.Fatal("карточка осталась в библиотеке")
	}
}

// Удаление последней галереи показывает пустую библиотеку и выключает 🎲.
func TestLibraryRemoveLast(t *testing.T) {
	e := newDeleteShell(t)
	l := e.s.Library()
	l.Remove(model.LocalKey("example.zip"))
	l.Remove(model.LocalKey("second.zip"))
	if got := l.Count(); got != i18n.N("library.count", 0) {
		t.Fatalf("счётчик %q", got)
	}
	if !l.RandomButton().Disabled() {
		t.Fatal("🎲 должна выключиться")
	}
}

func TestCopyTitle(t *testing.T) {
	e := newDeleteShell(t)
	e.s.copyTitle(e.g)
	e.pump()
	if got := fyne.CurrentApp().Clipboard().Content(); got != "english name" {
		t.Fatalf("буфер обмена %q", got)
	}
	if got := e.s.Toast.label.Text; got != i18n.T("actions.title_copied") {
		t.Fatalf("toast %q", got)
	}
}

func TestDeleteErrorText(t *testing.T) {
	if got := deleteErrorText(storage.ErrUnsupported); !strings.Contains(got, screens.ErrorText(storage.ErrUnsupported)) || got == screens.ErrorText(storage.ErrUnsupported) {
		t.Fatalf("текст ошибки %q", got)
	}
}

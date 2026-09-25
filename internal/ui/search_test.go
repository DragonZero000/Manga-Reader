package ui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// newTestShell — оболочка над библиотекой с example.zip и индексом в памяти;
// UI-функции копятся в очереди и выполняются pump в горутине теста.
func newTestShell(t *testing.T) (*Shell, *library.Source, func()) {
	t.Helper()
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	idx := search.NewMemIndex()
	src := library.NewDirSource(dir, idx)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	svc := app.NewForTest(src, idx, storage.NewMemSettings())
	s := NewShell(a, svc)
	q := make(chan func(), 4096)
	do := func(f func()) { q <- f }
	s.Details.SetDispatcher(do)
	s.Reader.SetDispatcher(do)
	s.Search().SetDispatcher(do)
	pump := func() {
		for {
			select {
			case f := <-q:
				f()
			default:
				return
			}
		}
	}
	return s, src, pump
}

func waitUI(t *testing.T, pump func(), what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pump()
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

func TestSearchScreen(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()

	// пустой запрос — справка
	sc.SetQuery("")
	if !sc.HelpVisible() {
		t.Fatal("при пустом запросе должна быть справка")
	}

	sc.SetQuery("japan")
	waitUI(t, pump, "результаты", func() bool { return sc.StatusText() == "Найдено: 1" })
	if items := sc.Results(); len(items) != 1 || items[0].Key != model.LocalKey("example.zip") {
		t.Fatalf("результаты: %v", items)
	}

	// ошибка разбора: сообщение, прежние результаты остаются
	sc.SetQuery("pages:>много")
	pump()
	if st := sc.StatusText(); st == "" || !strings.Contains(st, "pages:>много") {
		t.Fatalf("статус ошибки: %q", st)
	}
	if len(sc.Results()) != 1 {
		t.Fatal("при ошибке прежние результаты должны сохраниться")
	}

	sc.SetQuery("zzzz")
	waitUI(t, pump, "нет результатов", func() bool { return sc.StatusText() == "Ничего не найдено" })
	if len(sc.Results()) != 0 {
		t.Fatal("сетка должна быть пуста")
	}
}

func TestSearchDebounce(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()
	sc.TypeText("eng")
	sc.TypeText("english")
	waitUI(t, pump, "поиск после паузы", func() bool { return sc.StatusText() == "Найдено: 1" })
	if sc.Query() != "english" {
		t.Fatalf("текст запроса %q", sc.Query())
	}
}

func TestTagToSearch(t *testing.T) {
	s, src, pump := newTestShell(t)
	g, _ := src.Get(model.LocalKey("example.zip"))
	s.Details.Open(g)
	s.SearchFor(`artist:"artist 1"`) // как нажатие на тег на странице произведения
	if s.Details.Visible() {
		t.Fatal("страница произведения должна закрыться")
	}
	if s.Tabs.Selected() != s.searchTab {
		t.Fatal("должна открыться вкладка «Поиск»")
	}
	sc := s.Search()
	if sc.Query() != `artist:"artist 1"` {
		t.Fatalf("запрос %q", sc.Query())
	}
	waitUI(t, pump, "результаты по тегу", func() bool { return sc.StatusText() == "Найдено: 1" })
}

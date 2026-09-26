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

// testDelay — задержка режима «При вводе» в тестах.
const testDelay = 50 * time.Millisecond

// newTestShell — оболочка над библиотекой с example.zip и индексом в памяти;
// UI-функции копятся в очереди и выполняются pump в горутине теста.
func newTestShell(t *testing.T) (*Shell, *library.Source, func()) {
	t.Helper()
	return newTestShellSettings(t, storage.NewMemSettings())
}

func newTestShellSettings(t *testing.T, settings storage.Settings) (*Shell, *library.Source, func()) {
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
	svc := app.NewForTest(src, idx, settings)
	s := NewShell(a, svc)
	q := make(chan func(), 4096)
	do := func(f func()) { q <- f }
	s.Details.SetDispatcher(do)
	s.Reader.SetDispatcher(do)
	s.Search().SetDispatcher(do)
	s.Search().SetDelay(testDelay)
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

// settle даёт истечь задержке ввода и выполняет накопленные UI-функции,
// включая результаты фонового поиска.
func settle(pump func()) {
	time.Sleep(3 * testDelay)
	pump()
	time.Sleep(50 * time.Millisecond)
	pump()
}

func TestSearchScreen(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()

	// до первого запроса — справка; пустой запрос ничего не меняет
	if !sc.HelpVisible() {
		t.Fatal("при первом открытии должна быть справка")
	}
	sc.SetQuery("")
	if !sc.HelpVisible() {
		t.Fatal("пустой запрос не должен убирать справку")
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
	if sc.SearchButtonVisible() {
		t.Fatal("в режиме «При вводе» кнопки поиска нет")
	}
	sc.TypeText("eng")
	sc.TypeText("english")
	waitUI(t, pump, "поиск после паузы", func() bool { return sc.StatusText() == "Найдено: 1" })
	if sc.Query() != "english" {
		t.Fatalf("текст запроса %q", sc.Query())
	}
}

func TestSearchDebounceRestart(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()
	sc.SetDelay(200 * time.Millisecond)
	sc.TypeText("zzz")
	time.Sleep(120 * time.Millisecond)
	pump()
	sc.TypeText("japan") // ввод до истечения задержки — отсчёт заново
	time.Sleep(120 * time.Millisecond)
	pump()
	if !sc.HelpVisible() || sc.StatusText() != "" {
		t.Fatalf("запрос выполнен до паузы: статус %q", sc.StatusText())
	}
	waitUI(t, pump, "поиск после паузы", func() bool { return sc.StatusText() == "Найдено: 1" })
	if sc.Seq() != 1 {
		t.Fatalf("выполнено запросов: %d", sc.Seq())
	}
}

func TestSearchEnterDynamic(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()
	sc.SetDelay(200 * time.Millisecond)
	sc.TypeText("japan")
	sc.Submit() // Enter только снимает фокус
	time.Sleep(50 * time.Millisecond)
	pump()
	if !sc.HelpVisible() {
		t.Fatal("Enter в режиме «При вводе» не должен запускать поиск")
	}
	waitUI(t, pump, "поиск по таймеру", func() bool { return sc.StatusText() == "Найдено: 1" })
}

func TestSearchSubmitMode(t *testing.T) {
	st := storage.NewMemSettings()
	st.SetString(app.KeySearchMode, app.SearchModeSubmit)
	s, _, pump := newTestShellSettings(t, st)
	sc := s.Search()
	if !sc.SearchButtonVisible() {
		t.Fatal("в режиме «По кнопке» должна быть кнопка поиска")
	}

	// ввод без кнопки ничего не выполняет, ошибка тоже не показывается
	sc.TypeText("pages:>много")
	settle(pump)
	if !sc.HelpVisible() || sc.StatusText() != "" {
		t.Fatalf("ввод выполнил запрос: статус %q", sc.StatusText())
	}

	sc.TypeText("japan")
	sc.PressSearch()
	waitUI(t, pump, "поиск по кнопке", func() bool { return sc.StatusText() == "Найдено: 1" })

	sc.TypeText("zzzz")
	sc.Submit()
	waitUI(t, pump, "поиск по Enter", func() bool { return sc.StatusText() == "Ничего не найдено" })

	// пустое поле по кнопке — прежний результат
	sc.TypeText("")
	sc.PressSearch()
	settle(pump)
	if sc.HelpVisible() || sc.StatusText() != "Ничего не найдено" {
		t.Fatalf("пустое поле изменило экран: статус %q", sc.StatusText())
	}
}

func TestSearchClear(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()
	sc.SetQuery("japan")
	waitUI(t, pump, "результаты", func() bool { return sc.StatusText() == "Найдено: 1" })

	sc.Clear()
	if sc.Query() != "" {
		t.Fatalf("поле после сброса %q", sc.Query())
	}
	settle(pump)
	if sc.HelpVisible() || sc.StatusText() != "Найдено: 1" || len(sc.Results()) != 1 {
		t.Fatalf("сброс изменил результаты: статус %q", sc.StatusText())
	}

	// сброс отменяет ожидающий запрос
	sc.TypeText("zzzz")
	sc.Clear()
	settle(pump)
	if sc.StatusText() != "Найдено: 1" {
		t.Fatalf("после сброса выполнен запрос: статус %q", sc.StatusText())
	}
}

func TestSearchKeepsLastResult(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()
	sc.SetQuery("japan")
	waitUI(t, pump, "результаты", func() bool { return sc.StatusText() == "Найдено: 1" })

	// ручное стирание и возврат на вкладку — справки нет, результат прежний
	sc.TypeText("")
	settle(pump)
	s.Tabs.SelectIndex(0)
	s.Tabs.Select(s.searchTab)
	settle(pump)
	if sc.HelpVisible() || sc.StatusText() != "Найдено: 1" {
		t.Fatalf("после стирания: справка %v, статус %q", sc.HelpVisible(), sc.StatusText())
	}
}

func TestSearchRerunLast(t *testing.T) {
	st := storage.NewMemSettings()
	st.SetString(app.KeySearchMode, app.SearchModeSubmit)
	s, _, pump := newTestShellSettings(t, st)
	sc := s.Search()

	sc.Rerun() // до первого запроса — справка
	settle(pump)
	if !sc.HelpVisible() {
		t.Fatal("Rerun до первого запроса не должен убирать справку")
	}

	sc.TypeText("japan")
	sc.PressSearch()
	waitUI(t, pump, "результаты", func() bool { return sc.StatusText() == "Найдено: 1" })
	sc.TypeText("zzzz") // не отправлен
	sc.Rerun()
	settle(pump)
	if sc.StatusText() != "Найдено: 1" || sc.Query() != "zzzz" {
		t.Fatalf("Rerun: статус %q, поле %q", sc.StatusText(), sc.Query())
	}
}

func TestSearchSetModeCancels(t *testing.T) {
	s, _, pump := newTestShell(t)
	sc := s.Search()
	sc.SetDelay(200 * time.Millisecond)
	sc.TypeText("japan")
	s.Settings().SelectSearchMode(app.SearchModeSubmit)
	time.Sleep(400 * time.Millisecond)
	pump()
	if !sc.HelpVisible() {
		t.Fatal("смена режима должна отменить ожидающий запрос")
	}
	if !sc.SearchButtonVisible() {
		t.Fatal("в режиме «По кнопке» должна быть кнопка поиска")
	}
}

func TestSearchModeSetting(t *testing.T) {
	st := storage.NewMemSettings()
	s, _, _ := newTestShellSettings(t, st)
	if got := s.Settings().SearchModeLabel(); got != "При вводе" {
		t.Fatalf("режим по умолчанию %q", got)
	}
	s.Settings().SelectSearchMode(app.SearchModeSubmit)
	if v := st.String(app.KeySearchMode, ""); v != app.SearchModeSubmit {
		t.Fatalf("сохранено %q", v)
	}

	// при новом запуске режим восстанавливается
	s2, _, _ := newTestShellSettings(t, st)
	if got := s2.Settings().SearchModeLabel(); got != "По кнопке" {
		t.Fatalf("восстановлен режим %q", got)
	}
	if !s2.Search().SearchButtonVisible() {
		t.Fatal("после перезапуска должна быть кнопка поиска")
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

	// заполнение поля не запускает второй запрос по таймеру
	seq := sc.Seq()
	settle(pump)
	if sc.Seq() != seq {
		t.Fatal("поиск по тегу выполнен повторно по таймеру")
	}
}

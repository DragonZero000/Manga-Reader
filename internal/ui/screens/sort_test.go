package screens

import (
	"reflect"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// sortGalleries: размер растёт с номером, время изменения убывает с номером,
// название идёт в обратном порядке номеров.
func sortGalleries() []model.Gallery {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(id, title string, pages int, size int64, age int) model.Gallery {
		return model.Gallery{Key: model.LocalKey(id), Title: title, Pages: make([]model.Page, pages),
			File: model.FileInfo{Size: size, ModTime: base.Add(-time.Duration(age) * time.Hour)}}
	}
	return []model.Gallery{
		mk("g1.zip", "Cc", 30, 100, 1),
		mk("g2.zip", "Bb", 10, 200, 2),
		mk("g3.zip", "Aa", 20, 300, 3),
	}
}

func idsOf(gs []model.Gallery) []string {
	out := make([]string, len(gs))
	for i, g := range gs {
		out[i] = g.Key.ID
	}
	return out
}

func newSortLibrary(t *testing.T, st storage.Settings) (*Library, *app.Services) {
	t.Helper()
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, st)
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, &GalleryActions{Open: func(model.Gallery) {}}, nil)
	a.NewWindow("t").SetContent(l.Content())
	return l, svc
}

func wantOrder(t *testing.T, what string, got []model.Gallery, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(idsOf(got), want) {
		t.Fatalf("%s: %v, want %v", what, idsOf(got), want)
	}
}

// Выбор поля, инверсия и сброс в Библиотеке; сканирование не запускается.
func TestLibrarySortControl(t *testing.T) {
	l, _ := newSortLibrary(t, storage.NewMemSettings())
	l.applyScan(library.ScanResult{Galleries: sortGalleries()}, nil)
	wantOrder(t, "по умолчанию (добавлено ↓)", l.Galleries(), "g1.zip", "g2.zip", "g3.zip")
	if l.Sort().Highlighted() || l.Sort().Label() != sortLabel(search.DefaultSort) {
		t.Fatalf("стартовая сортировка подсвечена: %q", l.Sort().Label())
	}

	l.Sort().Select(search.FieldSize)
	wantOrder(t, "размер ↓", l.Galleries(), "g3.zip", "g2.zip", "g1.zip")
	if !l.Sort().Highlighted() || l.Sort().Label() != sortLabel(search.Sort{Field: search.FieldSize, Desc: true}) {
		t.Fatalf("нет подсветки или подпись %q", l.Sort().Label())
	}
	l.Sort().Select(search.FieldSize)
	wantOrder(t, "размер ↑", l.Galleries(), "g1.zip", "g2.zip", "g3.zip")
	if l.Sort().Sort().Desc {
		t.Fatal("повторный выбор поля должен менять направление")
	}
	l.Sort().Select(search.FieldTitle)
	wantOrder(t, "название ↑", l.Galleries(), "g3.zip", "g2.zip", "g1.zip")
	if l.Sort().Sort().Desc {
		t.Fatal("название по умолчанию по возрастанию")
	}
	l.Sort().Select(search.FieldPages)
	wantOrder(t, "страницы ↓", l.Galleries(), "g1.zip", "g3.zip", "g2.zip")

	l.Sort().Reset()
	wantOrder(t, "после сброса", l.Galleries(), "g1.zip", "g2.zip", "g3.zip")
	if l.Sort().Highlighted() {
		t.Fatal("после сброса подсветка должна сняться")
	}
	if l.Scanning() {
		t.Fatal("смена сортировки запустила сканирование")
	}
}

// Меню: пять полей и «По умолчанию»; текущее поле отмечено.
func TestSortMenu(t *testing.T) {
	l, _ := newSortLibrary(t, storage.NewMemSettings())
	l.Sort().Select(search.FieldPages)
	m := l.Sort().Menu()
	if len(m.Items) != 7 { // пять полей, разделитель, «По умолчанию»
		t.Fatalf("пунктов меню %d", len(m.Items))
	}
	for i, f := range app.SortFields {
		if m.Items[i].Checked != (f == search.FieldPages) {
			t.Fatalf("пункт %d: Checked=%v", i, m.Items[i].Checked)
		}
	}
	m.Items[6].Action() // «По умолчанию»
	if l.Sort().Sort() != search.DefaultSort {
		t.Fatalf("после «По умолчанию» %+v", l.Sort().Sort())
	}
}

// Выбранная сортировка сохраняется после сканирования.
func TestLibrarySortSurvivesScan(t *testing.T) {
	l, _ := newSortLibrary(t, storage.NewMemSettings())
	l.applyScan(library.ScanResult{Galleries: sortGalleries()}, nil)
	l.Sort().Select(search.FieldTitle)
	l.applyScan(library.ScanResult{Galleries: sortGalleries()}, nil)
	wantOrder(t, "после сканирования", l.Galleries(), "g3.zip", "g2.zip", "g1.zip")
}

// Сортировка не меняет список, который отдал сервис.
func TestSortDoesNotMutateScanResult(t *testing.T) {
	l, _ := newSortLibrary(t, storage.NewMemSettings())
	res := sortGalleries()
	l.applyScan(library.ScanResult{Galleries: res}, nil)
	l.Sort().Select(search.FieldSize)
	wantOrder(t, "результат скана", res, "g1.zip", "g2.zip", "g3.zip")
}

// Смена сортировки прокручивает сетку в начало.
func TestSortScrollsToTop(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, &GalleryActions{Open: func(model.Gallery) {}}, nil)
	w := a.NewWindow("t")
	w.SetContent(l.Content())
	w.Resize(fyne.NewSize(400, 300))
	var many []model.Gallery
	for i := range 200 {
		g := sortGalleries()[0]
		g.Key = model.LocalKey("g" + string(rune('a'+i%26)) + string(rune('a'+i/26)) + ".zip")
		g.File.Size = int64(i)
		many = append(many, g)
	}
	l.applyScan(library.ScanResult{Galleries: many}, nil)
	l.Scroller().ScrollToOffset(500)
	if l.Scroller().ScrollOffset() == 0 {
		t.Skip("тестовый драйвер не прокручивает сетку")
	}
	l.Sort().Select(search.FieldSize)
	if off := l.Scroller().ScrollOffset(); off != 0 {
		t.Fatalf("смещение после смены сортировки %v", off)
	}
}

// Поиск: новая выдача и пересортировка без повторного запроса.
func TestSearchSortWithoutRequery(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), search.NewMemIndex(), storage.NewMemSettings())
	s := NewSearch(svc, NewGridMetrics(svc.Settings), &GalleryActions{Open: func(model.Gallery) {}})
	a.NewWindow("t").SetContent(s.Content())

	s.Sort().Select(search.FieldPages) // страницы ↓ до выдачи
	s.apply(sortGalleries(), 3, nil)
	wantOrder(t, "новая выдача", s.Results(), "g1.zip", "g3.zip", "g2.zip")

	s.Sort().Select(search.FieldSize)
	wantOrder(t, "размер ↓", s.Results(), "g3.zip", "g2.zip", "g1.zip")
	s.Sort().Reset()
	wantOrder(t, "по умолчанию", s.Results(), "g1.zip", "g2.zip", "g3.zip")
	if s.Seq() != 0 {
		t.Fatalf("выполнено запросов: %d", s.Seq())
	}
}

// Вкладки независимы; смена стартовой сортировки применяется только к
// вкладкам без ручного выбора.
func TestSortTabsIndependentAndStartChange(t *testing.T) {
	st := storage.NewMemSettings()
	l, svc := newSortLibrary(t, st)
	s := NewSearch(svc, NewGridMetrics(svc.Settings), &GalleryActions{Open: func(model.Gallery) {}})
	l.applyScan(library.ScanResult{Galleries: sortGalleries()}, nil)
	s.apply(sortGalleries(), 3, nil)

	l.Sort().Select(search.FieldTitle)
	if s.Sort().Sort() != search.DefaultSort || s.Sort().Highlighted() {
		t.Fatal("сортировка Поиска изменилась вместе с Библиотекой")
	}

	app.SetStartSort(st, search.Sort{Field: search.FieldPages, Desc: true})
	l.StartSortChanged()
	s.StartSortChanged()
	wantOrder(t, "Поиск на новой стартовой", s.Results(), "g1.zip", "g3.zip", "g2.zip")
	if s.Sort().Highlighted() {
		t.Fatal("Поиск на стартовой сортировке подсвечен")
	}
	wantOrder(t, "Библиотека осталась на ручной", l.Galleries(), "g3.zip", "g2.zip", "g1.zip")
	if !l.Sort().Highlighted() {
		t.Fatal("ручная сортировка Библиотеки должна быть подсвечена")
	}
	l.Sort().Reset()
	wantOrder(t, "сброс на новую стартовую", l.Galleries(), "g1.zip", "g3.zip", "g2.zip")
}

// Настройка сохраняется и сообщает о смене.
func TestSettingsStartSort(t *testing.T) {
	st := storage.NewMemSettings()
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, st)
	s := NewSettings(a, a.NewWindow("t"), svc, func(string) {}, nil, nil, nil, nil)
	if got := app.StartSort(st); got != search.DefaultSort {
		t.Fatalf("по умолчанию %+v", got)
	}
	changed := 0
	s.OnSortChanged = func() { changed++ }
	s.SelectSortDesc(false)
	s.SelectSortField(search.FieldPages)
	if got := app.StartSort(st); got != (search.Sort{Field: search.FieldPages}) || changed == 0 {
		t.Fatalf("после выбора %+v, уведомлений %d", got, changed)
	}
	s2 := NewSettings(a, a.NewWindow("t2"), svc, func(string) {}, nil, nil, nil, nil)
	if s2.sortField.Selected != s.sortField.Selected || s2.sortDir.Selected != s.sortDir.Selected {
		t.Fatal("настройка не восстановилась при создании экрана")
	}
}

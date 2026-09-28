package screens

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/storage"
)

func newTestSettings(t *testing.T, st storage.Settings) *Settings {
	t.Helper()
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, st)
	return NewSettings(a, a.NewWindow("t"), svc, func(string) {}, nil, nil, nil, nil)
}

// phoneWidth — ширина узкого телефона: экран настроек не должен быть шире,
// иначе текст справа обрезается.
const phoneWidth = 320

// Экран настроек на телефоне (со всеми разделами Android) не шире узкого
// телефона: заголовки и подзаголовки разделов не переносятся.
func TestSettingsFitPhoneWidth(t *testing.T) {
	oldMobile, oldDisplay := isMobile, displaySupported
	isMobile, displaySupported = func() bool { return true }, true
	t.Cleanup(func() { isMobile, displaySupported = oldMobile, oldDisplay })
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	svc.CanChooseFolder, svc.MobileBrowser = true, true
	s := NewSettings(a, a.NewWindow("t"), svc, func(string) {}, func() {}, nil, nil, nil)
	if w := s.Content().MinSize().Width; w > phoneWidth {
		t.Fatalf("минимальная ширина экрана настроек %.0f > %d", w, phoneWidth)
	}
}

func TestDisplayCardAbsentWithoutSupport(t *testing.T) {
	old := displaySupported
	displaySupported = false
	t.Cleanup(func() { displaySupported = old })
	if s := newTestSettings(t, storage.NewMemSettings()); s.DisplayCheck() != nil {
		t.Fatal("без поддержки раздела «Экран» быть не должно")
	}
}

func TestDisplayCard(t *testing.T) {
	oldSup, oldSet := displaySupported, setMax60
	var applied []bool
	displaySupported = true
	setMax60 = func(on bool) error { applied = append(applied, on); return nil }
	t.Cleanup(func() { displaySupported, setMax60 = oldSup, oldSet })

	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	c := s.DisplayCheck()
	if c == nil || !c.Checked {
		t.Fatal("по умолчанию «Ограничить 60 Гц» включено")
	}
	c.SetChecked(false) // как нажатие пользователя
	if st.String(app.KeyDisplayMax60, "") != "0" || len(applied) != 1 || applied[0] {
		t.Fatalf("выключение: настройка %q, применено %v", st.String(app.KeyDisplayMax60, ""), applied)
	}
	// после перезапуска флажок восстанавливается
	if newTestSettings(t, st).DisplayCheck().Checked {
		t.Fatal("выключенный флажок не восстановился")
	}
}

// Библиотека из каталога показывается до сканирования: сетка и счётчик,
// ошибки прошлых запусков — без уведомления о «новых».
func TestLibraryShowCached(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	var notes []string
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(s string) { notes = append(notes, s) }, func(model.Gallery) {}, nil)
	w := a.NewWindow("t")
	w.SetContent(l.Content())

	l.ShowCached(library.ScanResult{}) // пустой каталог — ничего не меняется
	if l.count.Text == "Галерей: 2" {
		t.Fatal("пустой результат не должен менять экран")
	}
	res := library.ScanResult{
		Galleries: []model.Gallery{galleryOf("a.zip"), galleryOf("b.zip")},
		Errors:    []library.ScanError{{RelPath: "page.html", Err: &library.UnsupportedError{Kind: library.KindHTML}}},
	}
	l.ShowCached(res)
	if l.count.Text != "Галерей: 2" || len(l.grid.Items()) != 2 || !l.grid.Widget().Visible() {
		t.Fatalf("из каталога: %q, в сетке %d", l.count.Text, len(l.grid.Items()))
	}
	if len(svc.Problems.Items()) != 1 || len(notes) != 0 {
		t.Fatalf("ошибки: %d, уведомления %v", len(svc.Problems.Items()), notes)
	}
}

// Если сканирование успело показать результат раньше, каталог его не
// перезаписывает.
func TestLibraryShowCachedAfterScan(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, func(model.Gallery) {}, nil)
	a.NewWindow("t").SetContent(l.Content())
	l.applyScan(library.ScanResult{Galleries: []model.Gallery{galleryOf("fresh.zip")}}, nil)
	l.ShowCached(library.ScanResult{Galleries: []model.Gallery{galleryOf("old1.zip"), galleryOf("old2.zip")}})
	if items := l.grid.Items(); len(items) != 1 || items[0].Key.ID != "fresh.zip" {
		t.Fatalf("каталог перезаписал результат сканирования: %v", items)
	}
}

// На телефоне раздел «Сетка» — «Карточек в ряду»: 3 по умолчанию, выбор
// сохраняется и сообщается колбэком.
func TestGridCardMobile(t *testing.T) {
	setMobile(t, true)
	a := test.NewTempApp(t)
	st := storage.NewMemSettings()
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, st)
	changed := 0
	s := NewSettings(a, a.NewWindow("t"), svc, func(string) {}, nil, nil, nil, func() { changed++ })
	if got := s.GridLabel(); got != "3" {
		t.Fatalf("по умолчанию %q", got)
	}
	s.SelectGrid("4")
	if app.GridColumns(st) != 4 || changed != 1 {
		t.Fatalf("сохранено %d, колбэков %d", app.GridColumns(st), changed)
	}
	if got := NewSettings(a, a.NewWindow("t"), svc, func(string) {}, nil, nil, nil, nil).GridLabel(); got != "4" {
		t.Fatalf("после перезапуска %q", got)
	}
}

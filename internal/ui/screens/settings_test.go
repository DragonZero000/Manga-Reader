package screens

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
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

// Выбор языка сохраняется, подсказка о перезапуске — на выбранном языке;
// возврат к языку запуска её скрывает.
func TestLanguageSetting(t *testing.T) {
	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	if s.LanguageHint() != "" {
		t.Fatal("до выбора подсказки быть не должно")
	}
	s.SelectLanguage("en")
	if got := app.UILanguage(st); got != "en" {
		t.Fatalf("сохранено %q", got)
	}
	if got := s.LanguageHint(); got != "The language will change after the app restarts." {
		t.Fatalf("подсказка %q", got)
	}
	s.SelectLanguage("ru")
	if got := s.LanguageHint(); got != "Язык изменится после перезапуска приложения." {
		t.Fatalf("подсказка %q", got)
	}
	s.SelectLanguage(i18n.Auto)
	if app.UILanguage(st) != i18n.Auto || s.LanguageHint() != "" {
		t.Fatalf("возврат к «Системный»: %q, подсказка %q", app.UILanguage(st), s.LanguageHint())
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
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(s string) { notes = append(notes, s) }, &GalleryActions{Open: func(model.Gallery) {}}, nil)
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
	if l.count.Text != "2 галереи" || len(l.grid.Items()) != 2 || !l.grid.Widget().Visible() {
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
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, &GalleryActions{Open: func(model.Gallery) {}}, nil)
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

// Срок хранения данных удалённых произведений: по умолчанию «1 месяц»,
// выбор варианта и своё число сохраняются, недопустимое число — ошибка
// в поле и прежнее значение.
func TestRetentionCard(t *testing.T) {
	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	entry := s.RetentionDaysEntry()
	if got := s.RetentionLabel(); got != "1 месяц" || entry.Visible() {
		t.Fatalf("по умолчанию %q, поле видно: %v", got, entry.Visible())
	}

	s.SelectRetention("1 неделя")
	if d := app.RetentionDays(st); d != 7 {
		t.Fatalf("1 неделя: сохранено %d", d)
	}
	s.SelectRetention("Бессрочно")
	if d := app.RetentionDays(st); d != 0 || st.String(app.KeyRetention, "") != "0" {
		t.Fatalf("бессрочно: сохранено %d", d)
	}

	s.SelectRetention("Своё число дней…")
	if !entry.Visible() {
		t.Fatal("поле своего числа не показано")
	}
	entry.SetText("45") // как ввод пользователя
	if d := app.RetentionDays(st); d != 45 || s.RetentionError() != "" {
		t.Fatalf("своё число: сохранено %d, ошибка %q", d, s.RetentionError())
	}
	for _, bad := range []string{"0", "abc", "36501", "-3"} {
		entry.SetText(bad)
		if d := app.RetentionDays(st); d != 45 {
			t.Fatalf("%q: сохранено %d, должно остаться 45", bad, d)
		}
		if s.RetentionError() == "" || entry.Validate() == nil {
			t.Fatalf("%q: нет ошибки в поле", bad)
		}
	}
	entry.SetText("45")
	if s.RetentionError() != "" {
		t.Fatal("ошибка не скрылась после исправления")
	}

	// сохранённое своё число показывается в поле после перезапуска
	s2 := newTestSettings(t, st)
	if s2.RetentionLabel() != "Своё число дней…" || !s2.RetentionDaysEntry().Visible() || s2.RetentionDaysEntry().Text != "45" {
		t.Fatalf("после перезапуска: %q, поле %q", s2.RetentionLabel(), s2.RetentionDaysEntry().Text)
	}
	// готовый вариант выбирается, если число с ним совпадает
	app.SetRetentionDays(st, 90)
	if got := newTestSettings(t, st).RetentionLabel(); got != "3 месяца" {
		t.Fatalf("90 дней показаны как %q", got)
	}
	s3 := newTestSettings(t, st)
	s3.SelectRetention("1 год")
	if d := app.RetentionDays(st); d != 365 || s3.RetentionDaysEntry().Visible() {
		t.Fatalf("1 год: %d", d)
	}
}

// Раздел срока хранения с полем и ошибкой не шире узкого телефона.
func TestRetentionCardFitsPhoneWidth(t *testing.T) {
	setMobile(t, true)
	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	s.SelectRetention("Своё число дней…")
	s.RetentionDaysEntry().SetText("abc")
	if s.RetentionError() == "" {
		t.Fatal("ошибка не показана")
	}
	if w := s.Content().MinSize().Width; w > phoneWidth {
		t.Fatalf("минимальная ширина экрана настроек %.0f > %d", w, phoneWidth)
	}
}

// Направление чтения: по умолчанию «Слева направо», выбор сохраняется и
// показывается после перезапуска.
func TestReaderDirectionCard(t *testing.T) {
	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	if got := s.ReaderDirectionLabel(); got != i18n.T("settings.reader.ltr") {
		t.Fatalf("по умолчанию %q", got)
	}
	s.SelectReaderDirection(app.ReaderDirectionRTL)
	if app.ReaderDirection(st) != app.ReaderDirectionRTL {
		t.Fatal("выбор не сохранён")
	}
	if got := newTestSettings(t, st).ReaderDirectionLabel(); got != i18n.T("settings.reader.rtl") {
		t.Fatalf("после перезапуска %q", got)
	}
}

// Двойной тап: по умолчанию 200%, выбор сохраняется и показывается после перезапуска.
func TestDoubleTapCard(t *testing.T) {
	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	if got := s.DoubleTapLabel(); got != "200%" {
		t.Fatalf("по умолчанию %q", got)
	}
	s.SelectDoubleTap(app.ReaderDoubleTapOff)
	if app.ReaderDoubleTap(st) != 0 {
		t.Fatal("«Выключен» не сохранён")
	}
	if got := newTestSettings(t, st).DoubleTapLabel(); got != i18n.T("settings.reader.double_tap_off") {
		t.Fatalf("после перезапуска %q", got)
	}
	s.SelectDoubleTap("300")
	if app.ReaderDoubleTap(st) != 3 {
		t.Fatal("300% не сохранено")
	}
}

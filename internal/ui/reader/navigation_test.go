package reader

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/app"
	"mangareader/internal/i18n"
	"mangareader/internal/model"
)

// fakeClock подменяет часы читалки; возвращает указатель на текущее время.
func fakeClock(r *Reader) *time.Time {
	now := time.Unix(1_000_000, 0)
	r.now = func() time.Time { return now }
	return &now
}

// shortTapWindow укорачивает окно двойного тапа на время теста.
func shortTapWindow(t *testing.T) {
	old := tapWindow
	tapWindow = 50 * time.Millisecond
	t.Cleanup(func() { tapWindow = old })
}

func wheelDown(r *Reader) {
	r.paged.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -10)})
}

// Одиночный тап по боковой трети листает после окна двойного тапа, а не
// сразу и не через системный интервал двойного щелчка.
func TestTapFlipsAfterWindow(t *testing.T) {
	shortTapWindow(t)
	r, g := setupPages(t, 5, 10, 10)
	r.Open(g)
	size := r.paged.Size()
	test.TapAt(r.paged, fyne.NewPos(size.Width-10, size.Height/2))
	if r.cur != 0 {
		t.Fatal("тап сработал до окна двойного тапа")
	}
	eventually(t, r, "листание тапом", func() bool { return r.cur == 1 })
}

// Двойной тап по боковой зоне увеличивает страницу и не листает.
func TestDoubleTapZoomsWithoutFlip(t *testing.T) {
	shortTapWindow(t)
	r, g := setupPages(t, 5, 10, 10)
	r.Open(g)
	eventually(t, r, "загрузка страницы", func() bool { return r.paged.img.Image != nil })
	size := r.paged.Size()
	p := fyne.NewPos(size.Width-10, size.Height/2)
	test.TapAt(r.paged, p)
	test.TapAt(r.paged, p.AddXY(5, 5))
	if r.paged.scale <= 1 {
		t.Fatal("двойной тап не увеличил страницу")
	}
	time.Sleep(2 * tapWindow)
	pump(r)
	if r.cur != 0 {
		t.Fatalf("двойной тап перелистнул: %d", r.cur)
	}
}

// Второй тап далеко от первого — два одиночных тапа.
func TestFarTapsAreSingle(t *testing.T) {
	shortTapWindow(t)
	r, g := setupPages(t, 5, 10, 10)
	r.Open(g)
	size := r.paged.Size()
	test.TapAt(r.paged, fyne.NewPos(size.Width-10, size.Height/2))
	test.TapAt(r.paged, fyne.NewPos(size.Width-10, size.Height/2+200))
	if r.cur != 1 {
		t.Fatalf("первый тап должен сработать сразу при втором: %d", r.cur)
	}
	eventually(t, r, "второй тап", func() bool { return r.cur == 2 })
}

// Колесо: серия событий за 100 мс — одна страница, события через 200 мс —
// по странице на каждое.
func TestWheelThrottle(t *testing.T) {
	r, g := setupPages(t, 10, 10, 10)
	now := fakeClock(r)
	r.Open(g)
	for i := 0; i < 5; i++ {
		wheelDown(r)
		*now = now.Add(20 * time.Millisecond)
	}
	if r.cur != 1 {
		t.Fatalf("серия событий колеса: страница %d, ждали 1", r.cur)
	}
	*now = now.Add(200 * time.Millisecond)
	wheelDown(r)
	*now = now.Add(200 * time.Millisecond)
	wheelDown(r)
	if r.cur != 3 {
		t.Fatalf("события через 200 мс: страница %d, ждали 3", r.cur)
	}
}

// Колесо на увеличенной странице листает и сбрасывает увеличение.
func TestWheelFlipsZoomed(t *testing.T) {
	r, g := setupPages(t, 5, 10, 10)
	fakeClock(r)
	r.Open(g)
	eventually(t, r, "загрузка страницы", func() bool { return r.paged.img.Image != nil })
	r.paged.doubleTap(fyne.NewPos(100, 100))
	if r.paged.scale <= 1 {
		t.Fatal("страница не увеличена")
	}
	wheelDown(r)
	if r.cur != 1 || r.paged.scale > 1 {
		t.Fatalf("после колеса: страница %d, увеличено %v", r.cur, r.paged.scale > 1)
	}
}

// Автоповтор клавиши ограничен интервалом листания, отдельные нажатия — нет.
func TestKeyRepeatThrottle(t *testing.T) {
	r, g := setupPages(t, 10, 10, 10)
	now := fakeClock(r)
	r.Open(g)
	press := func() { r.KeyDown(fyne.KeyRight); r.TypedKey(fyne.KeyRight) }
	press()
	press() // отдельное нажатие сразу после первого
	if r.cur != 2 {
		t.Fatalf("нажатия: страница %d, ждали 2", r.cur)
	}
	r.TypedKey(fyne.KeyRight) // автоповтор без KeyDown
	r.TypedKey(fyne.KeyRight)
	if r.cur != 2 {
		t.Fatalf("автоповтор чаще интервала: страница %d", r.cur)
	}
	*now = now.Add(flipInterval)
	r.TypedKey(fyne.KeyRight)
	if r.cur != 3 {
		t.Fatalf("автоповтор после интервала: страница %d", r.cur)
	}
}

// Предзагрузка: 2 страницы в направлении листания и 1 в обратном.
func TestPreloadDirection(t *testing.T) {
	r, g := setupPages(t, 20, 10, 10)
	r.Open(g)
	cached := func(p int) bool {
		_, ok := r.loader.Cached(r.paged.request(p, 1))
		return ok
	}
	r.TypedKey(fyne.KeyRight) // страница 2 после листания вперёд
	eventually(t, r, "предзагрузка вперёд", func() bool { return cached(2) && cached(3) && cached(0) })

	r.goTo(10)
	eventually(t, r, "страница 11", func() bool { return cached(10) })
	r.TypedKey(fyne.KeyLeft) // страница 10 после листания назад
	eventually(t, r, "предзагрузка назад", func() bool { return cached(8) && cached(7) && cached(10) })
}

// Справа налево: левая треть и ← — вперёд, → — назад, колесо как обычно,
// бегунок первой страницы у правого края.
func TestRightToLeft(t *testing.T) {
	shortTapWindow(t)
	r, g := setupPages(t, 87, 10, 10)
	app.SetReaderDirection(r.settings, app.ReaderDirectionRTL)
	fakeClock(r)
	r.Open(g)
	if v := r.panel.slider.Value; v != 87 {
		t.Fatalf("ползунок на странице 1: %v, ждали 87", v)
	}
	size := r.paged.Size()
	test.TapAt(r.paged, fyne.NewPos(10, size.Height/2))
	eventually(t, r, "левая треть", func() bool { return r.cur == 1 })
	r.TypedKey(fyne.KeyRight)
	if r.cur != 0 {
		t.Fatalf("→ справа налево: %d", r.cur)
	}
	r.TypedKey(fyne.KeyLeft)
	if r.cur != 1 {
		t.Fatalf("← справа налево: %d", r.cur)
	}
	r.lastFlip = time.Time{} // часы стоят: колесо не должно упереться в интервал
	wheelDown(r)
	if r.cur != 2 {
		t.Fatalf("колесо справа налево: %d", r.cur)
	}
}

// Ввод номера страницы: переход, прижатие к границам, отмена.
func TestPageInput(t *testing.T) {
	r, g := setupPages(t, 87, 10, 10)
	r.Open(g)
	r.goTo(11)
	enter := func(text string, key fyne.KeyName) {
		t.Helper()
		r.panel.editPage()
		if !r.panel.entry.Visible() || r.win.Canvas().Focused() != r.panel.entry {
			t.Fatal("поле ввода не открыто или без фокуса")
		}
		if r.panel.entry.Text != "12" && r.cur == 11 {
			t.Fatalf("в поле %q, ждали текущий номер", r.panel.entry.Text)
		}
		if text == "" {
			r.panel.entry.SetText("")
		} else {
			test.Type(r.panel.entry, text) // поверх выделенного номера
		}
		r.panel.entry.TypedKey(&fyne.KeyEvent{Name: key})
	}

	enter("34", fyne.KeyReturn)
	if r.cur != 33 || r.panel.pageLbl.Text != "34 / 87" {
		t.Fatalf("34: страница %d, надпись %q", r.cur, r.panel.pageLbl.Text)
	}
	if r.panel.entry.Visible() || r.win.Canvas().Focused() != nil {
		t.Fatal("после ввода поле должно закрыться и отдать фокус")
	}
	enter("999", fyne.KeyReturn)
	if r.cur != 86 {
		t.Fatalf("999: страница %d", r.cur)
	}
	enter("0", fyne.KeyReturn)
	if r.cur != 0 {
		t.Fatalf("0: страница %d", r.cur)
	}
	enter("4a5", fyne.KeyReturn) // буквы не вводятся
	if r.cur != 44 {
		t.Fatalf("4a5: страница %d", r.cur)
	}
	enter("70", fyne.KeyEscape)
	if r.cur != 44 || r.panel.entry.Visible() || !r.Visible() {
		t.Fatalf("Esc: страница %d, поле %v, читалка %v", r.cur, r.panel.entry.Visible(), r.Visible())
	}
	enter("", fyne.KeyReturn)
	if r.cur != 44 {
		t.Fatalf("пустое значение: страница %d", r.cur)
	}
}

// G показывает панель и переводит фокус в поле ввода номера.
func TestKeyGFocusesPageInput(t *testing.T) {
	r, g := setupPages(t, 5, 10, 10)
	r.Open(g)
	r.TypedKey(fyne.KeyG)
	if !r.panel.layer.Visible() || r.win.Canvas().Focused() != r.panel.entry {
		t.Fatal("G должна открыть панель и поле ввода")
	}
}

// Ползунок вживую: страница меняется сразу, номер на панели — после
// отпускания, пройденные страницы не декодируются.
func TestSliderPreview(t *testing.T) {
	r, g := setupPages(t, 87, 10, 10)
	r.Open(g)
	r.goTo(11)
	eventually(t, r, "страница 12", func() bool { return r.paged.img.Image != nil })
	r.panel.show()

	drag := func(v float64) { // перемещение без отпускания (SetValue — с отпусканием)
		r.panel.slider.Value = v
		r.panel.slider.OnChanged(v)
	}
	drag(34)
	if r.cur != 33 || r.paged.page != 33 {
		t.Fatalf("во время перемещения показана страница %d", r.paged.page)
	}
	if r.panel.pageLbl.Text != "12 / 87" {
		t.Fatalf("номер во время перемещения: %q", r.panel.pageLbl.Text)
	}
	if !r.panel.bubble.Visible() || r.panel.bubbleTxt.Text != "34" {
		t.Fatalf("подсказка: видна %v, текст %q", r.panel.bubble.Visible(), r.panel.bubbleTxt.Text)
	}
	r.panel.slider.OnChangeEnded(34)
	if r.panel.pageLbl.Text != "34 / 87" || r.panel.bubble.Visible() {
		t.Fatalf("после отпускания: %q, подсказка %v", r.panel.pageLbl.Text, r.panel.bubble.Visible())
	}

	// быстрое движение: устаревшие запросы не выполняются
	eventually(t, r, "загрузки утихли", func() bool { return r.paged.img.Image != nil })
	time.Sleep(100 * time.Millisecond)
	before := r.loader.Decodes()
	for v := 40; v <= 87; v++ {
		drag(float64(v))
	}
	r.panel.slider.OnChangeEnded(87)
	eventually(t, r, "последняя страница", func() bool {
		return r.paged.img.Image != nil && r.paged.want.Page == g.Pages[86].Name
	})
	if n := r.loader.Decodes() - before; n > 10 {
		t.Fatalf("быстрое движение ползунка: %d декодирований", n)
	}
}

// Кнопка «Домой» вызывает обработчик; без него — закрывает читалку.
func TestHome(t *testing.T) {
	r, g := setupPages(t, 5, 10, 10)
	r.Open(g)
	r.home()
	if r.Visible() {
		t.Fatal("без обработчика «Домой» закрывает читалку")
	}
	called := 0
	r.SetOnHome(func() { called++ })
	r.Open(g)
	r.home()
	if called != 1 {
		t.Fatalf("обработчик «Домой» вызван %d раз", called)
	}
}

// Завершение чтения: подсказка, подтверждение после паузы, защита от
// инерции колеса, отмена возвратом.
func TestFinish(t *testing.T) {
	r, g := setupPages(t, 3, 10, 10)
	now := fakeClock(r)
	var hints []string
	r.notify = func(s string) { hints = append(hints, s) }
	var finished []model.Gallery
	r.SetOnFinished(func(fg model.Gallery) { finished = append(finished, fg) })
	r.Open(g)
	r.TypedKey(fyne.KeyEnd)

	r.TypedKey(fyne.KeyRight)
	if !r.Visible() || len(hints) != 1 || hints[0] != i18n.T("reader.end_hint") {
		t.Fatalf("первая попытка: открыта %v, подсказки %q", r.Visible(), hints)
	}
	// инерция колеса: события каждые 100 мс не закрывают
	for i := 0; i < 3; i++ {
		*now = now.Add(100 * time.Millisecond)
		r.lastFlip = time.Time{}
		wheelDown(r)
	}
	if !r.Visible() {
		t.Fatal("серия событий колеса закрыла читалку")
	}
	// возврат на предыдущую страницу отменяет ожидание
	*now = now.Add(time.Second)
	r.TypedKey(fyne.KeyLeft)
	r.TypedKey(fyne.KeyRight)
	*now = now.Add(time.Second)
	r.TypedKey(fyne.KeyRight)
	if !r.Visible() || len(hints) != 2 {
		t.Fatalf("после возврата: открыта %v, подсказок %d", r.Visible(), len(hints))
	}
	// подтверждение через 1 с
	*now = now.Add(time.Second)
	r.TypedKey(fyne.KeyRight)
	if r.Visible() || len(finished) != 1 || finished[0].Key != g.Key {
		t.Fatalf("подтверждение: открыта %v, завершений %d", r.Visible(), len(finished))
	}
}

// Ожидание подтверждения истекает через finishTTL: снова подсказка.
func TestFinishExpires(t *testing.T) {
	r, g := setupPages(t, 2, 10, 10)
	now := fakeClock(r)
	hints := 0
	r.notify = func(string) { hints++ }
	r.Open(g)
	r.TypedKey(fyne.KeyEnd)
	r.TypedKey(fyne.KeyRight)
	*now = now.Add(finishTTL + time.Second)
	r.TypedKey(fyne.KeyRight)
	if !r.Visible() || hints != 2 {
		t.Fatalf("после истечения: открыта %v, подсказок %d", r.Visible(), hints)
	}
}

// Лента: блок «Конец» после последней страницы, кнопка завершает чтение;
// PgDn у конца ленты ведёт себя как «вперёд» на последней странице.
func TestStripFinish(t *testing.T) {
	r, g := setupPages(t, 3, 100, 100)
	now := fakeClock(r)
	finished := 0
	r.SetOnFinished(func(model.Gallery) { finished++ })
	r.setMode(ModeStrip)
	r.Open(g)
	v := r.strip
	eventually(t, r, "раскладка ленты", func() bool { return v.ready })
	if !v.end.Visible() {
		t.Fatal("блок «Конец» не показан")
	}
	if got := v.contentHeight(); got != v.prefix[len(v.prefix)-1]+endHeight {
		t.Fatalf("высота ленты %v не учитывает блок «Конец»", got)
	}
	v.scrollBy(v.contentHeight())
	if !v.atBottom() {
		t.Fatal("лента не у конца")
	}
	r.TypedKey(fyne.KeyPageDown)
	if !r.Visible() {
		t.Fatal("первый PgDn у конца ленты закрыл читалку")
	}
	*now = now.Add(time.Second)
	r.TypedKey(fyne.KeyPageDown)
	if r.Visible() || finished != 1 {
		t.Fatalf("второй PgDn: открыта %v, завершений %d", r.Visible(), finished)
	}

	r.Open(g)
	eventually(t, r, "раскладка ленты", func() bool { return v.ready })
	btn := v.end.Objects[0].(*fyne.Container).Objects[1].(*widget.Button)
	test.Tap(btn)
	if r.Visible() || finished != 2 {
		t.Fatalf("кнопка «Завершить чтение»: открыта %v, завершений %d", r.Visible(), finished)
	}
}

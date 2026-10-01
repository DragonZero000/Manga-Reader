package reader

import (
	"math"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
)

// imagePoint — точка изображения (в долях его размера) под точкой p экрана.
func imagePoint(v *pagedView, p fyne.Position) (float32, float32) {
	r := v.displayRect()
	return (p.X - r.pos.X) / r.size.Width, (p.Y - r.pos.Y) / r.size.Height
}

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func withCtrl(t *testing.T) {
	old := ctrlPressed
	ctrlPressed = func() bool { return true }
	t.Cleanup(func() { ctrlPressed = old })
}

func loaded(t *testing.T, r *Reader) {
	t.Helper()
	eventually(t, r, "загрузка страницы", func() bool { return r.paged.img.Image != nil })
}

// Масштаб сохраняет точку привязки; границы 100–500%; смена страницы — 100%.
func TestZoomAnchorAndBounds(t *testing.T) {
	r, g := setupPages(t, 3, 10, 10)
	r.Open(g)
	loaded(t, r)
	v := r.paged
	p := fyne.NewPos(400, 300)
	x0, y0 := imagePoint(v, p)
	v.zoomTo(3, p)
	if x, y := imagePoint(v, p); v.scale != 3 || !near(x, x0) || !near(y, y0) {
		t.Fatalf("увеличение: масштаб %v, точка (%v,%v) → (%v,%v)", v.scale, x0, y0, x, y)
	}
	v.zoomTo(1.5, p)
	if x, y := imagePoint(v, p); !near(x, x0) || !near(y, y0) {
		t.Fatalf("уменьшение сдвинуло точку: (%v,%v) → (%v,%v)", x0, y0, x, y)
	}
	v.zoomTo(0.5, p)
	if v.scale != 1 {
		t.Fatalf("ниже 100%%: %v", v.scale)
	}
	for i := 0; i < 20; i++ {
		r.Zoom(ZoomIn)
	}
	if v.scale != maxScale {
		t.Fatalf("выше 500%%: %v", v.scale)
	}
	r.TypedKey(fyne.KeyRight)
	if v.scale != 1 || r.cur != 1 {
		t.Fatalf("смена страницы: масштаб %v, страница %d", v.scale, r.cur)
	}
}

// Ctrl+колесо: шаг ×1.25 с точкой под курсором, страница не меняется.
func TestCtrlWheel(t *testing.T) {
	withCtrl(t)
	r, g := setupPages(t, 3, 10, 10)
	r.Open(g)
	loaded(t, r)
	v := r.paged
	p := fyne.NewPos(450, 500)
	x0, _ := imagePoint(v, p)
	for i := 0; i < 2; i++ {
		v.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: p}, Scrolled: fyne.NewDelta(0, 10)})
	}
	// по высоте квадратная страница в портретном окне ещё меньше окна —
	// центрируется; по ширине точка под курсором на месте
	if x, _ := imagePoint(v, p); !near(v.scale, 1.5625) || !near(x, x0) || r.cur != 0 {
		t.Fatalf("масштаб %v, точка по X %v → %v, страница %d", v.scale, x0, x, r.cur)
	}
	v.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: p}, Scrolled: fyne.NewDelta(0, 10)}) // 195%: больше окна по обеим осям
	v.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: p}, Scrolled: fyne.NewDelta(0, 10)}) // 244%
	x2, y2 := imagePoint(v, p)
	v.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: p}, Scrolled: fyne.NewDelta(0, 10)}) // 305%
	if x, y := imagePoint(v, p); !near(x, x2) || !near(y, y2) {
		t.Fatalf("точка под курсором сдвинулась: (%v,%v) → (%v,%v)", x2, y2, x, y)
	}
	for i := 0; i < 3; i++ {
		v.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: p}, Scrolled: fyne.NewDelta(0, -10)})
	}
	v.Scrolled(&fyne.ScrollEvent{PointEvent: fyne.PointEvent{Position: p}, Scrolled: fyne.NewDelta(0, -10)})
	if !near(v.scale, 1.25) {
		t.Fatalf("Ctrl+колесо вниз: %v", v.scale)
	}
}

// Сочетания: Ctrl+«=», Ctrl+«+», Ctrl+«−», Ctrl+0; в ленте — ничего.
func TestZoomShortcuts(t *testing.T) {
	r, g := setupPages(t, 3, 10, 10)
	r.Open(g)
	loaded(t, r)
	sc := func(k fyne.KeyName) {
		r.ZoomShortcut(&desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault})
	}
	sc(fyne.KeyMinus)
	if r.Scale() != 1 {
		t.Fatalf("Ctrl+− на 100%%: %v", r.Scale())
	}
	sc(fyne.KeyEqual)
	sc(fyne.KeyPlus)
	if !near(r.Scale(), 1.5625) || r.badge.Text() != "156 %" {
		t.Fatalf("Ctrl+= и Ctrl++: %v, индикатор %q", r.Scale(), r.badge.Text())
	}
	sc(fyne.Key0)
	if r.Scale() != 1 {
		t.Fatalf("Ctrl+0: %v", r.Scale())
	}
	if n := len(r.ZoomShortcuts()); n != 4 {
		t.Fatalf("сочетаний %d", n)
	}

	r.setMode(ModeStrip)
	sc(fyne.KeyEqual)
	if r.Scale() != 1 || r.badge.Visible() {
		t.Fatal("в ленте масштаб не меняется")
	}
}

func touch(id int, x, y float32) *mobile.TouchEvent {
	return &mobile.TouchEvent{PointEvent: fyne.PointEvent{Position: fyne.NewPos(x, y)}, ID: id}
}

// Щипок: разведение пальцев вдвое — 200%, середина на месте; страница и
// панель не меняются; тап после щипка игнорируется, следующий жест работает.
func TestPinch(t *testing.T) {
	r, g := setupPages(t, 3, 10, 10)
	r.Open(g)
	loaded(t, r)
	r.doubleTap = 0 // тапы без окна ожидания — проверка сразу
	v := r.paged
	mid := fyne.NewPos(300, 400)
	x0, y0 := imagePoint(v, mid)

	v.TouchDown(touch(0, 250, 400))
	v.TouchDown(touch(1, 350, 400))
	v.Dragged(&fyne.DragEvent{Dragged: fyne.NewDelta(-50, 0)}) // первый палец — тоже перетаскивание
	v.TouchMoved(touch(0, 200, 400))
	v.TouchMoved(touch(1, 400, 400))
	if x, y := imagePoint(v, mid); !near(v.scale, 2) || !near(x, x0) || !near(y, y0) {
		t.Fatalf("щипок: масштаб %v, середина (%v,%v) → (%v,%v)", v.scale, x0, y0, x, y)
	}
	v.TouchUp(touch(1, 400, 400))
	v.Tapped(&fyne.PointEvent{Position: fyne.NewPos(400, 400)})
	v.TouchUp(touch(0, 200, 400))
	v.DragEnd()
	if r.cur != 0 || r.panel.layer.Visible() || !near(v.scale, 2) {
		t.Fatalf("после щипка: страница %d, панель %v, масштаб %v", r.cur, r.panel.layer.Visible(), v.scale)
	}

	// новый жест: тап по центру увеличенной страницы открывает панель
	v.TouchDown(touch(0, 300, 400))
	v.TouchUp(touch(0, 300, 400))
	v.Tapped(&fyne.PointEvent{Position: fyne.NewPos(300, 400)})
	if !r.panel.layer.Visible() {
		t.Fatal("тап после щипка не работает")
	}
}

// Двойной тап: значение из настроек, возврат к 100%.
func TestDoubleTapSetting(t *testing.T) {
	shortTapWindow(t)
	r, g := setupPages(t, 5, 10, 10)
	app.SetReaderDoubleTap(r.settings, "300")
	r.Open(g)
	loaded(t, r)
	p := fyne.NewPos(300, 400)
	test.TapAt(r.paged, p)
	test.TapAt(r.paged, p)
	if r.Scale() != 3 {
		t.Fatalf("двойной тап 300%%: %v", r.Scale())
	}
	r.paged.zoomTo(2.6, p)
	test.TapAt(r.paged, p)
	test.TapAt(r.paged, p)
	if r.Scale() != 1 {
		t.Fatalf("возврат двойным тапом: %v", r.Scale())
	}
}

// Выключенный двойной тап: тап листает сразу, два быстрых тапа — две страницы.
func TestDoubleTapOff(t *testing.T) {
	r, g := setupPages(t, 5, 10, 10)
	app.SetReaderDoubleTap(r.settings, app.ReaderDoubleTapOff)
	r.Open(g)
	loaded(t, r)
	size := r.paged.Size()
	p := fyne.NewPos(size.Width-10, size.Height/2)
	test.TapAt(r.paged, p)
	if r.cur != 1 {
		t.Fatalf("тап без ожидания: %d", r.cur)
	}
	test.TapAt(r.paged, p)
	if r.cur != 2 || r.Scale() != 1 {
		t.Fatalf("два тапа: страница %d, масштаб %v", r.cur, r.Scale())
	}
}

// Индикатор: виден при увеличении, при 100% скрывается; над панелью; тап — 100%.
func TestZoomBadge(t *testing.T) {
	r, g := setupPages(t, 3, 10, 10)
	r.Open(g)
	loaded(t, r)
	r.Zoom(ZoomIn)
	if !r.badge.Visible() || r.badge.Text() != "125 %" {
		t.Fatalf("индикатор: виден %v, %q", r.badge.Visible(), r.badge.Text())
	}
	view := r.viewSize()
	if b := r.badge; b.Position().X+b.Size().Width > view.Width || b.Position().Y+b.Size().Height > view.Height ||
		b.Position().X < view.Width/2 || b.Position().Y < view.Height/2 {
		t.Fatalf("индикатор не в правом нижнем углу: %v %v", b.Position(), b.Size())
	}
	r.panel.show()
	if b := r.badge; b.Position().Y+b.Size().Height > view.Height-r.panel.bottomBar.Size().Height {
		t.Fatalf("индикатор под панелью: %v", b.Position())
	}
	r.panel.hide()

	r.paged.zoomTo(2.5, r.paged.center())
	test.Tap(r.badge)
	if r.Scale() != 1 || r.cur != 0 || r.panel.layer.Visible() {
		t.Fatalf("тап по индикатору: масштаб %v, страница %d, панель %v", r.Scale(), r.cur, r.panel.layer.Visible())
	}
	if r.badge.Text() != "100 %" {
		t.Fatalf("индикатор после сброса: %q", r.badge.Text())
	}
	eventually(t, r, "индикатор скрыт", func() bool { return !r.badge.Visible() })
}

// Чёткая версия загружается после паузы, одна на серию изменений.
func TestSharpen(t *testing.T) {
	r, g := setupPages(t, 1, 2000, 2000)
	r.Open(g)
	loaded(t, r)
	time.Sleep(100 * time.Millisecond)
	pump(r)
	v := r.paged
	fitW := v.imgPx.W
	before := r.loader.Decodes()
	for i := 0; i < 10; i++ {
		r.Zoom(ZoomIn)
		r.Zoom(ZoomOut)
	}
	r.Zoom(ZoomIn)
	r.Zoom(ZoomIn) // 156%
	if n := r.loader.Decodes() - before; n != 0 {
		t.Fatalf("загрузки во время изменения масштаба: %d", n)
	}
	eventually(t, r, "чёткая версия", func() bool { return v.imgPx.W > fitW })
	time.Sleep(2 * sharpDelay)
	pump(r)
	if n := r.loader.Decodes() - before; n != 1 {
		t.Fatalf("загрузок после паузы: %d, ждали 1", n)
	}
	if !near(v.scale, 1.5625) {
		t.Fatalf("масштаб изменился после догрузки: %v", v.scale)
	}
}

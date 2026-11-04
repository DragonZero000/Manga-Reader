package screens

import (
	"io"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/model"
	"mangareader/internal/storage"
	"mangareader/internal/thumbs"
)

// fakeTarget — прокрутка с наибольшим смещением maxOff.
type fakeTarget struct{ off, maxOff float32 }

func (f *fakeTarget) ScrollOffset() float32     { return f.off }
func (f *fakeTarget) MaxScrollOffset() float32  { return f.maxOff }
func (f *fakeTarget) ScrollToOffset(y float32)  { f.off = max(0, min(y, f.maxOff)) }
func (f *fakeTarget) Draggable() fyne.Draggable { return DragOf(f) }

// smootherWithTicks — сглаживатель, анимации которого тест ведёт сам: tick(f)
// — такт последней запущенной анимации.
func smootherWithTicks(target ScrollTarget) (*WheelSmoother, func(float32)) {
	w := NewWheelSmoother(target)
	var last *fyne.Animation
	w.startAnim = func(a *fyne.Animation) { last = a }
	return w, func(f float32) { last.Tick(f) }
}

func wheel(w *WheelSmoother, dy float32) {
	w.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, dy)})
}

// Один щелчок: то же итоговое смещение, что сдвигает Scroll Fyne за одно
// событие, но плавно.
func TestWheelOneClick(t *testing.T) {
	test.NewTempApp(t)
	// эталон: Scroll Fyne без сглаживания
	content := widget.NewLabel("")
	content.Resize(fyne.NewSize(100, 1000))
	sc := container.NewVScroll(container.NewGridWrap(fyne.NewSize(100, 1000), content))
	sc.Resize(fyne.NewSize(100, 200))
	sc.Scrolled(&fyne.ScrollEvent{Scrolled: fyne.NewDelta(0, -30)})
	want := sc.Offset.Y
	if want != 30 {
		t.Fatalf("эталон: %v", want)
	}

	target := &fakeTarget{maxOff: 800}
	w, tick := smootherWithTicks(target)
	wheel(w, -30)
	if target.off != 0 {
		t.Fatal("смещение не должно меняться скачком")
	}
	tick(0.5)
	if target.off <= 0 || target.off >= want {
		t.Fatalf("середина анимации: %v", target.off)
	}
	tick(1)
	if target.off != want {
		t.Fatalf("итог %v, ждали %v", target.off, want)
	}
}

// Три щелчка подряд: сумма смещений, без отката назад.
func TestWheelClicksAccumulate(t *testing.T) {
	test.NewTempApp(t)
	target := &fakeTarget{maxOff: 800}
	w, tick := smootherWithTicks(target)
	prev := float32(0)
	for i := range 3 {
		wheel(w, -30)
		tick(0.5)
		if target.off < prev {
			t.Fatalf("щелчок %d: откат %v → %v", i+1, prev, target.off)
		}
		prev = target.off
	}
	tick(1)
	if target.off != 90 {
		t.Fatalf("итог %v, ждали 90", target.off)
	}
	// обратно — вверх от текущего
	wheel(w, 30)
	tick(1)
	if target.off != 60 {
		t.Fatalf("вверх: %v", target.off)
	}
}

// Анимация останавливается на границе содержимого.
func TestWheelBounds(t *testing.T) {
	test.NewTempApp(t)
	target := &fakeTarget{off: 40, maxOff: 50}
	w, tick := smootherWithTicks(target)
	wheel(w, -30)
	wheel(w, -30)
	tick(1)
	if target.off != 50 {
		t.Fatalf("низ: %v", target.off)
	}
	wheel(w, 100)
	tick(1)
	if target.off != 0 {
		t.Fatalf("верх: %v", target.off)
	}
}

// Нажатие проходит сквозь сглаживатель к карточке, колесо получает
// сглаживатель.
func TestWheelTransparentForTaps(t *testing.T) {
	setMobile(t, false)
	a := test.NewTempApp(t)
	var opened []model.Key
	th := thumbs.New(func(model.Key, string) (io.ReadCloser, error) { return nil, io.EOF }, 1<<20, 1)
	g := newGalleryGrid(th, NewGridMetrics(storage.NewMemSettings()),
		&GalleryActions{Open: func(gal model.Gallery) { opened = append(opened, gal.Key) }})
	g.do = func(func()) {} // обложки не нужны
	g.items = []model.Gallery{galleryOf("a.zip"), galleryOf("b.zip")}
	if _, ok := g.Widget().(*fyne.Container); !ok {
		t.Fatal("на ПК сетка должна быть со сглаживателем колеса")
	}
	w := a.NewWindow("t")
	w.SetContent(g.Widget())
	w.Resize(fyne.NewSize(600, 600))
	g.grid.Refresh()
	test.TapCanvas(w.Canvas(), fyne.NewPos(40, 60))
	if len(opened) != 1 || opened[0] != model.LocalKey("a.zip") {
		t.Fatalf("открыто %v", opened)
	}
}

package reader

import (
	"image"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

// testWarmer — прогрев с подменёнными часами; такты — только вызовом step.
func testWarmer(clock *time.Time) *warmer {
	w := newWarmer(false)
	w.now = func() time.Time { return *clock }
	w.startAnim = func() {}
	return w
}

func warmImage(w, h int) *canvas.Image {
	img := canvas.NewImageFromImage(image.NewRGBA(image.Rect(0, 0, w, h)))
	img.Resize(fyne.NewSize(float32(w), float32(h)))
	return img
}

// После прогрева объект на прежнем месте с прежним изображением; на такт
// прогрева он сдвинут в точку и, если был скрыт, показан.
func TestWarmerMovesForOneTick(t *testing.T) {
	test.NewTempApp(t)
	clock := time.Unix(1e6, 0)
	w := testWarmer(&clock)
	img := warmImage(100, 100)
	home := fyne.NewPos(0, 5000)
	img.Move(home)
	img.Hide()
	src := img.Image
	spot := fyne.NewPos(0, 799)
	w.add(img, func() (fyne.Position, warmState) { return spot, warmNow }, nil)

	clock = clock.Add(16 * time.Millisecond)
	w.step(clock)
	if img.Position() != spot || !img.Visible() || w.warmed != 1 {
		t.Fatalf("такт прогрева: %v, виден %v, прогрето %d", img.Position(), img.Visible(), w.warmed)
	}
	clock = clock.Add(16 * time.Millisecond)
	w.step(clock)
	if img.Position() != home || img.Visible() || img.Image != src {
		t.Fatalf("после прогрева: %v, виден %v", img.Position(), img.Visible())
	}
	if w.running {
		t.Fatal("пустая очередь должна останавливать такты")
	}
}

// Время прогрева одинаковой очереди не зависит от частоты тактов.
func TestWarmerBudgetIndependentOfFrameRate(t *testing.T) {
	test.NewTempApp(t)
	measure := func(dt time.Duration) time.Duration {
		clock := time.Unix(1e6, 0)
		w := testWarmer(&clock)
		for range 10 {
			img := warmImage(1440, 2048) // кусок ленты ≈ 2,9 Мпикс
			w.add(img, func() (fyne.Position, warmState) { return fyne.Position{}, warmNow }, nil)
		}
		start := clock
		for i := 0; w.warmed < 10; i++ {
			if i > 100000 {
				t.Fatal("очередь не опустела")
			}
			clock = clock.Add(dt)
			w.step(clock)
		}
		return clock.Sub(start)
	}
	fast, slow := measure(2*time.Millisecond), measure(16700*time.Microsecond)
	if d := fast - slow; d > 17*time.Millisecond || d < -17*time.Millisecond {
		t.Fatalf("500 Гц: %v, 60 Гц: %v", fast, slow)
	}
	want := time.Duration(10 * 1440 * 2048 / warmRateDesktop * float64(time.Second))
	if fast < want || fast > want+50*time.Millisecond {
		t.Fatalf("прогрев за %v, ждали ~%v", fast, want)
	}
}

// Время кадра бывает впереди time.Now: шаг назад по часам не уменьшает и не
// удваивает бюджет.
func TestWarmerClockBack(t *testing.T) {
	test.NewTempApp(t)
	run := func(back bool) float64 {
		clock := time.Unix(1e6, 0)
		w := testWarmer(&clock)
		w.last = clock
		// ждущий объект держит очередь: бюджет копится
		w.add(warmImage(10, 10), func() (fyne.Position, warmState) { return fyne.Position{}, warmLater }, nil)
		w.step(clock.Add(10 * time.Millisecond))
		if back {
			w.step(clock)
		}
		w.step(clock.Add(60 * time.Millisecond))
		return w.allowance
	}
	if a, b := run(false), run(true); a == 0 || a != b {
		t.Fatalf("бюджет %v, с шагом назад — %v", a, b)
	}
}

// Объект, который пока нельзя прогреть, ждёт; ненужный — удаляется.
func TestWarmerLaterAndDrop(t *testing.T) {
	test.NewTempApp(t)
	clock := time.Unix(1e6, 0)
	w := testWarmer(&clock)
	state := warmLater
	img := warmImage(10, 10)
	w.add(img, func() (fyne.Position, warmState) { return fyne.Position{}, state }, nil)
	for range 5 {
		clock = clock.Add(16 * time.Millisecond)
		w.step(clock)
	}
	if w.warmed != 0 || len(w.queue) != 1 {
		t.Fatalf("отложенный: прогрето %d, в очереди %d", w.warmed, len(w.queue))
	}
	state = warmDrop
	clock = clock.Add(16 * time.Millisecond)
	w.step(clock)
	if len(w.queue) != 0 || w.running {
		t.Fatalf("ненужный: в очереди %d", len(w.queue))
	}
}

// Перелистывание на заранее загруженную страницу не присваивает изображение
// и не меняет размер объекта: только переключает видимость.
func TestPagedFlipToPreparedPage(t *testing.T) {
	r, g := setupPages(t, 5, 400, 600)
	r.warm.startAnim = func() {}
	r.setMode(ModePaged)
	r.Open(g)
	v := r.paged
	eventually(t, r, "страница 1 и соседняя", func() bool {
		return v.cur != nil && len(v.wanted) > 1 && v.objFor(v.wanted[1]) != nil
	})
	next := v.objFor(v.wanted[1])
	if next.img.Visible() || next == v.cur {
		t.Fatal("соседняя страница должна быть скрыта")
	}
	src, size := next.img.Image, next.img.Size()
	v.step(1)
	if v.cur != next || next.img.Image != src || next.img.Size() != size || !next.img.Visible() {
		t.Fatalf("после перелистывания: текущий %v, тот же объект %v, размер %v → %v",
			v.cur != nil, v.cur == next, size, next.img.Size())
	}
	if r.cur != 1 {
		t.Fatalf("страница %d", r.cur)
	}
}

// Лента: куски страницы на экране не ждут прогрева, страницы ниже экрана —
// в очереди прогрева.
func TestStripWarmsOffscreenOnly(t *testing.T) {
	r, g := setupPages(t, 6, 400, 600)
	r.warm.startAnim = func() {}
	r.setMode(ModeStrip)
	r.Open(g)
	v := r.strip
	eventually(t, r, "страницы окна загружены", func() bool {
		if !v.ready || len(v.items) < 2 {
			return false
		}
		for _, it := range v.items {
			if !it.loaded {
				return false
			}
		}
		return true
	})
	queued := map[*canvas.Image]bool{}
	for _, it := range r.warm.queue {
		queued[it.img] = true
	}
	off, h := v.scroll.Offset.Y, v.scroll.Size().Height
	var below int
	for page, it := range v.items {
		top := v.prefix[page]
		onScreen := top < off+h && top+v.heights[page] > off
		for i, img := range it.parts {
			if i >= len(it.res.Parts) {
				break
			}
			if onScreen && queued[img] {
				t.Fatalf("кусок страницы %d на экране ждёт прогрева", page)
			}
			if top >= off+h {
				below++
				if !queued[img] {
					t.Fatalf("кусок страницы %d ниже экрана не в очереди", page)
				}
			}
		}
	}
	if below == 0 {
		t.Fatal("нет страниц ниже экрана")
	}

	// прогрев: кусок ниже экрана касается нижней строки экрана, затем на месте
	it := r.warm.queue[0]
	home := it.img.Position()
	r.warm.step(time.Now().Add(time.Second))
	if it.img.Position() == home {
		t.Fatal("кусок не сдвинут для прогрева")
	}
	r.warm.step(time.Now().Add(2 * time.Second))
	if it.img.Position() != home {
		t.Fatal("кусок не вернулся на место")
	}
}

package screens

import (
	"math/rand/v2"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/storage"
)

func testPicker(mode string, seed uint64) *randomPicker {
	p := newRandomPicker(mode)
	p.rnd = rand.New(rand.NewPCG(seed, seed))
	return p
}

func galleries(ids ...string) []model.Gallery {
	items := make([]model.Gallery, len(ids))
	for i, id := range ids {
		items[i] = galleryOf(id)
	}
	return items
}

func pick(t *testing.T, p *randomPicker, items []model.Gallery) model.Key {
	t.Helper()
	g, ok := p.Pick(items)
	if !ok {
		t.Fatal("из непустого набора ничего не выбрано")
	}
	return g.Key
}

func TestRandomPickerEmpty(t *testing.T) {
	for _, mode := range []string{app.RandomModeRepeat, app.RandomModeNoRepeat} {
		if _, ok := testPicker(mode, 1).Pick(nil); ok {
			t.Fatalf("%s: из пустого набора выбрана галерея", mode)
		}
	}
}

func TestRandomPickerSingle(t *testing.T) {
	items := galleries("a.zip")
	for _, mode := range []string{app.RandomModeRepeat, app.RandomModeNoRepeat} {
		p := testPicker(mode, 1)
		for range 5 {
			if k := pick(t, p, items); k != items[0].Key {
				t.Fatalf("%s: выбрана %v", mode, k)
			}
		}
	}
}

// С повторами каждая галерея выпадает примерно одинаково часто.
func TestRandomPickerRepeatUniform(t *testing.T) {
	items := galleries("a.zip", "b.zip", "c.zip", "d.zip")
	p := testPicker(app.RandomModeRepeat, 7)
	const n = 40000
	counts := map[model.Key]int{}
	for range n {
		counts[pick(t, p, items)]++
	}
	want := n / len(items)
	for _, g := range items {
		if c := counts[g.Key]; c < want*95/100 || c > want*105/100 {
			t.Fatalf("%s выпала %d раз из %d, ожидалось около %d", g.Key.ID, c, n, want)
		}
	}
}

// Без повторов: в круге каждая галерея один раз; новый круг не начинается
// с последней выпавшей.
func TestRandomPickerNoRepeatRounds(t *testing.T) {
	items := galleries("a.zip", "b.zip", "c.zip")
	for seed := range uint64(50) {
		p := testPicker(app.RandomModeNoRepeat, seed)
		var prev model.Key
		for round := range 5 {
			seen := map[model.Key]bool{}
			for i := range len(items) {
				k := pick(t, p, items)
				if seen[k] {
					t.Fatalf("seed %d, круг %d: %s выпала повторно", seed, round, k.ID)
				}
				if i == 0 && round > 0 && k == prev {
					t.Fatalf("seed %d: круг %d начался с последней выпавшей %s", seed, round, k.ID)
				}
				seen[k] = true
				prev = k
			}
		}
	}
}

// Смена набора начинает круг заново: прежний остаток не учитывается.
func TestRandomPickerNoRepeatSetChanged(t *testing.T) {
	a := galleries("a.zip", "b.zip", "c.zip")
	b := galleries("a.zip", "b.zip", "c.zip", "d.zip")
	for seed := range uint64(50) {
		p := testPicker(app.RandomModeNoRepeat, seed)
		pick(t, p, a)
		pick(t, p, a)
		seen := map[model.Key]bool{}
		for range len(b) {
			seen[pick(t, p, b)] = true
		}
		if len(seen) != len(b) {
			t.Fatalf("seed %d: после смены набора за круг выпали %d из %d", seed, len(seen), len(b))
		}
	}
}

// Смена режима начинает круг заново.
func TestRandomPickerSetMode(t *testing.T) {
	items := galleries("a.zip", "b.zip", "c.zip")
	p := testPicker(app.RandomModeNoRepeat, 3)
	pick(t, p, items)
	p.SetMode(app.RandomModeNoRepeat)
	seen := map[model.Key]bool{}
	for range len(items) {
		seen[pick(t, p, items)] = true
	}
	if len(seen) != len(items) {
		t.Fatalf("после смены режима за круг выпали %d из %d", len(seen), len(items))
	}
}

// Кнопка 🎲 в библиотеке: неактивна без галерей, открывает галерею из сетки.
func TestLibraryRandomButton(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	var opened []model.Gallery
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, func(g model.Gallery) { opened = append(opened, g) }, nil)
	a.NewWindow("t").SetContent(l.Content())

	if !l.random.btn.Disabled() {
		t.Fatal("без галерей 🎲 должна быть неактивна")
	}
	l.applyScan(library.ScanResult{}, nil)
	if !l.random.btn.Disabled() {
		t.Fatal("после пустого сканирования 🎲 должна быть неактивна")
	}

	// библиотека заполнилась после сканирования
	items := galleries("a.zip", "b.zip", "c.zip")
	l.applyScan(library.ScanResult{Galleries: items}, nil)
	if l.random.btn.Disabled() {
		t.Fatal("с галереями 🎲 должна быть активна")
	}
	test.Tap(l.random.btn)
	if len(opened) != 1 || !containsKey(items, opened[0].Key) {
		t.Fatalf("открыто %v", opened)
	}

	// без повторов — три нажатия, три разные галереи
	l.SetRandomMode(app.RandomModeNoRepeat)
	opened = nil
	for range len(items) {
		test.Tap(l.random.btn)
	}
	seen := map[model.Key]bool{}
	for _, g := range opened {
		seen[g.Key] = true
	}
	if len(seen) != len(items) {
		t.Fatalf("без повторов открыты %v", opened)
	}
}

// Кнопка 🎲 активна после каталога, выбранного до сканирования.
func TestLibraryRandomAfterCached(t *testing.T) {
	test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, func(model.Gallery) {}, nil)
	l.ShowCached(library.ScanResult{Galleries: galleries("a.zip")})
	if l.random.btn.Disabled() {
		t.Fatal("после каталога 🎲 должна быть активна")
	}
}

// Кнопка 🎲 на панели, отрисованная неактивной, после появления галерей
// выглядит активной: обновляется рендерер объекта, который на экране.
func TestLibraryRandomButtonLooksEnabled(t *testing.T) {
	test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	l := NewLibrary(svc, NewGridMetrics(svc.Settings), func(string) {}, func(model.Gallery) {}, nil)
	shown, ok := l.tools.Objects[0].(fyne.Widget)
	if !ok {
		t.Fatalf("первый объект панели — %T, ожидалась кнопка 🎲", l.tools.Objects[0])
	}
	r := test.WidgetRenderer(shown) // отрисована неактивной
	if got := iconName(r); got != "disabled_dice.svg" {
		t.Fatalf("без галерей значок %q, ожидался неактивный", got)
	}
	l.applyScan(library.ScanResult{Galleries: galleries("a.zip")}, nil)
	if got := iconName(r); got != "foreground_dice.svg" {
		t.Fatalf("с галереями значок %q, ожидался активный", got)
	}
}

// iconName — имя ресурса значка в рендерере кнопки («» — значка нет).
func iconName(r fyne.WidgetRenderer) string {
	for _, o := range r.Objects() {
		if img, ok := o.(*canvas.Image); ok && img.Resource != nil {
			return img.Resource.Name()
		}
	}
	return ""
}

func containsKey(items []model.Gallery, k model.Key) bool {
	for _, g := range items {
		if g.Key == k {
			return true
		}
	}
	return false
}

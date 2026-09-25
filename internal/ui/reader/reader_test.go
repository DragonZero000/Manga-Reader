package reader

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// setup создаёт окно с читалкой поверх заглушки и библиотеку с example.zip.
func setup(t *testing.T) (*Reader, fyne.Window, model.Gallery) {
	t.Helper()
	a := test.NewTempApp(t)
	dir := t.TempDir()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "example.zip"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	src := library.NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	g, ok := src.Get(model.LocalKey("example.zip"))
	if !ok {
		t.Fatal("нет галереи")
	}
	w := a.NewWindow("t")
	r := New(a, w, src, storage.NewMemSettings(), func(string) {})
	// колбэки загрузки выполняются в горутине теста (см. eventually)
	q := make(chan func(), 1024)
	queues[r] = q
	r.do = func(f func()) { q <- f }
	w.SetContent(container.NewStack(widget.NewLabel("вкладки"), r.Layer()))
	w.Resize(fyne.NewSize(600, 800))
	return r, w, g
}

// queues — «UI-поток» каждого теста: функции из r.do выполняются в горутине
// теста. Очередь своя у каждого Reader — воркеры прошлых тестов пишут в свои.
var queues = map[*Reader]chan func(){}

// pump выполняет накопившиеся UI-функции читалки r.
func pump(r *Reader) {
	for {
		select {
		case f := <-queues[r]:
			f()
		default:
			return
		}
	}
}

func eventually(t *testing.T, r *Reader, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pump(r)
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

func capture(t *testing.T, w fyne.Window, name string) {
	t.Helper()
	dir := os.Getenv("READER_CAPTURE_DIR")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, w.Canvas().Capture())
}

func TestPagedNavigationAndPanel(t *testing.T) {
	r, w, g := setup(t)
	r.setMode(ModePaged)
	r.Open(g)
	eventually(t, r, "загрузка страницы 1", func() bool { return r.paged.img.Image != nil })
	capture(t, w, "paged-1")

	r.TypedKey(fyne.KeyRight)
	if r.cur != 1 || r.paged.page != 1 {
		t.Fatalf("после → страница %d", r.cur)
	}
	r.TypedKey(fyne.KeyRight) // граница книги
	if r.cur != 1 {
		t.Fatalf("переход за последнюю страницу: %d", r.cur)
	}
	r.TypedKey(fyne.KeyHome)
	if r.cur != 0 {
		t.Fatalf("Home: %d", r.cur)
	}

	size := r.paged.Size()
	test.TapAt(r.paged, fyne.NewPos(size.Width/2, size.Height/2))
	eventually(t, r, "панель показана", func() bool { return r.panel.layer.Visible() })
	if s := r.panel.layer.Size(); s.Width == 0 || s.Height == 0 {
		t.Fatalf("панель нулевого размера: %v", s)
	}
	capture(t, w, "paged-panel")
	if r.panel.pageLbl.Text != "1 / 2" {
		t.Errorf("номер страницы: %q", r.panel.pageLbl.Text)
	}
}

func TestTapZones(t *testing.T) {
	r, _, g := setup(t)
	r.setMode(ModePaged)
	r.Open(g)
	size := r.paged.Size()
	test.TapAt(r.paged, fyne.NewPos(size.Width-10, size.Height/2))
	if r.cur != 1 {
		t.Fatalf("правая треть: %d", r.cur)
	}
	test.TapAt(r.paged, fyne.NewPos(10, size.Height/2))
	if r.cur != 0 {
		t.Fatalf("левая треть: %d", r.cur)
	}
}

func TestCloseReleases(t *testing.T) {
	r, _, g := setup(t)
	r.Open(g)
	eventually(t, r, "загрузка", func() bool { return r.loader.Bytes() > 0 })
	r.Close()
	if r.Visible() || r.layer.Visible() || r.loader.Bytes() != 0 {
		t.Fatalf("после закрытия: visible=%v bytes=%d", r.Visible(), r.loader.Bytes())
	}
}

package details

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
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
	"mangareader/internal/thumbs"
)

// pump выполняет накопившиеся UI-функции в горутине теста: тестовый
// драйвер Fyne не сериализует fyne.Do. Очередь своя у каждого теста —
// воркеры загрузчиков прошлых тестов пишут в свои очереди.
func pump(q chan func()) {
	for {
		select {
		case f := <-q:
			f()
		default:
			return
		}
	}
}

func eventually(t *testing.T, q chan func(), what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pump(q)
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("не дождались: %s", what)
}

type fixture struct {
	d        *Details
	win      fyne.Window
	src      *library.Source
	q        chan func()
	reads    []model.Gallery
	searches []string
}

// setup создаёт страницу произведения над заглушкой вкладок; files — архивы
// из testdata или сгенерированные тестом (имя → содержимое).
func setup(t *testing.T, files map[string][]byte) *fixture {
	t.Helper()
	a := test.NewTempApp(t)
	dir := t.TempDir()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	src := library.NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	f := &fixture{src: src, win: a.NewWindow("t")}
	th := thumbs.New(src.OpenPage, 10, 1)
	f.d = New(f.win, src, th, func(g model.Gallery) { f.reads = append(f.reads, g) },
		func(q string) { f.searches = append(f.searches, q) })
	q := make(chan func(), 1024)
	f.q = q
	f.d.SetDispatcher(func(fn func()) { q <- fn })
	f.win.SetContent(container.NewStack(widget.NewLabel("вкладки"), f.d.Layer()))
	f.win.Resize(fyne.NewSize(480, 800))
	return f
}

func exampleData(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// texts собирает тексты всех надписей и кнопок содержимого.
func texts(o fyne.CanvasObject) []string {
	var out []string
	var walk func(fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		switch w := o.(type) {
		case *widget.Label:
			out = append(out, w.Text)
		case *widget.Button:
			out = append(out, w.Text)
		case *fyne.Container:
			for _, c := range w.Objects {
				walk(c)
			}
		}
	}
	walk(o)
	return out
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func findButton(o fyne.CanvasObject, text string) *widget.Button {
	switch w := o.(type) {
	case *widget.Button:
		if w.Text == text {
			return w
		}
	case *fyne.Container:
		for _, c := range w.Objects {
			if b := findButton(c, text); b != nil {
				return b
			}
		}
	}
	return nil
}

func TestOpenExample(t *testing.T) {
	f := setup(t, map[string][]byte{"example.zip": exampleData(t)})
	g, _ := f.src.Get(model.LocalKey("example.zip"))
	f.d.Open(g)
	if !f.d.Visible() || !f.d.Layer().Visible() {
		t.Fatal("страница не открыта")
	}
	got := texts(f.d.body)
	for _, want := range []string{"english name", "japanease name", "Читать",
		"Автор", "artist 1", "Язык", "japanese", "Теги", "tag 3",
		"Страниц", "2", "ID", "535147", "Загружено", "15.10.2024", "Избранное", "806", "Размер", "31,0 КБ"} {
		if !contains(got, want) {
			t.Errorf("нет %q на странице", want)
		}
	}
	if contains(got, "Сканлейтор") {
		t.Error("пустой сканлейтор показан")
	}
	if f.title().Text != "english name" {
		t.Errorf("заголовок полосы: %q", f.title().Text)
	}
	eventually(t, f.q, "обложка загружена", func() bool { return f.d.cover.img.Image != nil })

	test.Tap(findButton(f.d.body, "Читать"))
	test.Tap(f.d.cover)
	if len(f.reads) != 2 || f.reads[0].Key != g.Key {
		t.Fatalf("«Читать» и обложка должны открывать читалку: %v", f.reads)
	}

	test.Tap(findButton(f.d.body, "artist 1"))
	test.Tap(findButton(f.d.body, "tag 2"))
	if want := []string{`artist:"artist 1"`, `tag:"tag 2"`}; len(f.searches) != 2 ||
		f.searches[0] != want[0] || f.searches[1] != want[1] {
		t.Fatalf("нажатие на тег: %q, ожидалось %q", f.searches, want)
	}

	f.d.Close()
	if f.d.Visible() || f.d.Layer().Visible() {
		t.Fatal("страница не закрыта")
	}
}

func (f *fixture) title() *widget.Label { return f.d.title }

func TestNoMetaHidesTags(t *testing.T) {
	f := setup(t, map[string][]byte{"plain.zip": zipWithImage(t)})
	g, ok := f.src.Get(model.LocalKey("plain.zip"))
	if !ok {
		t.Fatal("нет галереи")
	}
	f.d.Open(g)
	got := texts(f.d.body)
	for _, hidden := range []string{"Теги", "Автор", "ID", "Загружено", "Избранное", "Сканлейтор"} {
		if contains(got, hidden) {
			t.Errorf("пустой раздел/поле %q показан", hidden)
		}
	}
	for _, want := range []string{"plain", "Страниц", "Файл", "plain.zip"} {
		if !contains(got, want) {
			t.Errorf("нет %q", want)
		}
	}
}

// zipWithImage — архив с одной PNG-страницей и без meta.json.
func zipWithImage(t *testing.T) []byte {
	t.Helper()
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewNRGBA(image.Rect(0, 0, 40, 60))); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("1.png")
	if err != nil {
		t.Fatal(err)
	}
	w.Write(img.Bytes())
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

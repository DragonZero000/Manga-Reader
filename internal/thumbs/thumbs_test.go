package thumbs

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"mangareader/internal/model"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func gallery(id string) model.Gallery {
	return model.Gallery{
		Key:   model.LocalKey(id),
		Pages: []model.Page{{Name: "1.png"}},
		File:  model.FileInfo{ModTime: time.Unix(1000, 0)},
	}
}

// fakeOpen отдаёт одно и то же содержимое и считает вызовы.
func fakeOpen(data []byte, calls *atomic.Int32, delay time.Duration) OpenFunc {
	return func(model.Key, string) (io.ReadCloser, error) {
		calls.Add(1)
		time.Sleep(delay)
		return io.NopCloser(bytes.NewReader(data)), nil
	}
}

type result struct {
	img image.Image
	err error
}

func load(c *Cache, g model.Gallery) result {
	ch := make(chan result, 1)
	c.Load(g, func(img image.Image, err error) { ch <- result{img, err} })
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		return result{err: errors.New("таймаут")}
	}
}

func TestDownscaleKeepsAspect(t *testing.T) {
	cases := []struct{ w, h, bw, bh, ww, wh int }{
		{1000, 1500, 320, 320, 213, 320},
		{1500, 1000, 320, 320, 320, 213},
		{200, 100, 320, 320, 200, 100}, // маленькое — без изменений
		{5000, 10, 320, 320, 320, 1},
		{1400, 2000, 450, 512, 358, 512}, // карточка 450×640, сторона ≤ 512
		{2000, 1400, 450, 512, 450, 315}, // горизонтальная — по ширине области
	}
	for _, c := range cases {
		out := Downscale(image.NewNRGBA(image.Rect(0, 0, c.w, c.h)), c.bw, c.bh)
		if b := out.Bounds(); b.Dx() != c.ww || b.Dy() != c.wh {
			t.Errorf("%dx%d в %dx%d → %dx%d, ожидалось %dx%d", c.w, c.h, c.bw, c.bh, b.Dx(), b.Dy(), c.ww, c.wh)
		}
		if out.Rect.Min != (image.Point{}) {
			t.Errorf("начало не в (0,0): %v", out.Rect)
		}
	}
}

func TestLoadUsesCache(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen(pngOf(t, 640, 960), &calls, 0), 1<<20, 2)
	g := gallery("a.zip")

	r := load(c, g)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if b := r.img.Bounds(); b.Dy() != MaxSide {
		t.Fatalf("размер миниатюры %v", b)
	}
	if _, ok := r.img.(*image.RGBA); !ok {
		t.Fatalf("миниатюра %T, нужна *image.RGBA", r.img)
	}
	if _, _, ok := c.Cached(g); !ok {
		t.Fatal("миниатюры нет в кэше")
	}
	load(c, g)
	if calls.Load() != 1 || c.decodes.Load() != 1 {
		t.Fatalf("повторное декодирование: open=%d decode=%d", calls.Load(), c.decodes.Load())
	}

	// изменённый архив — новая миниатюра
	g.File.ModTime = g.File.ModTime.Add(time.Second)
	load(c, g)
	if calls.Load() != 2 {
		t.Fatalf("изменённый архив взят из кэша")
	}
}

func TestSetBox(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen(pngOf(t, 1400, 2000), &calls, 0), 8<<20, 1)
	g := gallery("a.zip")
	c.SetBox(450, 640) // область карточки; высота ограничена MaxSide
	r := load(c, g)
	if b := r.img.Bounds(); b.Dx() != 358 || b.Dy() != 512 {
		t.Fatalf("миниатюра %v, ожидалось 358×512", b)
	}
	c.SetBox(450, 640) // тот же размер — кэш сохраняется
	if _, _, ok := c.Cached(g); !ok {
		t.Fatal("кэш очищен без смены размера")
	}
	c.SetBox(200, 300) // другой размер — кэш очищается
	if _, _, ok := c.Cached(g); ok {
		t.Fatal("после смены размера кэш должен очиститься")
	}
	if b := load(c, g).img.Bounds(); b.Dx() != 200 || b.Dy() != 286 {
		t.Fatalf("после смены размера %v", b)
	}
}

func TestConcurrentRequestsMerged(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen(pngOf(t, 10, 10), &calls, 50*time.Millisecond), 1<<20, 4)
	g := gallery("a.zip")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if r := load(c, g); r.err != nil {
				t.Error(r.err)
			}
		}()
	}
	wg.Wait()
	if n := calls.Load(); n != 1 {
		t.Fatalf("одновременные запросы не объединены: %d открытий", n)
	}
}

// orderedOpen — первое открытие ждёт release (занимает единственный воркер),
// порядок открытий записывается.
type orderedOpen struct {
	data    []byte
	release chan struct{}
	mu      sync.Mutex
	order   []string
}

func (o *orderedOpen) open(k model.Key, _ string) (io.ReadCloser, error) {
	o.mu.Lock()
	o.order = append(o.order, k.ID)
	first := len(o.order) == 1
	o.mu.Unlock()
	if first {
		<-o.release
	}
	return io.NopCloser(bytes.NewReader(o.data)), nil
}

func (o *orderedOpen) opened() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.order)
}

func waitOpened(t *testing.T, o *orderedOpen, n int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(o.opened()) < n {
		if time.Now().After(deadline) {
			t.Fatalf("открыто %v, ждали %d", o.opened(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestNewestFirst(t *testing.T) {
	o := &orderedOpen{data: pngOf(t, 8, 8), release: make(chan struct{})}
	c := New(o.open, 1<<20, 1)
	done := make(chan string, 4)
	req := func(id string) { c.Load(gallery(id), func(image.Image, error) { done <- id }) }
	req("busy.zip")
	waitOpened(t, o, 1) // воркер занят
	req("old.zip")
	req("new.zip")
	close(o.release)
	for range 3 {
		<-done
	}
	if got := o.opened(); !slices.Equal(got, []string{"busy.zip", "new.zip", "old.zip"}) {
		t.Fatalf("порядок %v: новые запросы должны выполняться раньше", got)
	}
}

func TestCancelBeforeDecode(t *testing.T) {
	o := &orderedOpen{data: pngOf(t, 8, 8), release: make(chan struct{})}
	c := New(o.open, 1<<20, 1)
	called := make(chan string, 4)
	c.Load(gallery("busy.zip"), func(image.Image, error) { called <- "busy" })
	waitOpened(t, o, 1)
	cancel := c.Load(gallery("gone.zip"), func(image.Image, error) { called <- "gone" })
	cancel() // карточка ушла с экрана до начала декодирования
	close(o.release)
	if got := <-called; got != "busy" {
		t.Fatalf("первым вызван %q", got)
	}
	// ещё один запрос проходит через тот же воркер — к этому времени
	// отменённое задание уже снято со стека
	if r := load(c, gallery("after.zip")); r.err != nil {
		t.Fatal(r.err)
	}
	if got := o.opened(); slices.Contains(got, "gone.zip") {
		t.Fatalf("отменённая обложка открыта: %v", got)
	}
	if c.decodes.Load() != 2 {
		t.Fatalf("декодирований %d, ожидалось 2", c.decodes.Load())
	}
	select {
	case id := <-called:
		t.Fatalf("колбэк отменённого запроса вызван: %s", id)
	default:
	}
}

func TestCancelOneOfMerged(t *testing.T) {
	o := &orderedOpen{data: pngOf(t, 8, 8), release: make(chan struct{})}
	c := New(o.open, 1<<20, 1)
	c.Load(gallery("busy.zip"), func(image.Image, error) {})
	waitOpened(t, o, 1)
	got := make(chan string, 2)
	cancel := c.Load(gallery("x.zip"), func(image.Image, error) { got <- "first" })
	c.Load(gallery("x.zip"), func(image.Image, error) { got <- "second" })
	cancel() // вторая карточка всё ещё ждёт — декодирование нужно
	close(o.release)
	if id := <-got; id != "second" {
		t.Fatalf("вызван %q", id)
	}
	select {
	case id := <-got:
		t.Fatalf("отменённый колбэк вызван: %s", id)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBrokenImage(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen([]byte("не картинка"), &calls, 0), 1<<20, 1)
	g := gallery("bad.zip")
	if r := load(c, g); r.err == nil {
		t.Fatal("ожидалась ошибка декодирования")
	}
	// ошибка тоже кэшируется — не декодируем битую обложку при каждой прокрутке
	if _, err, ok := c.Cached(g); !ok || err == nil {
		t.Fatal("ошибка не закэширована")
	}
	if r := load(c, model.Gallery{Key: model.LocalKey("empty.zip")}); !errors.Is(r.err, ErrNoCover) {
		t.Fatalf("без страниц: %v", r.err)
	}
}

func TestLRUEvictionByBytes(t *testing.T) {
	var calls atomic.Int32
	// миниатюра 4×4 = 64 байта; лимит — две миниатюры
	c := New(fakeOpen(pngOf(t, 4, 4), &calls, 0), 128, 1)
	a, b, d := gallery("a.zip"), gallery("b.zip"), gallery("d.zip")
	load(c, a)
	load(c, b)
	c.Cached(a) // a — недавно использована
	load(c, d)  // вытесняет b
	if c.Len() != 2 || c.Bytes() > 128 {
		t.Fatalf("кэш: %d записей, %d байт", c.Len(), c.Bytes())
	}
	if _, _, ok := c.Cached(b); ok {
		t.Error("b должна быть вытеснена")
	}
	if _, _, ok := c.Cached(a); !ok {
		t.Error("a должна остаться")
	}
}

// memStore — хранилище миниатюр в памяти.
type memStore struct {
	mu    sync.Mutex
	items map[string][]byte // key|w|h
}

func (m *memStore) Load(key string, w, h int) ([]byte, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	d, ok := m.items[fmt.Sprintf("%s|%d|%d", key, w, h)]
	return d, ok
}

func (m *memStore) Save(key, _ string, w, h int, jpeg []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items[fmt.Sprintf("%s|%d|%d", key, w, h)] = jpeg
}

// Второй кэш над тем же хранилищем («перезапуск») не открывает архив;
// другая область обложки — декодирование заново.
func TestStoreSkipsArchive(t *testing.T) {
	store := &memStore{items: map[string][]byte{}}
	var calls atomic.Int32
	open := fakeOpen(pngOf(t, 700, 1000), &calls, 0)
	g := gallery("a.zip")

	c1 := New(open, 1<<20, 1)
	c1.SetStore(store)
	c1.SetBox(200, 300)
	first := load(c1, g)
	if first.err != nil || calls.Load() != 1 || len(store.items) != 1 {
		t.Fatalf("первая загрузка: err=%v, открытий %d, в хранилище %d", first.err, calls.Load(), len(store.items))
	}

	c2 := New(open, 1<<20, 1)
	c2.SetStore(store)
	c2.SetBox(200, 300)
	r := load(c2, g)
	if r.err != nil || calls.Load() != 1 || c2.storeHits.Load() != 1 || c2.decodes.Load() != 0 {
		t.Fatalf("из хранилища: err=%v, открытий %d, попаданий %d", r.err, calls.Load(), c2.storeHits.Load())
	}
	if _, ok := r.img.(*image.RGBA); !ok || r.img.Bounds() != first.img.Bounds() {
		t.Fatalf("миниатюра из хранилища: %T %v", r.img, r.img.Bounds())
	}

	c3 := New(open, 1<<20, 1)
	c3.SetStore(store)
	c3.SetBox(100, 150) // другой экран
	if r := load(c3, g); r.err != nil || calls.Load() != 2 || r.img.Bounds().Dy() != 143 {
		t.Fatalf("другая область: err=%v, открытий %d, %v", r.err, calls.Load(), r.img.Bounds())
	}
}

func TestStoreBrokenNotSaved(t *testing.T) {
	store := &memStore{items: map[string][]byte{}}
	var calls atomic.Int32
	c := New(fakeOpen([]byte("не картинка"), &calls, 0), 1<<20, 1)
	c.SetStore(store)
	if r := load(c, gallery("bad.zip")); r.err == nil || r.img != nil {
		t.Fatalf("битая обложка: img=%v err=%v", r.img, r.err)
	}
	if len(store.items) != 0 {
		t.Fatal("ошибки в хранилище не сохраняются")
	}
}

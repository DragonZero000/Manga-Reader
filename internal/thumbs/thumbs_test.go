package thumbs

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
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
	cases := []struct{ w, h, ww, wh int }{
		{1000, 1500, 213, 320},
		{1500, 1000, 320, 213},
		{200, 100, 200, 100}, // маленькое — без изменений
		{5000, 10, 320, 1},
	}
	for _, c := range cases {
		out := Downscale(image.NewNRGBA(image.Rect(0, 0, c.w, c.h)), 320)
		if b := out.Bounds(); b.Dx() != c.ww || b.Dy() != c.wh {
			t.Errorf("%dx%d → %dx%d, ожидалось %dx%d", c.w, c.h, b.Dx(), b.Dy(), c.ww, c.wh)
		}
	}
}

func TestLoadUsesCache(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen(pngOf(t, 640, 960), &calls, 0), 10, 2)
	g := gallery("a.zip")

	r := load(c, g)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if b := r.img.Bounds(); b.Dy() != 320 {
		t.Fatalf("размер миниатюры %v", b)
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

func TestConcurrentRequestsMerged(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen(pngOf(t, 10, 10), &calls, 50*time.Millisecond), 10, 4)
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

func TestBrokenImage(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen([]byte("не картинка"), &calls, 0), 10, 1)
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

func TestLRUEviction(t *testing.T) {
	var calls atomic.Int32
	c := New(fakeOpen(pngOf(t, 4, 4), &calls, 0), 2, 1)
	a, b, d := gallery("a.zip"), gallery("b.zip"), gallery("d.zip")
	load(c, a)
	load(c, b)
	c.Cached(a) // a — недавно использована
	load(c, d)  // вытесняет b
	if c.Len() != 2 {
		t.Fatalf("размер кэша %d", c.Len())
	}
	if _, _, ok := c.Cached(b); ok {
		t.Error("b должна быть вытеснена")
	}
	if _, _, ok := c.Cached(a); !ok {
		t.Error("a должна остаться")
	}
}

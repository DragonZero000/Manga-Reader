package pages

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"mangareader/internal/model"
)

func pngOf(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// fakeSource отдаёт страницы из map и может «задерживать» открытие.
type fakeSource struct {
	pages map[string][]byte
	gate  chan struct{} // если не nil — open ждёт сигнала
	opens atomic.Int32
}

func (f *fakeSource) open(_ model.Key, page string) (io.ReadCloser, error) {
	f.opens.Add(1)
	if f.gate != nil {
		<-f.gate
	}
	data, ok := f.pages[page]
	if !ok {
		return nil, errors.New("нет страницы")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func wait(t *testing.T, ch <-chan Result) Result {
	t.Helper()
	select {
	case r := <-ch:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("таймаут")
		return Result{}
	}
}

func load(l *Loader, r Request, p Priority) <-chan Result {
	ch := make(chan Result, 1)
	l.Load(r, p, func(res Result) { ch <- res })
	return ch
}

var key = model.LocalKey("a.zip")

func TestLoaderScalesAndCaches(t *testing.T) {
	src := &fakeSource{pages: map[string][]byte{"1.png": pngOf(t, 1200, 1700)}}
	l := NewLoader(src.open, 64<<20, 2)

	req := Request{Key: key, Page: "1.png", W: 600, H: 600}
	r := wait(t, load(l, req, PriorityCurrent))
	if r.Err != nil {
		t.Fatal(r.Err)
	}
	// область округлена до 640×640, вписываем 1200×1700 → высота 640
	if r.H != 640 || r.W != 452 || len(r.Parts) != 1 {
		t.Fatalf("результат %dx%d, кусков %d", r.W, r.H, len(r.Parts))
	}
	// повторный запрос с чуть другим размером попадает в тот же ключ кэша
	if _, ok := l.Cached(Request{Key: key, Page: "1.png", W: 610, H: 630}); !ok {
		t.Fatal("нет в кэше после округления размера")
	}
	wait(t, load(l, req, PriorityCurrent))
	if src.opens.Load() != 1 || l.decodes.Load() != 1 {
		t.Fatalf("повторное декодирование: opens=%d decodes=%d", src.opens.Load(), l.decodes.Load())
	}
}

func TestLoaderNoUpscaleAndSlices(t *testing.T) {
	src := &fakeSource{pages: map[string][]byte{
		"small.png": pngOf(t, 100, 100),
		"tall.png":  pngOf(t, 400, 5000),
	}}
	l := NewLoader(src.open, 64<<20, 1)
	r := wait(t, load(l, Request{Key: key, Page: "small.png", W: 1000, H: 1000}, 0))
	if r.W != 100 || r.H != 100 {
		t.Fatalf("маленькое изображение увеличено: %dx%d", r.W, r.H)
	}
	r = wait(t, load(l, Request{Key: key, Page: "tall.png", W: 400, SliceH: 2048}, 0))
	if r.H != 5000 || len(r.Parts) != 3 {
		t.Fatalf("длинная страница: %dx%d, кусков %d", r.W, r.H, len(r.Parts))
	}
}

// Результат загрузчика готов для текстуры: только *image.RGBA, в том числе
// без уменьшения, после уменьшения и для кусков ленты.
func TestLoaderResultIsRGBA(t *testing.T) {
	src := &fakeSource{pages: map[string][]byte{
		"small.png": pngOf(t, 100, 100),
		"big.png":   pngOf(t, 1200, 1700),
		"tall.png":  pngOf(t, 400, 5000),
	}}
	l := NewLoader(src.open, 64<<20, 1)
	for _, req := range []Request{
		{Key: key, Page: "small.png", W: 1000, H: 1000},
		{Key: key, Page: "big.png", W: 600, H: 600},
		{Key: key, Page: "tall.png", W: 400, SliceH: 2048},
	} {
		r := wait(t, load(l, req, 0))
		for i, p := range r.Parts {
			if _, ok := p.(*image.RGBA); !ok {
				t.Errorf("%s, кусок %d: %T", req.Page, i, p)
			}
		}
	}
}

func TestLoaderEvictsByBytes(t *testing.T) {
	src := &fakeSource{pages: map[string][]byte{
		"1.png": pngOf(t, 64, 64), "2.png": pngOf(t, 64, 64), "3.png": pngOf(t, 64, 64),
	}}
	one := int64(64 * 64 * 4)
	l := NewLoader(src.open, 2*one, 1)
	for _, p := range []string{"1.png", "2.png", "3.png"} {
		wait(t, load(l, Request{Key: key, Page: p, W: 64, H: 64}, 0))
	}
	if l.Bytes() != 2*one {
		t.Fatalf("объём кэша %d, ожидалось %d", l.Bytes(), 2*one)
	}
	if _, ok := l.Cached(Request{Key: key, Page: "1.png", W: 64, H: 64}); ok {
		t.Error("самая старая страница должна быть вытеснена")
	}
	l.Clear()
	if l.Bytes() != 0 {
		t.Error("Clear не освободил кэш")
	}
}

func TestLoaderDropsStale(t *testing.T) {
	gate := make(chan struct{})
	src := &fakeSource{pages: map[string][]byte{"1.png": pngOf(t, 8, 8), "2.png": pngOf(t, 8, 8)}, gate: gate}
	l := NewLoader(src.open, 64<<20, 1)

	first := load(l, Request{Key: key, Page: "1.png", W: 8, H: 8}, 0) // занимает единственного воркера
	time.Sleep(50 * time.Millisecond)
	stale := load(l, Request{Key: key, Page: "2.png", W: 8, H: 8}, 0)
	l.NewGeneration() // страница 2 больше не нужна
	close(gate)

	if r := wait(t, first); r.Err != nil {
		t.Fatalf("начатый запрос: %v", r.Err)
	}
	if r := wait(t, stale); !errors.Is(r.Err, ErrStale) {
		t.Fatalf("устаревший запрос: %v", r.Err)
	}
	if src.opens.Load() != 1 {
		t.Fatalf("устаревшая страница открывалась: %d", src.opens.Load())
	}
}

func TestLoaderPriority(t *testing.T) {
	gate := make(chan struct{})
	src := &fakeSource{pages: map[string][]byte{
		"0.png": pngOf(t, 8, 8), "n.png": pngOf(t, 8, 8), "c.png": pngOf(t, 8, 8),
	}, gate: gate}
	l := NewLoader(src.open, 64<<20, 1)

	var neighborDone atomic.Bool
	l.Load(Request{Key: key, Page: "0.png", W: 8, H: 8}, PriorityCurrent, nil)
	time.Sleep(50 * time.Millisecond) // единственный воркер занят страницей 0
	l.Load(Request{Key: key, Page: "n.png", W: 8, H: 8}, PriorityNeighbor, func(Result) { neighborDone.Store(true) })
	current := make(chan bool, 1)
	l.Load(Request{Key: key, Page: "c.png", W: 8, H: 8}, PriorityCurrent, func(Result) { current <- neighborDone.Load() })
	close(gate)

	select {
	case before := <-current:
		if before {
			t.Fatal("соседняя страница загружена раньше текущей")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("таймаут")
	}
}

func TestLoaderBrokenPage(t *testing.T) {
	src := &fakeSource{pages: map[string][]byte{"bad.png": []byte("не картинка")}}
	l := NewLoader(src.open, 64<<20, 1)
	if r := wait(t, load(l, Request{Key: key, Page: "bad.png", W: 8, H: 8}, 0)); r.Err == nil {
		t.Fatal("ожидалась ошибка декодирования")
	}
	if r := wait(t, load(l, Request{Key: key, Page: "missing.png", W: 8, H: 8}, 0)); r.Err == nil {
		t.Fatal("ожидалась ошибка открытия")
	}
}

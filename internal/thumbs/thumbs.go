// Package thumbs загружает миниатюры обложек: декодирует изображение в фоне,
// уменьшает до области обложки карточки и кэширует в памяти. Пакет не
// зависит от UI.
package thumbs

import (
	"container/list"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // регистрация декодеров
	_ "image/jpeg" //
	_ "image/png"  //
	"io"
	"runtime"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" //

	"mangareader/internal/model"
	"mangareader/internal/pages"
)

const (
	// MaxSide — наибольшая сторона миниатюры в пикселях.
	MaxSide = 512
	// DefaultLimit — объём кэша миниатюр в байтах по умолчанию.
	DefaultLimit = 64 << 20
	// errorBytes — «вес» закэшированной ошибки: кэш ошибок тоже ограничен.
	errorBytes = 64
)

// ErrNoCover — у галереи нет страниц.
var ErrNoCover = errors.New("нет обложки")

// OpenFunc открывает страницу галереи (library.Source.OpenPage).
type OpenFunc func(k model.Key, page string) (io.ReadCloser, error)

// Callback получает результат загрузки. Вызывается из фоновой горутины.
type Callback func(img image.Image, err error)

// Cache — LRU-кэш миниатюр, ограниченный по объёму, с фиксированным числом
// фоновых декодеров. Ожидающие задания выполняются с конца (новые — раньше):
// при быстрой прокрутке сначала грузятся видимые сейчас карточки. Задание,
// которое никому больше не нужно (все отменили), выбрасывается без
// декодирования.
type Cache struct {
	open  OpenFunc
	limit int64

	mu       sync.Mutex
	cond     *sync.Cond
	boxW     int // область миниатюры в пикселях (каждая сторона ≤ MaxSide)
	boxH     int
	lru      *list.List               // *entry, начало — недавно использованные
	items    map[string]*list.Element // по ключу кэша
	bytes    int64
	stack    []*job          // ожидающие задания; вершина — конец среза
	jobs     map[string]*job // ожидающие и выполняющиеся, по ключу кэша
	waiterID int

	decodes atomic.Int64 // число декодирований (для тестов)
}

type entry struct {
	key   string
	img   image.Image
	err   error
	bytes int64
}

type job struct {
	key     string
	g       model.Gallery
	running bool
	waiters map[int]Callback
}

// New создаёт кэш объёмом limitBytes (≤ 0 — DefaultLimit) и workers
// декодерами (≤ 0 — min(4, NumCPU)).
func New(open OpenFunc, limitBytes int64, workers int) *Cache {
	if limitBytes <= 0 {
		limitBytes = DefaultLimit
	}
	if workers <= 0 {
		workers = min(4, runtime.NumCPU())
	}
	c := &Cache{
		open:  open,
		limit: limitBytes,
		boxW:  MaxSide,
		boxH:  MaxSide,
		lru:   list.New(),
		items: map[string]*list.Element{},
		jobs:  map[string]*job{},
	}
	c.cond = sync.NewCond(&c.mu)
	for range workers {
		go c.worker()
	}
	return c
}

// SetBox задаёт область миниатюры в физических пикселях (область обложки
// карточки); каждая сторона ограничивается MaxSide. При изменении кэш
// очищается: прежние миниатюры другого размера.
func (c *Cache) SetBox(w, h int) {
	w, h = max(1, min(w, MaxSide)), max(1, min(h, MaxSide))
	c.mu.Lock()
	defer c.mu.Unlock()
	if w == c.boxW && h == c.boxH {
		return
	}
	c.boxW, c.boxH = w, h
	c.lru.Init()
	c.items = map[string]*list.Element{}
	c.bytes = 0
}

// CacheKey — ключ миниатюры: изменённый архив получает новую обложку.
func CacheKey(g model.Gallery) string {
	return g.Key.String() + "@" + strconv.FormatInt(g.File.ModTime.UnixNano(), 10)
}

// Cached возвращает миниатюру из кэша (ok=false, если её там нет).
// err != nil — обложку ранее не удалось загрузить.
func (c *Cache) Cached(g model.Gallery) (img image.Image, err error, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[CacheKey(g)]
	if !ok {
		return nil, nil, false
	}
	c.lru.MoveToFront(el)
	e := el.Value.(*entry)
	return e.img, e.err, true
}

// Load загружает миниатюру обложки в фоне и вызывает cb из фоновой горутины.
// Одновременные запросы одной миниатюры объединяются. cancel отменяет запрос:
// cb больше не будет вызван; если миниатюра никому больше не нужна и её
// декодирование не началось, оно не выполняется.
func (c *Cache) Load(g model.Gallery, cb Callback) (cancel func()) {
	key := CacheKey(g)
	c.mu.Lock()
	if el, ok := c.items[key]; ok {
		c.lru.MoveToFront(el)
		e := el.Value.(*entry)
		c.mu.Unlock()
		go cb(e.img, e.err)
		return func() {}
	}
	j, ok := c.jobs[key]
	if !ok {
		j = &job{key: key, g: g, waiters: map[int]Callback{}}
		c.jobs[key] = j
	} else if !j.running {
		// снова нужна — в начало очереди
		if i := slices.Index(c.stack, j); i >= 0 {
			c.stack = slices.Delete(c.stack, i, i+1)
		}
	}
	c.waiterID++
	id := c.waiterID
	j.waiters[id] = cb
	if !j.running {
		c.stack = append(c.stack, j)
		c.cond.Signal()
	}
	c.mu.Unlock()
	return func() {
		c.mu.Lock()
		delete(j.waiters, id)
		c.mu.Unlock()
	}
}

// Len возвращает число миниатюр в кэше.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// Bytes возвращает объём кэша в байтах.
func (c *Cache) Bytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.bytes
}

func (c *Cache) worker() {
	for {
		c.mu.Lock()
		for len(c.stack) == 0 {
			c.cond.Wait()
		}
		j := c.stack[len(c.stack)-1]
		c.stack = c.stack[:len(c.stack)-1]
		if len(j.waiters) == 0 {
			delete(c.jobs, j.key) // все отменили — не декодируем
			c.mu.Unlock()
			continue
		}
		j.running = true
		w, h := c.boxW, c.boxH
		c.mu.Unlock()

		img, err := c.load(j.g, w, h)

		c.mu.Lock()
		if w == c.boxW && h == c.boxH { // область не сменилась, пока декодировали
			c.put(j.key, img, err)
		}
		delete(c.jobs, j.key)
		waiters := make([]Callback, 0, len(j.waiters))
		for _, cb := range j.waiters {
			waiters = append(waiters, cb)
		}
		c.mu.Unlock()
		for _, cb := range waiters {
			cb(img, err)
		}
	}
}

func (c *Cache) load(g model.Gallery, w, h int) (image.Image, error) {
	cover, ok := g.Cover()
	if !ok {
		return nil, ErrNoCover
	}
	rc, err := c.open(g.Key, cover.Name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	c.decodes.Add(1)
	src, _, err := image.Decode(rc)
	if err != nil {
		return nil, fmt.Errorf("декодирование %s: %w", cover.Name, err)
	}
	return Downscale(src, w, h), nil
}

// put добавляет запись и вытесняет давно не использованные, пока объём
// больше лимита. Вызывается под c.mu.
func (c *Cache) put(key string, img image.Image, err error) {
	size := int64(errorBytes)
	if img != nil {
		b := img.Bounds()
		size = int64(b.Dx()) * int64(b.Dy()) * 4
	}
	if size > c.limit {
		return // не кэшируем то, что не помещается целиком
	}
	if el, ok := c.items[key]; ok {
		c.bytes -= el.Value.(*entry).bytes
		c.lru.Remove(el)
	}
	c.items[key] = c.lru.PushFront(&entry{key, img, err, size})
	c.bytes += size
	for c.bytes > c.limit && c.lru.Len() > 0 {
		last := c.lru.Back()
		e := last.Value.(*entry)
		c.lru.Remove(last)
		delete(c.items, e.key)
		c.bytes -= e.bytes
	}
}

// Downscale уменьшает изображение так, чтобы оно поместилось в w×h,
// сохраняя пропорции; не увеличивает. Результат — *image.RGBA: Fyne
// загружает его в текстуру без конвертации в UI-потоке.
func Downscale(src image.Image, w, h int) *image.RGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	scale := pages.Fit(pages.Sz{W: float32(sw), H: float32(sh)}, pages.Sz{W: float32(w), H: float32(h)})
	if scale >= 1 {
		return pages.ToRGBA(src)
	}
	dw := max(1, int(float32(sw)*scale+0.5))
	dh := max(1, int(float32(sh)*scale+0.5))
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// Package thumbs загружает миниатюры обложек: декодирует изображение в фоне,
// уменьшает и кэширует в памяти. Пакет не зависит от UI.
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
	"strconv"
	"sync"
	"sync/atomic"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" //

	"mangareader/internal/model"
)

const (
	// DefaultMaxSide — большая сторона миниатюры в пикселях.
	DefaultMaxSide = 320
	// DefaultCapacity — число миниатюр в кэше.
	DefaultCapacity = 300
)

// ErrNoCover — у галереи нет страниц.
var ErrNoCover = errors.New("нет обложки")

// OpenFunc открывает страницу галереи (library.Source.OpenPage).
type OpenFunc func(k model.Key, page string) (io.ReadCloser, error)

// Callback получает результат загрузки. Вызывается из фоновой горутины.
type Callback func(img image.Image, err error)

// Cache — LRU-кэш миниатюр с ограниченным числом фоновых декодеров.
type Cache struct {
	open     OpenFunc
	maxSide  int
	capacity int
	sem      chan struct{}

	mu       sync.Mutex
	lru      *list.List               // *entry, начало — недавно использованные
	items    map[string]*list.Element // по ключу кэша
	inflight map[string][]Callback    // ожидающие одной и той же миниатюры

	decodes atomic.Int64 // число декодирований (для тестов)
}

type entry struct {
	key string
	img image.Image
	err error
}

// New создаёт кэш. workers ≤ 0 — min(4, NumCPU).
func New(open OpenFunc, capacity, workers int) *Cache {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	if workers <= 0 {
		workers = min(4, runtime.NumCPU())
	}
	return &Cache{
		open:     open,
		maxSide:  DefaultMaxSide,
		capacity: capacity,
		sem:      make(chan struct{}, workers),
		lru:      list.New(),
		items:    map[string]*list.Element{},
		inflight: map[string][]Callback{},
	}
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
// Одновременные запросы одной миниатюры объединяются.
func (c *Cache) Load(g model.Gallery, cb Callback) {
	if img, err, ok := c.Cached(g); ok {
		go cb(img, err)
		return
	}
	key := CacheKey(g)
	c.mu.Lock()
	if waiters, busy := c.inflight[key]; busy {
		c.inflight[key] = append(waiters, cb)
		c.mu.Unlock()
		return
	}
	c.inflight[key] = []Callback{cb}
	c.mu.Unlock()

	go func() {
		c.sem <- struct{}{}
		img, err := c.load(g)
		<-c.sem

		c.mu.Lock()
		c.put(key, img, err)
		waiters := c.inflight[key]
		delete(c.inflight, key)
		c.mu.Unlock()
		for _, w := range waiters {
			w(img, err)
		}
	}()
}

func (c *Cache) load(g model.Gallery) (image.Image, error) {
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
	return Downscale(src, c.maxSide), nil
}

// put добавляет запись и вытесняет старые. Вызывается под c.mu.
func (c *Cache) put(key string, img image.Image, err error) {
	if el, ok := c.items[key]; ok {
		el.Value = &entry{key, img, err}
		c.lru.MoveToFront(el)
		return
	}
	c.items[key] = c.lru.PushFront(&entry{key, img, err})
	for c.lru.Len() > c.capacity {
		last := c.lru.Back()
		c.lru.Remove(last)
		delete(c.items, last.Value.(*entry).key)
	}
}

// Len возвращает число миниатюр в кэше.
func (c *Cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lru.Len()
}

// Downscale уменьшает изображение так, чтобы большая сторона была ≤ maxSide,
// сохраняя пропорции. Маленькие изображения возвращаются без изменений.
func Downscale(src image.Image, maxSide int) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= maxSide && h <= maxSide {
		return src
	}
	if w >= h {
		h = max(1, h*maxSide/w)
		w = maxSide
	} else {
		w = max(1, w*maxSide/h)
		h = maxSide
	}
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

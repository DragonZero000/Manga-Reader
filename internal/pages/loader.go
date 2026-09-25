package pages

import (
	"container/list"
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // регистрация декодеров
	_ "image/jpeg" //
	_ "image/png"  //
	"io"
	"sort"
	"sync"
	"sync/atomic"

	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp" //

	"mangareader/internal/model"
)

// sizeStep — шаг округления целевого размера: мелкий ресайз окна не
// порождает новых декодирований.
const sizeStep = 64

// ErrStale — запрос устарел (сменилась страница или галерея) и не выполнялся.
var ErrStale = errors.New("запрос устарел")

// OpenFunc открывает страницу галереи (library.Source.OpenPage).
type OpenFunc func(k model.Key, page string) (io.ReadCloser, error)

// Request — что загрузить. W, H — целевая область в физических пикселях;
// H <= 0 — вписать только по ширине. Изображение никогда не увеличивается.
// SliceH > 0 — резать результат на куски не выше SliceH.
type Request struct {
	Key    model.Key
	Page   string
	W, H   int
	SliceH int
}

// Result — загруженная страница. Parts — один или несколько кусков сверху вниз;
// W, H — полный размер результата в пикселях.
type Result struct {
	Parts []image.Image
	W, H  int
	Err   error
}

// Priority — очерёдность: меньше — раньше.
type Priority int

const (
	PriorityCurrent  Priority = 0
	PriorityNeighbor Priority = 1
)

// Loader декодирует страницы в фоне с кэшем LRU, ограниченным по байтам.
type Loader struct {
	open  OpenFunc
	limit int64

	mu       sync.Mutex
	cond     *sync.Cond
	lru      *list.List
	items    map[string]*list.Element
	bytes    int64
	queue    []*job
	inflight map[string]*job
	gen      int64
	seq      int64

	decodes atomic.Int64 // число декодирований (для тестов)
}

type job struct {
	key  string
	req  Request
	prio Priority
	gen  int64
	seq  int64
	cbs  []func(Result)
}

type cacheEntry struct {
	key   string
	res   Result
	bytes int64
}

// NewLoader создаёт загрузчик с лимитом кэша limitBytes и workers воркерами.
func NewLoader(open OpenFunc, limitBytes int64, workers int) *Loader {
	l := &Loader{
		open:     open,
		limit:    limitBytes,
		lru:      list.New(),
		items:    map[string]*list.Element{},
		inflight: map[string]*job{},
	}
	l.cond = sync.NewCond(&l.mu)
	for i := 0; i < max(1, workers); i++ {
		go l.worker()
	}
	return l
}

// Normalize округляет целевой размер запроса вверх до шага sizeStep.
func Normalize(r Request) Request {
	r.W = roundUp(r.W)
	if r.H > 0 {
		r.H = roundUp(r.H)
	}
	return r
}

func roundUp(v int) int {
	if v <= 0 {
		return sizeStep
	}
	return (v + sizeStep - 1) / sizeStep * sizeStep
}

func cacheKey(r Request) string {
	return fmt.Sprintf("%s|%s|%dx%d|%d", r.Key, r.Page, r.W, r.H, r.SliceH)
}

// Cached возвращает результат из кэша без загрузки.
func (l *Loader) Cached(r Request) (Result, bool) {
	r = Normalize(r)
	l.mu.Lock()
	defer l.mu.Unlock()
	el, ok := l.items[cacheKey(r)]
	if !ok {
		return Result{}, false
	}
	l.lru.MoveToFront(el)
	return el.Value.(*cacheEntry).res, true
}

// Load ставит загрузку в очередь; cb (может быть nil) вызывается из фоновой
// горутины. Одинаковые запросы объединяются. Если запрос устареет до начала
// выполнения (NewGeneration), cb получает ErrStale.
func (l *Loader) Load(r Request, prio Priority, cb func(Result)) {
	r = Normalize(r)
	key := cacheKey(r)
	l.mu.Lock()
	if el, ok := l.items[key]; ok {
		l.lru.MoveToFront(el)
		res := el.Value.(*cacheEntry).res
		l.mu.Unlock()
		if cb != nil {
			go cb(res)
		}
		return
	}
	if j, ok := l.inflight[key]; ok {
		if cb != nil {
			j.cbs = append(j.cbs, cb)
		}
		// повышаем приоритет и «освежаем» поколение, если запрос снова нужен
		j.prio = min(j.prio, prio)
		j.gen = l.gen
		l.mu.Unlock()
		return
	}
	l.seq++
	j := &job{key: key, req: r, prio: prio, gen: l.gen, seq: l.seq}
	if cb != nil {
		j.cbs = append(j.cbs, cb)
	}
	l.inflight[key] = j
	l.queue = append(l.queue, j)
	l.cond.Signal()
	l.mu.Unlock()
}

// NewGeneration помечает все ещё не начатые запросы устаревшими.
func (l *Loader) NewGeneration() {
	l.mu.Lock()
	l.gen++
	l.mu.Unlock()
}

// Clear очищает кэш и отменяет не начатые запросы.
func (l *Loader) Clear() {
	l.mu.Lock()
	l.gen++
	l.lru.Init()
	l.items = map[string]*list.Element{}
	l.bytes = 0
	l.mu.Unlock()
}

// Bytes возвращает объём кэша в байтах.
func (l *Loader) Bytes() int64 {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.bytes
}

func (l *Loader) worker() {
	for {
		l.mu.Lock()
		for len(l.queue) == 0 {
			l.cond.Wait()
		}
		// сначала текущее поколение, затем приоритет, затем порядок постановки
		sort.Slice(l.queue, func(a, b int) bool {
			qa, qb := l.queue[a], l.queue[b]
			if qa.gen != qb.gen {
				return qa.gen > qb.gen
			}
			if qa.prio != qb.prio {
				return qa.prio < qb.prio
			}
			return qa.seq < qb.seq
		})
		j := l.queue[0]
		l.queue = l.queue[1:]
		stale := j.gen != l.gen
		if stale {
			delete(l.inflight, j.key)
			cbs := j.cbs
			l.mu.Unlock()
			for _, cb := range cbs {
				cb(Result{Err: ErrStale})
			}
			continue
		}
		l.mu.Unlock()

		res := l.decode(j.req)

		l.mu.Lock()
		l.put(j.key, res)
		delete(l.inflight, j.key)
		cbs := j.cbs
		l.mu.Unlock()
		for _, cb := range cbs {
			cb(res)
		}
	}
}

func (l *Loader) decode(r Request) Result {
	rc, err := l.open(r.Key, r.Page)
	if err != nil {
		return Result{Err: err}
	}
	defer rc.Close()
	l.decodes.Add(1)
	src, _, err := image.Decode(rc)
	if err != nil {
		return Result{Err: fmt.Errorf("декодирование %s: %w", r.Page, err)}
	}
	img := scaleToBox(src, r.W, r.H)
	b := img.Bounds()
	return Result{Parts: Slice(img, r.SliceH), W: b.Dx(), H: b.Dy()}
}

// scaleToBox уменьшает изображение, чтобы оно поместилось в w×h
// (h <= 0 — только по ширине). Изображение не увеличивается.
func scaleToBox(src image.Image, w, h int) image.Image {
	b := src.Bounds()
	sw, sh := float32(b.Dx()), float32(b.Dy())
	scale := FitWidth(sw, float32(w))
	if h > 0 {
		scale = Fit(Sz{sw, sh}, Sz{float32(w), float32(h)})
	}
	if scale >= 1 {
		return src
	}
	dw, dh := max(1, int(sw*scale+0.5)), max(1, int(sh*scale+0.5))
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	draw.ApproxBiLinear.Scale(dst, dst.Bounds(), src, b, draw.Src, nil)
	return dst
}

// put кладёт результат в кэш и вытесняет старые записи. Вызывается под mu.
func (l *Loader) put(key string, res Result) {
	var size int64
	for _, p := range res.Parts {
		b := p.Bounds()
		size += int64(b.Dx()) * int64(b.Dy()) * 4
	}
	if size > l.limit {
		return // не кэшируем то, что не помещается целиком
	}
	if el, ok := l.items[key]; ok {
		l.bytes -= el.Value.(*cacheEntry).bytes
		l.lru.Remove(el)
	}
	l.items[key] = l.lru.PushFront(&cacheEntry{key: key, res: res, bytes: size})
	l.bytes += size
	for l.bytes > l.limit && l.lru.Len() > 0 {
		last := l.lru.Back()
		e := last.Value.(*cacheEntry)
		l.lru.Remove(last)
		delete(l.items, e.key)
		l.bytes -= e.bytes
	}
}

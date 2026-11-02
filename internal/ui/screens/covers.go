package screens

import (
	"container/list"
	"image"
	"slices"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

const (
	// coverWarm — объект обложки, видный на экране меньше coverWarm назад,
	// считается «тёплым»: его текстура ещё в кэше Fyne (Fyne освобождает
	// текстуру объекта, который не рисовался минуту).
	coverWarm = 30 * time.Second
	// Бюджет показа новых обложек, обложек в секунду: каждая новая обложка —
	// загрузка текстуры в UI-потоке.
	coverRateMobile  = 60
	coverRateDesktop = 120
	// coverMaxStep — наибольший учитываемый промежуток между тактами: после
	// паузы обложки не появляются залпом.
	coverMaxStep = 100 * time.Millisecond
	// poolScreens — ёмкость пула объектов изображений в экранах карточек.
	poolScreens = 3
)

// pooledImage — объект изображения обложки в пуле сетки.
type pooledImage struct {
	key   string
	img   *canvas.Image
	owner *galleryCard // карточка, которая показывает объект (nil — ни одна)
	seen  time.Time    // когда объект последний раз был на экране
	el    *list.Element
	pool  *imagePool
}

// imagePool — объекты изображений обложек по ключу миниатюры (LRU; ключ
// включает время изменения архива и область, поэтому содержимое объекта
// под ключом не меняется). Объект переиспользуется для той же галереи: его
// текстура остаётся в кэше Fyne, и при возврате прокрутки обложка не
// загружается заново. Вытесненный объект отпускает изображение, Fyne
// освобождает его текстуру по сроку. Только UI-поток.
type imagePool struct {
	limit int
	lru   *list.List // *pooledImage, начало — недавно использованные
	items map[string]*pooledImage
	// now — текущее время; подменяется в тестах.
	now func() time.Time
}

func newImagePool() *imagePool {
	return &imagePool{limit: 1, lru: list.New(), items: map[string]*pooledImage{}, now: time.Now}
}

// get — объект key (nil — нет); отмечается как недавно использованный.
func (p *imagePool) get(key string) *pooledImage {
	e := p.items[key]
	if e != nil {
		p.lru.MoveToFront(e.el)
	}
	return e
}

// add создаёт объект для миниатюры img и вытесняет лишние.
func (p *imagePool) add(key string, img image.Image) *pooledImage {
	ci := canvas.NewImageFromImage(img)
	ci.FillMode = canvas.ImageFillContain
	ci.ScaleMode = canvas.ImageScaleFastest // миниатюра уже по размеру карточки
	e := &pooledImage{key: key, img: ci, pool: p}
	e.el = p.lru.PushFront(e)
	p.items[key] = e
	p.evict()
	return e
}

// setLimit задаёт ёмкость пула.
func (p *imagePool) setLimit(n int) {
	p.limit = max(1, n)
	p.evict()
}

// evict вытесняет давно не использованные объекты сверх ёмкости; объекты
// на карточках не вытесняются.
func (p *imagePool) evict() {
	for el := p.lru.Back(); el != nil && p.lru.Len() > p.limit; {
		prev := el.Prev()
		if e := el.Value.(*pooledImage); e.owner == nil {
			p.remove(e)
		}
		el = prev
	}
}

func (p *imagePool) remove(e *pooledImage) {
	e.img.Image = nil
	p.lru.Remove(e.el)
	delete(p.items, e.key)
}

// clear снимает все объекты с карточек и очищает пул (смена размера карточек).
func (p *imagePool) clear() {
	for _, e := range p.items {
		if e.owner != nil {
			e.owner.detach()
		}
		p.remove(e)
	}
}

// Len — число объектов в пуле.
func (p *imagePool) Len() int { return p.lru.Len() }

// coverPresenter ставит обложки в карточки сетки. Тёплый объект пула
// ставится сразу. Новая обложка — загрузка текстуры в кадре, поэтому они
// появляются по бюджету rate обложек в секунду, посчитанному по
// фактическому времени между тактами (а не по числу кадров: скорость не
// зависит от частоты экрана), построчно по направлению прокрутки. Пока
// обложка ждёт очереди, карточка показывает заглушку. Только UI-поток.
type coverPresenter struct {
	g    *galleryGrid
	pool *imagePool
	rate float64

	queue     []*galleryCard // карточки, ждущие обложки (card.want, card.ready)
	allowance float64        // обложек, которые можно показать
	last      time.Time      // время прошлого такта
	anim      *fyne.Animation
	running   bool
	// startAnim запускает такты (anim.Start); подменяется в тестах: тестовый
	// драйвер Fyne выполняет один такт сразу при запуске.
	startAnim func()
	// uploads — обложки, поставленные по бюджету (для тестов).
	uploads int
}

func newCoverPresenter(g *galleryGrid) *coverPresenter {
	p := &coverPresenter{g: g, pool: newImagePool(), rate: coverRateDesktop}
	if g.metrics.mobile {
		p.rate = coverRateMobile
	}
	p.anim = fyne.NewAnimation(time.Second, func(float32) {
		if p.running {
			p.step(p.pool.now())
		}
	})
	p.anim.RepeatCount = fyne.AnimationRepeatForever
	p.anim.Curve = fyne.AnimationLinear
	p.startAnim = p.anim.Start
	return p
}

// present показывает в карточке c миниатюру img ключа key (ключ пула —
// миниатюра и область) или значок ошибки.
func (p *coverPresenter) present(c *galleryCard, key string, img image.Image, err error) {
	p.dequeue(c)
	if err != nil || img == nil {
		c.showError()
		return
	}
	c.want = key
	if c.shown != nil && c.shown.key == key {
		return // карточка уже показывает этот объект: без работы
	}
	if e := p.pool.get(key); e != nil && p.warm(e) {
		p.attach(c, e)
		return
	}
	c.showLoading()
	c.ready = img
	p.queue = append(p.queue, c)
	p.start()
}

// warm — объект на экране сейчас или был на нём меньше coverWarm назад.
func (p *coverPresenter) warm(e *pooledImage) bool {
	return p.pool.now().Sub(e.seen) < coverWarm
}

// attach ставит объект e в карточку c, снимая его с прежней карточки.
func (p *coverPresenter) attach(c *galleryCard, e *pooledImage) {
	if e.owner != nil && e.owner != c {
		e.owner.showLoading()
	}
	c.setImage(e)
	e.seen = p.pool.now()
}

// dequeue убирает карточку из очереди.
func (p *coverPresenter) dequeue(c *galleryCard) {
	if c.ready == nil {
		return
	}
	c.ready = nil
	if i := slices.Index(p.queue, c); i >= 0 {
		p.queue = slices.Delete(p.queue, i, i+1)
	}
}

// start запускает такты, пока очередь не пуста.
func (p *coverPresenter) start() {
	if p.running {
		return
	}
	p.running = true
	p.last = p.pool.now()
	p.allowance = 1 // одна обложка — сразу, в ближайшем кадре
	p.startAnim()
}

// step — такт: бюджет растёт на rate × dt, обложки из начала очереди
// (по направлению прокрутки) ставятся, пока бюджет позволяет. Карточки,
// ушедшие с экрана, снимаются с очереди без загрузки.
func (p *coverPresenter) step(now time.Time) {
	dt := min(now.Sub(p.last), coverMaxStep)
	p.last = now
	p.allowance += p.rate * dt.Seconds()
	p.g.track()
	from, to := p.g.visibleRange()
	p.sortQueue()
	for len(p.queue) > 0 && p.allowance >= 1 {
		c := p.queue[0]
		p.queue = p.queue[1:]
		img := c.ready
		c.ready = nil
		if c.id < from || c.id >= to {
			continue // ушла с экрана: обложку поставит updateCard при возврате
		}
		e := p.pool.get(c.want)
		if e == nil {
			e = p.pool.add(c.want, img)
		}
		p.attach(c, e) // остывший объект: текстура загрузится заново при отрисовке
		p.allowance--
		p.uploads++
	}
	if len(p.queue) == 0 {
		p.allowance = min(p.allowance, 1) // остаток не копится в залп
		p.running = false
		p.anim.Stop()
	}
}

// sortQueue упорядочивает очередь: строки по направлению прокрутки, внутри
// строки — слева направо.
func (p *coverPresenter) sortQueue() {
	cols := max(1, p.g.grid.ColumnCount())
	down := p.g.dir >= 0
	slices.SortStableFunc(p.queue, func(a, b *galleryCard) int {
		ra, rb := a.id/cols, b.id/cols
		if ra != rb {
			if down == (ra < rb) {
				return -1
			}
			return 1
		}
		return a.id%cols - b.id%cols
	})
}

// seeVisible отмечает объекты карточек видимого диапазона [from, to) как
// показанные сейчас.
func (p *coverPresenter) seeVisible(from, to int) {
	now := p.pool.now()
	for _, e := range p.pool.items {
		if e.owner != nil && e.owner.id >= from && e.owner.id < to {
			e.seen = now
		}
	}
}

// reset очищает очередь и пул (смена размера карточек).
func (p *coverPresenter) reset() {
	for _, c := range p.queue {
		c.ready = nil
	}
	p.queue = nil
	p.pool.clear()
}

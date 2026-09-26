package reader

import (
	"errors"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/pages"
)

const (
	// unknownAspect — высота/ширина для страницы с неизвестным размером (A-формат).
	unknownAspect = 1.414
	// keyScrollShare — доля высоты экрана для прокрутки клавишами.
	keyScrollShare = 0.9
	// errorHeight — высота полосы на месте нечитаемой страницы.
	errorHeight = 120
)

// stripView — лента: страницы подряд по ширине экрана. Виртуализация
// собственная: на экране только страницы в окне [offset−H, offset+2H].
type stripView struct {
	widget.BaseWidget
	r *Reader

	g       model.Gallery
	sizes   []library.PageSize
	heights []float32
	prefix  []float32
	width   float32
	ready   bool // размеры известны и раскладка построена
	pending int  // страница, которую показать после построения раскладки
	top     int

	scroll  *container.Scroll
	content *stripContent
	spinner *widget.Activity
	items   map[int]*stripItem
	pool    []*stripItem

	// Окно размещения [lastFrom, lastTo) и видимые страницы
	// [lastScreenFrom, lastScreenTo) при последнем updateVisible: прокрутка
	// внутри окна ничего не перестраивает. lastFrom < 0 — пересчитать заново.
	lastFrom, lastTo             int
	lastScreenFrom, lastScreenTo int
	// windowUpdates — число перестроек окна (для тестов).
	windowUpdates int
}

// stripItem — одна страница ленты: куски изображения и надпись состояния.
type stripItem struct {
	page    int
	box     *fyne.Container
	parts   []*canvas.Image
	status  *widget.Label
	req     pages.Request
	pending bool // запрос отправлен
	loaded  bool
	res     pages.Result
}

func newStripView(r *Reader) *stripView {
	v := &stripView{r: r, spinner: widget.NewActivity(), items: map[int]*stripItem{}, lastFrom: -1}
	v.content = newStripContent(v)
	v.scroll = container.NewVScroll(v.content)
	v.scroll.OnScrolled = func(fyne.Position) { v.updateVisible() }
	v.ExtendBaseWidget(v)
	return v
}

func (v *stripView) CreateRenderer() fyne.WidgetRenderer {
	return &stripRenderer{v: v, center: container.NewCenter(v.spinner)}
}

// --- view ---

func (v *stripView) show(g model.Gallery, page int) {
	v.releaseAll()
	v.g = g
	v.pending = page
	v.ready = false
	v.heights, v.prefix = nil, pages.Prefix(nil)
	v.content.Refresh()
	v.scroll.ScrollToOffset(fyne.Position{})
	v.spinner.Start()
	v.spinner.Show()
	if v.sizes != nil && len(v.sizes) == len(g.Pages) {
		v.layout(v.Size().Width)
	}
}

func (v *stripView) setSizes(sizes []library.PageSize) {
	v.sizes = sizes
	if !v.Visible() {
		return
	}
	v.layout(v.Size().Width)
}

func (v *stripView) goTo(page int) {
	if !v.ready {
		v.pending = page
		return
	}
	page = max(0, min(page, len(v.heights)-1))
	v.scroll.ScrollToOffset(fyne.NewPos(0, v.prefix[page]))
	v.top = page
	v.r.pageChanged(page)
	v.updateVisible()
}

func (v *stripView) reset() {
	v.releaseAll()
	v.sizes = nil
	v.ready = false
	v.spinner.Stop()
	v.spinner.Hide()
}

func (v *stripView) typedKey(k fyne.KeyName) {
	if !v.ready {
		return
	}
	h := v.scroll.Size().Height
	switch k {
	case fyne.KeyDown, fyne.KeyPageDown, fyne.KeySpace:
		v.scrollBy(h * keyScrollShare)
	case fyne.KeyUp, fyne.KeyPageUp:
		v.scrollBy(-h * keyScrollShare)
	case fyne.KeyRight:
		v.goTo(v.top + 1)
	case fyne.KeyLeft:
		v.goTo(v.top - 1)
	case fyne.KeyHome:
		v.goTo(0)
	case fyne.KeyEnd:
		v.goTo(len(v.heights) - 1)
	}
}

func (v *stripView) scrollBy(dy float32) {
	total := v.prefix[len(v.prefix)-1]
	maxY := max(0, total-v.scroll.Size().Height)
	y := max(0, min(v.scroll.Offset.Y+dy, maxY))
	v.scroll.ScrollToOffset(fyne.NewPos(0, y))
	v.updateVisible()
}

// --- раскладка ---

// layout пересчитывает высоты страниц для ширины w, сохраняя текущую страницу.
func (v *stripView) layout(w float32) {
	if w <= 0 || v.sizes == nil || len(v.sizes) != len(v.g.Pages) {
		return
	}
	keep := v.pending
	if v.ready {
		keep = v.top
	}
	v.width = w
	v.heights = make([]float32, len(v.sizes))
	for i, s := range v.sizes {
		v.heights[i] = v.heightFor(i, s.Width, s.Height)
	}
	v.prefix = pages.Prefix(v.heights)
	v.releaseAll() // разрешение изображений зависит от ширины
	v.ready = true
	v.spinner.Stop()
	v.spinner.Hide()
	v.content.Refresh()
	v.scroll.Refresh()
	v.goTo(keep)
}

func (v *stripView) heightFor(_ int, w, h int) float32 {
	if w <= 0 || h <= 0 {
		return v.width * unknownAspect
	}
	return v.width * float32(h) / float32(w)
}

// updateVisible размещает страницы окна [offset−H, offset+2H] и обновляет
// номер текущей страницы. Вызывается на каждое событие прокрутки, поэтому
// работает только при смене окна или видимых страниц: страницы внутри
// содержимого сдвигает сам контейнер прокрутки.
func (v *stripView) updateVisible() {
	if !v.ready || !v.r.visible {
		return
	}
	off := v.scroll.Offset.Y
	h := v.scroll.Size().Height
	from, to := pages.Visible(v.prefix, off-h, off+2*h)
	onScreenFrom, onScreenTo := pages.Visible(v.prefix, off, off+h)
	windowChanged := from != v.lastFrom || to != v.lastTo
	screenChanged := onScreenFrom != v.lastScreenFrom || onScreenTo != v.lastScreenTo

	if windowChanged || screenChanged {
		v.windowUpdates++
		v.lastFrom, v.lastTo = from, to
		v.lastScreenFrom, v.lastScreenTo = onScreenFrom, onScreenTo
		if windowChanged {
			for page, it := range v.items {
				if page < from || page >= to {
					v.release(it)
					delete(v.items, page)
				}
			}
		}
		// новые приоритеты: видимые страницы — раньше соседних
		v.r.loader.NewGeneration()
		for page := from; page < to; page++ {
			it, ok := v.items[page]
			if !ok {
				it = v.acquire(page)
				v.items[page] = it
			}
			prio := pages.PriorityNeighbor
			if page >= onScreenFrom && page < onScreenTo {
				prio = pages.PriorityCurrent
			}
			v.request(it, prio)
		}
		if windowChanged {
			v.content.Refresh() // новые страницы — в дерево объектов
		}
	}

	if top := pages.TopPage(v.prefix, off+1); top != v.top {
		v.top = top
		v.r.pageChanged(top)
	}
}

// --- элементы ---

func (v *stripView) acquire(page int) *stripItem {
	var it *stripItem
	if n := len(v.pool); n > 0 {
		it, v.pool = v.pool[n-1], v.pool[:n-1]
	} else {
		it = &stripItem{status: widget.NewLabel("")}
		it.status.Alignment = fyne.TextAlignCenter
		it.box = container.NewWithoutLayout(it.status)
	}
	it.page = page
	it.loaded, it.pending = false, false
	it.res = pages.Result{}
	it.status.SetText("Загрузка…")
	it.status.Show()
	size := v.r.physical(v.width, 0)
	it.req = pages.Normalize(pages.Request{
		Key: v.g.Key, Page: v.g.Pages[page].Name, W: size, SliceH: pages.MaxSliceHeight,
	})
	return it
}

// request запрашивает страницу элемента (или освежает приоритет запроса).
func (v *stripView) request(it *stripItem, prio pages.Priority) {
	if it.loaded {
		return
	}
	if res, ok := v.r.loader.Cached(it.req); ok {
		v.apply(it, it.req, res)
		return
	}
	var cb func(pages.Result)
	if !it.pending {
		req := it.req
		cb = func(res pages.Result) { v.r.do(func() { v.apply(it, req, res) }) }
	}
	it.pending = true
	v.r.loader.Load(it.req, prio, cb)
}

func (v *stripView) apply(it *stripItem, req pages.Request, res pages.Result) {
	if it.req != req || it.loaded || !v.r.visible {
		return // элемент уже показывает другую страницу
	}
	it.pending = false
	if errors.Is(res.Err, pages.ErrStale) {
		if cur, ok := v.items[it.page]; ok && cur == it {
			v.request(it, pages.PriorityNeighbor)
		}
		return
	}
	it.loaded = true
	it.res = res
	if res.Err != nil || len(res.Parts) == 0 {
		it.status.SetText("Не удалось открыть страницу " + itoa(it.page+1))
		v.setHeight(it.page, errorHeight) // не оставлять большой пустой области
		v.content.Refresh()
		return
	}
	it.status.Hide()
	for len(it.parts) < len(res.Parts) {
		img := canvas.NewImageFromImage(nil)
		img.FillMode = canvas.ImageFillStretch
		img.ScaleMode = canvas.ImageScaleFastest // кусок уже нужного размера: без пересчёта в UI-потоке
		it.parts = append(it.parts, img)
		it.box.Add(img)
	}
	for i, img := range it.parts {
		if i < len(res.Parts) {
			img.Image = res.Parts[i]
			img.Show()
			img.Refresh()
		} else {
			img.Image = nil
			img.Hide()
		}
	}
	// размер из заголовка был неизвестен — уточняем высоту страницы
	if s := v.sizes[it.page]; s.Width <= 0 || s.Height <= 0 {
		v.setHeight(it.page, v.width*float32(res.H)/float32(res.W))
	}
	v.content.Refresh()
}

// setHeight меняет высоту страницы; если она выше текущей позиции,
// смещение прокрутки корректируется, чтобы экран не «прыгал».
func (v *stripView) setHeight(page int, h float32) {
	d := h - v.heights[page]
	if math.Abs(float64(d)) <= 1 {
		return
	}
	v.heights[page] = h
	v.prefix = pages.Prefix(v.heights)
	v.lastFrom = -1 // границы страниц сдвинулись
	v.scroll.Refresh()
	if v.prefix[page] < v.scroll.Offset.Y && page != v.top {
		v.scroll.ScrollToOffset(fyne.NewPos(0, max(0, v.scroll.Offset.Y+d)))
	}
}

func (v *stripView) release(it *stripItem) {
	for _, img := range it.parts {
		img.Image = nil
		img.Hide()
	}
	it.req = pages.Request{}
	it.loaded, it.pending = false, false
	it.res = pages.Result{}
	v.pool = append(v.pool, it)
}

func (v *stripView) releaseAll() {
	for page, it := range v.items {
		v.release(it)
		delete(v.items, page)
	}
	v.lastFrom = -1 // окно пусто — следующий updateVisible строит его заново
}

// layoutItem размещает куски страницы по целым физическим пикселям.
func (v *stripView) layoutItem(it *stripItem, w, h float32) {
	it.status.Move(fyne.NewPos(0, max(0, h/2-it.status.MinSize().Height/2)))
	it.status.Resize(fyne.NewSize(w, it.status.MinSize().Height))
	if !it.loaded || it.res.W <= 0 {
		return
	}
	scale := v.r.scale()
	perPx := w / float32(it.res.W) // логических единиц на пиксель изображения
	y := 0
	for i, img := range it.parts {
		if i >= len(it.res.Parts) {
			break
		}
		ph := it.res.Parts[i].Bounds().Dy()
		top := snap(float32(y)*perPx, scale)
		bottom := snap(float32(y+ph)*perPx, scale)
		img.Move(fyne.NewPos(0, top))
		img.Resize(fyne.NewSize(w, bottom-top))
		y += ph
	}
}

func snap(v, scale float32) float32 {
	return float32(math.Round(float64(v*scale))) / scale
}

type stripRenderer struct {
	v      *stripView
	center *fyne.Container
}

func (s *stripRenderer) Layout(size fyne.Size) {
	s.v.scroll.Resize(size)
	s.center.Resize(size)
	if size.Width > 0 && size.Width != s.v.width && s.v.sizes != nil {
		s.v.layout(size.Width)
	} else {
		s.v.updateVisible()
	}
}

func (s *stripRenderer) MinSize() fyne.Size { return fyne.NewSize(1, 1) }
func (s *stripRenderer) Refresh()           { s.Layout(s.v.Size()) }
func (s *stripRenderer) Destroy()           {}
func (s *stripRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{s.v.scroll, s.center}
}

// stripContent — содержимое прокрутки: общая высота всех страниц,
// дочерние объекты — только размещённые страницы. Тап переключает панель.
type stripContent struct {
	widget.BaseWidget
	v *stripView
}

func newStripContent(v *stripView) *stripContent {
	c := &stripContent{v: v}
	c.ExtendBaseWidget(c)
	return c
}

func (c *stripContent) Tapped(*fyne.PointEvent) { c.v.r.togglePanel() }

func (c *stripContent) CreateRenderer() fyne.WidgetRenderer { return &contentRenderer{c: c} }

type contentRenderer struct{ c *stripContent }

func (r *contentRenderer) Layout(size fyne.Size) {
	v := r.c.v
	for page, it := range v.items {
		if page >= len(v.heights) {
			continue
		}
		it.box.Move(fyne.NewPos(0, v.prefix[page]))
		it.box.Resize(fyne.NewSize(size.Width, v.heights[page]))
		v.layoutItem(it, size.Width, v.heights[page])
	}
}

func (r *contentRenderer) MinSize() fyne.Size {
	v := r.c.v
	if len(v.prefix) == 0 {
		return fyne.NewSize(1, 1)
	}
	return fyne.NewSize(1, max(1, v.prefix[len(v.prefix)-1]))
}

func (r *contentRenderer) Refresh() {
	r.Layout(r.c.Size())
	r.c.v.r.win.Canvas().Refresh(r.c)
}

func (r *contentRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.c.v.items))
	for _, it := range r.c.v.items {
		objs = append(objs, it.box)
	}
	return objs
}

func (r *contentRenderer) Destroy() {}

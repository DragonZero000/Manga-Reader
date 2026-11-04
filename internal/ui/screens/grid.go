package screens

import (
	"image"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/model"
	"mangareader/internal/thumbs"
)

// galleryGrid — сетка карточек галерей (обложка + название) с фоновой
// загрузкой миниатюр. Общая для библиотеки и поиска. Меняется только в UI-потоке.
type galleryGrid struct {
	thumbs  *thumbs.Cache
	metrics *GridMetrics
	actions *GalleryActions
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах.
	do func(func())

	items []model.Gallery
	grid  *widget.GridWrap
	// box — GridWrap в контейнере, который подгоняет размер карточек под
	// ширину сетки перед раскладкой; view — box со сглаживанием колеса (ПК).
	box  *fyne.Container
	view fyne.CanvasObject
	// cellH — высота карточки (0 — пересчитать).
	cellH float32
	// boxKey — область миниатюры («WxH»), часть ключа пула обложек.
	boxKey string
	// covers показывает обложки по бюджету и хранит пул их объектов.
	covers *coverPresenter

	// Прокрутка при прошлом track: смещение, направление (+1 вниз, −1 вверх),
	// видимый диапазон и число галерей.
	lastOff                   float32
	dir                       int
	lastFrom, lastTo, lastDir int
	lastLen                   int
}

// newGalleryGrid создаёт сетку; нажатие на карточку — actions.Open, меню
// карточки («⋮», правый клик, долгое нажатие) — actions.CardMenu.
func newGalleryGrid(th *thumbs.Cache, m *GridMetrics, actions *GalleryActions) *galleryGrid {
	g := &galleryGrid{thumbs: th, metrics: m, actions: actions, do: fyne.Do, dir: 1, lastFrom: -1}
	g.covers = newCoverPresenter(g)
	m.grids = append(m.grids, g)
	g.grid = widget.NewGridWrap(
		func() int { return len(g.items) },
		func() fyne.CanvasObject { return g.newCard() },
		func(id widget.GridWrapItemID, o fyne.CanvasObject) { g.updateCard(id, o.(*galleryCard)) },
	)
	// выбор с клавиатуры (ПК); нажатия мышью и пальцем обрабатывает карточка
	g.grid.OnSelected = func(id widget.GridWrapItemID) {
		g.grid.UnselectAll()
		g.open(id)
	}
	g.box = container.New(gridLayout{g}, g.grid)
	g.view = WithWheel(g, g.box)
	return g
}

func (g *galleryGrid) newCard() *galleryCard {
	c := newGalleryCard(g.metrics)
	c.onTap = g.open
	c.onMenu = g.showMenu
	return c
}

// item — галерея на позиции id; false — позиции уже нет.
func (g *galleryGrid) item(id widget.GridWrapItemID) (model.Gallery, bool) {
	if id < 0 || id >= len(g.items) {
		return model.Gallery{}, false
	}
	return g.items[id], true
}

func (g *galleryGrid) open(id widget.GridWrapItemID) {
	if it, ok := g.item(id); ok && g.actions.Open != nil {
		g.actions.Open(it)
	}
}

// showMenu показывает меню галереи id: в точке pos или, если pos == nil,
// под объектом anchor.
func (g *galleryGrid) showMenu(id widget.GridWrapItemID, anchor fyne.CanvasObject, pos *fyne.Position) {
	it, ok := g.item(id)
	if !ok {
		return
	}
	m := g.actions.CardMenu(it)
	if pos == nil {
		ShowMenuBelow(m, anchor)
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(anchor); c != nil && len(m.Items) > 0 {
		showMenu(m, c, *pos)
	}
}

// Remove убирает галерею с ключом k из сетки (после удаления файла);
// false — её в сетке нет.
func (g *galleryGrid) Remove(k model.Key) bool {
	for i, it := range g.items {
		if it.Key == k {
			items := make([]model.Gallery, 0, len(g.items)-1)
			g.items = append(append(items, g.items[:i]...), g.items[i+1:]...)
			g.grid.Refresh()
			return true
		}
	}
	return false
}

// Widget — объект сетки для размещения на экране.
func (g *galleryGrid) Widget() fyne.CanvasObject { return g.view }

// ScrollOffset — смещение прокрутки сетки (ScrollTarget).
func (g *galleryGrid) ScrollOffset() float32 { return g.grid.GetScrollOffset() }

// MaxScrollOffset — наибольшее смещение: высота всех строк карточек минус
// высота сетки (ScrollTarget).
func (g *galleryGrid) MaxScrollOffset() float32 {
	cols := g.grid.ColumnCount()
	if len(g.items) == 0 || cols < 1 {
		return 0
	}
	pad := theme.Padding()
	rows := (len(g.items) + cols - 1) / cols
	return max(0, (g.cellHeight()+pad)*float32(rows)-pad-g.grid.Size().Height)
}

// ScrollToOffset прокручивает сетку к смещению y (ScrollTarget).
func (g *galleryGrid) ScrollToOffset(y float32) {
	g.grid.ScrollToOffset(max(0, min(y, g.MaxScrollOffset())))
}

// Draggable — цель броска для сценариев замера (ScrollTarget).
func (g *galleryGrid) Draggable() fyne.Draggable { return DragOf(g) }

// cellHeight — высота карточки (строки сетки без отступа).
func (g *galleryGrid) cellHeight() float32 {
	if g.cellH == 0 {
		g.cellH = newGalleryCard(g.metrics).MinSize().Height
	}
	return g.cellH
}

// gridLayout раскладывает GridWrap на всю площадь, перед этим подгоняя
// размер карточек под ширину сетки (телефон).
type gridLayout struct{ g *galleryGrid }

func (l gridLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	l.g.fit(size.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(size)
}

func (l gridLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// fit подгоняет ширину карточки на телефоне под ширину сетки width. Ширина
// считается по короткой стороне экрана: в альбомной ориентации карточки
// того же размера, что в портретной, а колонок больше.
func (g *galleryGrid) fit(width float32) {
	if !g.metrics.mobile || width <= 0 {
		return
	}
	c := g.canvas()
	if c == nil {
		return
	}
	size := c.Size()
	_, area := c.InteractiveArea()
	// поля вокруг сетки (отступы экрана) — те же в любой ориентации
	avail := min(size.Width, size.Height) - (area.Width - width)
	if g.metrics.fit(avail) {
		g.metrics.changed()
	}
}

// canvas — холст сетки; до первой отрисовки — холст окна приложения (на
// телефоне оно одно). nil — окна ещё нет.
func (g *galleryGrid) canvas() fyne.Canvas {
	if c := fyne.CurrentApp().Driver().CanvasForObject(g.grid); c != nil {
		return c
	}
	if ws := fyne.CurrentApp().Driver().AllWindows(); len(ws) > 0 {
		return ws[0].Canvas()
	}
	return nil
}

// apply перестраивает сетку под новый размер карточек: GridWrap пересчитывает
// размер ячейки и число колонок, карточки заново запрашивают обложки под
// новую область (до загрузки — заглушка).
func (g *galleryGrid) apply() {
	g.cellH = 0
	g.lastFrom = -1
	g.covers.reset()
	g.setThumbBox()
	g.grid.Refresh()
	g.grid.Resize(g.grid.Size()) // сбрасывает кэш числа колонок GridWrap
}

// SetItems заменяет содержимое сетки и прокручивает её в начало.
func (g *galleryGrid) SetItems(items []model.Gallery) {
	g.items = items
	g.grid.Refresh()
}

// Items — текущие галереи сетки.
func (g *galleryGrid) Items() []model.Gallery { return g.items }

// updateCard заполняет переиспользуемую карточку данными галереи. Если
// карточка уже показывает обложку этой галереи, ничего не меняется: полное
// обновление сетки (сканирование, поиск, смена вкладки) не загружает
// обложки заново.
func (g *galleryGrid) updateCard(id widget.GridWrapItemID, c *galleryCard) {
	if id < 0 || id >= len(g.items) {
		return
	}
	gal := g.items[id]
	c.id = id
	c.setTitle(gal.Title)
	g.track()

	key := thumbs.CacheKey(gal)
	if c.thumbKey != key {
		if c.cancel != nil {
			c.cancel() // карточка теперь для другой галереи — прежняя обложка не нужна
			c.cancel = nil
		}
		g.covers.dequeue(c)
	}
	c.thumbKey = key
	if !g.setThumbBox() {
		g.covers.dequeue(c)
		c.showLoading() // размер экрана ещё неизвестен — обложка при следующем обновлении
		return
	}
	poolKey := key + "@" + g.boxKey
	if c.want == poolKey && (c.shown != nil || c.ready != nil) {
		return // показывает или ждёт очереди показа
	}
	if img, err, ok := g.thumbs.Cached(gal); ok {
		g.covers.present(c, poolKey, img, err)
		return
	}
	if c.cancel != nil {
		return // эта обложка уже грузится для карточки
	}
	c.showLoading()
	c.cancel = g.thumbs.Load(gal, func(img image.Image, err error) {
		g.do(func() {
			// карточка могла быть переиспользована под другую галерею
			if c.thumbKey == key {
				c.cancel = nil
				g.covers.present(c, poolKey, img, err)
			}
		})
	})
}

// track следит за прокруткой: направление (по изменению смещения), отметка
// видимых объектов пула, ёмкость пула и окно предзагрузки миниатюр.
// Работает, только когда видимый диапазон или направление изменились.
func (g *galleryGrid) track() {
	off := g.grid.GetScrollOffset()
	if off > g.lastOff {
		g.dir = 1
	} else if off < g.lastOff {
		g.dir = -1
	}
	g.lastOff = off
	from, to := g.visibleRange()
	if from == g.lastFrom && to == g.lastTo && g.dir == g.lastDir && len(g.items) == g.lastLen {
		return
	}
	// объекты прежнего диапазона были на экране до сих пор, нового — сейчас
	g.covers.seeVisible(g.lastFrom, g.lastTo)
	g.covers.seeVisible(from, to)
	g.lastFrom, g.lastTo, g.lastDir, g.lastLen = from, to, g.dir, len(g.items)
	cols := max(1, g.grid.ColumnCount())
	g.covers.pool.setLimit(cols * g.visibleRows() * poolScreens)
	ids := prefetchWindow(from, to, cols, g.visibleRows(), len(g.items), g.dir)
	gs := make([]model.Gallery, len(ids))
	for i, id := range ids {
		gs[i] = g.items[id]
	}
	g.thumbs.Prefetch(gs)
}

// rowHeight — высота строки сетки с отступом.
func (g *galleryGrid) rowHeight() float32 { return g.cellHeight() + theme.Padding() }

// visibleRows — сколько строк помещается на экране (как считает GridWrap).
func (g *galleryGrid) visibleRows() int {
	return int(math.Ceil(float64(g.grid.Size().Height/g.rowHeight()))) + 1
}

// visibleRange — карточки [from, to), которые GridWrap держит на экране при
// текущем смещении. Формула GridWrap: последняя карточка включительно, то
// есть одна лишняя в строке ниже экрана — её UpdateItem при прокрутке уже
// не вызывается, поэтому она тоже считается видимой.
func (g *galleryGrid) visibleRange() (from, to int) {
	n := len(g.items)
	cols := max(1, g.grid.ColumnCount())
	row := int(g.grid.GetScrollOffset() / g.rowHeight())
	lastRow := min(row+g.visibleRows(), (n+cols-1)/cols)
	return min(row*cols, n), min(lastRow*cols, n-1) + 1
}

// prefetchWindow — карточки для предзагрузки миниатюр в порядке важности:
// экран (rows строк) выше и ниже видимых [from, to), а в направлении
// прокрутки dir — два экрана. Сначала карточки по направлению прокрутки от
// ближней, затем в обратную сторону. Видимые не входят: их грузит updateCard.
func prefetchWindow(from, to, cols, rows, n, dir int) []int {
	if cols < 1 || rows < 1 || from >= to {
		return nil
	}
	down, up := 2*rows*cols, rows*cols
	if dir < 0 {
		down, up = up, down
	}
	below := make([]int, 0, down)
	for id := to; id < min(n, to+down); id++ {
		below = append(below, id)
	}
	above := make([]int, 0, up)
	for id := from - 1; id >= max(0, from-up); id-- {
		above = append(above, id)
	}
	if dir < 0 {
		return append(above, below...)
	}
	return append(below, above...)
}

// setThumbBox задаёт кэшу миниатюр область обложки карточки в физических
// пикселях экрана (boxKey — она же в ключе пула). false — сетка ещё не на
// экране и масштаб неизвестен.
func (g *galleryGrid) setThumbBox() bool {
	c := fyne.CurrentApp().Driver().CanvasForObject(g.grid)
	if c == nil {
		return false
	}
	scale, cover := c.Scale(), g.metrics.Cover()
	w, h := int(math.Ceil(float64(cover.Width*scale))), int(math.Ceil(float64(cover.Height*scale)))
	g.thumbs.SetBox(w, h)
	g.boxKey = strconv.Itoa(w) + "x" + strconv.Itoa(h)
	return true
}

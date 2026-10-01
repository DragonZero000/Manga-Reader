package reader

import (
	"errors"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/pages"
)

const (
	swipeMinDP    = 48
	swipeMinShare = 0.15
	// tapRadius — наибольшее расстояние между тапами двойного тапа.
	tapRadius = 40
	// previewDelay — задержка запроса страницы при перемещении ползунка:
	// пройденные страницы не декодируются.
	previewDelay = 80 * time.Millisecond
)

var (
	// tapWindow — окно ожидания второго тапа. Читалка распознаёт двойной
	// тап сама: драйвер Fyne задерживает Tapped у DoubleTappable на
	// системный интервал двойного щелчка (обычно 500 мс).
	tapWindow = 250 * time.Millisecond
	// doubleTapSides — двойной тап распознаётся и в боковых зонах (тогда
	// их тап ждёт tapWindow); false — только в центре, боковые листают сразу.
	doubleTapSides = true
)

// pagedView — постраничный режим: одна страница, вписанная в экран,
// тап-зоны, свайп, масштаб (двойной тап, Ctrl+колесо, щипок) и
// перетаскивание. Тапы приходят без задержки (виджет не DoubleTappable),
// двойной тап распознаёт сам вид.
type pagedView struct {
	widget.BaseWidget
	r *Reader

	g    model.Gallery
	page int

	img     *canvas.Image
	imgPx   pages.Sz // размер загруженного изображения в пикселях
	status  *widget.Label
	spinner *widget.Activity

	want pages.Request // ожидаемый результат; остальные отбрасываются
	// scale — масштаб относительно вписанной страницы (1 — вписано);
	// offset — левый верхний угол изображения при scale > 1
	scale   float32
	offset  pages.Pt
	dragAcc fyne.Delta
	// dir — направление последнего перелистывания (+1/−1) для предзагрузки.
	dir int

	// tapPending — первый тап ждёт второго (tapPos); tapSeq отбрасывает
	// устаревшие таймеры.
	tapPending bool
	tapPos     fyne.Position
	tapSeq     int
	// previewSeq отбрасывает устаревшие отложенные запросы ползунка.
	previewSeq int
	// sharpSeq отбрасывает устаревшие отложенные загрузки чёткой версии.
	sharpSeq int

	// touches — пальцы на странице (Android); pinching — идёт щипок;
	// suppressTap — тапы после щипка игнорируются до следующего жеста.
	touches     map[int]fyne.Position
	pinching    bool
	pinch       pinchState
	suppressTap bool
}

func newPagedView(r *Reader) *pagedView {
	v := &pagedView{
		r:       r,
		img:     canvas.NewImageFromImage(nil),
		status:  widget.NewLabel(""),
		spinner: widget.NewActivity(),
		dir:     1,
		scale:   1,
		touches: map[int]fyne.Position{},
	}
	v.img.FillMode = canvas.ImageFillStretch
	v.img.ScaleMode = canvas.ImageScaleFastest // картинка уже нужного размера: без пересчёта в UI-потоке
	v.status.Alignment = fyne.TextAlignCenter
	v.status.Wrapping = fyne.TextWrapWord
	v.ExtendBaseWidget(v)
	return v
}

func (v *pagedView) CreateRenderer() fyne.WidgetRenderer {
	center := container.NewCenter(container.NewVBox(v.spinner, v.status))
	return &pagedRenderer{v: v, center: center, objects: []fyne.CanvasObject{v.img, center}}
}

// --- view ---

func (v *pagedView) show(g model.Gallery, page int) {
	v.g = g
	v.dir = 1
	v.showPage(page)
}

func (v *pagedView) goTo(page int) { v.showPage(page) }

func (v *pagedView) setSizes([]library.PageSize) {}

func (v *pagedView) reset() {
	v.tapPending = false
	v.tapSeq++
	v.previewSeq++
	v.want = pages.Request{}
	v.img.Image = nil
	v.img.Hide()
	v.imgPx = pages.Sz{}
	v.resetZoom()
	v.touches = map[int]fyne.Position{}
	v.pinching, v.suppressTap = false, false
	v.setLoading(false)
	v.status.SetText("")
	v.Refresh()
}

func (v *pagedView) typedKey(k fyne.KeyName, repeat bool) {
	fwd, back := fyne.KeyRight, fyne.KeyLeft
	if v.r.rtl {
		fwd, back = back, fwd
	}
	switch k {
	case fwd, fyne.KeyPageDown, fyne.KeySpace, fyne.KeyDown:
		if !v.r.throttled(repeat) {
			v.step(1)
		}
	case back, fyne.KeyPageUp, fyne.KeyUp:
		if !v.r.throttled(repeat) {
			v.step(-1)
		}
	case fyne.KeyHome:
		v.showPage(0)
	case fyne.KeyEnd:
		v.showPage(len(v.g.Pages) - 1)
	}
}

// --- загрузка ---

// step листает на d страниц (+1 — вперёд). Вперёд с последней страницы —
// завершение чтения, назад с первой — ничего.
func (v *pagedView) step(d int) {
	n := len(v.g.Pages)
	next := v.page + d
	switch {
	case n == 0 || next < 0:
		return // граница книги
	case next >= n:
		v.r.tryFinish()
		return
	}
	v.dir = d
	probeFlip()
	v.showPage(next)
}

// showPage показывает страницу: сразу убирает прежнее изображение,
// запрашивает текущую страницу и предзагружает 2 страницы в направлении
// листания и 1 в обратном.
func (v *pagedView) showPage(page int) {
	if len(v.g.Pages) == 0 {
		v.reset()
		v.status.SetText(i18n.T("reader.no_pages"))
		return
	}
	page = max(0, min(page, len(v.g.Pages)-1))
	v.page = page
	v.resetZoom()
	v.previewSeq++
	v.r.pageChanged(page)
	v.r.loader.NewGeneration()

	req := v.request(page, 1)
	v.load(req, pages.PriorityCurrent)
	for _, n := range v.neighbors(page) {
		v.r.loader.Load(v.request(n, 1), pages.PriorityNeighbor, nil)
	}
}

// neighbors — страницы для предзагрузки в порядке очереди.
func (v *pagedView) neighbors(page int) []int {
	var out []int
	for _, n := range []int{page + v.dir, page + 2*v.dir, page - v.dir} {
		if n >= 0 && n < len(v.g.Pages) {
			out = append(out, n)
		}
	}
	return out
}

// preview показывает страницу при перемещении ползунка: из кэша — сразу,
// иначе индикатор и запрос через previewDelay (новое перемещение его
// отменяет). Соседние страницы не загружаются.
func (v *pagedView) preview(page int) {
	if len(v.g.Pages) == 0 {
		return
	}
	page = max(0, min(page, len(v.g.Pages)-1))
	v.page = page
	v.resetZoom()
	v.r.pageChanged(page)
	v.r.loader.NewGeneration()

	req := pages.Normalize(v.request(page, 1))
	v.want = req
	v.previewSeq++
	if res, ok := v.r.loader.Cached(req); ok {
		v.apply(req, res)
		return
	}
	v.showLoading()
	seq := v.previewSeq
	time.AfterFunc(previewDelay, func() {
		v.r.do(func() {
			if seq != v.previewSeq || req != v.want {
				return
			}
			v.fetch(req, pages.PriorityCurrent)
		})
	})
}

// request — запрос страницы в разрешении «вписать × zoom» (физ. px), не
// больше ограничения текстуры.
func (v *pagedView) request(page int, zoom float32) pages.Request {
	size := v.r.viewSize()
	limit := textureLimit()
	return pages.Request{
		Key:  v.g.Key,
		Page: v.g.Pages[page].Name,
		W:    v.r.physical(size.Width*zoom, limit),
		H:    v.r.physical(size.Height*zoom, limit),
	}
}

func (v *pagedView) load(req pages.Request, prio pages.Priority) {
	req = pages.Normalize(req)
	v.want = req
	if res, ok := v.r.loader.Cached(req); ok {
		probeShown(true)
		v.apply(req, res)
		return
	}
	v.showLoading()
	v.fetch(req, prio)
}

// showLoading убирает изображение прежней страницы и показывает индикатор.
func (v *pagedView) showLoading() {
	v.img.Image = nil
	v.img.Hide()
	v.status.SetText("")
	v.setLoading(true)
	v.Refresh()
}

func (v *pagedView) fetch(req pages.Request, prio pages.Priority) {
	v.r.loader.Load(req, prio, func(res pages.Result) {
		v.r.do(func() { v.apply(req, res) })
	})
}

func (v *pagedView) apply(req pages.Request, res pages.Result) {
	if req != v.want || !v.r.visible {
		return // устаревший результат
	}
	if errors.Is(res.Err, pages.ErrStale) {
		return
	}
	v.setLoading(false)
	if res.Err != nil || len(res.Parts) == 0 {
		v.img.Image = nil
		v.img.Hide()
		v.imgPx = pages.Sz{}
		v.status.SetText(i18n.T("reader.page_failed", "Page", v.page+1))
		v.Refresh()
		return
	}
	v.status.SetText("")
	probeShown(false)
	v.img.Image = res.Parts[0]
	v.imgPx = pages.Sz{W: float32(res.W), H: float32(res.H)}
	v.img.Show()
	v.img.Refresh()
	v.Refresh()
}

// setLoading показывает индикатор загрузки; остановленный индикатор
// скрывается (иначе остаётся точкой в центре страницы).
func (v *pagedView) setLoading(on bool) {
	if on {
		v.spinner.Show()
		v.spinner.Start()
	} else {
		v.spinner.Stop()
		v.spinner.Hide()
	}
}

// --- жесты ---

// Tapped приходит сразу при отпускании. Тап ждёт второго не дольше
// tapWindow: второй тап рядом — двойной тап, иначе — одиночный. Если
// двойной тап выключен в настройках, тап срабатывает сразу.
func (v *pagedView) Tapped(ev *fyne.PointEvent) {
	if v.suppressTap {
		return // отпускание пальцев после щипка
	}
	pos := ev.Position
	if v.r.doubleTap == 0 {
		v.singleTap(pos)
		return
	}
	if v.tapPending {
		v.tapPending = false
		v.tapSeq++
		if dist(pos, v.tapPos) <= tapRadius {
			v.doubleTap(pos)
			return
		}
		v.singleTap(v.tapPos) // второй тап далеко — первый был одиночным
	}
	if !doubleTapSides && v.zone(pos) != 0 && v.scale <= 1 {
		v.singleTap(pos)
		return
	}
	v.tapPending = true
	v.tapPos = pos
	v.tapSeq++
	seq := v.tapSeq
	time.AfterFunc(tapWindow, func() {
		v.r.do(func() {
			if seq != v.tapSeq || !v.tapPending || !v.r.visible {
				return
			}
			v.tapPending = false
			v.singleTap(v.tapPos)
		})
	})
}

// zone — тап-зона точки: −1 левая треть, +1 правая, 0 центр.
func (v *pagedView) zone(p fyne.Position) int {
	w := v.Size().Width
	switch {
	case p.X <= w/3:
		return -1
	case p.X >= w*2/3:
		return 1
	}
	return 0
}

// singleTap: центр (или увеличенная страница) — панель, боковые трети —
// листание с учётом направления чтения.
func (v *pagedView) singleTap(p fyne.Position) {
	z := v.zone(p)
	if v.scale > 1 || z == 0 {
		v.r.togglePanel()
		return
	}
	if v.r.rtl {
		z = -z
	}
	v.step(z)
}

// doubleTap: вписанная страница — масштаб из настроек, увеличенная — 100%.
func (v *pagedView) doubleTap(p fyne.Position) {
	if v.scale > 1 {
		v.zoomTo(1, p)
		return
	}
	v.zoomTo(v.r.doubleTap, p)
}

func (v *pagedView) Dragged(ev *fyne.DragEvent) {
	if v.pinching {
		return // масштаб и сдвиг задаёт щипок
	}
	if v.scale > 1 {
		v.offset.X += ev.Dragged.DX
		v.offset.Y += ev.Dragged.DY
		v.clamp()
		v.Refresh()
		return
	}
	v.dragAcc.DX += ev.Dragged.DX
	v.dragAcc.DY += ev.Dragged.DY
}

func (v *pagedView) DragEnd() {
	d := v.dragAcc
	v.dragAcc = fyne.Delta{}
	if v.scale > 1 || v.pinching || v.suppressTap {
		return
	}
	threshold := max(float32(swipeMinDP), v.Size().Width*swipeMinShare)
	if abs(d.DX) < threshold || abs(d.DX) < abs(d.DY) {
		return
	}
	fwd := d.DX < 0 // палец справа налево — вперёд при чтении слева направо
	if v.r.rtl {
		fwd = !fwd
	}
	if fwd {
		v.step(1)
	} else {
		v.step(-1)
	}
}

// Scrolled: Ctrl+колесо меняет масштаб с точкой под курсором; без Ctrl
// колесо листает и на увеличенной странице (масштаб сбрасывается), не
// чаще flipInterval.
func (v *pagedView) Scrolled(ev *fyne.ScrollEvent) {
	if ev.Scrolled.DY == 0 {
		return
	}
	if ctrlPressed() {
		if ev.Scrolled.DY > 0 {
			v.zoomTo(v.scale*zoomStep, ev.Position)
		} else {
			v.zoomTo(v.scale/zoomStep, ev.Position)
		}
		return
	}
	if v.r.throttled(true) {
		return
	}
	if ev.Scrolled.DY < 0 {
		v.step(1)
	} else {
		v.step(-1)
	}
}

func dist(a, b fyne.Position) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

// --- геометрия ---

type rect struct {
	pos  fyne.Position
	size fyne.Size
}

// displayRect — где рисовать изображение с учётом масштаба и смещения.
func (v *pagedView) displayRect() rect {
	view := v.Size()
	if v.imgPx.W <= 0 || v.imgPx.H <= 0 {
		return rect{}
	}
	fit := pages.Fit(v.imgPx, pages.Sz{W: view.Width, H: view.Height})
	w, h := v.imgPx.W*fit, v.imgPx.H*fit
	if v.scale > 1 {
		w, h = w*v.scale, h*v.scale
		off := pages.ClampOffset(v.offset, pages.Sz{W: w, H: h}, pages.Sz{W: view.Width, H: view.Height})
		return rect{fyne.NewPos(off.X, off.Y), fyne.NewSize(w, h)}
	}
	return rect{fyne.NewPos((view.Width-w)/2, (view.Height-h)/2), fyne.NewSize(w, h)}
}

func (v *pagedView) clamp() {
	r := v.displayRect()
	v.offset = pages.Pt{X: r.pos.X, Y: r.pos.Y}
}

type pagedRenderer struct {
	v       *pagedView
	center  *fyne.Container
	objects []fyne.CanvasObject
}

func (p *pagedRenderer) Layout(size fyne.Size) {
	r := p.v.displayRect()
	p.v.img.Move(r.pos)
	p.v.img.Resize(r.size)
	p.center.Resize(size)
	p.v.r.badge.reposition()
}

func (p *pagedRenderer) MinSize() fyne.Size { return fyne.NewSize(theme.Padding(), theme.Padding()) }

func (p *pagedRenderer) Refresh() {
	p.Layout(p.v.Size())
	p.v.r.win.Canvas().Refresh(p.v)
}

func (p *pagedRenderer) Objects() []fyne.CanvasObject { return p.objects }

func (p *pagedRenderer) Destroy() {}

func abs(f float32) float32 {
	if f < 0 {
		return -f
	}
	return f
}

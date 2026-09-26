package reader

import (
	"errors"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/library"
	"mangareader/internal/model"
	"mangareader/internal/pages"
)

const (
	zoomFactor     = 2
	swipeMinDP     = 48
	swipeMinShare  = 0.15
	wheelInterval  = 250 * time.Millisecond
	wheelPanFactor = 1
)

// pagedView — постраничный режим: одна страница, вписанная в экран,
// тап-зоны, свайп, двойной тап ×2 и перетаскивание.
type pagedView struct {
	widget.BaseWidget
	r *Reader

	g    model.Gallery
	page int

	img     *canvas.Image
	imgPx   pages.Sz // размер загруженного изображения в пикселях
	status  *widget.Label
	spinner *widget.Activity

	want     pages.Request // ожидаемый результат; остальные отбрасываются
	zoomed   bool
	offset   pages.Pt
	dragAcc  fyne.Delta
	lastStep time.Time
}

func newPagedView(r *Reader) *pagedView {
	v := &pagedView{
		r:       r,
		img:     canvas.NewImageFromImage(nil),
		status:  widget.NewLabel(""),
		spinner: widget.NewActivity(),
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
	v.showPage(page)
}

func (v *pagedView) goTo(page int) { v.showPage(page) }

func (v *pagedView) setSizes([]library.PageSize) {}

func (v *pagedView) reset() {
	v.want = pages.Request{}
	v.img.Image = nil
	v.img.Hide()
	v.imgPx = pages.Sz{}
	v.zoomed = false
	v.setLoading(false)
	v.status.SetText("")
	v.Refresh()
}

func (v *pagedView) typedKey(k fyne.KeyName) {
	switch k {
	case fyne.KeyRight, fyne.KeyPageDown, fyne.KeySpace, fyne.KeyDown:
		v.step(1)
	case fyne.KeyLeft, fyne.KeyPageUp, fyne.KeyUp:
		v.step(-1)
	case fyne.KeyHome:
		v.showPage(0)
	case fyne.KeyEnd:
		v.showPage(len(v.g.Pages) - 1)
	}
}

// --- загрузка ---

func (v *pagedView) step(d int) {
	next := v.page + d
	if next < 0 || next >= len(v.g.Pages) {
		return // граница книги
	}
	v.showPage(next)
}

// showPage показывает страницу: сразу убирает прежнее изображение,
// запрашивает текущую страницу и предзагружает соседние.
func (v *pagedView) showPage(page int) {
	if len(v.g.Pages) == 0 {
		v.reset()
		v.status.SetText("В галерее нет страниц")
		return
	}
	page = max(0, min(page, len(v.g.Pages)-1))
	v.page = page
	v.zoomed = false
	v.offset = pages.Pt{}
	v.r.pageChanged(page)
	v.r.loader.NewGeneration()

	req := v.request(page, 1)
	v.load(req, pages.PriorityCurrent)
	for _, n := range []int{page + 1, page - 1} {
		if n >= 0 && n < len(v.g.Pages) {
			v.r.loader.Load(v.request(n, 1), pages.PriorityNeighbor, nil)
		}
	}
}

// request — запрос страницы в разрешении «вписать × zoom» (физ. px).
func (v *pagedView) request(page int, zoom float32) pages.Request {
	size := v.r.viewSize()
	return pages.Request{
		Key:  v.g.Key,
		Page: v.g.Pages[page].Name,
		W:    v.r.physical(size.Width*zoom, maxTexture),
		H:    v.r.physical(size.Height*zoom, maxTexture),
	}
}

func (v *pagedView) load(req pages.Request, prio pages.Priority) {
	req = pages.Normalize(req)
	v.want = req
	if res, ok := v.r.loader.Cached(req); ok {
		v.apply(req, res)
		return
	}
	// оставляем растянутое изображение только при догрузке высокого разрешения
	if !v.zoomed {
		v.img.Image = nil
		v.img.Hide()
		v.status.SetText("")
		v.setLoading(true)
		v.Refresh() // убрать изображение прежней страницы с экрана
	}
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
		v.status.SetText("Не удалось открыть страницу " + itoa(v.page+1))
		v.Refresh()
		return
	}
	v.status.SetText("")
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

func (v *pagedView) Tapped(ev *fyne.PointEvent) {
	w := v.Size().Width
	switch {
	case v.zoomed || (ev.Position.X > w/3 && ev.Position.X < w*2/3):
		v.r.togglePanel()
	case ev.Position.X <= w/3:
		v.step(-1)
	default:
		v.step(1)
	}
}

func (v *pagedView) DoubleTapped(ev *fyne.PointEvent) {
	if v.img.Image == nil {
		return
	}
	if v.zoomed {
		v.zoomed = false
		v.offset = pages.Pt{}
		v.Refresh()
		return
	}
	fitRect := v.displayRect()
	v.zoomed = true
	p := pages.Pt{X: ev.Position.X, Y: ev.Position.Y}
	v.offset = pages.ZoomAt(pages.Pt{X: fitRect.pos.X, Y: fitRect.pos.Y}, p, zoomFactor)
	v.clamp()
	v.Refresh()
	// более чёткая версия для увеличенного вида
	v.load(v.request(v.page, zoomFactor), pages.PriorityCurrent)
}

func (v *pagedView) Dragged(ev *fyne.DragEvent) {
	if v.zoomed {
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
	if v.zoomed {
		return
	}
	threshold := max(float32(swipeMinDP), v.Size().Width*swipeMinShare)
	if abs(d.DX) < threshold || abs(d.DX) < abs(d.DY) {
		return
	}
	if d.DX < 0 {
		v.step(1) // палец справа налево — следующая
	} else {
		v.step(-1)
	}
}

func (v *pagedView) Scrolled(ev *fyne.ScrollEvent) {
	if v.zoomed {
		v.offset.X += ev.Scrolled.DX * wheelPanFactor
		v.offset.Y += ev.Scrolled.DY * wheelPanFactor
		v.clamp()
		v.Refresh()
		return
	}
	if time.Since(v.lastStep) < wheelInterval || ev.Scrolled.DY == 0 {
		return
	}
	v.lastStep = time.Now()
	if ev.Scrolled.DY < 0 {
		v.step(1)
	} else {
		v.step(-1)
	}
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
	if v.zoomed {
		w, h = w*zoomFactor, h*zoomFactor
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

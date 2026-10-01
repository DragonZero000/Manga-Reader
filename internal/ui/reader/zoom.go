package reader

import (
	"fmt"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/driver/mobile"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/pages"
)

const (
	// maxScale — наибольший масштаб относительно вписанной страницы.
	maxScale = 5
	// zoomStep — шаг масштаба колесом и клавишами.
	zoomStep = 1.25
	// sharpDelay — пауза в изменении масштаба перед загрузкой чёткой версии.
	sharpDelay = 200 * time.Millisecond
	// badgeHide — через сколько скрывается индикатор при масштабе 100%.
	badgeHide = 1500 * time.Millisecond
)

// ZoomAction — действие сочетания клавиш масштаба.
type ZoomAction int

const (
	ZoomIn ZoomAction = iota + 1
	ZoomOut
	ZoomReset
)

// zoomShortcuts — сочетания масштаба: Ctrl+«=», Ctrl+«+» (цифровой блок),
// Ctrl+«−», Ctrl+0.
var zoomShortcuts = map[fyne.KeyName]ZoomAction{
	fyne.KeyEqual: ZoomIn,
	fyne.KeyPlus:  ZoomIn,
	fyne.KeyMinus: ZoomOut,
	fyne.Key0:     ZoomReset,
}

// ZoomShortcuts — сочетания масштаба для регистрации на canvas (ПК).
func (r *Reader) ZoomShortcuts() []fyne.Shortcut {
	out := make([]fyne.Shortcut, 0, len(zoomShortcuts))
	for k := range zoomShortcuts {
		out = append(out, &desktop.CustomShortcut{KeyName: k, Modifier: fyne.KeyModifierShortcutDefault})
	}
	return out
}

// ZoomShortcut выполняет сочетание масштаба: только в открытой читалке в
// постраничном режиме.
func (r *Reader) ZoomShortcut(s fyne.Shortcut) {
	cs, ok := s.(*desktop.CustomShortcut)
	if !ok {
		return
	}
	if a, ok := zoomShortcuts[cs.KeyName]; ok {
		r.Zoom(a)
	}
}

// Zoom меняет масштаб страницы относительно центра.
func (r *Reader) Zoom(a ZoomAction) {
	if !r.visible || r.mode != ModePaged {
		return
	}
	v := r.paged
	c := v.center()
	switch a {
	case ZoomIn:
		v.zoomTo(v.scale*zoomStep, c)
	case ZoomOut:
		v.zoomTo(v.scale/zoomStep, c)
	case ZoomReset:
		v.zoomTo(1, c)
	}
}

// Scale — текущий масштаб постраничного режима (1 — вписано).
func (r *Reader) Scale() float32 { return r.paged.scale }

// ctrlPressed — зажат ли Ctrl (только ПК); подменяется в тестах.
var ctrlPressed = func() bool {
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		return d.CurrentKeyModifiers()&fyne.KeyModifierControl != 0
	}
	return false
}

// textureLimit — наибольшая сторона изображения страницы: на телефоне
// меньше, чтобы увеличенная страница не вытесняла весь кэш.
func textureLimit() int {
	if fyne.CurrentDevice().IsMobile() {
		return maxTextureMobile
	}
	return maxTexture
}

// --- масштаб ---

func (v *pagedView) center() fyne.Position {
	s := v.Size()
	return fyne.NewPos(s.Width/2, s.Height/2)
}

// zoomTo меняет масштаб (в пределах 1…maxScale) так, что точка p области
// просмотра остаётся над той же точкой изображения.
func (v *pagedView) zoomTo(s float32, p fyne.Position) {
	if v.img.Image == nil {
		return
	}
	s = max(1, min(s, maxScale))
	r := v.displayRect()
	v.offset = pages.ZoomAt(pages.Pt{X: r.pos.X, Y: r.pos.Y}, pages.Pt{X: p.X, Y: p.Y}, s/v.scale)
	v.scale = s
	v.clamp()
	v.Refresh()
	v.zoomChanged()
}

// resetZoom возвращает вписанный размер без индикатора (смена страницы).
func (v *pagedView) resetZoom() {
	v.scale = 1
	v.offset = pages.Pt{}
	v.sharpSeq++
	v.r.badge.hideNow()
}

// zoomChanged показывает индикатор и откладывает загрузку чёткой версии.
func (v *pagedView) zoomChanged() {
	v.r.badge.show(v.scale)
	v.sharpSeq++
	seq := v.sharpSeq
	time.AfterFunc(sharpDelay, func() {
		v.r.do(func() {
			if seq == v.sharpSeq && v.r.visible {
				v.sharpen()
			}
		})
	})
}

// sharpen загружает страницу в разрешении текущего масштаба; растянутое
// изображение остаётся на экране до прихода чёткого.
func (v *pagedView) sharpen() {
	req := pages.Normalize(v.request(v.page, v.scale))
	if req == v.want {
		return
	}
	v.want = req
	if res, ok := v.r.loader.Cached(req); ok {
		v.apply(req, res)
		return
	}
	v.fetch(req, pages.PriorityCurrent)
}

// --- щипок (Android) ---

// TouchDown — палец коснулся страницы; второй палец начинает щипок.
func (v *pagedView) TouchDown(ev *mobile.TouchEvent) {
	if len(v.touches) == 0 {
		v.suppressTap = false // новый жест
	}
	v.touches[ev.ID] = ev.Position
	if len(v.touches) == 2 && v.img.Image != nil {
		v.startPinch()
	}
}

// TouchMoved — движение пальца; при двух пальцах меняет масштаб.
func (v *pagedView) TouchMoved(ev *mobile.TouchEvent) {
	if _, ok := v.touches[ev.ID]; !ok {
		return
	}
	v.touches[ev.ID] = ev.Position
	if v.pinching {
		v.updatePinch()
	}
}

// TouchUp — палец отпущен; конец щипка подавляет тапы до следующего жеста.
func (v *pagedView) TouchUp(ev *mobile.TouchEvent) { v.touchEnd(ev.ID) }

// TouchCancel — касание отменено.
func (v *pagedView) TouchCancel(ev *mobile.TouchEvent) { v.touchEnd(ev.ID) }

func (v *pagedView) touchEnd(id int) {
	delete(v.touches, id)
	if v.pinching && len(v.touches) < 2 {
		v.pinching = false
		v.suppressTap = true
		v.dragAcc = fyne.Delta{}
	}
}

// twoTouches — две точки касания в постоянном порядке.
func (v *pagedView) twoTouches() (a, b fyne.Position) {
	ids := make([]int, 0, 2)
	for id := range v.touches {
		ids = append(ids, id)
	}
	if ids[0] > ids[1] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	return v.touches[ids[0]], v.touches[ids[1]]
}

func (v *pagedView) startPinch() {
	a, b := v.twoTouches()
	d := dist(a, b)
	if d <= 0 {
		return
	}
	r := v.displayRect()
	v.pinching = true
	v.tapPending = false
	v.tapSeq++
	v.dragAcc = fyne.Delta{}
	v.pinch = pinchState{d0: d, s0: v.scale, mid0: midpoint(a, b), off0: pages.Pt{X: r.pos.X, Y: r.pos.Y}}
}

// updatePinch: масштаб — по отношению расстояний, точка между пальцами
// остаётся над той же точкой изображения и следует за пальцами.
func (v *pagedView) updatePinch() {
	a, b := v.twoTouches()
	p := v.pinch
	s := max(1, min(p.s0*dist(a, b)/p.d0, maxScale))
	mid := midpoint(a, b)
	off := pages.ZoomAt(p.off0, pages.Pt{X: p.mid0.X, Y: p.mid0.Y}, s/p.s0)
	v.offset = pages.Pt{X: off.X + mid.X - p.mid0.X, Y: off.Y + mid.Y - p.mid0.Y}
	v.scale = s
	v.clamp()
	v.Refresh()
	v.zoomChanged()
}

// pinchState — начало щипка: расстояние, масштаб, середина и положение
// изображения.
type pinchState struct {
	d0, s0 float32
	mid0   fyne.Position
	off0   pages.Pt
}

func midpoint(a, b fyne.Position) fyne.Position {
	return fyne.NewPos((a.X+b.X)/2, (a.Y+b.Y)/2)
}

// --- индикатор ---

// zoomBadge — индикатор масштаба в правом нижнем углу; тап — 100%.
type zoomBadge struct {
	widget.BaseWidget
	r    *Reader
	text *canvas.Text
	seq  int // отбрасывает устаревшие таймеры скрытия
}

func newZoomBadge(r *Reader) *zoomBadge {
	b := &zoomBadge{r: r, text: canvas.NewText("", theme.Color(theme.ColorNameForeground))}
	b.text.TextStyle.Bold = true
	b.ExtendBaseWidget(b)
	b.Hide()
	return b
}

func (b *zoomBadge) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	bg.CornerRadius = theme.InputRadiusSize()
	bg.StrokeColor = theme.Color(theme.ColorNameInputBorder)
	bg.StrokeWidth = 1
	return widget.NewSimpleRenderer(container.NewStack(bg, container.NewPadded(b.text)))
}

// Tapped возвращает масштаб 100%.
func (b *zoomBadge) Tapped(*fyne.PointEvent) {
	v := b.r.paged
	v.zoomTo(1, v.center())
}

// Text — подпись индикатора (для тестов).
func (b *zoomBadge) Text() string { return b.text.Text }

// show показывает масштаб s; при 100% индикатор скрывается через badgeHide.
func (b *zoomBadge) show(s float32) {
	b.text.Text = fmt.Sprintf("%d %%", int(math.Round(float64(s)*100)))
	b.text.Color = theme.Color(theme.ColorNameForeground) // тема могла смениться
	b.text.Refresh()
	b.seq++
	b.Show()
	b.reposition()
	if s > 1 {
		return
	}
	seq := b.seq
	time.AfterFunc(badgeHide, func() {
		b.r.do(func() {
			if seq == b.seq {
				b.hideNow()
			}
		})
	})
}

func (b *zoomBadge) hideNow() {
	b.seq++
	if b.Visible() {
		b.Hide()
		b.r.win.Canvas().Refresh(b)
	}
}

// reposition ставит индикатор в правый нижний угол, над нижней полосой
// панели, если она открыта.
func (b *zoomBadge) reposition() {
	if !b.Visible() {
		return
	}
	size := b.MinSize()
	b.Resize(size)
	view := b.r.viewSize()
	pad := theme.Padding()
	bottom := view.Height - pad
	if p := b.r.panel; p.layer.Visible() {
		bottom -= p.bottomBar.Size().Height
	}
	b.Move(fyne.NewPos(view.Width-size.Width-pad, bottom-size.Height))
	b.r.win.Canvas().Refresh(b)
}

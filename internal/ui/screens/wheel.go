package screens

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ScrollTarget — вертикальная прокрутка, которой управляют программно:
// сглаживание колеса мыши и сценарии замера кадров.
type ScrollTarget interface {
	// ScrollOffset — текущее смещение.
	ScrollOffset() float32
	// MaxScrollOffset — наибольшее смещение (0 — содержимое помещается).
	MaxScrollOffset() float32
	// ScrollToOffset прокручивает к смещению y (в пределах 0…MaxScrollOffset).
	ScrollToOffset(y float32)
	// Draggable — цель программного броска (инерции) для сценариев замера;
	// обычно DragOf(t).
	Draggable() fyne.Draggable
}

// ScrollOf — ScrollTarget вертикального container.Scroll.
func ScrollOf(s *container.Scroll) ScrollTarget { return scrollTarget{s} }

type scrollTarget struct{ s *container.Scroll }

func (t scrollTarget) ScrollOffset() float32 { return t.s.Offset.Y }

func (t scrollTarget) Draggable() fyne.Draggable { return DragOf(t) }

// DragOf — цель броска для t: Dragged сдвигает t через ScrollToOffset, так
// что бросок идёт одним путём на ПК и на телефоне, а ScrollOffset сообщает
// броску край прокрутки.
func DragOf(t ScrollTarget) fyne.Draggable { return targetDrag{t} }

type targetDrag struct{ t ScrollTarget }

func (d targetDrag) Dragged(e *fyne.DragEvent) {
	d.t.ScrollToOffset(d.t.ScrollOffset() - e.Dragged.DY)
}

func (targetDrag) DragEnd() {}

// ScrollOffset — смещение цели (для остановки броска на краю).
func (d targetDrag) ScrollOffset() fyne.Position { return fyne.NewPos(0, d.t.ScrollOffset()) }

func (t scrollTarget) MaxScrollOffset() float32 {
	return max(0, t.s.Content.MinSize().Height-t.s.Size().Height)
}

func (t scrollTarget) ScrollToOffset(y float32) {
	t.s.ScrollToOffset(fyne.NewPos(t.s.Offset.X, max(0, min(y, t.MaxScrollOffset()))))
}

// wheelDuration — длительность плавной прокрутки колесом: по времени, а не
// по числу кадров — одинаково на экранах любой частоты.
const wheelDuration = 100 * time.Millisecond

// WheelSmoother — плавная прокрутка колесом мыши (ПК): событие колеса
// сдвигает цель на то же расстояние, что Fyne сдвигал за одно событие, а
// смещение доезжает до цели анимацией wheelDuration. Новое событие во время
// анимации продолжает её от текущего смещения к новой цели. Виджет кладётся
// в Stack поверх цели и принимает только колесо: нажатия, наведение и
// перетаскивание полосы прокрутки проходят к объектам под ним.
type WheelSmoother struct {
	widget.BaseWidget
	target ScrollTarget

	from, goal float32
	running    bool
	anim       *fyne.Animation
	// startAnim запускает анимацию (a.Start); подменяется в тестах.
	startAnim func(*fyne.Animation)
}

var _ fyne.Scrollable = (*WheelSmoother)(nil)

// NewWheelSmoother создаёт сглаживатель колеса для target.
func NewWheelSmoother(target ScrollTarget) *WheelSmoother {
	w := &WheelSmoother{target: target, startAnim: (*fyne.Animation).Start}
	w.ExtendBaseWidget(w)
	return w
}

// WithWheel кладёт сглаживатель колеса поверх obj, прокрутку которого
// задаёт target. На телефоне колеса нет — obj без изменений.
func WithWheel(target ScrollTarget, obj fyne.CanvasObject) fyne.CanvasObject {
	if isMobile() {
		return obj
	}
	return container.NewStack(obj, NewWheelSmoother(target))
}

func (w *WheelSmoother) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(container.NewWithoutLayout())
}

// Scrolled сдвигает цель и (пере)запускает анимацию от текущего смещения.
func (w *WheelSmoother) Scrolled(ev *fyne.ScrollEvent) {
	if ev.Scrolled.DY == 0 {
		return
	}
	cur := w.target.ScrollOffset()
	if !w.running {
		w.goal = cur
	}
	w.goal = max(0, min(w.goal-ev.Scrolled.DY, w.target.MaxScrollOffset()))
	w.from = cur
	if w.anim != nil {
		w.anim.Stop()
	}
	w.anim = fyne.NewAnimation(wheelDuration, w.tick)
	w.anim.Curve = fyne.AnimationEaseOut
	w.running = true
	w.startAnim(w.anim)
}

func (w *WheelSmoother) tick(f float32) {
	w.target.ScrollToOffset(w.from + (w.goal-w.from)*f)
	if f >= 1 {
		w.running = false
	}
}

package ui

import (
	"image/color"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// DefaultToastDuration — время показа уведомления по умолчанию.
const DefaultToastDuration = 3 * time.Second

// Toast — слой кратких уведомлений поверх экрана. Кладётся в container.NewStack
// над основным содержимым; содержимое окна не заменяется.
// Методы Show/ShowFor безопасно вызывать из любой горутины.
type Toast struct {
	layer *fyne.Container
	label *widget.Label
	// do выполняет функцию в UI-потоке (fyne.Do); подменяется в тестах,
	// где тестовый драйвер Fyne не сериализует вызовы.
	do func(func())

	mu  sync.Mutex
	gen int // номер последнего уведомления: старые таймеры не скрывают новое
}

// NewToast создаёт скрытый слой уведомлений.
// bottomInset — отступ снизу (например, чтобы не перекрывать нижние вкладки).
func NewToast(bottomInset float32) *Toast {
	t := &Toast{label: widget.NewLabel(""), do: fyne.Do}
	t.label.Wrapping = fyne.TextWrapWord
	t.label.Alignment = fyne.TextAlignCenter

	bg := canvas.NewRectangle(toastBackground())
	bg.CornerRadius = theme.InputRadiusSize() * 2
	card := container.NewStack(bg, container.NewPadded(t.label))

	inset := canvas.NewRectangle(color.Transparent)
	inset.SetMinSize(fyne.NewSize(0, bottomInset))

	// Пустые контейнеры и надписи не принимают нажатия, поэтому слой
	// не мешает работе с экраном под ним.
	t.layer = container.NewBorder(nil, container.NewVBox(
		container.New(layout.NewCustomPaddedLayout(0, 0, theme.Padding()*4, theme.Padding()*4), card),
		inset,
	), nil, nil)
	t.layer.Hide()
	return t
}

// Layer возвращает объект слоя для размещения в Stack.
func (t *Toast) Layer() fyne.CanvasObject {
	return t.layer
}

// Show показывает уведомление на DefaultToastDuration.
func (t *Toast) Show(text string) {
	t.ShowFor(text, DefaultToastDuration)
}

// ShowFor показывает уведомление на заданное время. Новое уведомление
// заменяет текущее и перезапускает таймер.
func (t *Toast) ShowFor(text string, d time.Duration) {
	t.mu.Lock()
	t.gen++
	gen := t.gen
	t.mu.Unlock()

	t.do(func() {
		t.label.SetText(text)
		t.layer.Show()
		t.layer.Refresh() // Container.Show сам не перерисовывает
	})
	time.AfterFunc(d, func() {
		t.mu.Lock()
		current := t.gen == gen
		t.mu.Unlock()
		if current {
			t.do(t.layer.Hide)
		}
	})
}

func toastBackground() color.Color {
	return theme.Color(theme.ColorNameOverlayBackground)
}

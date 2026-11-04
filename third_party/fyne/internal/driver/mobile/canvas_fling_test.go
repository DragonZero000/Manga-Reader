//go:build mobile && (!windows || !ci)

// mangareader: касание во время броска (инерция прокрутки).

package mobile

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/internal/fling"
	"fyne.io/fyne/v2/widget"
	"github.com/stretchr/testify/assert"
)

// holdFling не даёт тестовому драйверу довести бросок до конца за один вызов.
func holdFling(t *testing.T) {
	fling.StartAnimation = func(*fyne.Animation) {}
	fling.StopAnimation = func(*fyne.Animation) {}
	t.Cleanup(func() {
		fling.Stop()
		fling.StartAnimation = func(a *fyne.Animation) { a.Start() }
		fling.StopAnimation = func(a *fyne.Animation) { a.Stop() }
	})
}

func newFlingCanvas() (*canvas, *container.Scroll, fyne.Position) {
	btn := widget.NewButton("tap", func() {})
	list := container.NewVBox(btn, widget.NewLabel("1\n2\n3\n4\n5\n6\n7\n8"))
	scroll := container.NewVScroll(list)
	c := newCanvas(fyne.CurrentDevice()).(*canvas)
	c.SetContent(scroll)
	c.Resize(fyne.NewSize(80, 60))
	return c, scroll, fyne.NewPos(20, 10)
}

func tapUpCount(c *canvas, pos fyne.Position, tapID int) (taps, drags int) {
	c.tapUp(pos, tapID,
		func(fyne.Tappable, *fyne.PointEvent) { taps++ },
		func(fyne.SecondaryTappable, *fyne.PointEvent) { taps++ },
		func(fyne.DoubleTappable, *fyne.PointEvent) { taps++ },
		func(wid fyne.Draggable, ev *fyne.DragEvent) { drags++; wid.DragEnd() })
	return
}

func TestTapDuringFlingOnlyStopsIt(t *testing.T) {
	holdFling(t)
	c, scroll, pos := newFlingCanvas()
	assert.True(t, fling.Start(scroll, fyne.Delta{DY: -3000}, nil))
	assert.True(t, fling.Active())

	c.tapDown(pos, 0)
	assert.False(t, fling.Active(), "касание останавливает бросок")
	taps, _ := tapUpCount(c, pos, 0)
	assert.Equal(t, 0, taps, "остановившее касание — не нажатие")

	c.tapDown(pos, 0) // бросок кончился: следующее касание — обычное нажатие
	taps, _ = tapUpCount(c, pos, 0)
	assert.Equal(t, 1, taps)
}

func TestTouchableUpDuringFling(t *testing.T) {
	holdFling(t)
	c, scroll, pos := newFlingCanvas()
	fling.Start(scroll, fyne.Delta{DY: -3000}, nil)
	c.tapDown(pos, 0)
	assert.True(t, c.caughtFling[0])
	tapUpCount(c, pos, 0)
	assert.False(t, c.caughtFling[0], "пометка снимается при отрыве")
}

func TestDragAfterCatchingFling(t *testing.T) {
	holdFling(t)
	c, scroll, pos := newFlingCanvas()
	fling.Start(scroll, fyne.Delta{DY: -3000}, nil)
	pos.Y = 40
	c.tapDown(pos, 0)
	c.tapMove(fyne.NewPos(pos.X, pos.Y-20), 0, func(wid fyne.Draggable, ev *fyne.DragEvent) { wid.Dragged(ev) })
	assert.Greater(t, scroll.Offset.Y, float32(0), "после остановки палец ведёт прокрутку")
	taps, drags := tapUpCount(c, fyne.NewPos(pos.X, pos.Y-20), 0)
	assert.Equal(t, 0, taps)
	assert.Equal(t, 1, drags)
}

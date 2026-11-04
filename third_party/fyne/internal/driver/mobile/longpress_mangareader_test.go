// mangareader: файл целиком наш — долгое нажатие во время удержания.
package mobile

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

// longPressCanvas — холст с одной меткой, которая считает нажатия.
func longPressCanvas() (*canvas, *tappableLabel) {
	obj := &tappableLabel{}
	obj.ExtendBaseWidget(obj)
	c := newCanvas(fyne.CurrentDevice()).(*canvas)
	c.SetContent(obj)
	c.Resize(fyne.NewSize(36, 24))
	obj.Move(fyne.NewPos(3, 3))
	return c, obj
}

func pressDown(c *canvas, pos fyne.Position, alts *int) {
	c.tapDown(pos, 0)
	c.armLongPress(pos, 0, func(wid fyne.SecondaryTappable, ev *fyne.PointEvent) {
		*alts++
		wid.TappedSecondary(ev)
	})
}

func pressUp(c *canvas, obj *tappableLabel, pos fyne.Position, alts *int) {
	c.tapUp(pos, 0, func(wid fyne.Tappable, ev *fyne.PointEvent) {
		obj.tap = true
	}, func(wid fyne.SecondaryTappable, ev *fyne.PointEvent) {
		*alts++
	}, func(wid fyne.DoubleTappable, ev *fyne.PointEvent) {
	}, func(wid fyne.Draggable, ev *fyne.DragEvent) {
	})
}

func TestLongPressFiresWhileHeld(t *testing.T) {
	c, obj := longPressCanvas()
	pos := fyne.NewPos(6, 6)
	alts := 0
	pressDown(c, pos, &alts)
	assert.Eventually(t, func() bool { return obj.altTap }, time.Second, 5*time.Millisecond, "долгое нажатие до подъёма пальца")
	assert.Equal(t, 1, alts)

	pressUp(c, obj, pos, &alts)
	assert.Equal(t, 1, alts, "подъём не даёт второго долгого нажатия")
	assert.False(t, obj.tap, "подъём не даёт обычного нажатия")
}

func TestLongPressCancelledByMove(t *testing.T) {
	c, obj := longPressCanvas()
	alts := 0
	pressDown(c, fyne.NewPos(6, 6), &alts)
	c.tapMove(fyne.NewPos(6, 6+tapMoveThreshold+2), 0, func(fyne.Draggable, *fyne.DragEvent) {})
	time.Sleep(tapSecondaryDelay + 100*time.Millisecond)
	assert.False(t, obj.altTap, "сдвиг пальца отменяет долгое нажатие")
	assert.Equal(t, 0, alts)
}

func TestLongPressNotOnShortTap(t *testing.T) {
	c, obj := longPressCanvas()
	pos := fyne.NewPos(6, 6)
	alts := 0
	pressDown(c, pos, &alts)
	pressUp(c, obj, pos, &alts)
	time.Sleep(tapSecondaryDelay + 100*time.Millisecond)
	assert.False(t, obj.altTap, "короткое нажатие не даёт долгого")
	assert.Equal(t, 0, alts)
}

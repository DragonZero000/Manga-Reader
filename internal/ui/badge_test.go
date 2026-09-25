package ui

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/theme"
)

var (
	badgeFG  = color.NRGBA{0x21, 0x96, 0xf3, 0xff} // заметно отличается от фона тестового холста
	badgeRed = color.NRGBA{0xf4, 0x43, 0x36, 0xff}
)

// renderIcon рисует ресурс 192×192 в тестовом окне.
func renderIcon(t *testing.T, res fyne.Resource) image.Image {
	t.Helper()
	test.NewTempApp(t)
	img := canvas.NewImageFromResource(res)
	w := test.NewTempWindow(t, img)
	w.SetPadded(false)
	w.Resize(fyne.NewSquareSize(192))
	img.Resize(fyne.NewSquareSize(192))
	out := w.Canvas().Capture()
	if p := os.Getenv("BADGE_PNG"); p != "" { // ручной просмотр
		f, _ := os.Create(p)
		png.Encode(f, out)
		f.Close()
	}
	return out
}

func near(c color.Color, want color.NRGBA) bool {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	d := func(a, b uint8) int {
		if a > b {
			return int(a - b)
		}
		return int(b - a)
	}
	return d(n.R, want.R) < 40 && d(n.G, want.G) < 40 && d(n.B, want.B) < 40 && n.A > 200
}

func TestBadgeIcon(t *testing.T) {
	base := theme.ErrorIcon()
	if BadgeIcon(base, 0, badgeFG, badgeRed) != base {
		t.Fatal("при 0 должна возвращаться исходная иконка")
	}
	three := BadgeIcon(base, 3, badgeFG, badgeRed)
	if !strings.Contains(three.Name(), "-3-") || BadgeIcon(base, 150, badgeFG, badgeRed).Name() == three.Name() {
		t.Fatalf("имя ресурса: %s", three.Name())
	}
	if !strings.Contains(BadgeIcon(base, 150, badgeFG, badgeRed).Name(), "-99+-") {
		t.Fatal("больше 99 — «99+»")
	}

	for _, res := range []fyne.Resource{three, BadgeIcon(base, 150, badgeFG, badgeRed)} {
		checkBadgeRender(t, renderIcon(t, res))
	}
}

// checkBadgeRender: в правой верхней четверти — кружок бейджа (красный
// и белые цифры), в левой нижней — иконка цвета badgeFG.
func checkBadgeRender(t *testing.T, img image.Image) {
	t.Helper()
	b := img.Bounds()
	var red, white, fg int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := img.At(x, y)
			top := x > b.Dx()/2 && y < b.Dy()/2
			switch {
			case top && near(c, badgeRed):
				red++
			case top && near(c, color.NRGBA{0xff, 0xff, 0xff, 0xff}):
				white++
			case !top && near(c, badgeFG):
				fg++
			}
		}
	}
	if red < 50 || white < 5 || fg < 50 {
		t.Fatalf("рендер: красных %d, белых %d, цвета иконки %d", red, white, fg)
	}
}

package ui

import (
	"fmt"
	"image/color"
	"regexp"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
)

// badgeMax — больше этого числа показывается «99+».
const badgeMax = 99

// BadgeIcon возвращает иконку base (SVG с viewBox 0 0 24 24) с кружком и
// числом n в правом верхнем углу. При n <= 0 возвращается сама base.
// fg — цвет исходной иконки, badge — цвет кружка; цифры рисуются белым.
//
// Текст в SVG рендер Fyne не поддерживает, поэтому цифры рисуются линиями
// в стиле семисегментного индикатора.
func BadgeIcon(base fyne.Resource, n int, fg, badge color.Color) fyne.Resource {
	if n <= 0 || base == nil {
		return base
	}
	label := strconv.Itoa(n)
	if n > badgeMax {
		label = strconv.Itoa(badgeMax) + "+"
	}
	inner := svgInner(base.Content())

	const (
		gw, gh = 3.0, 5.4 // размер цифры
		gap    = 1.2      // между цифрами
		stroke = 1.3
		cy     = 6.5 // центр кружка по вертикали
		r      = 6.5 // радиус кружка
	)
	textW := float64(len(label))*gw + float64(len(label)-1)*gap
	// кружок для одной цифры, «таблетка» для нескольких
	w := max(2*r, textW+2*2.2)
	x0 := 24 - w
	x := x0 + (w-textW)/2
	y := cy - gh/2

	var glyphs strings.Builder
	for _, ch := range label {
		glyphs.WriteString(glyphPath(ch, x, y, gw, gh))
		x += gw + gap
	}

	// Скругления rx и transform групп рендер Fyne не поддерживает:
	// «таблетка» — два круга и прямоугольник между ними.
	bc := hexColor(badge)
	pill := fmt.Sprintf(`<circle cx="%.2f" cy="%.2f" r="%.2f" fill="%s"/><circle cx="%.2f" cy="%.2f" r="%.2f" fill="%s"/>`,
		x0+r, cy, r, bc, 24-r, cy, r, bc)
	if w > 2*r {
		pill += fmt.Sprintf(`<rect x="%.2f" y="%.2f" width="%.2f" height="%.2f" fill="%s"/>`, x0+r, cy-r, w-2*r, 2*r, bc)
	}
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" width="24px" height="24px" fill="%s">`+
		`%s%s<path d="%s" fill="none" stroke="#ffffff" stroke-width="%.2f" stroke-linecap="round" stroke-linejoin="round"/>`+
		`</svg>`,
		hexColor(fg), inner, pill, glyphs.String(), stroke)
	// Fyne кэширует растр по имени ресурса — в имени всё, что влияет на вид
	name := fmt.Sprintf("badge-%s-%s-%s-%s.svg", base.Name(), label, hexColor(fg)[1:], hexColor(badge)[1:])
	return fyne.NewStaticResource(name, []byte(svg))
}

// fillColor — явный цвет заливки элемента (fill="none" не трогаем).
var fillColor = regexp.MustCompile(`\sfill="#[0-9a-fA-F]+"`)

// svgInner возвращает содержимое корневого элемента <svg> без явных цветов
// заливки: иконка темы уже перекрашена, а цвет задаёт корень новой SVG.
func svgInner(src []byte) string {
	return fillColor.ReplaceAllString(svgBody(src), "")
}

func svgBody(src []byte) string {
	s := string(src)
	start := strings.Index(s, "<svg")
	if start < 0 {
		return ""
	}
	open := strings.IndexByte(s[start:], '>')
	end := strings.LastIndex(s, "</svg>")
	if open < 0 || end < start+open {
		return ""
	}
	return s[start+open+1 : end]
}

// Сегменты: a — верх, b — справа сверху, c — справа снизу, d — низ,
// e — слева снизу, f — слева сверху, g — середина.
var digitSegments = map[rune]string{
	'0': "abcdef", '1': "bc", '2': "abged", '3': "abgcd", '4': "fgbc",
	'5': "afgcd", '6': "afgedc", '7': "abc", '8': "abcdefg", '9': "abcdfg",
}

// glyphPath — команды пути для символа в прямоугольнике (x, y, w, h).
func glyphPath(ch rune, x, y, w, h float64) string {
	line := func(x1, y1, x2, y2 float64) string {
		return fmt.Sprintf("M%.2f %.2fL%.2f %.2f", x1, y1, x2, y2)
	}
	if ch == '+' {
		cx, cy := x+w/2, y+h/2
		return line(x, cy, x+w, cy) + line(cx, cy-w/2, cx, cy+w/2)
	}
	m := y + h/2
	seg := map[rune]string{
		'a': line(x, y, x+w, y),
		'b': line(x+w, y, x+w, m),
		'c': line(x+w, m, x+w, y+h),
		'd': line(x, y+h, x+w, y+h),
		'e': line(x, m, x, y+h),
		'f': line(x, y, x, m),
		'g': line(x, m, x+w, m),
	}
	var b strings.Builder
	for _, s := range digitSegments[ch] {
		b.WriteString(seg[s])
	}
	return b.String()
}

func hexColor(c color.Color) string {
	nc := color.NRGBAModel.Convert(c).(color.NRGBA)
	return fmt.Sprintf("#%02x%02x%02x", nc.R, nc.G, nc.B)
}

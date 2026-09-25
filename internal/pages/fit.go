// Package pages загружает страницы для читалки и содержит геометрию
// отображения. Пакет не зависит от виджетов.
package pages

import "sort"

// Pt — точка или смещение в логических единицах.
type Pt struct{ X, Y float32 }

// Sz — размер в логических единицах.
type Sz struct{ W, H float32 }

// Fit — масштаб, при котором изображение img целиком помещается в box.
func Fit(img, box Sz) float32 {
	if img.W <= 0 || img.H <= 0 {
		return 1
	}
	return min(box.W/img.W, box.H/img.H)
}

// FitWidth — масштаб, при котором ширина изображения равна ширине box.
func FitWidth(imgW, boxW float32) float32 {
	if imgW <= 0 {
		return 1
	}
	return boxW / imgW
}

// ZoomAt меняет масштаб в factor раз так, чтобы точка p области просмотра
// осталась над той же точкой изображения. offset — позиция левого верхнего
// угла изображения в области просмотра.
func ZoomAt(offset, p Pt, factor float32) Pt {
	return Pt{
		X: p.X - (p.X-offset.X)*factor,
		Y: p.Y - (p.Y-offset.Y)*factor,
	}
}

// ClampOffset ограничивает смещение изображения content в области view:
// изображение не отходит от краёв, а если по оси оно меньше области —
// центрируется.
func ClampOffset(offset Pt, content, view Sz) Pt {
	return Pt{X: clampAxis(offset.X, content.W, view.W), Y: clampAxis(offset.Y, content.H, view.H)}
}

func clampAxis(off, content, view float32) float32 {
	if content <= view {
		return (view - content) / 2
	}
	return max(view-content, min(0, off))
}

// Prefix возвращает префиксные суммы высот: prefix[i] — верх страницы i,
// prefix[len] — общая высота.
func Prefix(heights []float32) []float32 {
	out := make([]float32, len(heights)+1)
	for i, h := range heights {
		out[i+1] = out[i] + h
	}
	return out
}

// TopPage — индекс страницы, которая находится на вертикальной позиции y.
func TopPage(prefix []float32, y float32) int {
	n := len(prefix) - 1
	if n <= 0 {
		return 0
	}
	// первая страница, чей низ ниже y
	i := sort.Search(n, func(i int) bool { return prefix[i+1] > y })
	return min(i, n-1)
}

// Visible возвращает диапазон страниц [from, to), пересекающих отрезок [top, bottom).
func Visible(prefix []float32, top, bottom float32) (from, to int) {
	n := len(prefix) - 1
	if n <= 0 || bottom <= top {
		return 0, 0
	}
	from = TopPage(prefix, max(0, top))
	to = sort.Search(n, func(i int) bool { return prefix[i] >= bottom })
	return from, max(to, from)
}

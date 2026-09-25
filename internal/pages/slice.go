package pages

import (
	"image"
	"image/draw"
)

// MaxSliceHeight — наибольшая высота куска в физических пикселях: с запасом
// ниже минимального реального ограничения размера текстуры GPU (4096).
const MaxSliceHeight = 2048

// Slice режет изображение на куски высотой не больше maxH. Каждый кусок —
// отдельное изображение (своя текстура). Изображение не выше maxH
// возвращается как есть.
func Slice(img image.Image, maxH int) []image.Image {
	b := img.Bounds()
	if maxH <= 0 || b.Dy() <= maxH {
		return []image.Image{img}
	}
	var parts []image.Image
	for y := b.Min.Y; y < b.Max.Y; y += maxH {
		h := min(maxH, b.Max.Y-y)
		part := image.NewNRGBA(image.Rect(0, 0, b.Dx(), h))
		draw.Draw(part, part.Bounds(), img, image.Pt(b.Min.X, y), draw.Src)
		parts = append(parts, part)
	}
	return parts
}

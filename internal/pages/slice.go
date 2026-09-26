package pages

import (
	"image"
)

// MaxSliceHeight — наибольшая высота куска в физических пикселях: с запасом
// ниже минимального реального ограничения размера текстуры GPU (4096).
const MaxSliceHeight = 2048

// Slice режет изображение на куски высотой не больше maxH. Каждый кусок —
// отдельное изображение (своя текстура). Куски — участки одного буфера
// *image.RGBA полной ширины, пиксели не копируются: строки куска идут в
// памяти подряд, как и ждёт загрузка текстуры. Изображение не выше maxH
// возвращается одним куском.
func Slice(img image.Image, maxH int) []image.Image {
	src := ToRGBA(img)
	b := src.Bounds()
	if maxH <= 0 || b.Dy() <= maxH {
		return []image.Image{src}
	}
	var parts []image.Image
	for y := b.Min.Y; y < b.Max.Y; y += maxH {
		parts = append(parts, src.SubImage(image.Rect(b.Min.X, y, b.Max.X, min(y+maxH, b.Max.Y))))
	}
	return parts
}

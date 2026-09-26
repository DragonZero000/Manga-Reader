package pages

import (
	"image"
	"image/color"
	"testing"
)

func near(a, b float32) bool {
	d := a - b
	return d < 0.001 && d > -0.001
}

func TestFit(t *testing.T) {
	cases := []struct {
		img, box Sz
		want     float32
	}{
		{Sz{1200, 1700}, Sz{600, 1700}, 0.5}, // упирается в ширину
		{Sz{1200, 1700}, Sz{1200, 850}, 0.5}, // упирается в высоту
		{Sz{100, 100}, Sz{400, 200}, 2},      // маленькое увеличивается
		{Sz{0, 100}, Sz{400, 200}, 1},        // неизвестный размер
	}
	for _, c := range cases {
		if got := Fit(c.img, c.box); !near(got, c.want) {
			t.Errorf("Fit(%v, %v) = %v, want %v", c.img, c.box, got, c.want)
		}
	}
	if got := FitWidth(800, 400); !near(got, 0.5) {
		t.Errorf("FitWidth = %v", got)
	}
}

func TestZoomAtKeepsPoint(t *testing.T) {
	offset := Pt{10, 20}
	p := Pt{110, 220}
	// точка изображения под p до увеличения: (p - offset) / 1
	imgPt := Pt{p.X - offset.X, p.Y - offset.Y}
	off2 := ZoomAt(offset, p, 2)
	// после увеличения та же точка изображения: off2 + imgPt*2
	if got := (Pt{off2.X + imgPt.X*2, off2.Y + imgPt.Y*2}); !near(got.X, p.X) || !near(got.Y, p.Y) {
		t.Fatalf("точка сместилась: %v, ожидалось %v", got, p)
	}
}

func TestClampOffset(t *testing.T) {
	view := Sz{400, 800}
	// меньше области — центрируется
	if got := ClampOffset(Pt{-50, 999}, Sz{200, 400}, view); got != (Pt{100, 200}) {
		t.Errorf("центрирование: %v", got)
	}
	// больше области — не отходит от краёв
	content := Sz{800, 1600}
	if got := ClampOffset(Pt{50, 50}, content, view); got != (Pt{0, 0}) {
		t.Errorf("левый верхний край: %v", got)
	}
	if got := ClampOffset(Pt{-1000, -2000}, content, view); got != (Pt{-400, -800}) {
		t.Errorf("правый нижний край: %v", got)
	}
	if got := ClampOffset(Pt{-100, -300}, content, view); got != (Pt{-100, -300}) {
		t.Errorf("внутри допустимого: %v", got)
	}
}

func TestPrefixTopPageVisible(t *testing.T) {
	prefix := Prefix([]float32{100, 200, 50})
	if want := []float32{0, 100, 300, 350}; len(prefix) != 4 || prefix[3] != want[3] || prefix[2] != want[2] {
		t.Fatalf("prefix = %v", prefix)
	}
	cases := []struct {
		y    float32
		want int
	}{{0, 0}, {99.9, 0}, {100, 1}, {299, 1}, {300, 2}, {1000, 2}, {-5, 0}}
	for _, c := range cases {
		if got := TopPage(prefix, c.y); got != c.want {
			t.Errorf("TopPage(%v) = %d, want %d", c.y, got, c.want)
		}
	}
	if from, to := Visible(prefix, 50, 320); from != 0 || to != 3 {
		t.Errorf("Visible(50,320) = %d,%d", from, to)
	}
	if from, to := Visible(prefix, 100, 300); from != 1 || to != 2 {
		t.Errorf("Visible(100,300) = %d,%d", from, to)
	}
	if from, to := Visible(Prefix(nil), 0, 100); from != 0 || to != 0 {
		t.Errorf("пустой список: %d,%d", from, to)
	}
}

func TestSlice(t *testing.T) {
	tall := image.NewNRGBA(image.Rect(0, 0, 800, 20000))
	tall.Set(5, 2048, color.White) // первый пиксель второго куска
	parts := Slice(tall, MaxSliceHeight)
	if len(parts) != 10 {
		t.Fatalf("кусков %d", len(parts))
	}
	total := 0
	for i, p := range parts {
		b := p.Bounds()
		if b.Dx() != 800 || b.Dy() > MaxSliceHeight {
			t.Errorf("кусок %d: %v", i, b)
		}
		total += b.Dy()
	}
	if total != 20000 {
		t.Errorf("суммарная высота %d", total)
	}
	// куски — участки исходника: координаты сохраняются, читаем от Bounds().Min
	if r, _, _, _ := parts[1].At(5, parts[1].Bounds().Min.Y).RGBA(); r == 0 {
		t.Error("содержимое второго куска смещено")
	}
	small := image.NewRGBA(image.Rect(0, 0, 10, 10))
	if got := Slice(small, MaxSliceHeight); len(got) != 1 || got[0] != image.Image(small) {
		t.Error("маленькое RGBA должно возвращаться без изменений")
	}
}

// Куски — участки одного буфера RGBA: без копирования, строки подряд.
func TestSliceSharesBuffer(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 300, 5000))
	for y := 0; y < 5000; y += 97 {
		src.Set(0, y, color.RGBA{uint8(y), uint8(y >> 8), 7, 255})
	}
	parts := Slice(src, MaxSliceHeight)
	if len(parts) != 3 {
		t.Fatalf("кусков %d", len(parts))
	}
	total := 0
	for i, p := range parts {
		r, ok := p.(*image.RGBA)
		if !ok {
			t.Fatalf("кусок %d: %T", i, p)
		}
		y := i * MaxSliceHeight
		if &r.Pix[0] != &src.Pix[src.PixOffset(0, y)] {
			t.Errorf("кусок %d скопирован, а не взят из буфера", i)
		}
		if r.Stride != 4*r.Rect.Dx() {
			t.Errorf("кусок %d: строки не подряд (stride %d)", i, r.Stride)
		}
		// текстура читается с начала Pix: первые 4 байта — пиксель (0, y) исходника
		want := src.Pix[src.PixOffset(0, y) : src.PixOffset(0, y)+4]
		if string(r.Pix[:4]) != string(want) {
			t.Errorf("кусок %d: начало Pix %v, ждали %v", i, r.Pix[:4], want)
		}
		total += r.Rect.Dx() * r.Rect.Dy()
	}
	if total != 300*5000 {
		t.Errorf("сумма площадей кусков %d", total)
	}
}

func TestToRGBA(t *testing.T) {
	rgba := image.NewRGBA(image.Rect(0, 0, 4, 4))
	if ToRGBA(rgba) != rgba {
		t.Error("RGBA с началом в (0,0) должно возвращаться как есть")
	}
	// участок RGBA — копия с началом в (0,0)
	sub := rgba.SubImage(image.Rect(1, 1, 3, 3))
	if got := ToRGBA(sub); got.Rect.Min != (image.Point{}) || got.Rect.Dx() != 2 {
		t.Errorf("участок: %v", got.Rect)
	}
	// JPEG-подобный YCbCr: те же пиксели
	ycc := image.NewYCbCr(image.Rect(0, 0, 8, 8), image.YCbCrSubsampleRatio420)
	for i := range ycc.Y {
		ycc.Y[i] = uint8(i * 3)
	}
	got := ToRGBA(ycc)
	for _, pt := range []image.Point{{0, 0}, {7, 7}, {3, 5}} {
		wr, wg, wb, _ := ycc.At(pt.X, pt.Y).RGBA()
		gr, gg, gb, _ := got.At(pt.X, pt.Y).RGBA()
		if wr>>8 != gr>>8 || wg>>8 != gg>>8 || wb>>8 != gb>>8 {
			t.Errorf("YCbCr %v: %v, ждали %v", pt, got.At(pt.X, pt.Y), ycc.At(pt.X, pt.Y))
		}
	}
	// NRGBA с прозрачностью → премультиплицированный RGBA
	n := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	n.SetNRGBA(0, 0, color.NRGBA{200, 100, 50, 128})
	if c := ToRGBA(n).RGBAAt(0, 0); c.A != 128 || c.R != 100 || c.G != 50 || c.B != 25 {
		t.Errorf("прозрачность: %v", c)
	}
}

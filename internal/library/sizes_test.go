package library

import (
	"bytes"
	"context"
	"image"
	"testing"

	"mangareader/internal/model"
)

func TestPageSizesExample(t *testing.T) {
	dir := t.TempDir()
	p := copyExample(t, dir, "example.zip")
	src := NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	opens := countOpens(t)

	sizes, err := src.PageSizes(model.LocalKey("example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if opens.Load() != 1 {
		t.Errorf("архив открыт %d раз, ожидался 1", opens.Load())
	}
	if len(sizes) != 2 {
		t.Fatalf("размеров %d", len(sizes))
	}
	for i, name := range []string{"example/1.jpg", "example/2.jpg"} {
		cfg, _, err := image.DecodeConfig(bytes.NewReader(readFromZip(t, p, name)))
		if err != nil {
			t.Fatal(err)
		}
		if sizes[i].Err != nil || sizes[i].Width != cfg.Width || sizes[i].Height != cfg.Height {
			t.Errorf("страница %d: %+v, ожидалось %dx%d", i, sizes[i], cfg.Width, cfg.Height)
		}
	}
}

func TestPageSizesBrokenPage(t *testing.T) {
	dir := t.TempDir()
	writeZip(t, dir, "b.zip",
		file{"1.png", pngBytes(t, 30, 40)},
		file{"2.png", []byte("не картинка")},
		file{"3.png", pngBytes(t, 50, 20)},
	)
	src := NewDirSource(dir, nil)
	if _, err := src.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	sizes, err := src.PageSizes(model.LocalKey("b.zip"))
	if err != nil {
		t.Fatal(err)
	}
	if sizes[0] != (PageSize{Width: 30, Height: 40}) {
		t.Errorf("страница 1: %+v", sizes[0])
	}
	if sizes[1].Err == nil || sizes[1].Width != 0 || sizes[1].Height != 0 {
		t.Errorf("битая страница: %+v", sizes[1])
	}
	if sizes[2] != (PageSize{Width: 50, Height: 20}) {
		t.Errorf("страница 3: %+v", sizes[2])
	}
	if _, err := src.PageSizes(model.LocalKey("nope.zip")); err == nil {
		t.Error("неизвестная галерея: ожидалась ошибка")
	}
}

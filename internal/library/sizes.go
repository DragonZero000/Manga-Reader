package library

import (
	"fmt"
	"image"
	_ "image/gif"  // регистрация декодеров для DecodeConfig
	_ "image/jpeg" //
	_ "image/png"  //
	"io/fs"

	_ "golang.org/x/image/webp" //

	"mangareader/internal/model"
)

// PageSize — размер страницы в пикселях. Err != nil — размер определить не удалось.
type PageSize struct {
	Width, Height int
	Err           error
}

// PageSizes возвращает размеры всех страниц галереи в порядке страниц,
// читая только заголовки изображений. Архив открывается один раз.
func (s *Source) PageSizes(k model.Key) ([]PageSize, error) {
	g, ok := s.Get(k)
	if !ok {
		return nil, fmt.Errorf("галерея %s не найдена", k)
	}
	zr, closeFn, err := s.openGallery(g)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	out := make([]PageSize, len(g.Pages))
	for i, p := range g.Pages {
		out[i] = pageSize(zr.Open, p.Name)
	}
	return out, nil
}

func pageSize(open func(string) (fs.File, error), name string) PageSize {
	f, err := open(name)
	if err != nil {
		return PageSize{Err: err}
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return PageSize{Err: fmt.Errorf("заголовок %s: %w", name, err)}
	}
	return PageSize{Width: cfg.Width, Height: cfg.Height}
}

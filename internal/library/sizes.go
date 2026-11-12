package library

import (
	"errors"
	"fmt"
	"image"
	_ "image/gif"  // регистрация декодеров для DecodeConfig
	_ "image/jpeg" //
	_ "image/png"  //

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
		return nil, fmt.Errorf("gallery %s not found", k)
	}
	c, closeFn, err := s.openGallery(g)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	out := make([]PageSize, len(g.Pages))
	for i, p := range g.Pages {
		out[i] = pageSize(c, p.Name)
	}
	return out, nil
}

// pageSize читает заголовок изображения записи name контейнера c.
func pageSize(c container, name string) PageSize {
	rc, err := c.Open(name)
	if err != nil {
		var nf errEntryNotFound
		if errors.As(err, &nf) {
			return PageSize{Err: fmt.Errorf("page %q not found in the archive", name)}
		}
		return PageSize{Err: err}
	}
	defer rc.Close()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil {
		return PageSize{Err: fmt.Errorf("header %q: %w", name, err)}
	}
	return PageSize{Width: cfg.Width, Height: cfg.Height}
}

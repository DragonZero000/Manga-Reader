package library

import (
	"archive/zip"
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
	zr, closeFn, err := s.openGallery(g)
	if err != nil {
		return nil, err
	}
	defer closeFn()

	// записи по сырому имени, как в findEntry: первая запись-файл
	entries := make(map[string]*zip.File, len(zr.File))
	for _, f := range zr.File {
		if _, ok := entries[f.Name]; !ok && !f.FileInfo().IsDir() {
			entries[f.Name] = f
		}
	}
	out := make([]PageSize, len(g.Pages))
	for i, p := range g.Pages {
		out[i] = pageSize(entries[p.Name], p.Name)
	}
	return out, nil
}

// pageSize читает заголовок изображения записи f (nil — записи нет).
func pageSize(f *zip.File, name string) PageSize {
	if f == nil {
		return PageSize{Err: fmt.Errorf("page %q not found in the archive", name)}
	}
	rc, err := f.Open()
	if err != nil {
		return PageSize{Err: err}
	}
	defer rc.Close()
	cfg, _, err := image.DecodeConfig(rc)
	if err != nil {
		return PageSize{Err: fmt.Errorf("header %q: %w", name, err)}
	}
	return PageSize{Width: cfg.Width, Height: cfg.Height}
}

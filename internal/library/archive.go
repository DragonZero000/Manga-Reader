package library

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

var (
	// ErrNotZip — файл не удалось открыть как zip-архив.
	ErrNotZip = errors.New("не zip-архив или архив повреждён")
	// ErrNoImages — в архиве нет изображений.
	ErrNoImages = errors.New("нет изображений")
)

// maxMetaSize ограничивает размер читаемого meta.json.
const maxMetaSize = 4 << 20

var imageExts = map[string]bool{
	".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".webp": true,
}

// ReadArchive разбирает открытый zip-архив в галерею.
// relPath — путь относительно папки библиотеки (для ключа), e — сведения
// о файле, display — путь для показа. Возвращает галерею и предупреждения
// (например, о некорректном meta.json).
func ReadArchive(f storage.File, relPath string, e storage.Entry, display string) (model.Gallery, []string, error) {
	zr, err := zip.NewReader(f, f.Size())
	if err != nil {
		return model.Gallery{}, nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}

	g := model.Gallery{
		Key:  model.LocalKey(relPath),
		File: model.FileInfo{Path: display, Size: e.Size, ModTime: e.ModTime},
	}

	var metaFile *zip.File
	for _, f := range zr.File {
		if f.FileInfo().IsDir() || isServicePath(f.Name) {
			continue
		}
		base := path2base(f.Name)
		if strings.EqualFold(base, "meta.json") {
			if metaFile == nil || depth(f.Name) < depth(metaFile.Name) {
				metaFile = f
			}
			continue
		}
		if imageExts[strings.ToLower(pathExt(base))] {
			g.Pages = append(g.Pages, model.Page{Name: f.Name})
		}
	}
	if len(g.Pages) == 0 {
		return model.Gallery{}, nil, ErrNoImages
	}
	g.SortPages()

	var warnings []string
	var meta *metaJSON
	if metaFile != nil {
		meta, err = readMeta(metaFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: некорректный %s: %v", relPath, metaFile.Name, err))
			meta = nil
		}
	}
	applyMeta(&g, meta, relPath)
	return g, warnings, nil
}

func readMeta(f *zip.File) (*metaJSON, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxMetaSize))
	if err != nil {
		return nil, err
	}
	return parseMeta(data)
}

// isServicePath отбрасывает служебные файлы: __MACOSX/ и имена на ".".
func isServicePath(name string) bool {
	for _, seg := range strings.Split(name, "/") {
		if seg == "__MACOSX" || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

func depth(name string) int {
	return strings.Count(strings.Trim(name, "/"), "/")
}

// Имена в zip всегда со слешами '/', поэтому используется пакет path.
func path2base(name string) string { return path.Base(name) }
func pathExt(name string) string   { return path.Ext(name) }

package library

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

var (
	// ErrNotZip — файл не удалось открыть как zip-архив.
	ErrNotZip = errors.New("not a zip archive or the archive is damaged")
	// ErrNoImages — в архиве нет изображений.
	ErrNoImages = errors.New("no images")
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
	var pageFiles []*zip.File
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
			pageFiles = append(pageFiles, f)
		}
	}
	if len(g.Pages) == 0 {
		return model.Gallery{}, nil, ErrNoImages
	}
	g.SortPages()
	g.Fingerprint = fingerprint(pageFiles)

	var warnings []string
	var meta *metaJSON
	if metaFile != nil {
		meta, err = readMeta(metaFile)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: invalid %s: %v", relPath, metaFile.Name, err))
			meta = nil
		}
	}
	applyMeta(&g, meta, relPath)
	return g, warnings, nil
}

// fingerprint — отпечаток содержимого по оглавлению архива, без распаковки:
// SHA-256 строк «путь\x00crc32\x00размер\n» страниц, упорядоченных по пути
// побайтно. Порядок записей в архиве, meta.json и служебные файлы не влияют.
func fingerprint(pages []*zip.File) string {
	sorted := append([]*zip.File(nil), pages...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	for _, f := range sorted {
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", f.Name, f.CRC32, f.UncompressedSize64)
	}
	return hex.EncodeToString(h.Sum(nil))
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

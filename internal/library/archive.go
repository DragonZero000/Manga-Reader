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
	"unicode/utf8"

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

	var warnings []string
	var metaFile *zip.File
	metaDepth := 0
	var pageFiles []*zip.File
	seen := map[string]bool{} // сырые имена страниц
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		// классификация — по пути, имя страницы — сырое (по нему запись
		// находится при открытии, см. findEntry)
		p := entryPath(f.Name)
		if isServicePath(p) {
			continue
		}
		base := path.Base(p)
		if strings.EqualFold(base, "meta.json") {
			if metaFile == nil || depth(p) < metaDepth {
				metaFile, metaDepth = f, depth(p)
			}
			continue
		}
		if imageExts[strings.ToLower(path.Ext(base))] {
			if seen[f.Name] {
				warnings = append(warnings, fmt.Sprintf("%s: duplicate entry %q skipped", relPath, f.Name))
				continue
			}
			seen[f.Name] = true
			g.Pages = append(g.Pages, model.Page{Name: f.Name})
			pageFiles = append(pageFiles, f)
		}
	}
	if len(g.Pages) == 0 {
		return model.Gallery{}, nil, ErrNoImages
	}
	g.SortPages()
	g.Fingerprint = fingerprint(pageFiles)

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

// entryPath — имя записи zip как путь со слешами «/» для классификации:
// пустые сегменты и «.» отбрасываются, в имени в UTF-8 «\» тоже разделитель.
// В имени не в UTF-8 «\» разделителем не считается: в Shift-JIS байт 0x5C
// бывает вторым байтом символа («ソ», «表»).
func entryPath(name string) string {
	sep := func(r rune) bool { return r == '/' }
	if utf8.ValidString(name) {
		sep = func(r rune) bool { return r == '/' || r == '\\' }
	}
	segs := strings.FieldsFunc(name, sep) // пустые сегменты FieldsFunc не возвращает
	kept := segs[:0]
	for _, s := range segs {
		if s != "." {
			kept = append(kept, s)
		}
	}
	return strings.Join(kept, "/")
}

// isServicePath отбрасывает служебные файлы: __MACOSX/ и имена на «.»
// (в том числе сегменты «..»). p — путь из entryPath.
func isServicePath(p string) bool {
	for _, seg := range strings.Split(p, "/") {
		if seg == "__MACOSX" || strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// depth — вложенность пути из entryPath.
func depth(p string) int { return strings.Count(p, "/") }

// findEntry — первая запись-файл оглавления с именем name (побайтно). Поиск
// идёт по сырым именам, а не через zip.Reader.Open: правила путей io/fs
// отвергают имена не в UTF-8, с «/» в начале и с сегментами «.».
func findEntry(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name && !f.FileInfo().IsDir() {
			return f
		}
	}
	return nil
}

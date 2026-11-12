package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

var (
	// ErrNotZip — файл не удалось открыть как архив (zip, RAR или 7z); имя
	// историческое, оно сохраняется в каталоге как вид ошибки not_zip.
	ErrNotZip = errors.New("not a valid archive or the archive is damaged")
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
	c, err := openContainer(f, nil, "")
	if err != nil {
		return model.Gallery{}, nil, err
	}
	defer c.Close()

	g := model.Gallery{
		Key:  model.LocalKey(relPath),
		File: model.FileInfo{Path: display, Size: e.Size, ModTime: e.ModTime},
	}

	var warnings []string
	var metaFile *archiveEntry
	metaDepth := 0
	var infoFile *archiveEntry // ComicInfo.xml
	infoDepth := 0
	var pageFiles []archiveEntry
	seen := map[string]bool{} // сырые имена страниц
	entries := c.Entries()
	for i := range entries {
		f := &entries[i]
		if f.IsDir {
			continue
		}
		// классификация — по пути, имя страницы — сырое (по нему запись
		// находится при открытии, см. container.Open)
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
		if isComicInfoName(base) {
			if infoFile == nil || depth(p) < infoDepth {
				infoFile, infoDepth = f, depth(p)
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
			pageFiles = append(pageFiles, *f)
		}
	}
	if len(g.Pages) == 0 {
		return model.Gallery{}, nil, ErrNoImages
	}
	g.SortPages()
	g.Fingerprint = fingerprint(pageFiles)

	var meta *metaJSON
	if metaFile != nil {
		meta, err = readMeta(c, metaFile.Name)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: invalid %s: %v", relPath, metaFile.Name, err))
			meta = nil
		}
	}
	applyMeta(&g, meta, relPath)
	g.HasMeta = meta != nil

	// ComicInfo.xml дополняет meta.json, не заменяя его значения
	if infoFile != nil {
		ci, err := readComicInfo(c, infoFile.Name)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: invalid %s: %v", relPath, infoFile.Name, err))
		} else {
			titled := meta != nil && (strings.TrimSpace(meta.Title.English) != "" || strings.TrimSpace(meta.Title.Japanese) != "")
			applyComicInfo(&g, ci, titled)
			g.HasMeta = true
		}
	}
	return g, warnings, nil
}

func readComicInfo(c container, name string) (*comicInfo, error) {
	data, err := readEntryLimited(c, name, maxMetaSize)
	if err != nil {
		return nil, err
	}
	return parseComicInfo(data)
}

// fingerprint — отпечаток содержимого по оглавлению архива, без распаковки:
// SHA-256 строк «путь\x00crc32\x00размер\n» страниц, упорядоченных по пути
// побайтно. Порядок записей в архиве, meta.json и служебные файлы не влияют.
func fingerprint(pages []archiveEntry) string {
	sorted := append([]archiveEntry(nil), pages...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	h := sha256.New()
	for _, f := range sorted {
		fmt.Fprintf(h, "%s\x00%d\x00%d\n", f.Name, f.CRC, f.Size)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func readMeta(c container, name string) (*metaJSON, error) {
	data, err := readEntryLimited(c, name, maxMetaSize)
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

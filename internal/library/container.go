package library

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"

	"mangareader/internal/storage"
)

// archiveEntry — запись оглавления архива. Name — сырое имя записи (по нему
// запись находится при открытии), CRC и Size берутся из оглавления без
// распаковки.
type archiveEntry struct {
	Name  string
	Size  uint64
	CRC   uint32
	IsDir bool
}

// container — открытый архив любого формата (zip, RAR, 7z). Разбор галереи и
// чтение страниц работают только через него.
type container interface {
	// Entries — записи оглавления в порядке архива.
	Entries() []archiveEntry
	// Open открывает первую запись-файл с точным (побайтным) именем name.
	// Поиск идёт по сырым именам, а не через io/fs: правила путей io/fs
	// отвергают имена не в UTF-8, с «/» в начале и с сегментами «.».
	Open(name string) (io.ReadCloser, error)
	Close() error
}

// errEntryNotFound — в оглавлении нет записи-файла с таким именем.
type errEntryNotFound string

func (e errEntryNotFound) Error() string {
	return fmt.Sprintf("entry %q not found in the archive", string(e))
}

// zipContainer — контейнер поверх archive/zip. Файл не закрывает: им владеет
// вызывающий (storage.File закрывается там, где открыт).
type zipContainer struct {
	files   []*zip.File
	byName  map[string]*zip.File // первая запись-файл с этим именем
	entries []archiveEntry
}

// openZip разбирает оглавление zip-архива. Ошибка оборачивает ErrNotZip.
func openZip(f storage.File) (*zipContainer, error) {
	zr, err := zip.NewReader(f, f.Size())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	c := &zipContainer{
		files:   zr.File,
		byName:  make(map[string]*zip.File, len(zr.File)),
		entries: make([]archiveEntry, 0, len(zr.File)),
	}
	for _, zf := range zr.File {
		dir := zf.FileInfo().IsDir()
		c.entries = append(c.entries, archiveEntry{
			Name: zf.Name, Size: zf.UncompressedSize64, CRC: zf.CRC32, IsDir: dir,
		})
		if _, ok := c.byName[zf.Name]; !ok && !dir {
			c.byName[zf.Name] = zf
		}
	}
	return c, nil
}

func (c *zipContainer) Entries() []archiveEntry { return c.entries }

func (c *zipContainer) Open(name string) (io.ReadCloser, error) {
	zf, ok := c.byName[name]
	if !ok {
		return nil, errEntryNotFound(name)
	}
	return zf.Open()
}

func (c *zipContainer) Close() error { return nil }

// openContainer открывает архив в подходящем контейнере: формат (7z, RAR, иначе
// zip) определяется по сигнатуре, а не по расширению файла. cache и cacheKey
// нужны только для чтения страниц solid-RAR (cache == nil — без кэша).
func openContainer(f storage.File, cache *SolidCache, cacheKey string) (container, error) {
	var head [8]byte
	n, _ := f.ReadAt(head[:], 0)
	switch {
	case bytes.HasPrefix(head[:n], sig7z):
		return openSeven(f)
	case bytes.HasPrefix(head[:n], sigRar):
		return openRar(f, cache, cacheKey)
	}
	return openZip(f)
}

// readEntryLimited читает запись целиком, не более limit байт.
func readEntryLimited(c container, name string, limit int64) ([]byte, error) {
	rc, err := c.Open(name)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, limit))
}

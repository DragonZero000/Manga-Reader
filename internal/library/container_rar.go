package library

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"time"

	"github.com/nwaples/rardecode/v2"

	"mangareader/internal/storage"
)

// rarVolumeName — имя единственного тома в fs.FS, которую видит rardecode.
const rarVolumeName = "archive.rar"

// ErrPassword — архив защищён паролем.
var ErrPassword = errors.New("archive is password protected")

// rarContainer — контейнер поверх rardecode. Оглавление читается без
// распаковки; у записей RAR нет CRC в публичном API, поэтому отпечаток
// страниц строится по имени и размеру.
//
// Обычные записи открываются по одной. Solid-архив читается только подряд:
// с кэшем (cache != nil) он целиком распаковывается в папку кэша при первом
// чтении; без кэша (разбор при сканировании) запись находится проходом по
// архиву и читается в память.
type rarContainer struct {
	src     storage.File
	files   map[string]*rardecode.File // первая запись-файл с этим именем
	entries []archiveEntry
	total   int64 // распакованный размер всех файлов
	solid   bool

	cache    *SolidCache
	cacheKey string
}

// openRar разбирает оглавление RAR. cache и cacheKey нужны только для чтения
// страниц solid-архива (cache == nil — читать без кэша).
func openRar(f storage.File, cache *SolidCache, cacheKey string) (*rarContainer, error) {
	list, err := rardecode.List(rarVolumeName, rardecode.FileSystem(readerAtFS{f}))
	if err != nil {
		if errors.Is(err, rardecode.ErrArchiveEncrypted) || errors.Is(err, rardecode.ErrArchivedFileEncrypted) {
			return nil, ErrPassword
		}
		return nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	c := &rarContainer{
		src:      f,
		files:    make(map[string]*rardecode.File, len(list)),
		entries:  make([]archiveEntry, 0, len(list)),
		cache:    cache,
		cacheKey: cacheKey,
	}
	for _, rf := range list {
		if rf.Encrypted && !rf.IsDir {
			return nil, ErrPassword
		}
		size := uint64(0)
		if rf.UnPackedSize > 0 {
			size = uint64(rf.UnPackedSize)
			c.total += rf.UnPackedSize
		}
		if rf.Solid {
			c.solid = true
		}
		c.entries = append(c.entries, archiveEntry{Name: rf.Name, Size: size, IsDir: rf.IsDir})
		if _, ok := c.files[rf.Name]; !ok && !rf.IsDir {
			c.files[rf.Name] = rf
		}
	}
	return c, nil
}

func (c *rarContainer) Entries() []archiveEntry { return c.entries }

func (c *rarContainer) Open(name string) (io.ReadCloser, error) {
	rf, ok := c.files[name]
	if !ok {
		return nil, errEntryNotFound(name)
	}
	if !c.solid {
		return rf.Open()
	}
	if c.cache != nil {
		return c.openCached(name)
	}
	return c.openSequential(name)
}

// openCached читает запись из кэша, при необходимости распаковав весь архив.
func (c *rarContainer) openCached(name string) (io.ReadCloser, error) {
	dir, err := c.cache.ensure(c.cacheKey, c.total, c.extractAll)
	if err != nil {
		return nil, err
	}
	return os.Open(entryFile(dir, name))
}

// extractAll распаковывает все файлы подряд через write.
func (c *rarContainer) extractAll(write func(name string, r io.Reader) error) error {
	r, err := c.sequentialReader()
	if err != nil {
		return err
	}
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if h.IsDir {
			continue
		}
		if err := write(h.Name, r); err != nil {
			return err
		}
	}
}

// openSequential находит запись проходом по архиву и читает её в память.
func (c *rarContainer) openSequential(name string) (io.ReadCloser, error) {
	r, err := c.sequentialReader()
	if err != nil {
		return nil, err
	}
	for {
		h, err := r.Next()
		if errors.Is(err, io.EOF) {
			return nil, errEntryNotFound(name)
		}
		if err != nil {
			return nil, err
		}
		if h.IsDir || h.Name != name {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(r, maxPageSize))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(bytes.NewReader(data)), nil
	}
}

func (c *rarContainer) sequentialReader() (*rardecode.Reader, error) {
	return rardecode.NewReader(io.NewSectionReader(c.src, 0, c.src.Size()))
}

func (c *rarContainer) Close() error { return nil }

// readerAtFS отдаёт rardecode один «файл» поверх storage.File. Файл не
// закрывает: им владеет вызывающий.
type readerAtFS struct{ f storage.File }

func (r readerAtFS) Open(name string) (fs.File, error) {
	if name != rarVolumeName {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}
	return &readerAtFile{SectionReader: io.NewSectionReader(r.f, 0, r.f.Size()), size: r.f.Size()}, nil
}

type readerAtFile struct {
	*io.SectionReader
	size int64
}

func (r *readerAtFile) Stat() (fs.FileInfo, error) { return readerAtInfo{r.size}, nil }
func (r *readerAtFile) Close() error               { return nil }

type readerAtInfo struct{ size int64 }

func (i readerAtInfo) Name() string       { return rarVolumeName }
func (i readerAtInfo) Size() int64        { return i.size }
func (i readerAtInfo) Mode() fs.FileMode  { return 0o444 }
func (i readerAtInfo) ModTime() time.Time { return time.Time{} }
func (i readerAtInfo) IsDir() bool        { return false }
func (i readerAtInfo) Sys() any           { return nil }

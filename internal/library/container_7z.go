package library

import (
	"errors"
	"fmt"
	"io"

	"github.com/bodgit/sevenzip"

	"mangareader/internal/storage"
)

// sevenContainer — контейнер поверх bodgit/sevenzip. Оглавление и CRC
// читаются из заголовков без распаковки.
type sevenContainer struct {
	files   map[string]*sevenzip.File // первая запись-файл с этим именем
	entries []archiveEntry
}

func openSeven(f storage.File) (*sevenContainer, error) {
	r, err := sevenzip.NewReader(f, f.Size())
	if err != nil {
		if isEncrypted7z(err) {
			return nil, ErrPassword
		}
		return nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	c := &sevenContainer{
		files:   make(map[string]*sevenzip.File, len(r.File)),
		entries: make([]archiveEntry, 0, len(r.File)),
	}
	var first *sevenzip.File
	for _, sf := range r.File {
		dir := sf.FileInfo().IsDir()
		c.entries = append(c.entries, archiveEntry{
			Name: sf.Name, Size: sf.UncompressedSize, CRC: sf.CRC32, IsDir: dir,
		})
		if _, ok := c.files[sf.Name]; !ok && !dir {
			c.files[sf.Name] = sf
			if first == nil && sf.UncompressedSize > 0 {
				first = sf
			}
		}
	}
	// зашифрованное содержимое при незашифрованных заголовках обнаруживается
	// только при чтении: пробуем прочитать байт первой непустой записи
	if first != nil {
		if err := probe7z(first); err != nil {
			return nil, err
		}
	}
	return c, nil
}

func probe7z(f *sevenzip.File) error {
	rc, err := f.Open()
	if err != nil {
		if isEncrypted7z(err) {
			return ErrPassword
		}
		return fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	defer rc.Close()
	var b [1]byte
	if _, err := io.ReadFull(rc, b[:]); err != nil && !errors.Is(err, io.EOF) {
		if isEncrypted7z(err) {
			return ErrPassword
		}
		return fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	return nil
}

func isEncrypted7z(err error) bool {
	var re *sevenzip.ReadError
	if errors.As(err, &re) {
		return re.Encrypted
	}
	var rv sevenzip.ReadError
	return errors.As(err, &rv) && rv.Encrypted
}

func (c *sevenContainer) Entries() []archiveEntry { return c.entries }

func (c *sevenContainer) Open(name string) (io.ReadCloser, error) {
	sf, ok := c.files[name]
	if !ok {
		return nil, errEntryNotFound(name)
	}
	return sf.Open()
}

func (c *sevenContainer) Close() error { return nil }

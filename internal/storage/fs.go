package storage

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// FS — папка библиотеки в обычной файловой системе.
type FS struct {
	root string
}

// NewFS создаёт хранилище для папки root.
func NewFS(root string) *FS { return &FS{root: root} }

func (s *FS) Name() string { return s.root }

func (s *FS) Walk(ctx context.Context, fn func(Entry) error) error {
	if s.root == "" {
		return fmt.Errorf("%w: папка не определена", ErrUnavailable)
	}
	if _, err := os.Stat(s.root); err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	return filepath.WalkDir(s.root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == s.root {
				return fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			return nil // недоступный элемент внутри — пропускаем
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if p == s.root {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(s.root, p)
		if err != nil {
			return nil
		}
		return fn(Entry{RelPath: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime(), IsDir: d.IsDir()})
	})
}

// path — путь файла на диске по относительному пути внутри папки.
func (s *FS) path(relPath string) (string, error) {
	clean := path.Clean("/" + relPath)[1:]
	if clean == "" || clean != relPath || strings.HasPrefix(clean, "../") {
		return "", fmt.Errorf("недопустимый путь %q", relPath)
	}
	return filepath.Join(s.root, filepath.FromSlash(clean)), nil
}

func (s *FS) Open(relPath string) (File, error) {
	p, err := s.path(relPath)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(p)
	if err != nil {
		if isBusy(err) {
			return nil, fmt.Errorf("%w: %v", ErrBusy, err)
		}
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	if info.IsDir() {
		f.Close()
		return nil, errors.New("это папка")
	}
	return &osFile{File: f, size: info.Size()}, nil
}

// osFile — *os.File с известным размером.
type osFile struct {
	*os.File
	size int64
}

func (f *osFile) Size() int64 { return f.size }

// NewOSFile оборачивает открытый файл известного размера (для Android SAF).
func NewOSFile(f *os.File, size int64) File { return &osFile{File: f, size: size} }

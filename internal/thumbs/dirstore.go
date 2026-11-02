package thumbs

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// DirStore — миниатюры файлами JPEG в папке (кэш обложек): на Android —
// системная папка кэша приложения, на ПК — cache/thumbs. Реализует Store.
// Файл — «<хэш ключа>_<w>x<h>.jpg»: у ключа один файл, другой размер
// области его заменяет. Методы безопасны для вызова из разных горутин.
type DirStore struct{ dir string }

// NewDirStore — хранилище в папке dir (создаётся при первом сохранении).
func NewDirStore(dir string) *DirStore { return &DirStore{dir: dir} }

// Dir — папка хранилища.
func (s *DirStore) Dir() string { return s.dir }

// keyHash — имя файла без размера: ключ кэша содержит путь архива, поэтому
// не годится в имя файла как есть.
func keyHash(key string) string {
	sum := sha1.Sum([]byte(key))
	return hex.EncodeToString(sum[:10])
}

func (s *DirStore) path(key string, w, h int) string {
	return filepath.Join(s.dir, fmt.Sprintf("%s_%dx%d.jpg", keyHash(key), w, h))
}

func (s *DirStore) Load(key string, w, h int) ([]byte, bool) {
	data, err := os.ReadFile(s.path(key, w, h))
	return data, err == nil && len(data) > 0
}

// Save записывает файл через временный и rename (недописанный файл не
// читается как обложка) и удаляет файлы того же ключа другого размера.
func (s *DirStore) Save(key, rel string, w, h int, jpeg []byte) {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		log.Printf("covers: %s: %v", rel, err)
		return
	}
	dst := s.path(key, w, h)
	tmp, err := os.CreateTemp(s.dir, ".tmp-*")
	if err != nil {
		log.Printf("covers: %s: %v", rel, err)
		return
	}
	_, werr := tmp.Write(jpeg)
	cerr := tmp.Close()
	if err := errors.Join(werr, cerr); err != nil {
		os.Remove(tmp.Name())
		log.Printf("covers: %s: %v", rel, err)
		return
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		os.Remove(tmp.Name())
		log.Printf("covers: %s: %v", rel, err)
		return
	}
	others, _ := filepath.Glob(filepath.Join(s.dir, keyHash(key)+"_*.jpg"))
	for _, p := range others {
		if p != dst {
			os.Remove(p)
		}
	}
}

// Size — объём файлов хранилища, байт (нет папки — 0).
func (s *DirStore) Size() (int64, error) {
	var n int64
	err := s.each(func(e os.DirEntry) error {
		info, err := e.Info()
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return nil // удалён между чтением папки и Info
			}
			return err
		}
		n += info.Size()
		return nil
	})
	return n, err
}

// Clear удаляет все файлы хранилища.
func (s *DirStore) Clear() error {
	return s.each(func(e os.DirEntry) error { return s.remove(e.Name()) })
}

// Prune удаляет обложки, ключей которых нет среди keys (удалённые и
// изменённые архивы: время изменения входит в ключ).
func (s *DirStore) Prune(keys []string) error {
	keep := make(map[string]bool, len(keys))
	for _, k := range keys {
		keep[keyHash(k)] = true
	}
	return s.each(func(e os.DirEntry) error {
		hash, _, ok := strings.Cut(e.Name(), "_")
		if ok && keep[hash] && strings.HasSuffix(e.Name(), ".jpg") {
			return nil
		}
		return s.remove(e.Name())
	})
}

// each вызывает fn для каждого файла папки (нет папки — ни для одного).
func (s *DirStore) each(fn func(os.DirEntry) error) error {
	entries, err := os.ReadDir(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var errs []error
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := fn(e); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *DirStore) remove(name string) error {
	if err := os.Remove(filepath.Join(s.dir, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

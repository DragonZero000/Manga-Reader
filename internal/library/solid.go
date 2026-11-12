package library

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// DefaultSolidCacheLimit — предельный размер кэша распакованных solid-архивов.
const DefaultSolidCacheLimit = 512 << 20

// ErrSolidTooLarge — solid-архив не помещается в кэш распаковки.
var ErrSolidTooLarge = errors.New("solid archive is too large to unpack into the cache")

// solidDone — файл-метка: архив распакован целиком.
const solidDone = ".done"

// SolidCache — временная папка с распакованными solid-архивами RAR. Из них
// нельзя достать одну страницу без распаковки всех предыдущих, поэтому
// архив распаковывается целиком при первом чтении страницы. Размер папки
// ограничен: при нехватке места удаляются давно не использованные архивы.
// Папку можно очистить целиком (Clear) при выходе из приложения.
type SolidCache struct {
	dir   string
	limit int64

	mu    sync.Mutex
	locks map[string]*sync.Mutex // блокировка на ключ архива
}

// NewSolidCache создаёт кэш в папке dir (создаётся при первой распаковке).
// limit <= 0 — DefaultSolidCacheLimit.
func NewSolidCache(dir string, limit int64) *SolidCache {
	if limit <= 0 {
		limit = DefaultSolidCacheLimit
	}
	return &SolidCache{dir: dir, limit: limit, locks: map[string]*sync.Mutex{}}
}

// Clear удаляет весь кэш.
func (c *SolidCache) Clear() {
	if c == nil || c.dir == "" {
		return
	}
	if err := os.RemoveAll(c.dir); err != nil {
		log.Printf("solid cache: %v", err)
	}
}

// solidKey — имя папки архива в кэше по ключу галереи, размеру и времени
// изменения файла: изменённый файл получает новую папку.
func solidKey(id string, size int64, mod time.Time) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d\x00%d", id, size, mod.UnixNano())))
	return hex.EncodeToString(h[:12])
}

func (c *SolidCache) lock(key string) *sync.Mutex {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.locks[key]
	if !ok {
		m = &sync.Mutex{}
		c.locks[key] = m
	}
	return m
}

// entryFile — путь распакованной записи по её имени.
func entryFile(dir, name string) string {
	h := sha256.Sum256([]byte(name))
	return filepath.Join(dir, hex.EncodeToString(h[:12]))
}

// ensure распаковывает архив key в кэш, если его там ещё нет. extract
// получает папку и пишет в неё записи через write. need — ожидаемый
// распакованный размер (для вытеснения других архивов).
func (c *SolidCache) ensure(key string, need int64, extract func(write func(name string, r io.Reader) error) error) (string, error) {
	if need > c.limit {
		return "", ErrSolidTooLarge
	}
	m := c.lock(key)
	m.Lock()
	defer m.Unlock()

	dir := filepath.Join(c.dir, key)
	if _, err := os.Stat(filepath.Join(dir, solidDone)); err == nil {
		now := time.Now()
		_ = os.Chtimes(dir, now, now) // использован недавно
		return dir, nil
	}
	c.makeRoom(need, key)
	// недоделанную (прерванную) распаковку начинаем заново
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	written := map[string]bool{}
	write := func(name string, r io.Reader) error {
		p := entryFile(dir, name)
		if written[p] {
			return nil // повторяющееся имя: первая запись
		}
		written[p] = true
		f, err := os.Create(p)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, r)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		return err
	}
	if err := extract(write); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, solidDone), nil, 0o644); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

// makeRoom удаляет самые давние архивы (кроме keep), пока need байт не
// поместится в лимит.
func (c *SolidCache) makeRoom(need int64, keep string) {
	items, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	type dirInfo struct {
		name string
		size int64
		mod  time.Time
	}
	var all []dirInfo
	var total int64
	for _, it := range items {
		if !it.IsDir() || it.Name() == keep {
			continue
		}
		var size int64
		_ = filepath.WalkDir(filepath.Join(c.dir, it.Name()), func(_ string, d os.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if fi, err := d.Info(); err == nil {
					size += fi.Size()
				}
			}
			return nil
		})
		fi, err := it.Info()
		if err != nil {
			continue
		}
		all = append(all, dirInfo{it.Name(), size, fi.ModTime()})
		total += size
	}
	sort.Slice(all, func(i, j int) bool { return all[i].mod.Before(all[j].mod) })
	for _, d := range all {
		if total+need <= c.limit {
			return
		}
		if err := os.RemoveAll(filepath.Join(c.dir, d.name)); err == nil {
			total -= d.size
		}
	}
}

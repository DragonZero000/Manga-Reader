package library

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// ErrScanInProgress — сканирование уже идёт.
var ErrScanInProgress = errors.New("сканирование уже идёт")

// maxPageSize ограничивает размер читаемой страницы (защита от «zip-бомб»).
const maxPageSize = 256 << 20

// Source — источник «локальная папка библиотеки»: список галерей и
// доступ к страницам. Методы безопасны для вызова из разных горутин.
type Source struct {
	index search.Index

	stMu    sync.RWMutex
	st      storage.Storage // nil — папка не выбрана (Android)
	scanner *Scanner

	scanMu sync.Mutex // занят на время сканирования
	// busyAsError — занятые файлы попадают в ошибки (под scanMu)
	busyAsError bool

	mu        sync.RWMutex
	galleries []model.Gallery // заменяется целиком, не изменяется
	byKey     map[model.Key]int
}

// NewSource создаёт источник для хранилища st (nil — папка не выбрана).
// idx получает изменения после каждого сканирования; nil — без индекса.
func NewSource(st storage.Storage, idx search.Index) *Source {
	if idx == nil {
		idx = search.NopIndex{}
	}
	s := &Source{index: idx, byKey: map[model.Key]int{}}
	s.st, s.scanner = st, newScannerFor(st)
	return s
}

// NewDirSource — источник для папки файловой системы.
func NewDirSource(dir string, idx search.Index) *Source {
	return NewSource(storage.NewFS(dir), idx)
}

func newScannerFor(st storage.Storage) *Scanner {
	if st == nil {
		return nil
	}
	return NewScanner(st)
}

// Root возвращает папку библиотеки для показа («» — не выбрана).
func (s *Source) Root() string {
	st := s.storage()
	if st == nil {
		return ""
	}
	return st.Name()
}

func (s *Source) storage() storage.Storage {
	s.stMu.RLock()
	defer s.stMu.RUnlock()
	return s.st
}

// SetStorage меняет папку библиотеки: прежние галереи убираются из списка
// и индекса, следующее сканирование читает новую папку целиком.
func (s *Source) SetStorage(st storage.Storage) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.stMu.Lock()
	s.st, s.scanner = st, newScannerFor(st)
	s.stMu.Unlock()

	s.mu.Lock()
	old := s.galleries
	s.galleries, s.byKey = nil, map[model.Key]int{}
	s.mu.Unlock()
	for _, g := range old {
		if err := s.index.Remove(context.Background(), g.Key); err != nil {
			log.Printf("индекс: удаление %s: %v", g.Key, err)
		}
	}
}

// SetBusyAsError включает режим, в котором файлы, занятые другим процессом,
// попадают в ошибки; без него они пропускаются (ScanResult.Busy). Действует
// со следующего сканирования.
func (s *Source) SetBusyAsError(on bool) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.busyAsError = on
}

// Invalidate заставляет следующее сканирование заново разобрать файл
// relPath (путь относительно папки библиотеки, с прямыми слешами). Ждёт
// окончания идущего сканирования — не вызывать из UI-потока.
func (s *Source) Invalidate(relPath string) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.stMu.RLock()
	scanner := s.scanner
	s.stMu.RUnlock()
	if scanner != nil {
		scanner.Invalidate(relPath)
	}
}

// Scan сканирует папку и обновляет список галерей и индекс.
// Если сканирование уже идёт, сразу возвращает ErrScanInProgress.
func (s *Source) Scan(ctx context.Context) (ScanResult, error) {
	if !s.scanMu.TryLock() {
		return ScanResult{}, ErrScanInProgress
	}
	defer s.scanMu.Unlock()

	s.stMu.RLock()
	scanner := s.scanner
	s.stMu.RUnlock()
	if scanner == nil {
		return ScanResult{}, fmt.Errorf("%w: папка библиотеки не выбрана", storage.ErrUnavailable)
	}
	start := time.Now()
	scanner.BusyAsError = s.busyAsError
	res, err := scanner.Scan(ctx)
	if err != nil {
		return ScanResult{}, err
	}
	log.Printf("библиотека: %d галерей, новых/изменённых %d, ошибок %d, за %v",
		len(res.Galleries), len(res.Added)+len(res.Changed), len(res.Errors), time.Since(start).Round(time.Millisecond))

	byKey := make(map[model.Key]int, len(res.Galleries))
	for i, g := range res.Galleries {
		byKey[g.Key] = i
	}
	s.mu.Lock()
	s.galleries, s.byKey = res.Galleries, byKey
	s.mu.Unlock()

	s.updateIndex(ctx, res)
	for _, e := range res.NewErrors {
		log.Printf("библиотека: %v", e)
	}
	for _, w := range res.Warnings {
		log.Printf("библиотека: %s", w)
	}
	return res, nil
}

func (s *Source) updateIndex(ctx context.Context, res ScanResult) {
	for _, gs := range [][]model.Gallery{res.Added, res.Changed} {
		for _, g := range gs {
			if err := s.index.Upsert(ctx, g); err != nil {
				log.Printf("индекс: %s: %v", g.Key, err)
			}
		}
	}
	for _, k := range res.Removed {
		if err := s.index.Remove(ctx, k); err != nil {
			log.Printf("индекс: удаление %s: %v", k, err)
		}
	}
}

// Galleries возвращает текущий список галерей (не изменять).
func (s *Source) Galleries() []model.Gallery {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.galleries
}

// Get возвращает галерею по ключу.
func (s *Source) Get(k model.Key) (model.Gallery, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	i, ok := s.byKey[k]
	if !ok {
		return model.Gallery{}, false
	}
	return s.galleries[i], true
}

// OpenPage читает страницу галереи. Архив открывается только на время
// чтения, поэтому файл не блокируется.
func (s *Source) OpenPage(k model.Key, page string) (io.ReadCloser, error) {
	g, ok := s.Get(k)
	if !ok {
		return nil, fmt.Errorf("галерея %s не найдена", k)
	}
	found := false
	for _, p := range g.Pages {
		if p.Name == page {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("страница %q не найдена в %s", page, k)
	}

	zr, closeFn, err := s.openGallery(g)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	f, err := zr.Open(page)
	if err != nil {
		return nil, fmt.Errorf("открытие %s: %w", page, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPageSize))
	if err != nil {
		return nil, fmt.Errorf("чтение %s: %w", page, err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// openGallery открывает архив галереи из хранилища. Файл закрывается
// возвращённой функцией — архив не удерживается дольше чтения.
func (s *Source) openGallery(g model.Gallery) (*zip.Reader, func(), error) {
	st := s.storage()
	if st == nil {
		return nil, nil, fmt.Errorf("%w: папка библиотеки не выбрана", storage.ErrUnavailable)
	}
	f, err := openArchive(st, g.Key.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("открытие %s: %w", g.Key.ID, err)
	}
	zr, err := zip.NewReader(f, f.Size())
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	return zr, func() { f.Close() }, nil
}

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
	"sync/atomic"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// ErrScanInProgress — сканирование уже идёт.
var ErrScanInProgress = errors.New("a scan is already running")

// maxPageSize ограничивает размер читаемой страницы (защита от «zip-бомб»).
const maxPageSize = 256 << 20

// Observer получает события источника в горутине сканирования или удаления
// (не в UI-потоке).
type Observer interface {
	// Scanned — успешный обход папки: вызывается под блокировкой сканирования
	// до публикации списка галерей и обновления индекса.
	Scanned(res ScanResult)
	// Deleted — архив rel удалён через Source.Delete.
	Deleted(rel string)
}

// Source — источник «локальная папка библиотеки»: список галерей и
// доступ к страницам. Методы безопасны для вызова из разных горутин.
type Source struct {
	index search.Index
	store ScanStore // nil — результаты сканирования только в памяти

	stMu     sync.RWMutex
	st       storage.Storage // nil — папка не выбрана (Android)
	scanner  *Scanner
	observer Observer // nil — нет
	overlay  Overlay  // nil — без наложения пользователя

	scanMu sync.Mutex // занят на время сканирования
	// edits — сколько правок (Refresh, Reindex) ждут или держат scanMu:
	// сканирование, начатое в это время, ждёт их, а не отменяется
	edits atomic.Int32
	// busyAsError — занятые файлы попадают в ошибки (под scanMu)
	busyAsError bool

	mu        sync.RWMutex
	galleries []model.Gallery // заменяется целиком, не изменяется
	byKey     map[model.Key]int
}

// NewSource создаёт источник для хранилища st (nil — папка не выбрана).
// idx получает изменения после каждого сканирования; nil — без индекса.
func NewSource(st storage.Storage, idx search.Index) *Source {
	return NewSourceWithStore(st, idx, nil)
}

// NewSourceWithStore — источник, сканер которого хранит результаты в store
// (каталог библиотеки) между запусками.
func NewSourceWithStore(st storage.Storage, idx search.Index, store ScanStore) *Source {
	if idx == nil {
		idx = search.NopIndex{}
	}
	s := &Source{index: idx, store: store, byKey: map[model.Key]int{}}
	s.st, s.scanner = st, s.newScannerFor(st)
	return s
}

// NewDirSource — источник для папки файловой системы.
func NewDirSource(dir string, idx search.Index) *Source {
	return NewSource(storage.NewFS(dir), idx)
}

func (s *Source) newScannerFor(st storage.Storage) *Scanner {
	if st == nil {
		return nil
	}
	return NewScannerWithStore(st, s.store)
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
	s.st, s.scanner = st, s.newScannerFor(st)
	s.stMu.Unlock()

	s.mu.Lock()
	old := s.galleries
	s.galleries, s.byKey = nil, map[model.Key]int{}
	s.mu.Unlock()
	for _, g := range old {
		if err := s.index.Remove(context.Background(), g.Key); err != nil {
			log.Printf("index: removing %s: %v", g.Key, err)
		}
	}
}

// SetObserver подписывает o на успешные сканирования и удаления (nil —
// отписать). Ждёт окончания идущего сканирования.
func (s *Source) SetObserver(o Observer) {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.stMu.Lock()
	s.observer = o
	s.stMu.Unlock()
}

func (s *Source) getObserver() Observer {
	s.stMu.RLock()
	defer s.stMu.RUnlock()
	return s.observer
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

// CanDelete сообщает, умеет ли хранилище библиотеки удалять файлы на этой
// платформе (Windows, Android).
func (s *Source) CanDelete() bool {
	_, ok := storage.DeleterOf(s.storage())
	return ok
}

// CanTrash сообщает, попадёт ли архив галереи k в корзину. Может обращаться
// к диску — не вызывать из UI-потока.
func (s *Source) CanTrash(k model.Key) (bool, error) {
	d, err := s.deleter(k)
	if err != nil {
		return false, err
	}
	return d.CanTrash(k.ID)
}

// Delete удаляет архив галереи k (в корзину или, если permanent,
// безвозвратно) и сразу убирает галерею из списка и индекса; каталог,
// ошибки и ссылки приводит в порядок следующее сканирование. Ждёт окончания
// идущего сканирования — не вызывать из UI-потока.
func (s *Source) Delete(k model.Key, permanent bool) error {
	d, err := s.deleter(k)
	if err != nil {
		return err
	}
	if err := d.Delete(k.ID, permanent); err != nil {
		return err
	}
	s.Invalidate(k.ID)

	s.mu.Lock()
	if i, ok := s.byKey[k]; ok {
		gs := make([]model.Gallery, 0, len(s.galleries)-1)
		gs = append(append(gs, s.galleries[:i]...), s.galleries[i+1:]...)
		byKey := make(map[model.Key]int, len(gs))
		for j, g := range gs {
			byKey[g.Key] = j
		}
		s.galleries, s.byKey = gs, byKey
	}
	s.mu.Unlock()
	if err := s.index.Remove(context.Background(), k); err != nil {
		log.Printf("index: removing %s: %v", k, err)
	}
	if o := s.getObserver(); o != nil {
		o.Deleted(k.ID)
	}
	return nil
}

func (s *Source) deleter(k model.Key) (storage.Deleter, error) {
	if k.Source != model.SourceLocal {
		return nil, fmt.Errorf("%w: gallery %s is not a local file", storage.ErrUnsupported, k)
	}
	st := s.storage()
	if st == nil {
		return nil, fmt.Errorf("%w: library folder is not selected", storage.ErrUnavailable)
	}
	d, ok := storage.DeleterOf(st)
	if !ok {
		return nil, storage.ErrUnsupported
	}
	return d, nil
}

// LoadCatalog показывает результаты прошлых сканирований из хранилища без
// обхода папки: галереи доступны сразу при запуске. К ним применяется
// наложение пользователя. Индекс не трогается — он хранится вместе с
// результатами (с наложением). Ждёт идущего сканирования.
func (s *Source) LoadCatalog() ScanResult {
	s.scanMu.Lock()
	defer s.scanMu.Unlock()
	s.stMu.RLock()
	scanner := s.scanner
	s.stMu.RUnlock()
	if scanner == nil {
		return ScanResult{}
	}
	start := time.Now()
	res := scanner.Cached()
	applyOverlay(s.getOverlay(), res.Galleries)
	byKey := make(map[model.Key]int, len(res.Galleries))
	for i, g := range res.Galleries {
		byKey[g.Key] = i
	}
	s.mu.Lock()
	s.galleries, s.byKey = res.Galleries, byKey
	s.mu.Unlock()
	log.Printf("catalog: loaded %d galleries and %d errors in %v",
		len(res.Galleries), len(res.Errors), time.Since(start).Round(time.Millisecond))
	return res
}

// Scan сканирует папку и обновляет список галерей и индекс.
// Если сканирование уже идёт, сразу возвращает ErrScanInProgress; идущую
// правку наложения (Refresh, Reindex) — ждёт.
func (s *Source) Scan(ctx context.Context) (ScanResult, error) {
	if !s.scanMu.TryLock() {
		if s.edits.Load() == 0 {
			return ScanResult{}, ErrScanInProgress
		}
		s.scanMu.Lock()
	}
	defer s.scanMu.Unlock()

	s.stMu.RLock()
	scanner, observer, overlay := s.scanner, s.observer, s.overlay
	s.stMu.RUnlock()
	if scanner == nil {
		return ScanResult{}, fmt.Errorf("%w: library folder is not selected", storage.ErrUnavailable)
	}
	start := time.Now()
	scanner.BusyAsError = s.busyAsError
	res, err := scanner.Scan(ctx)
	if err != nil {
		return ScanResult{}, err
	}
	log.Printf("library: %d galleries, %d new/changed, %d errors, %d files opened, in %v",
		len(res.Galleries), len(res.Added)+len(res.Changed), len(res.Errors), res.Opened, time.Since(start).Round(time.Millisecond))

	// порядок: сверка пользовательских данных (перенос по отпечатку) →
	// наложение → публикация и индекс
	if observer != nil {
		observer.Scanned(res)
	}
	applyOverlay(overlay, res.Galleries, res.Added, res.Changed)
	byKey := make(map[model.Key]int, len(res.Galleries))
	for i, g := range res.Galleries {
		byKey[g.Key] = i
	}
	s.mu.Lock()
	s.galleries, s.byKey = res.Galleries, byKey
	s.mu.Unlock()

	s.updateIndex(ctx, res)
	for _, e := range res.NewErrors {
		log.Printf("library: %v", e)
	}
	for _, w := range res.Warnings {
		log.Printf("library: %s", w)
	}
	return res, nil
}

func (s *Source) updateIndex(ctx context.Context, res ScanResult) {
	for _, gs := range [][]model.Gallery{res.Added, res.Changed} {
		for _, g := range gs {
			if err := s.index.Upsert(ctx, g); err != nil {
				log.Printf("index: %s: %v", g.Key, err)
			}
		}
	}
	for _, k := range res.Removed {
		if err := s.index.Remove(ctx, k); err != nil {
			log.Printf("index: removing %s: %v", k, err)
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
		return nil, fmt.Errorf("gallery %s not found", k)
	}
	found := false
	for _, p := range g.Pages {
		if p.Name == page {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("page %q not found in %s", page, k)
	}

	zr, closeFn, err := s.openGallery(g)
	if err != nil {
		return nil, err
	}
	defer closeFn()
	f, err := zr.Open(page)
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", page, err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxPageSize))
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", page, err)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// openGallery открывает архив галереи из хранилища. Файл закрывается
// возвращённой функцией — архив не удерживается дольше чтения.
func (s *Source) openGallery(g model.Gallery) (*zip.Reader, func(), error) {
	st := s.storage()
	if st == nil {
		return nil, nil, fmt.Errorf("%w: library folder is not selected", storage.ErrUnavailable)
	}
	f, err := openArchive(st, g.Key.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("opening %s: %w", g.Key.ID, err)
	}
	zr, err := zip.NewReader(f, f.Size())
	if err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("%w: %v", ErrNotZip, err)
	}
	return zr, func() { f.Close() }, nil
}

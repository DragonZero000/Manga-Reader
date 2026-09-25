package library

import (
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// ScanError — файл, который не удалось открыть как галерею.
type ScanError struct {
	RelPath string
	Size    int64
	ModTime time.Time
	Err     error
}

func (e ScanError) Error() string { return e.RelPath + ": " + e.Err.Error() }

// ScanResult — итог сканирования папки библиотеки.
type ScanResult struct {
	Galleries []model.Gallery // все галереи: новые сверху
	Errors    []ScanError     // все ошибочные файлы (включая известные с прошлых сканов)
	NewErrors []ScanError     // ошибки файлов, разобранных в этом скане
	Warnings  []string        // предупреждения архивов, разобранных в этом скане
	// Busy — файлы, занятые другим процессом (сканирование стоит повторить).
	// Заполняется всегда; в ошибки они попадают только при BusyAsError.
	Busy []string

	Added   []model.Gallery
	Changed []model.Gallery
	Removed []model.Key
}

// Scanner обходит папку библиотеки и проверяет все файлы: .zip разбираются
// как архивы, остальные попадают в ошибки. Неизменённые файлы (размер и
// время изменения совпадают) повторно не открываются.
// Scanner не потокобезопасен — синхронизацию обеспечивает Source.
type Scanner struct {
	st    storage.Storage
	cache map[string]cacheEntry // по относительному пути

	// BusyAsError — занятые файлы попадают в ошибки («Файл занят другой
	// программой»), иначе пропускаются до следующего сканирования.
	BusyAsError bool
}

type cacheEntry struct {
	size    int64
	modTime time.Time
	gallery model.Gallery
	err     error // архив нечитаем — не открываем, пока файл не изменится
	// transient — результат временный (файл был занят): при следующем
	// сканировании файл проверяется заново, даже если не изменился.
	transient bool
}

func NewScanner(st storage.Storage) *Scanner {
	return &Scanner{st: st, cache: map[string]cacheEntry{}}
}

// Scan обходит папку библиотеки.
func (s *Scanner) Scan(ctx context.Context) (ScanResult, error) {
	var res ScanResult
	seen := map[string]bool{}

	var entries []storage.Entry
	paths := map[string]bool{}
	err := s.st.Walk(ctx, func(e storage.Entry) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if e.IsDir || isHidden(e.RelPath) {
			return nil
		}
		entries = append(entries, e)
		paths[strings.ToLower(e.RelPath)] = true
		return nil
	})
	if err != nil {
		return ScanResult{}, fmt.Errorf("сканирование %s: %w", s.st.Name(), err)
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return ScanResult{}, ctx.Err()
		}
		if isDownloading(e, paths) {
			continue // идёт загрузка: ни галерея, ни ошибка, в кэш не попадает
		}
		seen[e.RelPath] = true
		s.scanFile(e, &res)
	}

	for rel, e := range s.cache {
		if !seen[rel] {
			if e.err == nil {
				res.Removed = append(res.Removed, e.gallery.Key)
			}
			delete(s.cache, rel)
		}
	}
	for rel, e := range s.cache {
		if e.err != nil {
			res.Errors = append(res.Errors, ScanError{rel, e.size, e.modTime, e.err})
		} else {
			res.Galleries = append(res.Galleries, e.gallery)
		}
	}
	sortGalleries(res.Galleries)
	sort.Slice(res.Errors, func(i, j int) bool { return res.Errors[i].RelPath < res.Errors[j].RelPath })
	return res, nil
}

func (s *Scanner) scanFile(e storage.Entry, res *ScanResult) {
	rel := e.RelPath
	prev, known := s.cache[rel]
	if known && !prev.transient && prev.size == e.Size && prev.modTime.Equal(e.ModTime) {
		return // не изменился
	}
	g, warnings, err := readEntry(s.st, e)
	busy := errors.Is(err, storage.ErrBusy)
	if busy {
		res.Busy = append(res.Busy, rel)
		if !s.BusyAsError {
			return // прежний результат (если был) остаётся до повторной попытки
		}
		if known && prev.err == nil {
			res.Removed = append(res.Removed, prev.gallery.Key)
		}
		s.cache[rel] = cacheEntry{size: e.Size, modTime: e.ModTime, err: err, transient: true}
		if !(known && prev.transient) {
			res.NewErrors = append(res.NewErrors, ScanError{rel, e.Size, e.ModTime, err})
		}
		return
	}
	entry := cacheEntry{size: e.Size, modTime: e.ModTime, gallery: g, err: err}
	s.cache[rel] = entry
	res.Warnings = append(res.Warnings, warnings...)
	switch {
	case err != nil:
		res.NewErrors = append(res.NewErrors, ScanError{rel, e.Size, e.ModTime, err})
		if known && prev.err == nil {
			res.Removed = append(res.Removed, prev.gallery.Key)
		}
	case known && prev.err == nil:
		res.Changed = append(res.Changed, g)
	default:
		res.Added = append(res.Added, g)
	}
}

// Invalidate заставляет следующее сканирование заново разобрать файл rel,
// даже если размер и время изменения не менялись (например, к файлу
// записан адрес страницы, с которой он скачан).
func (s *Scanner) Invalidate(rel string) {
	if e, ok := s.cache[rel]; ok {
		e.transient = true
		s.cache[rel] = e
	}
}

// sortGalleries: недавно изменённые сверху, при равенстве — по ключу.
func sortGalleries(gs []model.Gallery) {
	sort.SliceStable(gs, func(i, j int) bool {
		ti, tj := gs[i].File.ModTime, gs[j].File.ModTime
		if !ti.Equal(tj) {
			return ti.After(tj)
		}
		return model.NaturalLess(gs[i].Key.ID, gs[j].Key.ID)
	})
}

// isHidden — путь содержит сегмент, начинающийся с «.».
func isHidden(rel string) bool {
	for _, seg := range strings.Split(rel, "/") {
		if strings.HasPrefix(seg, ".") {
			return true
		}
	}
	return false
}

// isDownloading — файл незавершённой загрузки Firefox: сам *.part или
// пустой файл-заглушка, рядом с которым лежит <имя>.part. paths — пути
// всех файлов папки в нижнем регистре.
func isDownloading(e storage.Entry, paths map[string]bool) bool {
	lower := strings.ToLower(e.RelPath)
	if strings.HasSuffix(lower, ".part") {
		return true
	}
	return e.Size == 0 && paths[lower+".part"]
}

// openHook вызывается при каждом открытии архива (подсчёт в тестах).
var openHook func()

// openArchive открывает архив из хранилища.
func openArchive(st storage.Storage, rel string) (storage.File, error) {
	if openHook != nil {
		openHook()
	}
	return st.Open(rel)
}

// readEntry проверяет файл: .zip разбирается как архив, остальные файлы —
// ошибка с типом, определённым по содержимому.
func readEntry(st storage.Storage, e storage.Entry) (model.Gallery, []string, error) {
	if e.Size == 0 {
		return model.Gallery{}, nil, ErrEmpty
	}
	f, err := openArchive(st, e.RelPath)
	if errors.Is(err, storage.ErrBusy) {
		return model.Gallery{}, nil, storage.ErrBusy // причина — ровно текст ErrBusy
	}
	if err != nil {
		return model.Gallery{}, nil, fmt.Errorf("не удалось открыть: %w", err)
	}
	defer f.Close()
	ext := path.Ext(e.RelPath)
	if !strings.EqualFold(ext, ".zip") {
		return model.Gallery{}, nil, &UnsupportedError{Kind: Sniff(f), Ext: ext}
	}
	g, warnings, err := ReadArchive(f, e.RelPath, e, displayPath(st, e.RelPath))
	if err == nil && g.SourceURL == "" {
		if src, ok := st.(sourcer); ok {
			g.SourceURL = src.SourceURL(e.RelPath)
		}
	}
	if errors.Is(err, ErrNotZip) {
		// уточняем причину, если содержимое распознано (картинка, HTML, …)
		if k := Sniff(f); k != KindUnknown && k != KindZip {
			return model.Gallery{}, nil, &UnsupportedError{Kind: k}
		}
	}
	return g, warnings, err
}

// sourcer — хранилище, знающее адрес страницы, с которой скачан файл
// (storage.FS на Windows — по метке загрузки Zone.Identifier).
type sourcer interface {
	SourceURL(relPath string) string
}

// displayPath — путь архива для показа пользователю.
func displayPath(st storage.Storage, rel string) string {
	if _, ok := st.(*storage.FS); ok {
		return filepath.Join(st.Name(), filepath.FromSlash(rel))
	}
	return st.Name() + "/" + rel
}

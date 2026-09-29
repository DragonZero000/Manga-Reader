package library

import (
	"context"
	"errors"
	"fmt"
	"log"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

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
	// Opened — сколько файлов открыто при этом сканировании.
	Opened int

	Added   []model.Gallery
	Changed []model.Gallery
	Removed []model.Key
}

// Scanner обходит папку библиотеки и проверяет все файлы: .zip разбираются
// как архивы, остальные попадают в ошибки. Неизменённые файлы (размер и
// время изменения совпадают) повторно не открываются; с хранилищем
// (ScanStore) — и после перезапуска приложения.
// Scanner не потокобезопасен — синхронизацию обеспечивает Source.
type Scanner struct {
	st    storage.Storage
	store ScanStore             // nil — результаты только в памяти
	cache map[string]cacheEntry // по относительному пути

	// BusyAsError — занятые файлы попадают в ошибки («Файл занят другой
	// программой»), иначе пропускаются до следующего сканирования.
	BusyAsError bool

	// revPending — сохранённые результаты получены по правилам разбора
	// старше ParseRevision: ревизия сохраняется сканированием, после
	// которого не осталось записей с recheck.
	revPending bool
}

type cacheEntry struct {
	size    int64
	modTime time.Time
	gallery model.Gallery
	err     error // архив нечитаем — не открываем, пока файл не изменится
	// transient — результат временный (файл был занят): при следующем
	// сканировании файл проверяется заново, даже если не изменился.
	transient bool
	// recheck — результат получен по прежним правилам разбора и может
	// измениться: файл разбирается заново, даже если не изменился. В отличие
	// от transient, до перепроверки запись показывается (Cached).
	recheck bool
}

// NewScanner — сканер без хранилища: результаты только в памяти.
func NewScanner(st storage.Storage) *Scanner { return NewScannerWithStore(st, nil) }

// NewScannerWithStore — сканер, который берёт прежние результаты из store и
// сохраняет туда изменения каждого сканирования.
func NewScannerWithStore(st storage.Storage, store ScanStore) *Scanner {
	s := &Scanner{st: st, store: store, cache: map[string]cacheEntry{}}
	if store == nil {
		return s
	}
	stored, err := store.Load()
	if err != nil {
		log.Printf("catalog: scan results not loaded: %v", err)
		return s
	}
	rev, err := store.ParseRevision()
	if err != nil {
		log.Printf("catalog: parse revision: %v", err)
		rev = 0 // перепроверить лишнее безопаснее, чем пропустить
	}
	s.revPending = rev < ParseRevision
	recheck := 0
	for rel, e := range stored {
		ce := cacheEntry{size: e.Size, modTime: e.ModTime, gallery: e.Gallery, err: e.Err}
		if s.revPending && needsRecheck(e) {
			ce.recheck = true
			recheck++
		}
		s.cache[rel] = ce
	}
	if recheck > 0 {
		log.Printf("catalog: parse rules revision %d → %d: %d files will be parsed again", rev, ParseRevision, recheck)
	}
	return s
}

// needsRecheck — сохранённый результат мог измениться с ревизией правил
// разбора ParseRevision: ошибка «нет изображений» (записи «./»); страницы с
// «\» в имени в UTF-8 (теперь разделитель), с U+FFFD (имя не в UTF-8,
// искажённое прежним сохранением в каталог) и с повторяющимися именами.
func needsRecheck(e StoredEntry) bool {
	if e.Err != nil {
		return errors.Is(e.Err, ErrNoImages)
	}
	seen := make(map[string]bool, len(e.Gallery.Pages))
	for _, p := range e.Gallery.Pages {
		n := p.Name
		if seen[n] || strings.Contains(n, "�") || (strings.Contains(n, `\`) && utf8.ValidString(n)) {
			return true
		}
		seen[n] = true
	}
	return false
}

// Cached — результаты из кэша без обхода папки (галереи — новые сверху,
// ошибки — по пути): что было известно на момент последнего сканирования.
func (s *Scanner) Cached() ScanResult {
	var res ScanResult
	for rel, e := range s.cache {
		if e.transient {
			continue
		}
		if e.err != nil {
			res.Errors = append(res.Errors, ScanError{rel, e.size, e.modTime, e.err})
		} else {
			res.Galleries = append(res.Galleries, e.gallery)
		}
	}
	sortGalleries(res.Galleries)
	sort.Slice(res.Errors, func(i, j int) bool { return res.Errors[i].RelPath < res.Errors[j].RelPath })
	return res
}

// Scan обходит папку библиотеки.
func (s *Scanner) Scan(ctx context.Context) (ScanResult, error) {
	var res ScanResult
	seen := map[string]bool{}
	delta := newDelta()

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
		return ScanResult{}, fmt.Errorf("scanning %s: %w", s.st.Name(), err)
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			return ScanResult{}, ctx.Err()
		}
		if isDownloading(e, paths) {
			continue // идёт загрузка: ни галерея, ни ошибка, в кэш не попадает
		}
		seen[e.RelPath] = true
		s.scanFile(e, &res, &delta)
	}

	for rel, e := range s.cache {
		if !seen[rel] {
			if e.err == nil {
				res.Removed = append(res.Removed, e.gallery.Key)
			}
			delete(s.cache, rel)
			delete(delta.Put, rel)
			delta.Delete = append(delta.Delete, rel)
		}
	}
	// ревизия — в той же транзакции, что и результаты перепроверки
	if s.revPending && !s.anyRecheck() {
		delta.ParseRevision = ParseRevision
	}
	if s.store != nil && !delta.Empty() {
		if err := s.store.Save(delta); err != nil {
			log.Printf("catalog: scan results not saved: %v", err)
			// результаты перепроверки могли не сохраниться: ревизию в этом
			// запуске не записываем, следующий перепроверит файлы заново
			s.revPending = false
		} else if delta.ParseRevision != 0 {
			s.revPending = false
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

// anyRecheck — остались записи, ожидающие перепроверки по новым правилам.
func (s *Scanner) anyRecheck() bool {
	for _, e := range s.cache {
		if e.recheck {
			return true
		}
	}
	return false
}

func (s *Scanner) scanFile(e storage.Entry, res *ScanResult, delta *ScanDelta) {
	rel := e.RelPath
	prev, known := s.cache[rel]
	if known && !prev.transient && !prev.recheck && prev.size == e.Size && prev.modTime.Equal(e.ModTime) {
		return // не изменился
	}
	res.Opened++
	g, warnings, err := readEntry(s.st, e)
	if err == nil {
		s.links(rel, &g, delta)
	}
	busy := errors.Is(err, storage.ErrBusy)
	if busy {
		res.Busy = append(res.Busy, rel)
		if !s.BusyAsError {
			return // прежний результат (если был) остаётся до повторной попытки
		}
		if known && prev.err == nil {
			res.Removed = append(res.Removed, prev.gallery.Key)
		}
		// перепроверка не выполнена — ревизия не сохраняется, пока файл занят
		s.cache[rel] = cacheEntry{size: e.Size, modTime: e.ModTime, err: err, transient: true, recheck: known && prev.recheck}
		if !(known && prev.transient) {
			res.NewErrors = append(res.NewErrors, ScanError{rel, e.Size, e.ModTime, err})
		}
		return
	}
	entry := cacheEntry{size: e.Size, modTime: e.ModTime, gallery: g, err: err}
	s.cache[rel] = entry
	delta.Put[rel] = StoredEntry{Size: e.Size, ModTime: e.ModTime, Gallery: g, Err: err}
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

// links: ссылка скачанного файла от хранилища (links.json, метка загрузки)
// сохраняется второй копией; если её там нет — берётся сохранённая копия.
// Ссылка из meta.json главнее — её не трогаем.
func (s *Scanner) links(rel string, g *model.Gallery, delta *ScanDelta) {
	if src, ok := s.st.(sourcer); ok {
		if u := model.WebURL(src.SourceURL(rel)); u != "" && g.SourceURL == u {
			delta.Links[rel] = u
			return
		}
	}
	if g.SourceURL == "" && s.store != nil {
		g.SourceURL = model.WebURL(s.store.StoredLink(rel))
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
		return model.Gallery{}, nil, fmt.Errorf("could not open: %w", err)
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

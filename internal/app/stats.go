package app

import (
	"context"
	"encoding/json"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/search"
	"mangareader/internal/stats"
	"mangareader/internal/storage"
)

// Настройки статистики (storage.Settings). Это настройки отображения, а не
// данные произведений, поэтому они не в user.db. Битое значение даёт
// значение по умолчанию и запись в журнал.
const (
	// KeyStatsFilter — текст фильтра статистики (синтаксис поиска).
	KeyStatsFilter = "stats.filter"
	// KeyStatsExcluded — постоянные исключения тегов, JSON {"artist":
	// ["artist:x"], "custom": [...]}: блок → канонические строки "тип:имя".
	// Временные исключения живут только в памяти экрана.
	KeyStatsExcluded = "stats.excluded"
	// KeyStatsBlocks — JSON-список выключенных блоков; нет значения — все
	// включены (новые блоки будущих версий видны по умолчанию).
	KeyStatsBlocks = "stats.blocks"
	// KeyStatsModes — JSON {"artist": "size"}: режим ранжирования топа.
	KeyStatsModes = "stats.modes"
	// KeyStatsOrder — JSON-список топов (и "largest") с порядком по
	// возрастанию; остальные — по убыванию.
	KeyStatsOrder = "stats.order"
	// KeyStatsLargest — величина списка самых больших: "size" или "pages".
	KeyStatsLargest = "stats.largest"
	// KeyStatsSummaryHidden — JSON-список скрытых полей Сводки
	// (stats.SummaryFields).
	KeyStatsSummaryHidden = "stats.summary.hidden"
	// KeyStatsBoundsPages, KeyStatsBoundsSize — границы корзин страниц и
	// объёма (МБ) строкой, как ввёл пользователь после проверки.
	KeyStatsBoundsPages = "stats.bounds.pages"
	KeyStatsBoundsSize  = "stats.bounds.size"
)

// StatsFilter возвращает сохранённый фильтр статистики.
func StatsFilter(s storage.Settings) string { return s.String(KeyStatsFilter, "") }

// SetStatsFilter сохраняет фильтр статистики.
func SetStatsFilter(s storage.Settings, filter string) { s.SetString(KeyStatsFilter, filter) }

// readJSON читает JSON-значение настройки; при ошибке пишет в журнал и
// возвращает false.
func readJSON(s storage.Settings, key string, v any) bool {
	raw := s.String(key, "")
	if raw == "" {
		return false
	}
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		log.Printf("settings %s: damaged value ignored: %v", key, err)
		return false
	}
	return true
}

func writeJSON(s storage.Settings, key string, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("settings %s: %v", key, err)
		return
	}
	s.SetString(key, string(b))
}

// knownStatsBlock — блоки, которые можно исключать/выключать (14 блоков).
func knownStatsBlock(id stats.BlockID) bool {
	for _, b := range stats.AllBlocks() {
		if b == id {
			return true
		}
	}
	return false
}

// StatsExcluded возвращает исключённые теги по блокам.
func StatsExcluded(s storage.Settings) map[stats.BlockID]map[string]bool {
	var raw map[string][]string
	out := map[stats.BlockID]map[string]bool{}
	if !readJSON(s, KeyStatsExcluded, &raw) {
		return out
	}
	for id, tags := range raw {
		b := stats.BlockID(id)
		if !knownStatsBlock(b) && b != stats.ExcludeCustom && b != stats.ExcludeHidden {
			continue
		}
		for _, t := range tags {
			if out[b] == nil {
				out[b] = map[string]bool{}
			}
			out[b][t] = true
		}
	}
	return out
}

// SetStatsExcluded сохраняет исключённые теги по блокам.
func SetStatsExcluded(s storage.Settings, ex map[stats.BlockID]map[string]bool) {
	raw := map[string][]string{}
	for id, set := range ex {
		for t := range set {
			raw[string(id)] = append(raw[string(id)], t)
		}
		sort.Strings(raw[string(id)])
	}
	writeJSON(s, KeyStatsExcluded, raw)
}

// StatsDisabled возвращает выключенные блоки.
func StatsDisabled(s storage.Settings) map[stats.BlockID]bool {
	var raw []string
	out := map[stats.BlockID]bool{}
	if !readJSON(s, KeyStatsBlocks, &raw) {
		return out
	}
	for _, id := range raw {
		if b := stats.BlockID(id); knownStatsBlock(b) {
			out[b] = true
		}
	}
	return out
}

// SetStatsDisabled сохраняет выключенные блоки.
func SetStatsDisabled(s storage.Settings, off map[stats.BlockID]bool) {
	raw := []string{}
	for _, id := range stats.AllBlocks() {
		if off[id] {
			raw = append(raw, string(id))
		}
	}
	writeJSON(s, KeyStatsBlocks, raw)
}

// StatsModes возвращает режимы ранжирования топов типов.
func StatsModes(s storage.Settings) map[stats.BlockID]stats.Mode {
	var raw map[string]string
	out := map[stats.BlockID]stats.Mode{}
	if !readJSON(s, KeyStatsModes, &raw) {
		return out
	}
	for id, m := range raw {
		b, mode := stats.BlockID(id), stats.Mode(m)
		if _, ok := stats.TypeOf(b); ok && mode.Valid() {
			out[b] = mode
		}
	}
	return out
}

// SetStatsModes сохраняет режимы ранжирования.
func SetStatsModes(s storage.Settings, modes map[stats.BlockID]stats.Mode) {
	raw := map[string]string{}
	for id, m := range modes {
		raw[string(id)] = string(m)
	}
	writeJSON(s, KeyStatsModes, raw)
}

// orderBlock — блоки, у которых есть порядок: топы типов, свои и скрытые
// теги, список самых больших.
func orderBlock(id stats.BlockID) bool {
	if _, ok := stats.TypeOf(id); ok {
		return true
	}
	return id == stats.ExcludeCustom || id == stats.ExcludeHidden || id == stats.BlockLargest
}

// StatsAscending возвращает блоки с порядком по возрастанию.
func StatsAscending(s storage.Settings) map[stats.BlockID]bool {
	var raw []string
	out := map[stats.BlockID]bool{}
	if !readJSON(s, KeyStatsOrder, &raw) {
		return out
	}
	for _, id := range raw {
		if b := stats.BlockID(id); orderBlock(b) {
			out[b] = true
		}
	}
	return out
}

// SetStatsAscending сохраняет блоки с порядком по возрастанию.
func SetStatsAscending(s storage.Settings, asc map[stats.BlockID]bool) {
	raw := []string{}
	for id, on := range asc {
		if on {
			raw = append(raw, string(id))
		}
	}
	sort.Strings(raw)
	writeJSON(s, KeyStatsOrder, raw)
}

// StatsLargestBy возвращает величину списка самых больших (ModeSize или
// ModePages).
func StatsLargestBy(s storage.Settings) stats.Mode {
	if stats.Mode(s.String(KeyStatsLargest, "")) == stats.ModePages {
		return stats.ModePages
	}
	return stats.ModeSize
}

// SetStatsLargestBy сохраняет величину списка самых больших.
func SetStatsLargestBy(s storage.Settings, m stats.Mode) { s.SetString(KeyStatsLargest, string(m)) }

// StatsSummaryHidden возвращает скрытые поля Сводки.
func StatsSummaryHidden(s storage.Settings) map[string]bool {
	var raw []string
	out := map[string]bool{}
	if !readJSON(s, KeyStatsSummaryHidden, &raw) {
		return out
	}
	known := map[string]bool{}
	for _, f := range stats.SummaryFields() {
		known[f] = true
	}
	for _, f := range raw {
		if known[f] {
			out[f] = true
		}
	}
	return out
}

// SetStatsSummaryHidden сохраняет скрытые поля Сводки.
func SetStatsSummaryHidden(s storage.Settings, hidden map[string]bool) {
	raw := []string{}
	for _, f := range stats.SummaryFields() {
		if hidden[f] {
			raw = append(raw, f)
		}
	}
	writeJSON(s, KeyStatsSummaryHidden, raw)
}

// StatsBounds возвращает границы корзин страниц и объёма (МБ); битое или
// отсутствующее значение даёт границы по умолчанию.
func StatsBounds(s storage.Settings) (pages, size []float64) {
	return statsBounds(s, KeyStatsBoundsPages, true, stats.DefaultPageBounds),
		statsBounds(s, KeyStatsBoundsSize, false, stats.DefaultSizeBounds)
}

func statsBounds(s storage.Settings, key string, integer bool, def []float64) []float64 {
	raw := s.String(key, "")
	if raw == "" {
		return def
	}
	b, err := stats.ParseBounds(raw, integer)
	if err != nil {
		log.Printf("settings %s: damaged value ignored: %v", key, err)
		return def
	}
	return b
}

// SetStatsBounds проверяет и сохраняет границы корзин; ошибка — *stats.BoundsError,
// прежние границы остаются. Пустая строка возвращает границы по умолчанию.
func SetStatsBounds(s storage.Settings, key, text string) error {
	if strings.TrimSpace(text) == "" {
		s.SetString(key, "")
		return nil
	}
	if _, err := stats.ParseBounds(text, key == KeyStatsBoundsPages); err != nil {
		return err
	}
	s.SetString(key, strings.TrimSpace(text))
	return nil
}

// StatsOptions собирает настройки подсчёта из Settings.
func StatsOptions(s storage.Settings) stats.Options {
	pages, size := StatsBounds(s)
	return stats.Options{
		Disabled:   StatsDisabled(s),
		Excluded:   StatsExcluded(s),
		Modes:      StatsModes(s),
		Ascending:  StatsAscending(s),
		LargestBy:  StatsLargestBy(s),
		PageBounds: pages,
		SizeBounds: size,
	}
}

// GallerySource — источник списка галерей (library.Source).
type GallerySource interface {
	Galleries() []model.Gallery
}

// StatsResponse — ответ на запрос статистики.
type StatsResponse struct {
	// Seq — номер запроса; экран принимает только ответ на последний.
	Seq uint64
	// Library — число произведений в библиотеке, Result.Works — в выборке.
	Library int
	Result  stats.Result
	// Err — ошибка разбора фильтра (*search.ParseError) или поиска;
	// подсчёта при ошибке нет. context.Canceled не доставляется.
	Err error
}

// StatsService считает статистику вне UI-потока: одна фоновая горутина на
// запрос, новый запрос отменяет предыдущий.
type StatsService struct {
	src GallerySource
	idx search.Index

	mu     sync.Mutex
	seq    uint64
	cancel context.CancelFunc
}

// NewStatsService создаёт сервис над источником галерей и индексом поиска.
func NewStatsService(src GallerySource, idx search.Index) *StatsService {
	return &StatsService{src: src, idx: idx}
}

// Request запускает подсчёт по фильтру filter и настройкам opt и возвращает
// номер запроса. Предыдущий подсчёт отменяется. done вызывается из фоновой
// горутины (не из UI-потока) ровно один раз, кроме отменённых запросов.
func (s *StatsService) Request(filter string, opt stats.Options, done func(StatsResponse)) uint64 {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.seq++
	seq := s.seq
	s.mu.Unlock()

	go func() {
		defer cancel()
		resp := s.compute(ctx, filter, opt)
		resp.Seq = seq
		if ctx.Err() != nil {
			return // устарел: результат не нужен
		}
		done(resp)
	}()
	return seq
}

// Latest сообщает, является ли seq номером последнего запроса.
func (s *StatsService) Latest(seq uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return seq == s.seq
}

func (s *StatsService) compute(ctx context.Context, filter string, opt stats.Options) StatsResponse {
	all := s.src.Galleries()
	resp := StatsResponse{Library: len(all)}
	sample := all
	if strings.TrimSpace(filter) != "" {
		q, err := search.Parse(filter)
		if err != nil {
			resp.Err = err
			return resp
		}
		keys, _, err := s.idx.Search(ctx, q)
		if err != nil {
			resp.Err = err
			return resp
		}
		want := make(map[model.Key]struct{}, len(keys))
		for _, k := range keys {
			want[k] = struct{}{}
		}
		sample = make([]model.Gallery, 0, len(keys))
		for i := range all {
			if _, ok := want[all[i].Key]; ok {
				sample = append(sample, all[i])
			}
		}
	}
	if opt.Loc == nil {
		opt.Loc = time.Local
	}
	res, err := stats.Compute(ctx, sample, opt)
	if err != nil {
		resp.Err = err
		return resp
	}
	resp.Result = res
	return resp
}

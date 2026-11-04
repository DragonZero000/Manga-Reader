// Package stats считает статистику состава библиотеки по списку галерей.
// Пакет без виджетов и языка интерфейса: результат — числа и ссылки на
// строки (RowRef), подписи и форматы строит экран. Query по ссылке строки
// собирает поисковый запрос, который находит то же, что посчитано в строке.
package stats

import (
	"context"
	"time"

	"mangareader/internal/model"
)

// BlockID — идентификатор блока статистики. Блоки топов типов тегов
// называются по типу ("artist", ...).
type BlockID string

// Блоки, кроме топов типов тегов.
const (
	BlockSummary  BlockID = "summary"
	BlockPages    BlockID = "pages"
	BlockSize     BlockID = "size"
	BlockUploaded BlockID = "uploaded"
	BlockGrowth   BlockID = "growth"
	BlockYours    BlockID = "yours"
	BlockQuality  BlockID = "quality"
)

// Топы внутри блока «Ваши теги»: у них свои исключения тегов.
const (
	ExcludeCustom BlockID = "custom"
	ExcludeHidden BlockID = "hidden"
)

// TypeBlocks — типы тегов с блоками-топами в порядке показа.
var TypeBlocks = []string{
	model.TagTypeArtist, model.TagTypeGroup, model.TagTypeParody, model.TagTypeCharacter,
	model.TagTypeLanguage, model.TagTypeCategory, model.TagTypeTag,
}

// AllBlocks возвращает 14 блоков в порядке показа.
func AllBlocks() []BlockID {
	out := []BlockID{BlockSummary}
	for _, typ := range TypeBlocks {
		out = append(out, BlockID(typ))
	}
	return append(out, BlockPages, BlockSize, BlockUploaded, BlockGrowth, BlockYours, BlockQuality)
}

// TypeOf возвращает тип тега блока-топа; false — блок не топ типа.
func TypeOf(id BlockID) (string, bool) {
	for _, typ := range TypeBlocks {
		if BlockID(typ) == id {
			return typ, true
		}
	}
	return "", false
}

// Mode — по какой величине ранжируется топ.
type Mode string

const (
	ModeWorks Mode = "works" // число произведений
	ModePages Mode = "pages" // сумма страниц
	ModeSize  Mode = "size"  // сумма объёма
)

// Valid сообщает, известен ли режим.
func (m Mode) Valid() bool { return m == ModeWorks || m == ModePages || m == ModeSize }

// Границы корзин по умолчанию: страницы и мегабайты.
var (
	DefaultPageBounds = []float64{20, 50, 100, 200, 500}
	DefaultSizeBounds = []float64{5, 10, 20, 50, 100, 200}
)

// TopLimit — число строк топа и списка самых больших по умолчанию (дальше —
// «Показать все»).
const TopLimit = 5

// BlockLargest — список самых больших произведений в блоке объёма: ключ его
// порядка в Options.Ascending (не отдельный блок).
const BlockLargest BlockID = "largest"

// monthsForYears — если между самым ранним и самым поздним месяцем роста
// меньше стольких месяцев, блок показывает месяцы, иначе годы.
const monthsForYears = 24

// Options — настройки подсчёта.
type Options struct {
	// Disabled — выключенные блоки (не считаются).
	Disabled map[BlockID]bool
	// Excluded — исключённые теги по блокам (типы тегов, ExcludeCustom,
	// ExcludeHidden): множество канонических строк "тип:имя".
	Excluded map[BlockID]map[string]bool
	// Modes — режим топа по блокам; пропуск — ModeWorks.
	Modes map[BlockID]Mode
	// Ascending — блоки с порядком по возрастанию: топы (типы, ExcludeCustom,
	// ExcludeHidden) и BlockLargest; по умолчанию — по убыванию.
	Ascending map[BlockID]bool
	// LargestBy — величина списка самых больших: ModeSize (по умолчанию) или
	// ModePages.
	LargestBy Mode
	// PageBounds и SizeBounds (МБ) — границы корзин; nil — по умолчанию.
	PageBounds []float64
	SizeBounds []float64
	// Loc — пояс для роста библиотеки; nil — местный.
	Loc *time.Location
}

func (o Options) enabled(id BlockID) bool { return !o.Disabled[id] }

func (o Options) largestBy() Mode {
	if o.LargestBy == ModePages {
		return ModePages
	}
	return ModeSize
}

func (o Options) mode(id BlockID) Mode {
	if m := o.Modes[id]; m.Valid() {
		return m
	}
	return ModeWorks
}

// RowKind — вид строки статистики для перехода в поиск.
type RowKind uint8

const (
	RowTag        RowKind = iota + 1 // тег топа типа
	RowCustom                        // свой тег
	RowHidden                        // скрытый тег
	RowPages                         // корзина страниц
	RowSize                          // корзина объёма
	RowUploaded                      // год загрузки
	RowNoDate                        // «Без даты» (не нажимается)
	RowAddedYear                     // год роста
	RowAddedMonth                    // месяц роста
	RowQuality                       // строка «Качества»
)

// Строки блока «Качество метаданных» (RowRef.Quality).
const (
	QualityNoMeta     = "meta"
	QualityNoArtist   = "artist"
	QualityNoLanguage = "language"
	QualityNoTags     = "tags"
)

// RowRef описывает условие строки.
type RowRef struct {
	Kind RowKind
	// Tag — тег (RowTag, RowCustom, RowHidden).
	Tag model.Tag
	// Lo, Hi — границы корзины: Lo — верхняя граница предыдущей корзины
	// (0 у первой), Hi — верхняя граница этой (0 у последней, «больше Lo»).
	// Страницы — число, объём — мегабайты.
	Lo, Hi float64
	// Year, Month — год и месяц (Month 1–12; 0 у годов).
	Year, Month int
	// Quality — QualityNoMeta и другие.
	Quality string
}

// Row — строка блока: условие, число произведений и доля выборки (0–1).
type Row struct {
	Ref   RowRef
	Count int
	Share float64
}

// TagRow — строка топа тегов.
type TagRow struct {
	Ref   RowRef
	Tag   model.Tag
	Works int
	Pages int64
	Size  int64
	// Value — величина режима топа, Share — её доля от суммы по выборке.
	Value int64
	Share float64
}

// Top — топ тегов: все строки (после исключений) по убыванию значения.
type Top struct {
	Mode Mode
	Rows []TagRow
}

// Summary — блок «Сводка».
type Summary struct {
	Works       int
	Pages, Size int64
	AvgPages    float64
	MedianPages float64
	AvgSize     float64
	MedianSize  float64
	// HasDates — у какого-то произведения есть дата загрузки.
	HasDates         bool
	Earliest, Latest time.Time
	Unique           map[string]int // уникальные теги по типам, без исключённых в топе
}

// Large — строка списка самых больших произведений.
type Large struct {
	Key   model.Key
	Title string
	Size  int64
	Pages int
}

// SizeBlock — блок «По объёму».
type SizeBlock struct {
	Rows []Row
	// Largest — вся выборка по величине By в порядке списка (Options).
	Largest []Large
	By      Mode
}

// GrowthYear — год роста библиотеки с месяцами для раскрытия.
type GrowthYear struct {
	Row    Row
	Months []Row
}

// Growth — блок «Рост библиотеки»: месяцы (ByYear == false) или годы.
type Growth struct {
	ByYear bool
	Months []Row
	Years  []GrowthYear
}

// Yours — блок «Ваши теги».
type Yours struct {
	WithCustom, WithHidden int
	Custom, Hidden         Top
}

// Result — результат подсчёта. Выключенные блоки — нулевые значения (nil).
type Result struct {
	// Works — число произведений выборки.
	Works   int
	Summary *Summary
	// Tops — топы типов по BlockID типа.
	Tops     map[BlockID]*Top
	Pages    []Row
	Size     *SizeBlock
	Uploaded []Row // годы по возрастанию, затем «Без даты» (RowNoDate)
	Growth   *Growth
	Yours    *Yours
	Quality  []Row

	// uniqueCounts — уникальные действующие теги по типам для Сводки.
	uniqueCounts map[string]int
}

// Compute считает статистику по выборке gs. Между блоками проверяет ctx:
// при отмене возвращает его ошибку.
func Compute(ctx context.Context, gs []model.Gallery, opt Options) (Result, error) {
	if opt.PageBounds == nil {
		opt.PageBounds = DefaultPageBounds
	}
	if opt.SizeBounds == nil {
		opt.SizeBounds = DefaultSizeBounds
	}
	if opt.Loc == nil {
		opt.Loc = time.Local
	}
	t := totals(gs)
	r := Result{Works: len(gs), Tops: map[BlockID]*Top{}}

	steps := []func(){
		func() { r.computeTags(gs, t, opt) },
		func() {
			if opt.enabled(BlockPages) {
				r.Pages = pageRows(gs, opt.PageBounds)
			}
		},
		func() {
			if opt.enabled(BlockSize) {
				r.Size = sizeBlock(gs, opt.SizeBounds, opt.largestBy(), opt.Ascending[BlockLargest])
			}
		},
		func() {
			if opt.enabled(BlockUploaded) {
				r.Uploaded = uploadedRows(gs)
			}
		},
		func() {
			if opt.enabled(BlockGrowth) {
				r.Growth = growth(gs, opt.Loc)
			}
		},
		func() {
			if opt.enabled(BlockYours) {
				r.Yours = yours(gs, t, opt)
			}
		},
		func() {
			if opt.enabled(BlockQuality) {
				r.Quality = quality(gs)
			}
		},
	}
	for _, step := range steps {
		if err := ctx.Err(); err != nil {
			return Result{}, err
		}
		step()
	}
	if opt.enabled(BlockSummary) {
		r.Summary = summary(gs, t, r.uniqueCounts)
	}
	return r, nil
}

// sum — суммы по выборке.
type sum struct {
	works       int
	pages, size int64
}

func totals(gs []model.Gallery) sum {
	t := sum{works: len(gs)}
	for i := range gs {
		t.pages += int64(len(gs[i].Pages))
		t.size += gs[i].File.Size
	}
	return t
}

// share — доля part от whole (0 при нулевом whole).
func share(part, whole float64) float64 {
	if whole <= 0 {
		return 0
	}
	return part / whole
}

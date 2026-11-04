package screens

import (
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"mangareader/internal/i18n"
	"mangareader/internal/stats"
)

// statsBlock — блок экрана статистики: заголовок и строки.
type statsBlock interface {
	// Object — содержимое блока для списка.
	Object() fyne.CanvasObject
	// Update показывает результат подсчёта.
	Update(res *stats.Result)
}

// blockTitle — заголовок блока на языке интерфейса.
func blockTitle(id stats.BlockID) string { return i18n.T("stats.block." + string(id)) }

func titleLabel(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.Truncation = fyne.TextTruncateEllipsis
	return l
}

// blockBox — карточка блока: заголовок (с дополнительным объектом справа) и
// содержимое под ним.
func blockBox(title string, right fyne.CanvasObject, body ...fyne.CanvasObject) *fyne.Container {
	head := fyne.CanvasObject(titleLabel(title))
	if right != nil {
		head = container.NewBorder(nil, nil, nil, right, head)
	}
	objs := append([]fyne.CanvasObject{head}, body...)
	objs = append(objs, widget.NewSeparator())
	return container.NewVBox(objs...)
}

// fracOf — длина полоски: значение относительно наибольшего в блоке.
func fracOf(v, maxV int64) float32 {
	if maxV <= 0 {
		return 0
	}
	return float32(v) / float32(maxV)
}

// percent — доля в процентах: целые от 10 %, иначе с одной цифрой.
func percent(share float64) string {
	p := share * 100
	s := strconv.FormatFloat(p, 'f', 0, 64)
	if p > 0 && p < 9.95 { // одна цифра после запятой, пока округление не даёт 10
		s = strconv.FormatFloat(p, 'f', 1, 64)
	}
	return strings.Replace(s, ".", i18n.T("number.decimal"), 1) + "%"
}

// fmtNum — число границы или подписи корзины с десятичным разделителем языка.
func fmtNum(v float64) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', -1, 64), ".", i18n.T("number.decimal"), 1)
}

// countValue — «число (доля)» для строки с количеством произведений.
func countValue(n int, share float64) string {
	return i18n.Int(int64(n)) + " (" + percent(share) + ")"
}

// bucketLabel — подпись корзины страниц или объёма.
func bucketLabel(ref stats.RowRef) string {
	switch {
	case ref.Hi == 0:
		return i18n.T("stats.bucket.over", "To", fmtNum(ref.Lo))
	case ref.Kind == stats.RowPages:
		if ref.Lo+1 == ref.Hi {
			return fmtNum(ref.Hi)
		}
		return fmtNum(ref.Lo+1) + "–" + fmtNum(ref.Hi)
	case ref.Lo == 0:
		return i18n.T("stats.bucket.upto", "To", fmtNum(ref.Hi))
	}
	return fmtNum(ref.Lo) + "–" + fmtNum(ref.Hi)
}

// maxCount — наибольшее число в строках.
func maxCount(rows []stats.Row) int64 {
	var m int64
	for _, r := range rows {
		m = max(m, int64(r.Count))
	}
	return m
}

// countRows превращает строки с числом произведений в данные строк;
// label строит подпись, ссылки строк ведут в поиск.
func (s *Stats) countRows(rows []stats.Row, label func(stats.Row) string) []rowData {
	mx := maxCount(rows)
	out := make([]rowData, len(rows))
	for i, r := range rows {
		out[i] = rowData{
			Label: label(r),
			Value: countValue(r.Count, r.Share),
			Frac:  fracOf(int64(r.Count), mx),
			Tap:   s.tapRef(r.Ref),
		}
	}
	return out
}

// tapRef — нажатие строки: переход в поиск (nil — строка не нажимается).
func (s *Stats) tapRef(ref stats.RowRef) func() {
	if stats.Query(ref, "") == "" {
		return nil
	}
	return func() { s.goSearch(ref) }
}

// --- Сводка ---

type summaryBlock struct {
	s    *Stats
	list *rowList
	box  *fyne.Container
}

func newSummaryBlock(s *Stats) *summaryBlock {
	b := &summaryBlock{s: s, list: newRowList()}
	b.box = blockBox(blockTitle(stats.BlockSummary), nil, b.list.box)
	return b
}

func (b *summaryBlock) Object() fyne.CanvasObject { return b.box }

func (b *summaryBlock) Update(res *stats.Result) {
	sm := res.Summary
	if sm == nil {
		return
	}
	dates := i18n.T("stats.no_data")
	if sm.HasDates {
		dates = i18n.Date(sm.Earliest) + " – " + i18n.Date(sm.Latest)
	}
	values := map[string]string{
		stats.SummaryWorks:       i18n.Int(int64(sm.Works)),
		stats.SummaryPages:       i18n.Int(sm.Pages),
		stats.SummarySize:        statSize(sm.Size),
		stats.SummaryAvgPages:    i18n.Int(int64(sm.AvgPages + 0.5)),
		stats.SummaryMedianPages: fmtNum(sm.MedianPages),
		stats.SummaryAvgSize:     statSize(int64(sm.AvgSize)),
		stats.SummaryMedianSize:  statSize(int64(sm.MedianSize)),
		stats.SummaryDates:       dates,
	}
	for _, typ := range stats.TypeBlocks {
		values[stats.SummaryUnique+typ] = i18n.Int(int64(sm.Unique[typ]))
	}
	var rows []rowData
	for _, f := range stats.SummaryFields() {
		if !b.s.sumHidden[f] {
			rows = append(rows, rowData{Label: summaryLabel(f), Value: values[f], NoBar: true})
		}
	}
	b.list.Set(rows)
}

// summaryLabel — подпись поля Сводки.
func summaryLabel(f string) string {
	if typ, ok := strings.CutPrefix(f, stats.SummaryUnique); ok {
		return i18n.T("stats.sum.unique", "Type", blockTitle(stats.BlockID(typ)))
	}
	return i18n.T("stats.sum." + f)
}

// --- Типы тегов ---

type typeBlock struct {
	id  stats.BlockID
	top *topWidget
}

func newTypeBlock(s *Stats, id stats.BlockID) *typeBlock {
	return &typeBlock{id: id, top: newTopWidget(s, id, blockTitle(id), true, false)}
}

func (b *typeBlock) Object() fyne.CanvasObject { return b.top.Object() }

func (b *typeBlock) Update(res *stats.Result) {
	if top := res.Tops[b.id]; top != nil {
		b.top.Set(*top)
	}
}

// --- Корзины страниц ---

type pagesBlock struct {
	s    *Stats
	list *rowList
	box  *fyne.Container
}

func newPagesBlock(s *Stats) *pagesBlock {
	b := &pagesBlock{s: s, list: newRowList()}
	b.box = blockBox(blockTitle(stats.BlockPages), nil, b.list.box)
	return b
}

func (b *pagesBlock) Object() fyne.CanvasObject { return b.box }

func (b *pagesBlock) Update(res *stats.Result) {
	b.list.Set(b.s.countRows(res.Pages, func(r stats.Row) string { return bucketLabel(r.Ref) }))
}

// --- Объём ---

type sizeBlock struct {
	s       *Stats
	list    *rowList
	largest *largestWidget
	box     *fyne.Container
}

func newSizeBlock(s *Stats) *sizeBlock {
	b := &sizeBlock{s: s, list: newRowList(), largest: newLargestWidget(s)}
	unit := widget.NewLabel(i18n.T("stats.size.unit"))
	unit.Importance = widget.LowImportance
	b.box = blockBox(blockTitle(stats.BlockSize), nil, unit, b.list.box, b.largest.box)
	return b
}

func (b *sizeBlock) Object() fyne.CanvasObject { return b.box }

func (b *sizeBlock) Update(res *stats.Result) {
	if res.Size == nil {
		return
	}
	b.list.Set(b.s.countRows(res.Size.Rows, func(r stats.Row) string { return bucketLabel(r.Ref) }))
	b.largest.Set(res.Size)
}

// --- Годы загрузки ---

type uploadedBlock struct {
	s    *Stats
	list *rowList
	box  *fyne.Container
}

func newUploadedBlock(s *Stats) *uploadedBlock {
	b := &uploadedBlock{s: s, list: newRowList()}
	b.box = blockBox(blockTitle(stats.BlockUploaded), nil, b.list.box)
	return b
}

func (b *uploadedBlock) Object() fyne.CanvasObject { return b.box }

func (b *uploadedBlock) Update(res *stats.Result) {
	b.list.Set(b.s.countRows(res.Uploaded, func(r stats.Row) string {
		if r.Ref.Kind == stats.RowNoDate {
			return i18n.T("stats.no_date")
		}
		return strconv.Itoa(r.Ref.Year)
	}))
}

// --- Рост библиотеки ---

type growthBlock struct {
	s        *Stats
	list     *rowList
	box      *fyne.Container
	growth   *stats.Growth
	expanded map[int]bool // раскрытые годы
}

func newGrowthBlock(s *Stats) *growthBlock {
	b := &growthBlock{s: s, list: newRowList(), expanded: map[int]bool{}}
	b.box = blockBox(blockTitle(stats.BlockGrowth), nil, b.list.box)
	return b
}

func (b *growthBlock) Object() fyne.CanvasObject { return b.box }

func monthLabel(r stats.Row) string {
	m := strconv.Itoa(r.Ref.Month)
	if r.Ref.Month < 10 {
		m = "0" + m
	}
	return strconv.Itoa(r.Ref.Year) + "-" + m
}

func (b *growthBlock) Update(res *stats.Result) {
	b.growth = res.Growth
	b.render()
}

// render строит строки роста: месяцы или годы с раскрытием месяцев.
func (b *growthBlock) render() {
	g := b.growth
	if g == nil {
		return
	}
	if !g.ByYear {
		b.list.Set(b.s.countRows(g.Months, monthLabel))
		return
	}
	var mx int64
	for _, y := range g.Years {
		mx = max(mx, int64(y.Row.Count))
		if b.expanded[y.Row.Ref.Year] {
			for _, m := range y.Months {
				mx = max(mx, int64(m.Count))
			}
		}
	}
	var data []rowData
	for _, y := range g.Years {
		year := y.Row.Ref.Year
		icon := theme.MenuDropDownIcon()
		if b.expanded[year] {
			icon = theme.MenuExpandIcon()
		}
		data = append(data, rowData{
			Label: strconv.Itoa(year), Value: countValue(y.Row.Count, y.Row.Share), Frac: fracOf(int64(y.Row.Count), mx),
			Tap: b.s.tapRef(y.Row.Ref), LeadIcon: icon,
			Lead: func() { b.expanded[year] = !b.expanded[year]; b.render() },
		})
		if !b.expanded[year] {
			continue
		}
		for _, m := range y.Months {
			data = append(data, rowData{
				Label: monthLabel(m), Value: countValue(m.Count, m.Share), Frac: fracOf(int64(m.Count), mx),
				Tap: b.s.tapRef(m.Ref), Indent: true,
			})
		}
	}
	b.list.Set(data)
}

// --- Ваши теги ---

type yoursBlock struct {
	s      *Stats
	counts *rowList
	custom *topWidget
	hidden *topWidget
	box    *fyne.Container
}

func newYoursBlock(s *Stats) *yoursBlock {
	b := &yoursBlock{s: s, counts: newRowList()}
	b.custom = newTopWidget(s, stats.ExcludeCustom, i18n.T("stats.yours.custom"), false, true)
	b.hidden = newTopWidget(s, stats.ExcludeHidden, i18n.T("stats.yours.hidden"), false, true)
	// без blockBox: разделитель в конце уже есть у вложенного топа скрытых
	b.box = container.NewVBox(titleLabel(blockTitle(stats.BlockYours)), b.counts.box, b.custom.Object(), b.hidden.Object())
	return b
}

func (b *yoursBlock) Object() fyne.CanvasObject { return b.box }

func (b *yoursBlock) Update(res *stats.Result) {
	y := res.Yours
	if y == nil {
		return
	}
	b.counts.Set([]rowData{
		{Label: i18n.T("stats.yours.with_custom"), Value: i18n.Int(int64(y.WithCustom)), NoBar: true},
		{Label: i18n.T("stats.yours.with_hidden"), Value: i18n.Int(int64(y.WithHidden)), NoBar: true},
	})
	b.custom.Set(y.Custom)
	b.hidden.Set(y.Hidden)
}

// --- Качество метаданных ---

type qualityBlock struct {
	s    *Stats
	list *rowList
	box  *fyne.Container
}

func newQualityBlock(s *Stats) *qualityBlock {
	b := &qualityBlock{s: s, list: newRowList()}
	b.box = blockBox(blockTitle(stats.BlockQuality), nil, b.list.box)
	return b
}

func (b *qualityBlock) Object() fyne.CanvasObject { return b.box }

func (b *qualityBlock) Update(res *stats.Result) {
	b.list.Set(b.s.countRows(res.Quality, func(r stats.Row) string {
		return i18n.T("stats.quality." + r.Ref.Quality)
	}))
}

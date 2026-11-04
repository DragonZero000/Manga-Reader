package stats

import (
	"sort"

	"mangareader/internal/model"
)

// tagAcc — накопитель по тегу: произведения, страницы, объём.
type tagAcc struct {
	works int
	pages int64
	size  int64
	// last — индекс последней учтённой галереи: тег, повторённый у одного
	// произведения, считается один раз.
	last int
}

// tagCounter копит теги одной группы.
type tagCounter map[model.Tag]*tagAcc

func (c tagCounter) add(t model.Tag, i int, g *model.Gallery) {
	a := c[t]
	if a == nil {
		a = &tagAcc{last: -1}
		c[t] = a
	}
	if a.last == i {
		return
	}
	a.last = i
	a.works++
	a.pages += int64(len(g.Pages))
	a.size += g.File.Size
}

// computeTags считает топы типов и число уникальных тегов по типам.
func (r *Result) computeTags(gs []model.Gallery, t sum, opt Options) {
	need := map[string]bool{} // типы, по которым нужны счётчики
	for _, typ := range TypeBlocks {
		if opt.enabled(BlockID(typ)) || opt.enabled(BlockSummary) {
			need[typ] = true
		}
	}
	if len(need) == 0 {
		return
	}
	counters := map[string]tagCounter{}
	for typ := range need {
		counters[typ] = tagCounter{}
	}
	for i := range gs {
		g := &gs[i]
		for _, tag := range g.EffectiveTags() {
			if c, ok := counters[tag.Type]; ok {
				c.add(tag, i, g)
			}
		}
	}
	if opt.enabled(BlockSummary) {
		r.uniqueCounts = map[string]int{}
		for typ, c := range counters {
			n := len(c)
			for key := range opt.Excluded[BlockID(typ)] {
				if _, ok := c[model.ParseTag(key)]; ok {
					n--
				}
			}
			r.uniqueCounts[typ] = n
		}
	}
	for _, typ := range TypeBlocks {
		id := BlockID(typ)
		if !opt.enabled(id) {
			continue
		}
		top := buildTop(counters[typ], opt.mode(id), t, opt.Excluded[id], RowTag, opt.Ascending[id])
		r.Tops[id] = &top
	}
}

// buildTop строит топ: убирает исключённые, считает значения и долю,
// сортирует по убыванию значения (asc — по возрастанию), при равенстве — по
// имени (натурально).
func buildTop(c tagCounter, mode Mode, t sum, excluded map[string]bool, kind RowKind, asc bool) Top {
	rows := make([]TagRow, 0, len(c))
	for tag, a := range c {
		if excluded[tag.String()] {
			continue
		}
		row := TagRow{Ref: RowRef{Kind: kind, Tag: tag}, Tag: tag, Works: a.works, Pages: a.pages, Size: a.size}
		switch mode {
		case ModePages:
			row.Value, row.Share = a.pages, share(float64(a.pages), float64(t.pages))
		case ModeSize:
			row.Value, row.Share = a.size, share(float64(a.size), float64(t.size))
		default:
			row.Value, row.Share = int64(a.works), share(float64(a.works), float64(t.works))
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Value != rows[j].Value {
			return rows[i].Value > rows[j].Value != asc
		}
		if rows[i].Tag.Name != rows[j].Tag.Name {
			return model.NaturalLess(rows[i].Tag.Name, rows[j].Tag.Name)
		}
		return rows[i].Tag.Type < rows[j].Tag.Type
	})
	return Top{Mode: mode, Rows: rows}
}

// yours считает блок «Ваши теги»: свои и скрытые теги по числу произведений.
func yours(gs []model.Gallery, t sum, opt Options) *Yours {
	y := &Yours{}
	custom, hidden := tagCounter{}, tagCounter{}
	for i := range gs {
		g := &gs[i]
		if len(g.Custom) > 0 {
			y.WithCustom++
		}
		for _, tag := range g.Custom {
			custom.add(tag, i, g)
		}
		hid := 0
		for _, tv := range g.TagViews() {
			if tv.Origin == model.OriginHidden {
				hid++
				hidden.add(tv.Tag, i, g)
			}
		}
		if hid > 0 {
			y.WithHidden++
		}
	}
	y.Custom = buildTop(custom, ModeWorks, t, opt.Excluded[ExcludeCustom], RowCustom, opt.Ascending[ExcludeCustom])
	y.Hidden = buildTop(hidden, ModeWorks, t, opt.Excluded[ExcludeHidden], RowHidden, opt.Ascending[ExcludeHidden])
	return y
}

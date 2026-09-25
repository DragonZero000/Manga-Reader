package search

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mangareader/internal/model"
)

// MemIndex — индекс поиска в памяти. Поиск — линейный проход по заранее
// подготовленным документам; для библиотек в тысячи произведений этого
// достаточно. Методы безопасны для вызова из разных горутин.
type MemIndex struct {
	mu   sync.RWMutex
	docs map[model.Key]*doc
}

// doc — произведение с подготовленными для поиска данными.
type doc struct {
	g     model.Gallery
	hay   []string           // нижний регистр: названия, теги, сканлейтор, файл, ID
	tags  map[model.Tag]bool // нормализованные теги
	names map[string]bool    // имена тегов без типа
	title string             // основное и альтернативное название, нижний регистр
	scan  string             // сканлейтор, нижний регистр
}

var _ Index = (*MemIndex)(nil)

func NewMemIndex() *MemIndex {
	return &MemIndex{docs: map[model.Key]*doc{}}
}

func newDoc(g model.Gallery) *doc {
	d := &doc{g: g, tags: map[model.Tag]bool{}, names: map[string]bool{}}
	add := func(s string) {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			d.hay = append(d.hay, s)
		}
	}
	add(g.Title)
	add(g.AltTitle)
	for _, t := range g.Tags {
		t = model.NewTag(t.Type, t.Name)
		d.tags[t] = true
		d.names[t.Name] = true
		add(t.Name)
		add(t.String())
	}
	add(g.Scanlator)
	add(g.Key.ID)
	if g.ExternalID > 0 {
		add(strconv.FormatInt(g.ExternalID, 10))
	}
	d.title = strings.ToLower(g.Title + "\n" + g.AltTitle)
	d.scan = strings.ToLower(strings.TrimSpace(g.Scanlator))
	return d
}

func (m *MemIndex) Upsert(_ context.Context, g model.Gallery) error {
	d := newDoc(g)
	m.mu.Lock()
	m.docs[g.Key] = d
	m.mu.Unlock()
	return nil
}

func (m *MemIndex) Remove(_ context.Context, k model.Key) error {
	m.mu.Lock()
	delete(m.docs, k)
	m.mu.Unlock()
	return nil
}

// Search возвращает ключи совпавших произведений (с учётом Limit/Offset)
// и общее число совпадений.
func (m *MemIndex) Search(ctx context.Context, q Query) ([]model.Key, int, error) {
	if err := q.Validate(); err != nil {
		return nil, 0, err
	}
	terms := q.Terms // Text — только исходная строка для отображения

	m.mu.RLock()
	var hits []*doc
	for _, d := range m.docs {
		if ctx.Err() != nil {
			m.mu.RUnlock()
			return nil, 0, ctx.Err()
		}
		if d.matchTerms(terms) && d.matchFilters(q.Filters) {
			hits = append(hits, d)
		}
	}
	m.mu.RUnlock()

	sortDocs(hits, q.Sort)
	total := len(hits)
	if q.Offset > 0 {
		hits = hits[min(q.Offset, len(hits)):]
	}
	if q.Limit > 0 && len(hits) > q.Limit {
		hits = hits[:q.Limit]
	}
	keys := make([]model.Key, len(hits))
	for i, d := range hits {
		keys[i] = d.g.Key
	}
	return keys, total, nil
}

// matchTerms: каждое слово — подстрока хотя бы одного поля.
func (d *doc) matchTerms(terms []string) bool {
	for _, t := range terms {
		found := false
		for _, h := range d.hay {
			if strings.Contains(h, t) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func (d *doc) matchFilters(filters []Filter) bool {
	for _, f := range filters {
		if !d.match(f) {
			return false
		}
	}
	return true
}

func (d *doc) match(f Filter) bool {
	g := d.g
	v := f.Value
	switch f.Field {
	case FieldTag:
		has := d.hasTag(v.Tag)
		if f.Op == OpNotHas {
			return !has
		}
		return has
	case FieldTitle:
		return strings.Contains(d.title, strings.ToLower(v.Text))
	case FieldScanlator:
		if d.scan == "" {
			return false
		}
		want := strings.ToLower(strings.TrimSpace(v.Text))
		if f.Op == OpEq {
			return d.scan == want
		}
		return strings.Contains(d.scan, want)
	case FieldID:
		return cmpNum(g.ExternalID, f)
	case FieldPages:
		return cmpNum(int64(len(g.Pages)), f)
	case FieldFavorites:
		return cmpNum(int64(g.Favorites), f)
	case FieldSize:
		return cmpNum(g.File.Size, f)
	case FieldUploaded:
		return cmpTime(g.Uploaded, f)
	case FieldAdded:
		return cmpTime(g.File.ModTime, f)
	}
	return false
}

func (d *doc) hasTag(t model.Tag) bool {
	if t.Type == "" {
		return d.names[t.Name]
	}
	return d.tags[t]
}

// cmpNum сравнивает число; нулевое значение (поле отсутствует) не проходит.
func cmpNum(n int64, f Filter) bool {
	if n <= 0 {
		return false
	}
	switch f.Op {
	case OpEq:
		return n == f.Value.Num
	case OpLt:
		return n < f.Value.Num
	case OpGt:
		return n > f.Value.Num
	case OpBetween:
		return n >= f.Value.Num && n <= f.Value.Num2
	}
	return false
}

// cmpTime сравнивает дату; нулевая дата не проходит.
func cmpTime(t time.Time, f Filter) bool {
	if t.IsZero() {
		return false
	}
	switch f.Op {
	case OpLt:
		return t.Before(f.Value.Time)
	case OpGt:
		return t.After(f.Value.Time)
	case OpBetween:
		return !t.Before(f.Value.Time) && !t.After(f.Value.Time2)
	}
	return false
}

// sortDocs упорядочивает результаты; при равенстве — ключ в натуральном порядке.
func sortDocs(ds []*doc, s Sort) {
	less := func(a, b *doc) int {
		ga, gb := a.g, b.g
		switch s.Field {
		case FieldTitle:
			return strings.Compare(strings.ToLower(ga.Title), strings.ToLower(gb.Title))
		case FieldID:
			return cmp64(ga.ExternalID, gb.ExternalID)
		case FieldPages:
			return cmp64(int64(len(ga.Pages)), int64(len(gb.Pages)))
		case FieldFavorites:
			return cmp64(int64(ga.Favorites), int64(gb.Favorites))
		case FieldSize:
			return cmp64(ga.File.Size, gb.File.Size)
		case FieldUploaded:
			return ga.Uploaded.Compare(gb.Uploaded)
		default: // FieldAdded
			return ga.File.ModTime.Compare(gb.File.ModTime)
		}
	}
	sort.SliceStable(ds, func(i, j int) bool {
		c := less(ds[i], ds[j])
		if c != 0 {
			if s.Desc {
				return c > 0
			}
			return c < 0
		}
		return model.NaturalLess(ds[i].g.Key.ID, ds[j].g.Key.ID)
	})
}

func cmp64(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// SuggestTags возвращает теги с именем, начинающимся с prefix (без учёта
// регистра), и числом произведений; при заданном типе — только этого типа.
func (m *MemIndex) SuggestTags(_ context.Context, tagType, prefix string, limit int) ([]TagCount, error) {
	prefix = strings.ToLower(strings.TrimSpace(prefix))
	tagType = strings.ToLower(strings.TrimSpace(tagType))
	counts := map[model.Tag]int{}
	m.mu.RLock()
	for _, d := range m.docs {
		for t := range d.tags {
			if (tagType == "" || t.Type == tagType) && strings.HasPrefix(t.Name, prefix) {
				counts[t]++
			}
		}
	}
	m.mu.RUnlock()

	out := make([]TagCount, 0, len(counts))
	for t, n := range counts {
		out = append(out, TagCount{Tag: t, Count: n})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Tag.String() < out[j].Tag.String()
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

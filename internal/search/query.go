package search

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"mangareader/internal/model"
)

// Value — значение фильтра. Используются поля, соответствующие ValueKind поля;
// Num2/Time2 — верхняя граница для OpBetween.
type Value struct {
	Text  string
	Tag   model.Tag
	Num   int64
	Num2  int64
	Time  time.Time
	Time2 time.Time
}

// Filter — условие «поле–операция–значение».
type Filter struct {
	Field Field
	Op    Op
	Value Value
}

func (f Filter) String() string {
	return fmt.Sprintf("%s %s", f.Field, f.Op)
}

// Sort — порядок выдачи.
type Sort struct {
	Field Field
	Desc  bool
}

// Query — запрос поиска. Фильтры объединяются логическим «И».
type Query struct {
	Text    string   // исходная строка запроса
	Terms   []string // слова и фразы свободного текста (нижний регистр)
	Filters []Filter
	Sort    Sort
	Limit   int // 0 — без ограничения
	Offset  int
}

// DefaultSort — сортировка по умолчанию: недавно добавленные сверху.
var DefaultSort = Sort{Field: FieldAdded, Desc: true}

// NewQuery создаёт пустой запрос («все произведения») с сортировкой по умолчанию.
func NewQuery() Query {
	return Query{Sort: DefaultSort}
}

// Validate проверяет запрос и нормализует теги в фильтрах.
func (q *Query) Validate() error {
	if q.Limit < 0 {
		return errors.New("limit must not be negative")
	}
	if q.Offset < 0 {
		return errors.New("offset must not be negative")
	}
	if q.Sort.Field == 0 {
		q.Sort = DefaultSort
	}
	if fi, ok := q.Sort.Field.Info(); !ok || !fi.Sortable {
		return fmt.Errorf("sorting by field %s is not available", q.Sort.Field)
	}
	for i := range q.Filters {
		if r := validateFilter(&q.Filters[i]); r != nil {
			return &FilterError{Index: i, Filter: q.Filters[i], Reason: *r}
		}
	}
	return nil
}

// validateFilter проверяет фильтр; nil — корректен.
func validateFilter(f *Filter) *Reason {
	fi, ok := f.Field.Info()
	if !ok {
		return reason(ReasonUnknownField)
	}
	if !fi.Allows(f.Op) {
		return reason(ReasonOpNotAllowed, f.Op.String(), fi.Name)
	}
	v := &f.Value
	switch fi.Kind {
	case KindText:
		if strings.TrimSpace(v.Text) == "" {
			return reason(ReasonEmptyText)
		}
	case KindTag:
		v.Tag = model.NewTag(v.Tag.Type, v.Tag.Name)
		if v.Tag.Name == "" {
			return reason(ReasonEmptyTag)
		}
		// пустой тип — «тег с таким именем любого типа»
	case KindNumber:
		if v.Num < 0 || (f.Op == OpBetween && v.Num2 < 0) {
			return reason(ReasonNegative)
		}
		if f.Op == OpBetween && v.Num > v.Num2 {
			return reason(ReasonBadRange, v.Num, v.Num2)
		}
	case KindDate:
		if v.Time.IsZero() || (f.Op == OpBetween && v.Time2.IsZero()) {
			return reason(ReasonNoDate)
		}
		if f.Op == OpBetween && v.Time.After(v.Time2) {
			return reason(ReasonBadDateRange)
		}
	}
	return nil
}

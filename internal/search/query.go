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

// FilterError — ошибка валидации конкретного фильтра.
type FilterError struct {
	Index  int
	Filter Filter
	Reason string
}

func (e *FilterError) Error() string {
	return fmt.Sprintf("фильтр #%d (%s): %s", e.Index+1, e.Filter, e.Reason)
}

// Validate проверяет запрос и нормализует теги в фильтрах.
func (q *Query) Validate() error {
	if q.Limit < 0 {
		return errors.New("лимит не может быть отрицательным")
	}
	if q.Offset < 0 {
		return errors.New("смещение не может быть отрицательным")
	}
	if q.Sort.Field == 0 {
		q.Sort = DefaultSort
	}
	if fi, ok := q.Sort.Field.Info(); !ok || !fi.Sortable {
		return fmt.Errorf("сортировка по полю %s недоступна", q.Sort.Field)
	}
	for i := range q.Filters {
		if reason := validateFilter(&q.Filters[i]); reason != "" {
			return &FilterError{Index: i, Filter: q.Filters[i], Reason: reason}
		}
	}
	return nil
}

func validateFilter(f *Filter) string {
	fi, ok := f.Field.Info()
	if !ok {
		return "неизвестное поле"
	}
	if !fi.Allows(f.Op) {
		return fmt.Sprintf("операция «%s» недопустима для поля %s", f.Op, fi.Name)
	}
	v := &f.Value
	switch fi.Kind {
	case KindText:
		if strings.TrimSpace(v.Text) == "" {
			return "пустой текст"
		}
	case KindTag:
		v.Tag = model.NewTag(v.Tag.Type, v.Tag.Name)
		if v.Tag.Name == "" {
			return "пустой тег"
		}
		// пустой тип — «тег с таким именем любого типа»
	case KindNumber:
		if v.Num < 0 || (f.Op == OpBetween && v.Num2 < 0) {
			return "отрицательное значение"
		}
		if f.Op == OpBetween && v.Num > v.Num2 {
			return fmt.Sprintf("некорректный диапазон: %d больше %d", v.Num, v.Num2)
		}
	case KindDate:
		if v.Time.IsZero() || (f.Op == OpBetween && v.Time2.IsZero()) {
			return "не задана дата"
		}
		if f.Op == OpBetween && v.Time.After(v.Time2) {
			return "некорректный диапазон дат: начало позже конца"
		}
	}
	return ""
}

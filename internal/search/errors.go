package search

import "fmt"

// ReasonCode — вид причины ошибки запроса. Текст на языке интерфейса UI
// строит по коду и параметрам; Error() — технический текст на английском.
type ReasonCode string

// Причины ошибок запроса; в скобках — параметры Reason.Args.
const (
	// ReasonNegationTagsOnly — «-» перед словом или полем, которое не тег.
	ReasonNegationTagsOnly ReasonCode = "negation_tags_only"
	// ReasonNoValue — пустое значение: «pages:».
	ReasonNoValue ReasonCode = "no_value"
	// ReasonNotNumber — не неотрицательное целое (значение).
	ReasonNotNumber ReasonCode = "not_number"
	// ReasonIDExactOnly — диапазон или сравнение для id.
	ReasonIDExactOnly ReasonCode = "id_exact_only"
	// ReasonBadSize — размер не в виде 10mb, 1,5gb, 500k (значение).
	ReasonBadSize ReasonCode = "bad_size"
	// ReasonBadDate — дата не в виде ГГГГ, ГГГГ-ММ, ГГГГ-ММ-ДД (значение).
	ReasonBadDate ReasonCode = "bad_date"
	// ReasonUnknownField — поле фильтра неизвестно.
	ReasonUnknownField ReasonCode = "unknown_field"
	// ReasonOpNotAllowed — операция недопустима для поля (операция, поле).
	ReasonOpNotAllowed ReasonCode = "op_not_allowed"
	// ReasonEmptyText — пустой текст фильтра.
	ReasonEmptyText ReasonCode = "empty_text"
	// ReasonEmptyTag — пустое имя тега.
	ReasonEmptyTag ReasonCode = "empty_tag"
	// ReasonNegative — отрицательное число.
	ReasonNegative ReasonCode = "negative"
	// ReasonBadRange — начало диапазона больше конца (начало, конец).
	ReasonBadRange ReasonCode = "bad_range"
	// ReasonNoDate — дата не задана.
	ReasonNoDate ReasonCode = "no_date"
	// ReasonBadDateRange — начало диапазона дат позже конца.
	ReasonBadDateRange ReasonCode = "bad_date_range"
)

// reasonText — технические тексты причин (fmt, параметры — Reason.Args).
var reasonText = map[ReasonCode]string{
	ReasonNegationTagsOnly: "negation is supported only for tags",
	ReasonNoValue:          "no value specified",
	ReasonNotNumber:        "expected a non-negative integer, got %q",
	ReasonIDExactOnly:      "id supports only an exact value",
	ReasonBadSize:          "expected a size like 10mb, 1,5gb or 500k, got %q",
	ReasonBadDate:          "expected a date YYYY, YYYY-MM or YYYY-MM-DD, got %q",
	ReasonUnknownField:     "unknown field",
	ReasonOpNotAllowed:     "operation %q is not allowed for field %s",
	ReasonEmptyText:        "empty text",
	ReasonEmptyTag:         "empty tag",
	ReasonNegative:         "negative value",
	ReasonBadRange:         "invalid range: %d is greater than %d",
	ReasonNoDate:           "no date specified",
	ReasonBadDateRange:     "invalid date range: start is after end",
}

// Reason — причина ошибки запроса: вид и параметры.
type Reason struct {
	Code ReasonCode
	Args []any
}

func reason(code ReasonCode, args ...any) *Reason { return &Reason{Code: code, Args: args} }

// Error — технический текст причины на английском.
func (r *Reason) Error() string {
	if f, ok := reasonText[r.Code]; ok {
		return fmt.Sprintf(f, r.Args...)
	}
	return string(r.Code)
}

// ParseError — ошибка разбора строки запроса: проблемный фрагмент и причина.
type ParseError struct {
	Token  string
	Reason Reason
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("%q: %s", e.Token, e.Reason.Error())
}

// FilterError — ошибка валидации конкретного фильтра.
type FilterError struct {
	Index  int
	Filter Filter
	Reason Reason
}

func (e *FilterError) Error() string {
	return fmt.Sprintf("filter #%d (%s): %s", e.Index+1, e.Filter, e.Reason.Error())
}

package search

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"

	"mangareader/internal/model"
)

// ParseError — ошибка разбора строки запроса: проблемный фрагмент и причина.
type ParseError struct {
	Token  string
	Reason string
}

func (e *ParseError) Error() string {
	return "«" + e.Token + "» — " + e.Reason
}

// parser описывает поле строки запроса.
type fieldParser struct {
	field   Field
	tagType string // для тегов: тип ("" — любой)
	isTag   bool
}

// queryFields — поля, распознаваемые в строке запроса.
var queryFields = map[string]fieldParser{
	"title":     {field: FieldTitle},
	"scanlator": {field: FieldScanlator},
	"tag":       {field: FieldTag, isTag: true},
	"artist":    {field: FieldTag, isTag: true, tagType: model.TagTypeArtist},
	"group":     {field: FieldTag, isTag: true, tagType: model.TagTypeGroup},
	"parody":    {field: FieldTag, isTag: true, tagType: model.TagTypeParody},
	"character": {field: FieldTag, isTag: true, tagType: model.TagTypeCharacter},
	"language":  {field: FieldTag, isTag: true, tagType: model.TagTypeLanguage},
	"category":  {field: FieldTag, isTag: true, tagType: model.TagTypeCategory},
	"id":        {field: FieldID},
	"pages":     {field: FieldPages},
	"favorites": {field: FieldFavorites},
	"size":      {field: FieldSize},
	"uploaded":  {field: FieldUploaded},
	"added":     {field: FieldAdded},
}

// Parse разбирает строку запроса: слова и фразы в кавычках ищутся как часть
// слова во всех полях, элементы «поле:значение» становятся фильтрами.
// Все условия объединяются «И».
func Parse(s string) (Query, error) {
	q := NewQuery()
	q.Text = s
	var filterTokens []string // фрагмент запроса для каждого фильтра
	for _, tok := range tokenize(s) {
		neg := strings.HasPrefix(tok, "-") && len(tok) > 1
		body := tok
		if neg {
			body = tok[1:]
		}
		name, value, isField := splitField(body)
		if !isField {
			if neg {
				return Query{}, &ParseError{tok, "исключение поддерживается только для тегов"}
			}
			if w := strings.ToLower(strings.TrimSpace(unquote(body))); w != "" {
				q.Terms = append(q.Terms, w)
			}
			continue
		}
		fp := queryFields[name]
		if neg && !fp.isTag {
			return Query{}, &ParseError{tok, "исключение поддерживается только для тегов"}
		}
		value = strings.TrimSpace(unquote(value))
		if value == "" {
			return Query{}, &ParseError{tok, "не указано значение"}
		}
		f, err := parseFilter(fp, value, neg)
		if err != nil {
			return Query{}, &ParseError{tok, err.Error()}
		}
		q.Filters = append(q.Filters, f)
		filterTokens = append(filterTokens, tok)
	}
	if err := q.Validate(); err != nil {
		var fe *FilterError
		if errors.As(err, &fe) && fe.Index < len(filterTokens) {
			return Query{}, &ParseError{filterTokens[fe.Index], fe.Reason}
		}
		return Query{}, err
	}
	return q, nil
}

// tokenize делит строку по пробелам вне кавычек. Кавычки сохраняются;
// незакрытая кавычка закрывается в конце строки.
func tokenize(s string) []string {
	var out []string
	var b strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			b.WriteRune(r)
		case unicode.IsSpace(r) && !inQuote:
			if b.Len() > 0 {
				out = append(out, b.String())
				b.Reset()
			}
		default:
			b.WriteRune(r)
		}
	}
	if b.Len() > 0 {
		out = append(out, b.String())
	}
	return out
}

// splitField выделяет «поле:значение», если до первого двоеточия (вне кавычек)
// стоит известное имя поля.
func splitField(tok string) (name, value string, ok bool) {
	if strings.HasPrefix(tok, `"`) {
		return "", "", false
	}
	i := strings.IndexByte(tok, ':')
	if i <= 0 || strings.Contains(tok[:i], `"`) {
		return "", "", false
	}
	name = strings.ToLower(tok[:i])
	if _, known := queryFields[name]; !known {
		return "", "", false
	}
	return name, tok[i+1:], true
}

func unquote(s string) string { return strings.ReplaceAll(s, `"`, "") }

func parseFilter(fp fieldParser, value string, neg bool) (Filter, error) {
	switch {
	case fp.isTag:
		op := OpHas
		if neg {
			op = OpNotHas
		}
		return Filter{Field: FieldTag, Op: op, Value: Value{Tag: model.Tag{Type: fp.tagType, Name: value}}}, nil
	case fp.field == FieldTitle || fp.field == FieldScanlator:
		return Filter{Field: fp.field, Op: OpContains, Value: Value{Text: strings.ToLower(value)}}, nil
	case fp.field == FieldUploaded:
		return parseDate(fp.field, value, time.UTC)
	case fp.field == FieldAdded:
		return parseDate(fp.field, value, time.Local)
	default:
		return parseNumber(fp.field, value)
	}
}

// splitOp отделяет оператор сравнения от значения.
func splitOp(v string) (op, rest string) {
	for _, o := range []string{">=", "<=", ">", "<"} {
		if strings.HasPrefix(v, o) {
			return o, strings.TrimSpace(v[len(o):])
		}
	}
	return "", v
}

func parseNumber(field Field, value string) (Filter, error) {
	num := strconv.ParseInt
	parse := func(s string) (int64, error) {
		if field == FieldSize {
			return parseSize(s)
		}
		n, err := num(strings.TrimSpace(s), 10, 64)
		if err != nil || n < 0 {
			return 0, fmt.Errorf("ожидается неотрицательное целое число, получено %q", s)
		}
		return n, nil
	}
	op, rest := splitOp(value)
	if field == FieldID && (op != "" || strings.Contains(rest, "..")) {
		return Filter{}, errors.New("для id поддерживается только точное значение")
	}
	if op == "" {
		if a, b, ok := strings.Cut(rest, ".."); ok {
			lo, err := parse(a)
			if err != nil {
				return Filter{}, err
			}
			hi, err := parse(b)
			if err != nil {
				return Filter{}, err
			}
			return Filter{Field: field, Op: OpBetween, Value: Value{Num: lo, Num2: hi}}, nil
		}
	}
	n, err := parse(rest)
	if err != nil {
		return Filter{}, err
	}
	switch op {
	case ">":
		return Filter{Field: field, Op: OpGt, Value: Value{Num: n}}, nil
	case "<":
		return Filter{Field: field, Op: OpLt, Value: Value{Num: n}}, nil
	case ">=":
		if n == 0 {
			// «>= 0» — любое значение; нулевые значения всё равно не проходят
			return Filter{Field: field, Op: OpGt, Value: Value{Num: 0}}, nil
		}
		return Filter{Field: field, Op: OpGt, Value: Value{Num: n - 1}}, nil
	case "<=":
		return Filter{Field: field, Op: OpLt, Value: Value{Num: n + 1}}, nil
	}
	if fi, _ := field.Info(); fi.Allows(OpEq) {
		return Filter{Field: field, Op: OpEq, Value: Value{Num: n}}, nil
	}
	return Filter{Field: field, Op: OpBetween, Value: Value{Num: n, Num2: n}}, nil
}

var sizeUnits = []struct {
	suffix string
	mul    float64
}{
	{"gb", 1 << 30}, {"mb", 1 << 20}, {"kb", 1 << 10},
	{"g", 1 << 30}, {"m", 1 << 20}, {"k", 1 << 10}, {"b", 1},
}

// parseSize разбирает размер: число (с «.» или «,») и необязательная единица.
func parseSize(s string) (int64, error) {
	v := strings.ToLower(strings.TrimSpace(s))
	mul := 1.0
	for _, u := range sizeUnits {
		if strings.HasSuffix(v, u.suffix) {
			v, mul = strings.TrimSpace(strings.TrimSuffix(v, u.suffix)), u.mul
			break
		}
	}
	f, err := strconv.ParseFloat(strings.Replace(v, ",", ".", 1), 64)
	if err != nil || f < 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return 0, fmt.Errorf("ожидается размер вида 10mb, 1,5gb или 500k, получено %q", s)
	}
	return int64(math.Round(f * mul)), nil
}

// period разбирает ГГГГ, ГГГГ-ММ или ГГГГ-ММ-ДД в интервал [start, end].
func period(s string, loc *time.Location) (start, end time.Time, err error) {
	s = strings.TrimSpace(s)
	for _, p := range []struct {
		layout string
		next   func(time.Time) time.Time
	}{
		{"2006-01-02", func(t time.Time) time.Time { return t.AddDate(0, 0, 1) }},
		{"2006-01", func(t time.Time) time.Time { return t.AddDate(0, 1, 0) }},
		{"2006", func(t time.Time) time.Time { return t.AddDate(1, 0, 0) }},
	} {
		if len(s) != len(p.layout) {
			continue
		}
		t, perr := time.ParseInLocation(p.layout, s, loc)
		if perr != nil {
			break
		}
		return t, p.next(t).Add(-time.Nanosecond), nil
	}
	return time.Time{}, time.Time{}, fmt.Errorf("ожидается дата ГГГГ, ГГГГ-ММ или ГГГГ-ММ-ДД, получено %q", s)
}

func parseDate(field Field, value string, loc *time.Location) (Filter, error) {
	op, rest := splitOp(value)
	if op == "" {
		if a, b, ok := strings.Cut(rest, ".."); ok {
			lo, _, err := period(a, loc)
			if err != nil {
				return Filter{}, err
			}
			_, hi, err := period(b, loc)
			if err != nil {
				return Filter{}, err
			}
			return Filter{Field: field, Op: OpBetween, Value: Value{Time: lo, Time2: hi}}, nil
		}
	}
	start, end, err := period(rest, loc)
	if err != nil {
		return Filter{}, err
	}
	switch op {
	case ">":
		return Filter{Field: field, Op: OpGt, Value: Value{Time: end}}, nil
	case "<":
		return Filter{Field: field, Op: OpLt, Value: Value{Time: start}}, nil
	case ">=":
		return Filter{Field: field, Op: OpGt, Value: Value{Time: start.Add(-time.Nanosecond)}}, nil
	case "<=":
		return Filter{Field: field, Op: OpLt, Value: Value{Time: end.Add(time.Nanosecond)}}, nil
	}
	return Filter{Field: field, Op: OpBetween, Value: Value{Time: start, Time2: end}}, nil
}

package stats

import (
	"strconv"
	"strings"

	"mangareader/internal/search"
)

// num — число для запроса: целое без дробной части, иначе с точкой.
func num(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// condition — условие поиска строки; "" — строка не нажимается.
func condition(r RowRef) string {
	switch r.Kind {
	case RowTag:
		return search.TagQuery(r.Tag.Type, r.Tag.Name)
	case RowCustom:
		return `custom-tag:"` + strings.ReplaceAll(r.Tag.Name, `"`, "") + `" ` + search.TagQuery(r.Tag.Type, r.Tag.Name)
	case RowHidden:
		return `hidden-tag:"` + strings.ReplaceAll(r.Tag.Name, `"`, "") + `"`
	case RowPages:
		if r.Hi == 0 {
			return "pages:>" + num(r.Lo)
		}
		return "pages:" + num(r.Lo+1) + ".." + num(r.Hi)
	case RowSize:
		switch {
		case r.Hi == 0:
			return "size:>" + num(r.Lo) + "mb"
		case r.Lo == 0:
			return "size:<=" + num(r.Hi) + "mb"
		}
		return "size:>" + num(r.Lo) + "mb size:<=" + num(r.Hi) + "mb"
	case RowUploaded:
		return "uploaded:" + year(r.Year)
	case RowAddedYear:
		return "added:" + year(r.Year)
	case RowAddedMonth:
		m := strconv.Itoa(r.Month)
		if r.Month < 10 {
			m = "0" + m
		}
		return "added:" + year(r.Year) + "-" + m
	case RowQuality:
		return "missing:" + r.Quality
	}
	return ""
}

// year — год в четыре цифры (запрос поиска принимает только такой).
func year(y int) string {
	s := strconv.Itoa(y)
	return strings.Repeat("0", max(0, 4-len(s))) + s
}

// Query — запрос поиска для перехода по строке: условие строки, перед
// которым через пробел стоит текст фильтра статистики, если он не пуст.
// Для строки без перехода («Без даты») возвращает "".
func Query(r RowRef, filter string) string {
	c := condition(r)
	if c == "" {
		return ""
	}
	if f := strings.TrimSpace(filter); f != "" {
		return f + " " + c
	}
	return c
}

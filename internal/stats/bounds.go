package stats

import (
	"regexp"
	"strconv"
	"strings"
)

// Ограничения числа границ корзин.
const (
	MinBounds = 1
	MaxBounds = 15
)

// Причины ошибки границ (BoundsError.Code); экран переводит их на язык интерфейса.
const (
	BoundsNotNumber  = "not_number"  // не положительное число (значение)
	BoundsNotInteger = "not_integer" // для страниц нужны целые (значение)
	BoundsNotRising  = "not_rising"  // не строго по возрастанию (значение)
	BoundsCount      = "count"       // меньше 1 или больше 15 границ
)

// BoundsError — ошибка строки границ: причина и проблемное значение.
type BoundsError struct {
	Code  string
	Value string
}

func (e *BoundsError) Error() string {
	if e.Value != "" {
		return "bounds: " + e.Code + ": " + strconv.Quote(e.Value)
	}
	return "bounds: " + e.Code
}

var boundNumber = regexp.MustCompile(`^[0-9]+(\.[0-9]+)?$`)

// ParseBounds разбирает границы корзин: числа через запятую. Границы
// положительные, строго по возрастанию, от 1 до 15 штук; при integer —
// целые (страницы), иначе допускаются дробные с точкой (мегабайты).
func ParseBounds(s string, integer bool) ([]float64, error) {
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if !boundNumber.MatchString(p) {
			return nil, &BoundsError{Code: BoundsNotNumber, Value: p}
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil || v <= 0 {
			return nil, &BoundsError{Code: BoundsNotNumber, Value: p}
		}
		if integer && v != float64(int64(v)) {
			return nil, &BoundsError{Code: BoundsNotInteger, Value: p}
		}
		if n := len(out); n > 0 && v <= out[n-1] {
			return nil, &BoundsError{Code: BoundsNotRising, Value: p}
		}
		out = append(out, v)
	}
	if len(out) < MinBounds || len(out) > MaxBounds {
		return nil, &BoundsError{Code: BoundsCount}
	}
	return out, nil
}

// FormatBounds записывает границы строкой для поля ввода: через запятую
// с пробелом, дробная часть через точку.
func FormatBounds(b []float64) string {
	parts := make([]string, len(b))
	for i, v := range b {
		parts[i] = strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strings.Join(parts, ", ")
}

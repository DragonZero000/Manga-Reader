package model

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// NaturalLess сравнивает строки в натуральном порядке: числовые фрагменты
// сравниваются как числа, остальное — без учёта регистра.
// "2.jpg" < "10.jpg", "Page2" < "page10".
func NaturalLess(a, b string) bool {
	if c := naturalCompare(strings.ToLower(a), strings.ToLower(b)); c != 0 {
		return c < 0
	}
	return a < b // стабильный порядок для строк, различающихся только регистром
}

func naturalCompare(a, b string) int {
	for a != "" && b != "" {
		ra, _ := utf8.DecodeRuneInString(a)
		rb, _ := utf8.DecodeRuneInString(b)
		if isDigit(ra) && isDigit(rb) {
			na, restA := splitDigits(a)
			nb, restB := splitDigits(b)
			if c := compareNumbers(na, nb); c != 0 {
				return c
			}
			a, b = restA, restB
			continue
		}
		if ra != rb {
			if ra < rb {
				return -1
			}
			return 1
		}
		a = a[utf8.RuneLen(ra):]
		b = b[utf8.RuneLen(rb):]
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

func isDigit(r rune) bool {
	return r < utf8.RuneSelf && unicode.IsDigit(r)
}

func splitDigits(s string) (digits, rest string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

// compareNumbers сравнивает десятичные строки любой длины без переполнения.
// При равном значении меньше та, у которой меньше ведущих нулей.
func compareNumbers(a, b string) int {
	ta := strings.TrimLeft(a, "0")
	tb := strings.TrimLeft(b, "0")
	if len(ta) != len(tb) {
		if len(ta) < len(tb) {
			return -1
		}
		return 1
	}
	if c := strings.Compare(ta, tb); c != 0 {
		return c
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	}
	return 0
}

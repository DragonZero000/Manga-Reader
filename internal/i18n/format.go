package i18n

import (
	"strconv"
	"strings"
	"time"
)

// Правила форматирования берутся из файлов переводов, поэтому новый язык
// не требует изменений кода:
//   - format.date, format.datetime — раскладки Go (time.Format);
//   - number.decimal, number.group — десятичный разделитель и разделитель разрядов;
//   - size.b … size.tb — единицы размера.

// Date — дата в UTC по формату языка (дата загрузки хранится в UTC и не
// должна «съезжать» на соседний день из-за часового пояса).
func Date(t time.Time) string {
	return t.UTC().Format(T("format.date"))
}

// DateTime — местные дата и время по формату языка.
func DateTime(t time.Time) string {
	return t.Local().Format(T("format.datetime"))
}

// Int — целое с разделителем разрядов языка: 12 345 / 12,345.
func Int(n int64) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	group := T("number.group")
	var b strings.Builder
	for i, r := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteString(group)
		}
		b.WriteRune(r)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// Size — размер в Б/КБ/МБ/ГБ/ТБ (основание 1024) с одной цифрой после
// десятичного разделителя языка: «1,5 МБ» / «1.5 MB».
func Size(n int64) string { return SizeDigits(n, 1) }

// SizeDigits — размер как у Size, но с digits цифрами после разделителя:
// «1,27 ГБ» при digits = 2; байты — без дробной части.
func SizeDigits(n int64, digits int) string {
	if n < 1024 {
		return strconv.FormatInt(n, 10) + " " + T("size.b")
	}
	units := []string{"size.kb", "size.mb", "size.gb", "size.tb"}
	v := float64(n) / 1024
	i := 0
	for v >= 1024 && i < len(units)-1 {
		v /= 1024
		i++
	}
	num := strings.Replace(strconv.FormatFloat(v, 'f', digits, 64), ".", T("number.decimal"), 1)
	return num + " " + T(units[i])
}

// MB — размер в мегабайтах (основание 1024) с одной цифрой после
// десятичного разделителя языка: «9,8 МБ» / «9.8 MB»; ноль — «0 МБ».
func MB(n int64) string {
	if n <= 0 {
		return "0 " + T("size.mb")
	}
	num := strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64)
	return strings.Replace(num, ".", T("number.decimal"), 1) + " " + T("size.mb")
}

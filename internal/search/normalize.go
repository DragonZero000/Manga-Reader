package search

import (
	"strings"

	"golang.org/x/text/unicode/norm"
)

// NormVersion — версия правил Normalize. Индексы, сохранённые на диске,
// пересоздаются при её изменении.
const NormVersion = 1

// Normalize приводит текст к виду для сравнения при поиске: совместимая
// декомпозиция Unicode (NFKC: полноширинные «ＡＢＣ» → «ABC», составные
// символы — к одному виду), нижний регистр для любого алфавита, «ё» → «е».
// Идемпотентна: Normalize(Normalize(s)) == Normalize(s).
func Normalize(s string) string {
	return strings.ReplaceAll(strings.ToLower(norm.NFKC.String(s)), "ё", "е")
}

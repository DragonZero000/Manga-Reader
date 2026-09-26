package search

import "testing"

func TestNormalize(t *testing.T) {
	for in, want := range map[string]string{
		"Ёлка":              "елка",   // ё → е, регистр кириллицы
		"ЁЛКА":              "елка",   //
		"School":            "school", // латиница
		"ＡＢＣ１２３":            "abc123", // полноширинные → обычные
		"café":             "café",   // e + комбинируемое ударение → é
		"café":              "café",   // готовый символ — тот же результат
		"Ⅻ":                 "xii",    // римская цифра (совместимая декомпозиция)
		"  пробелы  ":       "  пробелы  ",
		"tag:\"Ёжик Ёжик\"": "tag:\"ежик ежик\"",
	} {
		got := Normalize(in)
		if got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
		if Normalize(got) != got {
			t.Errorf("Normalize не идемпотентна для %q", in)
		}
	}
}

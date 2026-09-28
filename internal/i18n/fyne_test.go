package i18n

import (
	"testing"

	"fyne.io/fyne/v2/lang"
)

// Язык приложения отличается от языка системы — встроенные тексты Fyne на
// языке приложения.
func TestApplyToFyne(t *testing.T) {
	sys := DetectSystemLocale()
	target, want := "ru", "Отмена"
	if baseLanguage(sys) == "ru" {
		target, want = "en", "Cancel"
	}
	use(t, target)
	if err := ApplyToFyne(sys); err != nil {
		t.Fatal(err)
	}
	if got := lang.L("Cancel"); got != want {
		t.Errorf("система %q, язык %s: Cancel = %q, ожидалось %q", sys, target, got, want)
	}
}

// Для каждого языка приложения есть копия переводов Fyne.
func TestFyneTranslationsForEveryLanguage(t *testing.T) {
	for _, c := range codes {
		if _, err := fyneFS.ReadFile("fyne/base." + c + ".json"); err != nil {
			t.Errorf("нет fyne/base.%s.json: %v", c, err)
		}
	}
}

package i18n

import (
	"embed"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"
)

// Копии встроенных переводов Fyne (fyne.io/fyne/v2/lang/translations,
// BSD-3-Clause): base.<язык>.json для каждого языка приложения.
//
//go:embed fyne/*.json
var fyneFS embed.FS

// DetectSystemLocale — основная локаль системы ("ru-RU"): Win32 на Windows,
// JNI на Android (через Fyne).
func DetectSystemLocale() string { return lang.SystemLocale().String() }

// ApplyToFyne переводит встроенные тексты Fyne (кнопки диалогов, меню полей
// ввода) на текущий язык приложения. Fyne выбирает перевод только по языку
// системы, поэтому переводы текущего языка загружаются под тегом системной
// локали systemLocale: go-i18n заменяет сообщения с теми же ключами, и Fyne,
// выбрав «системный» язык, показывает тексты языка приложения. Вызывать
// после Init/Start, до создания окна.
func ApplyToFyne(systemLocale string) error {
	name := "fyne/base." + Lang() + ".json"
	data, err := fyneFS.ReadFile(name)
	if err != nil {
		return fmt.Errorf("fyne translations for %s: %w", Lang(), err)
	}
	if systemLocale == "" {
		systemLocale = Fallback
	}
	return lang.AddTranslationsForLocale(data, fyne.Locale(systemLocale))
}

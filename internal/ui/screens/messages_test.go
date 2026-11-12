package screens

import (
	"errors"
	"fmt"
	"testing"

	"mangareader/internal/app"
	"mangareader/internal/browser"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// useLang задаёт язык интерфейса на время теста (по умолчанию в тестах — ru).
func useLang(t *testing.T, lang string) {
	t.Helper()
	i18n.Init(lang)
	t.Cleanup(func() { i18n.Init("ru") })
}

func TestProblemText(t *testing.T) {
	cases := []struct {
		err    error
		ru, en string
	}{
		{&library.UnsupportedError{Kind: library.KindImage}, "Изображение — поддерживаются только архивы zip, RAR и 7z", "Image - only zip, RAR and 7z archives are supported"},
		{&library.UnsupportedError{Kind: library.KindVideo}, "Видео — поддерживаются только архивы zip, RAR и 7z", "Video - only zip, RAR and 7z archives are supported"},
		{&library.UnsupportedError{Kind: library.KindAudio}, "Аудио — поддерживаются только архивы zip, RAR и 7z", "Audio - only zip, RAR and 7z archives are supported"},
		{&library.UnsupportedError{Kind: library.KindPDF}, "PDF — поддерживаются только архивы zip, RAR и 7z", "PDF - only zip, RAR and 7z archives are supported"},
		{&library.UnsupportedError{Kind: library.KindOtherArchive}, "Архив RAR/7z — не удалось прочитать", "RAR/7z archive - could not be read"},
		{&library.UnsupportedError{Kind: library.KindText}, "Текстовый файл — поддерживаются только архивы zip, RAR и 7z", "Text file - only zip, RAR and 7z archives are supported"},
		{&library.UnsupportedError{Kind: library.KindHTML}, "Веб-страница (HTML) вместо архива — вероятно, сайт вернул страницу с ошибкой или проверкой",
			"Web page (HTML) instead of an archive - the site probably returned an error or a check page"},
		{&library.UnsupportedError{Kind: library.KindUnknown}, "Неподдерживаемый формат", "Unsupported format"},
		{library.ErrEmpty, "Пустой файл", "Empty file"},
		{library.ErrNoImages, "нет изображений", "no images"},
		{fmt.Errorf("%w: zip: not a valid zip file", library.ErrNotZip), "не архив или архив повреждён", "not a valid archive or the archive is damaged"},
		{storage.ErrBusy, "Файл занят другой программой", "The file is in use by another program"},
		{errors.New("could not open: access denied"), "Не удалось прочитать файл: could not open: access denied", "Could not read the file: could not open: access denied"},
	}
	for _, c := range cases {
		if got := ProblemText(c.err); got != c.ru {
			t.Errorf("ru %v: %q, ожидалось %q", c.err, got, c.ru)
		}
	}
	useLang(t, "en")
	for _, c := range cases {
		if got := ProblemText(c.err); got != c.en {
			t.Errorf("en %v: %q, ожидалось %q", c.err, got, c.en)
		}
	}
}

// Причина, восстановленная из каталога, переводится по виду.
func TestProblemTextRestored(t *testing.T) {
	useLang(t, "en")
	err := library.RestoreError(library.ErrorKind(&library.UnsupportedError{Kind: library.KindImage}), "image - only zip, RAR and 7z archives are supported")
	if got := ProblemText(err); got != "Image - only zip, RAR and 7z archives are supported" {
		t.Errorf("восстановленная причина: %q", got)
	}
}

func TestErrorText(t *testing.T) {
	_, parseErr := search.Parse("uploaded:2024-13")
	cases := []struct {
		err    error
		ru, en string
	}{
		{parseErr, "«uploaded:2024-13» — ожидается дата ГГГГ, ГГГГ-ММ или ГГГГ-ММ-ДД, получено «2024-13»",
			"“uploaded:2024-13” - expected a date YYYY, YYYY-MM or YYYY-MM-DD, got “2024-13”"},
		{&browser.NotFoundError{Path: `C:\app\browser\firefox\firefox.exe`}, `Браузер не найден: C:\app\browser\firefox\firefox.exe`,
			`Browser not found: C:\app\browser\firefox\firefox.exe`},
		{fmt.Errorf("%w: no permission for the folder", storage.ErrUnavailable), "Папка библиотеки недоступна", "The library folder is unavailable"},
		{storage.ErrBusy, "Файл занят другой программой", "The file is in use by another program"},
		{browser.ErrUnsupported, "Встроенный браузер на этой платформе недоступен", "The built-in browser is unavailable on this platform"},
		{mobilebrowser.ErrUnsupported, "Встроенный браузер на этой платформе недоступен", "The built-in browser is unavailable on this platform"},
		{app.ErrChooseNotSupported, "Выбор папки не поддерживается", "Folder selection is not supported"},
		{app.ErrTagExists, "Такой тег уже есть", "This tag is already there"},
		{app.ErrTagInvalid, `Имя тега: от 1 до 100 символов, без кавычек "`, `Tag name: 1 to 100 characters, no " quotes`},
		{app.ErrUserDataUnavailable, "Пользовательские данные недоступны", "User data is unavailable"},
		{library.ErrEmpty, "Пустой файл", "Empty file"},
		{errors.New("open x: The system cannot find the file"), "open x: The system cannot find the file", "open x: The system cannot find the file"},
	}
	for _, c := range cases {
		if got := ErrorText(c.err); got != c.ru {
			t.Errorf("ru %v: %q, ожидалось %q", c.err, got, c.ru)
		}
	}
	useLang(t, "en")
	for _, c := range cases {
		if got := ErrorText(c.err); got != c.en {
			t.Errorf("en %v: %q, ожидалось %q", c.err, got, c.en)
		}
	}
}

// Все причины ошибок запроса переведены на оба языка.
func TestReasonTextAllCodes(t *testing.T) {
	codes := []search.ReasonCode{
		search.ReasonNegationTagsOnly, search.ReasonNoValue, search.ReasonNotNumber, search.ReasonIDExactOnly,
		search.ReasonBadSize, search.ReasonBadDate, search.ReasonUnknownField, search.ReasonOpNotAllowed,
		search.ReasonEmptyText, search.ReasonEmptyTag, search.ReasonNegative, search.ReasonBadRange,
		search.ReasonNoDate, search.ReasonBadDateRange, search.ReasonUnknownValue,
	}
	for _, lang := range []string{"ru", "en"} {
		useLang(t, lang)
		for _, c := range codes {
			key := "search.reason." + string(c)
			if got := reasonText(search.Reason{Code: c, Args: []any{"x", "y"}}); got == key || got == "" {
				t.Errorf("%s: нет перевода %s", lang, key)
			}
		}
	}
}

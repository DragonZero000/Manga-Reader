package screens

import (
	"errors"
	"io/fs"

	"mangareader/internal/app"
	"mangareader/internal/browser"
	"mangareader/internal/i18n"
	"mangareader/internal/library"
	"mangareader/internal/mobilebrowser"
	"mangareader/internal/search"
	"mangareader/internal/storage"
)

// ErrorText — текст ошибки на языке интерфейса. Сервисные пакеты не знают
// языка: их ошибки несут вид и параметры, а err.Error() — технический текст
// на английском (он пишется в журнал). Известные виды переводятся здесь,
// остальные (например, ошибки ОС) показываются как есть — вызывающий код
// добавляет переведённое пояснение.
func ErrorText(err error) string {
	if err == nil {
		return ""
	}
	var (
		pe *search.ParseError
		nf *browser.NotFoundError
	)
	switch {
	case errors.As(err, &pe):
		return i18n.T("search.parse_error", "Token", pe.Token, "Reason", reasonText(pe.Reason))
	case errors.As(err, &nf):
		return i18n.T("error.browser_not_found", "Path", nf.Path)
	case errors.Is(err, storage.ErrBusy):
		return i18n.T("error.busy")
	case errors.Is(err, storage.ErrUnavailable):
		return i18n.T("error.storage_unavailable")
	case errors.Is(err, browser.ErrUnsupported), errors.Is(err, mobilebrowser.ErrUnsupported):
		return i18n.T("error.browser_unsupported")
	case errors.Is(err, app.ErrChooseNotSupported):
		return i18n.T("error.choose_unsupported")
	case errors.Is(err, app.ErrTagExists):
		return i18n.T("error.tag_exists")
	case errors.Is(err, app.ErrTagInvalid):
		return i18n.T("error.tag_invalid", "Max", app.MaxTagName)
	case errors.Is(err, app.ErrUserDataUnavailable):
		return i18n.T("error.userdata_unavailable")
	case errors.Is(err, storage.ErrUnsupported):
		return i18n.T("error.unsupported")
	case errors.Is(err, storage.ErrNoWriteAccess):
		return i18n.T("error.no_write_access")
	case errors.Is(err, storage.ErrNoTrash):
		return i18n.T("error.no_trash")
	case errors.Is(err, fs.ErrNotExist):
		return i18n.T("error.not_found")
	}
	if s, ok := problemKind(err); ok {
		return s
	}
	return err.Error()
}

// ProblemText — причина ошибочного файла библиотеки на языке интерфейса;
// причина без известного вида — с пояснением «Не удалось прочитать файл».
func ProblemText(err error) string {
	if s, ok := problemKind(err); ok {
		return s
	}
	if errors.Is(err, storage.ErrBusy) {
		return i18n.T("error.busy")
	}
	return i18n.T("problem.other", "Error", err.Error())
}

// problemKind переводит причины, которые определяет сканер.
func problemKind(err error) (string, bool) {
	var ue *library.UnsupportedError
	switch {
	case errors.As(err, &ue):
		switch ue.Kind {
		case library.KindImage:
			return i18n.T("problem.image"), true
		case library.KindVideo:
			return i18n.T("problem.video"), true
		case library.KindAudio:
			return i18n.T("problem.audio"), true
		case library.KindPDF:
			return i18n.T("problem.pdf"), true
		case library.KindOtherArchive:
			return i18n.T("problem.other_archive"), true
		case library.KindText:
			return i18n.T("problem.text"), true
		case library.KindHTML:
			return i18n.T("problem.html"), true
		case library.KindZip:
			return i18n.T("problem.zip_ext", "Ext", ue.Ext), true
		}
		return i18n.T("problem.unknown"), true
	case errors.Is(err, library.ErrEmpty):
		return i18n.T("problem.empty"), true
	case errors.Is(err, library.ErrNoImages):
		return i18n.T("problem.no_images"), true
	case errors.Is(err, library.ErrNotZip):
		return i18n.T("problem.not_zip"), true
	}
	return "", false
}

// reasonText — причина ошибки запроса на языке интерфейса; параметры
// причины подставляются как {{.A}}, {{.B}}.
func reasonText(r search.Reason) string {
	var kv []any
	for i, a := range r.Args {
		if i < 2 {
			kv = append(kv, string(rune('A'+i)), a)
		}
	}
	return i18n.T("search.reason."+string(r.Code), kv...)
}

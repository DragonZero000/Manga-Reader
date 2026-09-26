package library

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"mangareader/internal/model"
)

// ScanStore — хранилище результатов сканирования между запусками (каталог
// библиотеки). Сканер берёт из него прежние результаты и сохраняет изменения,
// поэтому неизменённые файлы не открываются и после перезапуска.
type ScanStore interface {
	// Load — все сохранённые результаты по пути файла.
	Load() (map[string]StoredEntry, error)
	// Save сохраняет изменения одного сканирования (одна транзакция).
	Save(d ScanDelta) error
	// StoredLink — сохранённая ссылка скачанного файла («» — нет).
	StoredLink(rel string) string
}

// StoredEntry — результат разбора файла: галерея или ошибка.
type StoredEntry struct {
	Size    int64
	ModTime time.Time
	Gallery model.Gallery // при Err == nil
	Err     error
}

// ScanDelta — изменения за одно сканирование.
type ScanDelta struct {
	Put    map[string]StoredEntry // новые и изменённые файлы, в том числе ошибки
	Delete []string               // файлов больше нет
	// Links — ссылки скачанных файлов, полученные при разборе от хранилища
	// (links.json на Android, метка загрузки на Windows): вторая копия.
	Links map[string]string
}

// Empty сообщает, нет ли изменений.
func (d ScanDelta) Empty() bool { return len(d.Put) == 0 && len(d.Delete) == 0 && len(d.Links) == 0 }

func newDelta() ScanDelta {
	return ScanDelta{Put: map[string]StoredEntry{}, Links: map[string]string{}}
}

// Виды сохраняемых ошибок: по ним ошибка восстанавливается с тем же текстом
// и тем же поведением errors.Is / errors.As.
const (
	errKindEmpty       = "empty"
	errKindNoImages    = "no_images"
	errKindNotZip      = "not_zip"
	errKindUnsupported = "unsupported" // unsupported:<Kind>:<Ext>
)

// ErrorKind — вид ошибки разбора для сохранения («» — только текст).
func ErrorKind(err error) string {
	var ue *UnsupportedError
	switch {
	case errors.As(err, &ue):
		return fmt.Sprintf("%s:%d:%s", errKindUnsupported, ue.Kind, ue.Ext)
	case errors.Is(err, ErrEmpty):
		return errKindEmpty
	case errors.Is(err, ErrNoImages):
		return errKindNoImages
	case errors.Is(err, ErrNotZip):
		return errKindNotZip
	}
	return ""
}

// RestoreError восстанавливает ошибку по виду и тексту.
func RestoreError(kind, text string) error {
	if rest, ok := strings.CutPrefix(kind, errKindUnsupported+":"); ok {
		k, ext, _ := strings.Cut(rest, ":")
		if n, err := strconv.Atoi(k); err == nil {
			return &UnsupportedError{Kind: Kind(n), Ext: ext}
		}
	}
	var base error
	switch kind {
	case errKindEmpty:
		base = ErrEmpty
	case errKindNoImages:
		base = ErrNoImages
	case errKindNotZip:
		base = ErrNotZip
	}
	if base != nil && text == base.Error() {
		return base
	}
	return &storedError{text: text, base: base}
}

// storedError — восстановленная ошибка: прежний текст, errors.Is по виду.
type storedError struct {
	text string
	base error
}

func (e *storedError) Error() string { return e.text }
func (e *storedError) Unwrap() error { return e.base }

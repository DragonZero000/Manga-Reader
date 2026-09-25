package library

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"
)

// ErrEmpty — файл нулевого размера.
var ErrEmpty = errors.New("Пустой файл")

// Kind — тип файла, определённый по содержимому.
type Kind int

const (
	KindUnknown Kind = iota
	KindZip
	KindImage
	KindVideo
	KindAudio
	KindHTML
	KindPDF
	KindOtherArchive
	KindText
)

// UnsupportedError — файл не является галереей; тип определён по содержимому.
type UnsupportedError struct {
	Kind Kind
	Ext  string // расширение файла (для KindZip)
}

const onlyZip = " — поддерживаются только zip-архивы"

func (e *UnsupportedError) Error() string {
	switch e.Kind {
	case KindImage:
		return "Изображение" + onlyZip
	case KindVideo:
		return "Видео" + onlyZip
	case KindAudio:
		return "Аудио" + onlyZip
	case KindPDF:
		return "PDF" + onlyZip
	case KindOtherArchive:
		return "Архив RAR/7z" + onlyZip
	case KindText:
		return "Текстовый файл" + onlyZip
	case KindHTML:
		return "Веб-страница (HTML) вместо архива — вероятно, сайт вернул страницу с ошибкой или проверкой"
	case KindZip:
		return "Zip-архив с расширением «" + e.Ext + "» — поддерживаются только файлы .zip"
	}
	return "Неподдерживаемый формат"
}

// sniffLen — сколько первых байт читается для определения типа.
const sniffLen = 512

// Sniff определяет тип файла по первым байтам.
func Sniff(r io.ReaderAt) Kind {
	buf := make([]byte, sniffLen)
	n, err := r.ReadAt(buf, 0)
	if err != nil && !errors.Is(err, io.EOF) && n == 0 {
		return KindUnknown
	}
	return sniffBytes(buf[:n])
}

var (
	sig7z       = []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c}
	sigRar      = []byte("Rar!\x1a\x07")
	sigMatroska = []byte{0x1a, 0x45, 0xdf, 0xa3}
)

// Бренды ISO BMFF, которые означают изображение (AVIF, HEIF), а не видео.
var imageBrands = map[string]bool{"avif": true, "avis": true, "heic": true, "heix": true, "mif1": true, "msf1": true}

func sniffBytes(b []byte) Kind {
	if len(b) == 0 {
		return KindUnknown
	}
	switch {
	case bytes.HasPrefix(b, sig7z), bytes.HasPrefix(b, sigRar):
		return KindOtherArchive
	case bytes.HasPrefix(b, sigMatroska):
		return KindVideo
	case len(b) >= 12 && string(b[4:8]) == "ftyp":
		if imageBrands[string(b[8:12])] {
			return KindImage
		}
		return KindVideo
	}
	ct := http.DetectContentType(b)
	if i := strings.IndexByte(ct, ';'); i >= 0 {
		ct = ct[:i]
	}
	switch {
	case ct == "application/zip":
		return KindZip
	case strings.HasPrefix(ct, "image/"):
		return KindImage
	case strings.HasPrefix(ct, "video/"):
		return KindVideo
	case strings.HasPrefix(ct, "audio/"), ct == "application/ogg":
		return KindAudio
	case ct == "text/html":
		return KindHTML
	case ct == "application/pdf":
		return KindPDF
	case ct == "application/x-rar-compressed":
		return KindOtherArchive
	case strings.HasPrefix(ct, "text/"):
		return KindText
	}
	return KindUnknown
}

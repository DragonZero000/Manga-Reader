package library

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"testing"

	"mangareader/internal/storage"
)

func jpegBytes(t testing.TB) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// ftyp — начало файла ISO BMFF с заданным брендом.
func ftyp(brand string) []byte {
	return append([]byte{0, 0, 0, 0x18, 'f', 't', 'y', 'p'}, []byte(brand+"\x00\x00\x02\x00")...)
}

func TestSniffBytes(t *testing.T) {
	var zipBuf bytes.Buffer
	zipBuf.Write([]byte("PK\x03\x04"))
	zipBuf.Write(make([]byte, 30))

	tests := []struct {
		name string
		data []byte
		want Kind
	}{
		{"jpeg", jpegBytes(t), KindImage},
		{"png", pngBytes(t, 2, 2), KindImage},
		{"gif", []byte("GIF89a\x01\x00\x01\x00"), KindImage},
		{"webp", []byte("RIFF\x00\x00\x00\x00WEBPVP8 "), KindImage},
		{"avif", ftyp("avif"), KindImage},
		{"mp4", ftyp("isom"), KindVideo},
		{"mov", ftyp("qt  "), KindVideo},
		{"mkv", []byte{0x1a, 0x45, 0xdf, 0xa3, 0x01, 0x00}, KindVideo},
		{"mp3", []byte("ID3\x03\x00\x00\x00\x00\x00\x00"), KindAudio},
		{"html", []byte("<!DOCTYPE html><html><body>captcha</body></html>"), KindHTML},
		{"html без doctype", []byte("\n  <html lang=\"en\"><head>"), KindHTML},
		{"pdf", []byte("%PDF-1.7\n"), KindPDF},
		{"rar", []byte("Rar!\x1a\x07\x01\x00"), KindOtherArchive},
		{"7z", []byte{'7', 'z', 0xbc, 0xaf, 0x27, 0x1c, 0, 4}, KindOtherArchive},
		{"text", []byte("просто заметки"), KindText},
		{"json", []byte(`{"id": 1}`), KindText},
		{"zip", zipBuf.Bytes(), KindZip},
		{"binary", []byte{0x00, 0x01, 0x02, 0xff, 0xfe}, KindUnknown},
		{"empty", nil, KindUnknown},
	}
	for _, tt := range tests {
		if got := sniffBytes(tt.data); got != tt.want {
			t.Errorf("%s: %v, ожидалось %v", tt.name, got, tt.want)
		}
	}
}

func scanErr(t *testing.T, dir, rel string) error {
	t.Helper()
	res, err := NewScanner(storage.NewFS(dir)).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range res.Errors {
		if e.RelPath == rel {
			return e.Err
		}
	}
	t.Fatalf("нет ошибки для %s: %v", rel, res.Errors)
	return nil
}

func TestScanReasons(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "g-2.zip", jpegBytes(t))
	writeFile(t, dir, "g-3.zip", []byte("<!DOCTYPE html><html><body>Проверка браузера</body></html>"))
	writeFile(t, dir, "clip.mp4", ftyp("isom"))
	writeFile(t, dir, "empty.zip", nil)
	writeFile(t, dir, "book.cbz", func() []byte {
		p := writeZip(t, t.TempDir(), "x.zip", file{"1.png", pngBytes(t, 2, 2)})
		data, _ := os.ReadFile(p)
		return data
	}())
	// обрезанный корректный архив
	full, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "part.zip", full[:len(full)/2])

	tests := []struct{ rel, want string }{
		{"g-2.zip", "Изображение — поддерживаются только zip-архивы"},
		{"g-3.zip", "Веб-страница (HTML) вместо архива — вероятно, сайт вернул страницу с ошибкой или проверкой"},
		{"clip.mp4", "Видео — поддерживаются только zip-архивы"},
		{"empty.zip", "Пустой файл"},
		{"book.cbz", "Zip-архив с расширением «.cbz» — поддерживаются только файлы .zip"},
		{"part.zip", ErrNotZip.Error()},
	}
	for _, tt := range tests {
		err := scanErr(t, dir, tt.rel)
		got := err.Error()
		if tt.rel == "part.zip" {
			if !errors.Is(err, ErrNotZip) {
				t.Errorf("part.zip: %v", err)
			}
			continue
		}
		if got != tt.want {
			t.Errorf("%s: %q, ожидалось %q", tt.rel, got, tt.want)
		}
	}
}

func TestScanSkipsDownloads(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "g-1.zip", nil)
	writeFile(t, dir, "g-1.zip.part", []byte("PK\x03\x04 половина"))
	writeFile(t, dir, "other.PART", []byte("x"))
	opens := countOpens(t)

	s := NewScanner(storage.NewFS(dir))
	res, err := s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Galleries)+len(res.Errors)+len(res.NewErrors) != 0 || opens.Load() != 0 {
		t.Fatalf("идёт загрузка: galleries=%v errors=%v opens=%d", keys(res.Galleries), res.Errors, opens.Load())
	}

	// загрузка завершилась: .part переименован в итоговый файл
	if err := os.Remove(filepath.Join(dir, "g-1.zip.part")); err != nil {
		t.Fatal(err)
	}
	writeZip(t, dir, "g-1.zip", file{"1.png", pngBytes(t, 2, 2)})
	res, err = s.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := keys(res.Galleries); len(got) != 1 || got[0] != "local:g-1.zip" || len(res.Errors) != 0 {
		t.Fatalf("после загрузки: %v, errors=%v", got, res.Errors)
	}
}

func TestScanEmptyWithoutPartIsError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "empty.zip", nil)
	if err := scanErr(t, dir, "empty.zip"); !errors.Is(err, ErrEmpty) {
		t.Fatalf("empty.zip: %v", err)
	}
}

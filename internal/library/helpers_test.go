package library

import (
	"archive/zip"
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"mangareader/internal/model"
	"mangareader/internal/storage"
)

// pngBytes возвращает корректное PNG-изображение w×h.
func pngBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// file — элемент архива в порядке записи.
type file struct {
	name string
	data []byte
}

// writeZip создаёт архив dir/rel с файлами в заданном порядке.
func writeZip(t testing.TB, dir, rel string, files ...file) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range files {
		w, err := zw.Create(f.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(f.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func writeFile(t testing.TB, dir, rel string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// setMtime выставляет время изменения файла.
func setMtime(t testing.TB, p string, tm time.Time) {
	t.Helper()
	if err := os.Chtimes(p, tm, tm); err != nil {
		t.Fatal(err)
	}
}

// copyExample копирует testdata/example.zip в dir/rel.
func copyExample(t testing.TB, dir, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "example.zip"))
	if err != nil {
		t.Fatal(err)
	}
	return writeFile(t, dir, rel, data)
}

func stat(t testing.TB, p string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	return info
}

// readArchivePath открывает архив по пути и разбирает его, как сканер.
func readArchivePath(t testing.TB, p, rel string, info os.FileInfo) (model.Gallery, []string, error) {
	t.Helper()
	st := storage.NewFS(filepath.Dir(p))
	e := storage.Entry{RelPath: filepath.Base(p), Size: info.Size(), ModTime: info.ModTime()}
	f, err := st.Open(e.RelPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	g, w, err := ReadArchive(f, rel, e, p)
	return g, w, err
}

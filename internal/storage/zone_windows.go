package storage

import (
	"io"
	"os"
	"strings"

	"mangareader/internal/model"
)

// sourceStream — NTFS-поток с адресом страницы, с которой скачан файл;
// записывается приложением по сообщению встроенного браузера.
const sourceStream = ":mangareader.source"

// SourceURL — адрес страницы, с которой скачан файл: из потока
// :mangareader.source (записан встроенным браузером), иначе ReferrerUrl из
// метки загрузки Zone.Identifier (ставят Firefox, Chrome, Edge; сайты часто
// обрезают его до адреса сайта). «» — адреса нет или он не http(s).
func (s *FS) SourceURL(relPath string) string {
	p, err := s.path(relPath)
	if err != nil {
		return ""
	}
	if data := readStream(p + sourceStream); data != nil {
		if u := model.WebURL(strings.SplitN(string(data), "\n", 2)[0]); u != "" {
			return u
		}
	}
	return referrerFromZone(readStream(p + ":Zone.Identifier"))
}

// WriteSourceURL записывает к файлу path адрес страницы, с которой он скачан.
// Поток переезжает вместе с файлом при переименовании и перемещении по NTFS.
func WriteSourceURL(path, url string) error {
	return os.WriteFile(path+sourceStream, []byte(url+"\n"), 0o644)
}

func readStream(p string) []byte {
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxZoneSize))
	if err != nil {
		return nil
	}
	return data
}

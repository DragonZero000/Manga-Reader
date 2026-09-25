package storage

import (
	"bufio"
	"bytes"
	"strings"

	"mangareader/internal/model"
)

// maxZoneSize ограничивает размер читаемой метки загрузки.
const maxZoneSize = 4 << 10

// referrerFromZone достаёт адрес страницы загрузки (ReferrerUrl, только
// http/https) из содержимого метки Zone.Identifier.
func referrerFromZone(data []byte) string {
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		k, v, ok := strings.Cut(strings.TrimSpace(sc.Text()), "=")
		if ok && strings.EqualFold(strings.TrimSpace(k), "ReferrerUrl") {
			return model.WebURL(v)
		}
	}
	return ""
}

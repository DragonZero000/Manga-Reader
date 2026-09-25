package model

import (
	"net/url"
	"strings"
)

// WebURL возвращает s без пробелов по краям, если это абсолютный адрес
// http или https с хостом, иначе «».
func WebURL(s string) string {
	s = strings.TrimSpace(s)
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		return s
	}
	return ""
}

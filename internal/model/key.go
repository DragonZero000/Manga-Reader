package model

import (
	"errors"
	"fmt"
	"strings"
)

// SourceLocal — источник «локальная папка библиотеки».
const SourceLocal = "local"

// Key — уникальный ключ произведения: "<источник>:<идентификатор>".
type Key struct {
	Source string
	ID     string
}

// LocalKey создаёт ключ для архива из папки библиотеки
// (relPath — путь относительно папки, с прямыми слешами).
func LocalKey(relPath string) Key {
	return Key{Source: SourceLocal, ID: relPath}
}

// ParseKey разбирает строку ключа.
func ParseKey(s string) (Key, error) {
	src, id, ok := strings.Cut(s, ":")
	if !ok {
		return Key{}, fmt.Errorf("ключ %q: нет разделителя ':'", s)
	}
	if src == "" {
		return Key{}, errors.New("ключ: пустой источник")
	}
	if id == "" {
		return Key{}, errors.New("ключ: пустой идентификатор")
	}
	return Key{Source: src, ID: id}, nil
}

func (k Key) String() string {
	return k.Source + ":" + k.ID
}

// IsZero сообщает, пуст ли ключ.
func (k Key) IsZero() bool {
	return k.Source == "" && k.ID == ""
}

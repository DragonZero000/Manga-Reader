// Package storage — доступ к файлам папки библиотеки и настройки приложения.
// На ПК — обычная файловая система, на Android — папка, выбранная
// пользователем через Storage Access Framework. Пакет не зависит от виджетов.
package storage

import (
	"context"
	"errors"
	"io"
	"time"
)

// ErrUnavailable — папка библиотеки недоступна: не выбрана, нет разрешения,
// удалена или её не удалось создать.
var ErrUnavailable = errors.New("хранилище недоступно")

// ErrBusy — файл временно занят другим процессом (антивирус, незавершённая
// запись). Открытие стоит повторить позже.
var ErrBusy = errors.New("Файл занят другой программой")

// Entry — элемент папки библиотеки.
type Entry struct {
	RelPath string // относительный путь с прямыми слешами
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// File — открытый для чтения файл с произвольным доступом.
type File interface {
	io.ReaderAt
	io.Closer
	Size() int64
}

// Storage — папка библиотеки.
type Storage interface {
	// Name — папка для показа пользователю: путь (ПК) или «Download/manga» (Android).
	Name() string
	// Walk перечисляет все элементы папки рекурсивно. Ошибка доступа к самой
	// папке оборачивает ErrUnavailable.
	Walk(ctx context.Context, fn func(Entry) error) error
	// Open открывает файл по относительному пути для чтения.
	Open(relPath string) (File, error)
}

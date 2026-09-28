package storage

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
)

// ErrUnsupported — действие недоступно на этой платформе или для этого хранилища.
var ErrUnsupported = errors.New("not supported on this platform")

// ErrNoWriteAccess — у приложения нет доступа на запись к папке библиотеки
// (Android: папка выбрана с доступом только на чтение).
var ErrNoWriteAccess = errors.New("no write access to the library folder")

// ErrNoTrash — корзины на диске нет: файл удаляется только безвозвратно, по
// явному запросу.
var ErrNoTrash = errors.New("the Recycle Bin is not available for this drive")

// Deleter — хранилище, которое умеет удалять файлы. Необязательный интерфейс:
// хранилища без удаления (тестовые, будущие) его не реализуют. Реализации:
// FS на Windows (корзина), SAF на Android (безвозвратно); на прочих
// платформах удаления нет (CanDelete = false, см. delete_other.go).
type Deleter interface {
	// CanTrash сообщает, попадёт ли файл relPath в корзину (удаление обратимо).
	// Может обращаться к диску (сетевой диск) — не вызывать из UI-потока.
	CanTrash(relPath string) (bool, error)
	// Delete удаляет файл relPath: в корзину или, если permanent, безвозвратно.
	// Занятый другим процессом файл даёт ErrBusy. Если корзины нет, а
	// permanent не задан, файл не удаляется (ErrNoTrash).
	Delete(relPath string, permanent bool) error
}

// DeleterOf возвращает удаление хранилища st, заглядывая в обёртки
// (метод Unwrap); false — удаление не поддерживается.
func DeleterOf(st Storage) (Deleter, bool) {
	for st != nil {
		if d, ok := st.(Deleter); ok {
			return d, true
		}
		u, ok := st.(interface{ Unwrap() Storage })
		if !ok {
			break
		}
		st = u.Unwrap()
	}
	return nil, false
}

// cleanRel проверяет относительный путь внутри папки библиотеки: только
// очищенный путь с прямыми слешами, без «..», не абсолютный и без имени диска.
func cleanRel(relPath string) (string, error) {
	clean := path.Clean("/" + relPath)[1:]
	if clean == "" || clean != relPath || !filepath.IsLocal(filepath.FromSlash(clean)) {
		return "", fmt.Errorf("invalid path %q", relPath)
	}
	return clean, nil
}

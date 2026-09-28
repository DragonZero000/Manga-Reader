//go:build !windows && !android

package storage

// Удаление файлов библиотеки на прочих платформах не поддерживается: FS не
// реализует Deleter, library.Source.Delete возвращает ErrUnsupported, пункт
// «Удалить…» не показывается.
const (
	CanDelete = false
	HasTrash  = false
)

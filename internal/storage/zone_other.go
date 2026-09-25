//go:build !windows

package storage

import "errors"

// WriteSourceURL: альтернативные потоки файлов есть только в NTFS (Windows).
func WriteSourceURL(path, url string) error {
	return errors.New("адрес страницы хранится только на Windows")
}

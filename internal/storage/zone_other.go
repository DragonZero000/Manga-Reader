//go:build !windows

package storage

import "errors"

// WriteSourceURL: альтернативные потоки файлов есть только в NTFS (Windows).
func WriteSourceURL(path, url string) error {
	return errors.New("the page address is stored only on Windows")
}

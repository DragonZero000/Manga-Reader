//go:build !windows

package storage

// CanReveal — «Показать в папке» поддерживается на этой платформе.
const CanReveal = false

// Reveal вне Windows не поддерживается.
func Reveal(root, relPath string) error { return ErrUnsupported }

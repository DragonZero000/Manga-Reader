package storage

import (
	"errors"
	"syscall"
)

// Коды Windows: ERROR_SHARING_VIOLATION и ERROR_LOCK_VIOLATION.
const (
	errSharingViolation syscall.Errno = 32
	errLockViolation    syscall.Errno = 33
)

// isBusy — файл занят другим процессом.
func isBusy(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == errSharingViolation || errno == errLockViolation)
}

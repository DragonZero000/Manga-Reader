//go:build !windows

package storage

// isBusy: вне Windows открытие для чтения не блокируется другими процессами.
func isBusy(error) bool { return false }

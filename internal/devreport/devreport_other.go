//go:build !android

// Package devreport сохраняет отчёт тестов производительности dev-сборки и
// отдаёт его в системное меню «Поделиться» (Android: Kotlin DevReport через
// JNI; остальные платформы — файл в папке, без «Поделиться»).
package devreport

import (
	"errors"
	"os"
	"path/filepath"
)

// CanShare — системного меню «Поделиться» на этой платформе нет.
const CanShare = false

// DirName — папка отчётов в папке приложения.
const DirName = "perf"

// ErrUnsupported — «Поделиться» есть только на Android.
var ErrUnsupported = errors.New("sharing is available only on Android")

// Save сохраняет text в файл name папки DirName внутри dir (папка
// приложения).
func Save(dir, name, text string) error {
	d := filepath.Join(dir, DirName)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, name), []byte(text), 0o644)
}

// Share: «Поделиться» есть только на Android.
func Share(subject, text string) error { return ErrUnsupported }

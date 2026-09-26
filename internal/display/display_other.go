//go:build !android

// Package display управляет частотой экрана окна приложения на Android
// (Kotlin DisplayRate через JNI). На остальных платформах — заглушка.
package display

import "errors"

// Supported — ограничение частоты экрана на этой платформе недоступно.
const Supported = false

// ErrUnsupported — функция есть только на Android.
var ErrUnsupported = errors.New("частота экрана настраивается только на Android")

// SetMax60: частота экрана настраивается только на Android.
func SetMax60(bool) error { return ErrUnsupported }

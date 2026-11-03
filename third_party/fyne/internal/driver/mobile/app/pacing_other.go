//go:build !android

// mangareader: темп кадров вне Android (third_party/fyne/PATCHES.md).

package app

import "time"

// Vsync: кадровых колбэков нет, драйвер рисует по таймеру.
func Vsync() <-chan time.Time { return nil }

// RequestVsync: кадровых колбэков нет.
func RequestVsync() {}

// SetFrameRate: частоту окна задать нельзя.
func SetFrameRate(float32) {}

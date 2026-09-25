//go:build !windows

package browser

const supported = false

func findWindows(string) []uintptr { return nil }
func attach(uintptr, uintptr)      {}
func foreground(uintptr)           {}
func closeWindow(uintptr)          {}
func countProcs(string) int        { return 0 }
func killProcs(string)             {}
func installerKeys() []string      { return nil }

// CleanRegistry: реестр есть только на Windows.
func CleanRegistry(string, []string) (int, error) { return 0, nil }

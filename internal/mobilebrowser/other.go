//go:build !android

package mobilebrowser

// Supported — встроенный браузер Android на этой платформе недоступен.
const Supported = false

// Open: браузер Android есть только на Android.
func Open(url, tree string, s Settings) error { return ErrUnsupported }

// Clear: браузер Android есть только на Android.
func Clear(kinds []string) error { return ErrUnsupported }

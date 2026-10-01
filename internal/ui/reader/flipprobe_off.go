//go:build !frameprobe

package reader

// Замер задержки перелистывания есть только в сборке с тегом frameprobe.
func probeFlip()      {}
func probeShown(bool) {}

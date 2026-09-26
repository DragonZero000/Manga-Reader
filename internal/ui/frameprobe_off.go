//go:build !frameprobe

package ui

// startFrameProbe — замер кадров есть только в сборке с тегом frameprobe.
func startFrameProbe() {}

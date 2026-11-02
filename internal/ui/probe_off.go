//go:build !frameprobe

package ui

// startProbe — замер кадров и его сценарии есть только в сборке с тегом
// frameprobe.
func (s *Shell) startProbe() {}

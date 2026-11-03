//go:build !wasm && !test_web_driver

// mangareader: частота монитора окна (third_party/fyne/PATCHES.md).

package glfw

import (
	"time"

	"github.com/go-gl/glfw/v3.4/glfw"

	"fyne.io/fyne/v2/internal/pacing"
)

// pacingMonitor — монитор, по которому взята частота. Только главный поток.
var pacingMonitor *glfw.Monitor

// updateRefreshRate берёт период vsync у монитора, на котором центр окна.
// Окно на другом мониторе — проверка vsync заново. Только главный поток.
func (w *window) updateRefreshRate() {
	if w.viewport == nil {
		return
	}
	m := w.getMonitorForWindow()
	if m == nil {
		return
	}
	mode := m.GetVideoMode()
	if mode == nil || mode.RefreshRate <= 0 {
		return
	}
	p := time.Second / time.Duration(mode.RefreshRate)
	if m != pacingMonitor || p != pacing.Period() {
		pacingMonitor = m
		pacing.SetPeriod(p)
		resetVsyncCheck()
	}
}

// monitorsWatched — колбэк мониторов задан. Только главный поток.
var monitorsWatched bool

// watchMonitorsOnce обновляет частоту окон при смене конфигурации мониторов.
func (d *gLDriver) watchMonitorsOnce() {
	if monitorsWatched {
		return
	}
	monitorsWatched = true
	glfw.SetMonitorCallback(func(*glfw.Monitor, glfw.PeripheralEvent) {
		pacingMonitor = nil // прежний монитор мог исчезнуть
		for _, win := range d.windowList() {
			win.(*window).updateRefreshRate()
		}
	})
}

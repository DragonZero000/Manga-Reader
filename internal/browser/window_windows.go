package browser

import (
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const supported = true

var (
	user32                  = windows.NewLazySystemDLL("user32.dll")
	procSetWindowLongPtr    = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLongPtr    = user32.NewProc("GetWindowLongPtrW")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procIsIconic            = user32.NewProc("IsIconic")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procPostMessage         = user32.NewProc("PostMessageW")
)

const (
	gwlpHwndParent = ^uintptr(8 - 1)  // GWLP_HWNDPARENT = -8
	gwlExStyle     = ^uintptr(20 - 1) // GWL_EXSTYLE = -20
	wsExAppWindow  = 0x00040000
	swRestore      = 9
	wmClose        = 0x0010
	mozillaClass   = "MozillaWindowClass"
)

// findWindows — видимые окна верхнего уровня Firefox, процесс которых
// запущен из exe. Поиск по образу, а не по PID: окно принадлежит дочернему
// процессу, а не запущенному лаунчеру.
func findWindows(exe string) []uintptr {
	var out []uintptr
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		if vis, _, _ := procIsWindowVisible.Call(uintptr(h)); vis == 0 || className(h) != mozillaClass {
			return 1
		}
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(h, &pid); err == nil && samePath(imagePath(pid), exe) {
			out = append(out, uintptr(h))
		}
		return 1
	})
	windows.EnumWindows(cb, nil)
	return out
}

// attach делает owner владельцем окна h: окно остаётся поверх главного,
// сворачивается вместе с ним и не имеет своей кнопки на панели задач.
func attach(h, owner uintptr) {
	procSetWindowLongPtr.Call(h, gwlpHwndParent, owner)
	ex, _, _ := procGetWindowLongPtr.Call(h, gwlExStyle)
	if ex&wsExAppWindow != 0 {
		procSetWindowLongPtr.Call(h, gwlExStyle, ex&^wsExAppWindow)
	}
}

// foreground выводит окно на передний план (восстанавливая из свёрнутого).
func foreground(h uintptr) {
	if iconic, _, _ := procIsIconic.Call(h); iconic != 0 {
		procShowWindow.Call(h, swRestore)
	}
	procSetForegroundWindow.Call(h)
}

func closeWindow(h uintptr) { procPostMessage.Call(h, wmClose, 0, 0) }

func className(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(h, &buf[0], int32(len(buf)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func imagePath(pid uint32) string {
	p, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(p)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(p, 0, &buf[0], &n); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func samePath(a, b string) bool { return a != "" && strings.EqualFold(a, b) }

// procs — PID процессов, запущенных из exe.
func procs(exe string) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []uint32
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		// быстрый отсев по имени, затем полный путь
		if strings.EqualFold(windows.UTF16ToString(e.ExeFile[:]), "firefox.exe") && samePath(imagePath(e.ProcessID), exe) {
			out = append(out, e.ProcessID)
		}
	}
	return out
}

func countProcs(exe string) int { return len(procs(exe)) }

func killProcs(exe string) {
	for _, pid := range procs(exe) {
		if p, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, pid); err == nil {
			windows.TerminateProcess(p, 1)
			windows.CloseHandle(p)
		}
	}
}

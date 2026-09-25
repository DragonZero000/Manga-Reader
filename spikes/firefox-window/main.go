// Спайк S1: портативный Firefox ESR как отдельное окно, привязанное к окну
// Fyne («как в Telegram»). Проверяет: поиск окна по образу процесса,
// привязку владельца, сворачивание вместе с главным окном, передачу URL в
// запущенный экземпляр (без -no-remote), закрытие через WM_CLOSE.
//
// Запуск: go run . <папка с firefox.exe> <папка профиля> [open-close <url>]
// Режим open-close: открыть url, подождать 5 с и закрыть (для S3/S4).
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/widget"
	"golang.org/x/sys/windows"
)

var (
	user32            = windows.NewLazySystemDLL("user32.dll")
	procSetWindowLong = user32.NewProc("SetWindowLongPtrW")
	procGetWindowLong = user32.NewProc("GetWindowLongPtrW")
	procGetWindow     = user32.NewProc("GetWindow")
	procShowWindow    = user32.NewProc("ShowWindow")
	procIsIconic      = user32.NewProc("IsIconic")
	procIsVisible     = user32.NewProc("IsWindowVisible")
	procPostMessage   = user32.NewProc("PostMessageW")
	procGetWindowText = user32.NewProc("GetWindowTextW")
)

const (
	gwlpHwndParent = ^uintptr(8 - 1) // -8
	gwlExStyle     = ^uintptr(20 - 1) // -20
	gwOwner        = 4
	wsExAppWindow  = 0x00040000
	wsExToolWindow = 0x00000080
	swMinimize     = 6
	swRestore      = 9
	wmClose        = 0x0010
)

func main() {
	if len(os.Args) < 3 {
		log.Fatal("go run . <папка firefox> <папка профиля>")
	}
	ffDir, profile := os.Args[1], os.Args[2]
	exe := filepath.Join(ffDir, "firefox.exe")

	a := app.NewWithID("io.github.mangareader.spike.firefox")
	w := a.NewWindow("MangaReader spike")
	w.SetContent(widget.NewLabel("главное окно"))
	w.Resize(fyne.NewSize(800, 600))

	a.Lifecycle().SetOnStarted(func() {
		if len(os.Args) >= 5 && os.Args[3] == "open-close" {
			go openClose(exe, profile, os.Args[4], a)
			return
		}
		go run(w, exe, profile, a)
	})
	w.ShowAndRun()
}

func run(w fyne.Window, exe, profile string, a fyne.App) {
	defer fyne.Do(a.Quit)
	var main windows.HWND
	done := make(chan struct{})
	fyne.Do(func() {
		w.(driver.NativeWindow).RunNative(func(ctx any) {
			main = windows.HWND(ctx.(driver.WindowsWindowContext).HWND)
		})
		close(done)
	})
	<-done
	fmt.Printf("главное окно HWND=%#x\n", main)

	env := append(os.Environ(), "MOZ_CRASHREPORTER_DISABLE=1")
	cmd := exec.Command(exe, "-profile", profile, "about:blank")
	cmd.Env = env
	start := time.Now()
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("запущен PID=%d\n", cmd.Process.Pid)
	go cmd.Wait()

	var ff windows.HWND
	for time.Since(start) < 30*time.Second {
		if hs := findWindows(exe); len(hs) > 0 {
			ff = hs[0]
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if ff == 0 {
		log.Fatal("окно Firefox не найдено")
	}
	_, pid, _ := windowPID(ff)
	fmt.Printf("окно Firefox HWND=%#x PID=%d (запущенный PID %d) за %v\n", ff, pid, cmd.Process.Pid, time.Since(start).Round(time.Millisecond))

	attach(ff, main)
	report("сразу после привязки", ff, main)
	time.Sleep(3 * time.Second)
	report("через 3 с", ff, main)

	procShowWindow.Call(uintptr(main), swMinimize)
	time.Sleep(time.Second)
	iconic, _, _ := procIsIconic.Call(uintptr(main))
	vis, _, _ := procIsVisible.Call(uintptr(ff))
	fmt.Printf("главное свернуто=%v, Firefox видим=%v (ожидается false)\n", iconic != 0, vis != 0)
	procShowWindow.Call(uintptr(main), swRestore)
	time.Sleep(time.Second)
	vis, _, _ = procIsVisible.Call(uintptr(ff))
	fmt.Printf("после восстановления Firefox видим=%v\n", vis != 0)

	// передача URL в запущенный экземпляр
	before := len(findWindows(exe))
	c2 := exec.Command(exe, "-profile", profile, "https://example.org/")
	c2.Env = env
	t2 := time.Now()
	out, err := c2.CombinedOutput()
	fmt.Printf("второй запуск: завершился за %v, err=%v, вывод=%q\n", time.Since(t2).Round(time.Millisecond), err, strings.TrimSpace(string(out)))
	time.Sleep(4 * time.Second)
	hs := findWindows(exe)
	fmt.Printf("окон Firefox: было %d, стало %d\n", before, len(hs))
	for _, h := range hs {
		fmt.Printf("  %#x «%s»\n", h, title(h))
	}

	// закрытие
	for _, h := range hs {
		procPostMessage.Call(uintptr(h), wmClose, 0, 0)
	}
	t3 := time.Now()
	for time.Since(t3) < 10*time.Second && len(findWindows(exe)) > 0 {
		time.Sleep(100 * time.Millisecond)
	}
	fmt.Printf("после WM_CLOSE окон: %d, за %v\n", len(findWindows(exe)), time.Since(t3).Round(time.Millisecond))
	time.Sleep(2 * time.Second)
	fmt.Printf("процессов firefox.exe из нашей папки: %d\n", countProcs(exe))
}

func openClose(exe, profile, url string, a fyne.App) {
	defer fyne.Do(a.Quit)
	cmd := exec.Command(exe, "-profile", profile, url)
	cmd.Env = append(os.Environ(), "MOZ_CRASHREPORTER_DISABLE=1")
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	go cmd.Wait()
	time.Sleep(8 * time.Second)
	for _, h := range findWindows(exe) {
		fmt.Printf("закрываю «%s»\n", title(h))
		procPostMessage.Call(uintptr(h), wmClose, 0, 0)
	}
	for t := time.Now(); time.Since(t) < 15*time.Second && countProcs(exe) > 0; {
		time.Sleep(200 * time.Millisecond)
	}
	fmt.Printf("процессов осталось: %d\n", countProcs(exe))
}

func attach(ff, owner windows.HWND) {
	procSetWindowLong.Call(uintptr(ff), gwlpHwndParent, uintptr(owner))
	ex, _, _ := procGetWindowLong.Call(uintptr(ff), gwlExStyle)
	procSetWindowLong.Call(uintptr(ff), gwlExStyle, ex&^wsExAppWindow)
}

func report(when string, ff, main windows.HWND) {
	owner, _, _ := procGetWindow.Call(uintptr(ff), gwOwner)
	ex, _, _ := procGetWindowLong.Call(uintptr(ff), gwlExStyle)
	fmt.Printf("%s: владелец=%#x (главное=%#x) APPWINDOW=%v TOOLWINDOW=%v\n",
		when, owner, main, ex&wsExAppWindow != 0, ex&wsExToolWindow != 0)
}

// findWindows — видимые окна верхнего уровня класса MozillaWindowClass,
// принадлежащие процессу с образом exe.
func findWindows(exe string) []windows.HWND {
	var out []windows.HWND
	cb := syscall.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		vis, _, _ := procIsVisible.Call(uintptr(h))
		if vis == 0 || className(h) != "MozillaWindowClass" {
			return 1
		}
		if _, pid, err := windowPID(h); err == nil && strings.EqualFold(imagePath(pid), exe) {
			out = append(out, h)
		}
		return 1
	})
	windows.EnumWindows(cb, nil)
	return out
}

func windowPID(h windows.HWND) (uint32, uint32, error) {
	var pid uint32
	tid, err := windows.GetWindowThreadProcessId(h, &pid)
	return tid, pid, err
}

func className(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(h, &buf[0], int32(len(buf)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

func title(h windows.HWND) string {
	buf := make([]uint16, 512)
	n, _, _ := procGetWindowText.Call(uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
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

func countProcs(exe string) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return -1
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	n := 0
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if strings.EqualFold(imagePath(e.ProcessID), exe) {
			n++
		}
	}
	return n
}

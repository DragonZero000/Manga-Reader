package storage

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// CanReveal — «Показать в папке» поддерживается на этой платформе.
const CanReveal = true

var (
	procILCreateFromPathW          = shell32.NewProc("ILCreateFromPathW")
	procILFree                     = shell32.NewProc("ILFree")
	procSHOpenFolderAndSelectItems = shell32.NewProc("SHOpenFolderAndSelectItems")
)

// Reveal открывает Проводник с папкой файла relPath папки библиотеки root и
// выделяет в ней файл. Уже открытое окно Проводника с этой папкой
// используется повторно. Не вызывать из UI-потока.
func Reveal(root, relPath string) error {
	p, err := NewFS(root).absPath(relPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(p); err != nil {
		return err
	}
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	return withCOM(func() error {
		pidl, _, _ := procILCreateFromPathW.Call(uintptr(unsafe.Pointer(name)))
		if pidl == 0 {
			return fmt.Errorf("no shell item for %q", p)
		}
		defer procILFree.Call(pidl)
		// cidl = 0: pidl — полный путь элемента, который нужно выделить
		if hr, _, _ := procSHOpenFolderAndSelectItems.Call(pidl, 0, 0, 0); hr != 0 {
			return fmt.Errorf("opening the folder of %q: HRESULT %#x", p, hr)
		}
		return nil
	})
}

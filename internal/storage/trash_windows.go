package storage

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Удаление на Windows: хранилище FS, файлы — в корзину, если она есть на диске.
const (
	CanDelete = true
	HasTrash  = true
)

var (
	shell32                = windows.NewLazySystemDLL("shell32.dll")
	procSHQueryRecycleBinW = shell32.NewProc("SHQueryRecycleBinW")
	procSHFileOperationW   = shell32.NewProc("SHFileOperationW")
)

// Флаги SHFileOperationW (shellapi.h).
const (
	foDelete            = 0x0003
	fofSilent           = 0x0004
	fofNoConfirmation   = 0x0010
	fofAllowUndo        = 0x0040
	fofNoErrorUI        = 0x0400
	fofWantNukeWarning  = 0x4000
	shqueryrbinfoSizeOf = uint32(unsafe.Sizeof(shQueryRBInfo{}))
)

// Коды CoInitializeEx, которые не мешают вызывать оболочку.
const (
	sFalse          syscall.Errno = 1
	rpcEChangedMode syscall.Errno = 0x80010106
)

// shFileOpStruct — SHFILEOPSTRUCTW. Раскладка совпадает с Windows только на
// 64-битных платформах (на 32-битных shellapi.h упаковывает структуру по 1 байту).
type shFileOpStruct struct {
	hwnd                  uintptr
	wFunc                 uint32
	pFrom                 *uint16
	pTo                   *uint16
	fFlags                uint16
	fAnyOperationsAborted int32
	hNameMappings         uintptr
	lpszProgressTitle     *uint16
}

// shQueryRBInfo — SHQUERYRBINFO.
type shQueryRBInfo struct {
	cbSize      uint32
	i64Size     int64
	i64NumItems int64
}

// CanTrash: корзина есть на локальных несъёмных дисках, для которых её
// можно опросить. Сетевые и съёмные диски (флешки) корзины не имеют, даже
// если SHQueryRecycleBinW отвечает успехом.
func (s *FS) CanTrash(relPath string) (bool, error) {
	p, err := s.absPath(relPath)
	if err != nil {
		return false, err
	}
	root, err := windows.UTF16PtrFromString(filepath.VolumeName(p) + `\`)
	if err != nil {
		return false, err
	}
	switch windows.GetDriveType(root) {
	case windows.DRIVE_REMOTE, windows.DRIVE_REMOVABLE, windows.DRIVE_NO_ROOT_DIR, windows.DRIVE_UNKNOWN:
		return false, nil
	}
	info := shQueryRBInfo{cbSize: shqueryrbinfoSizeOf}
	hr, _, _ := procSHQueryRecycleBinW.Call(uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&info)))
	return hr == 0, nil // S_OK
}

// Delete: в корзину — SHFileOperationW с FOF_ALLOWUNDO, безвозвратно —
// os.Remove. FOF_WANTNUKEWARNING: если корзины всё же нет, Windows спросит
// пользователя, а не удалит файл молча.
func (s *FS) Delete(relPath string, permanent bool) error {
	p, err := s.absPath(relPath)
	if err != nil {
		return err
	}
	if err := checkDeletable(p); err != nil {
		return err
	}
	if permanent {
		if err := os.Remove(p); err != nil {
			if isBusy(err) {
				return fmt.Errorf("%w: %v", ErrBusy, err)
			}
			return err
		}
		return nil
	}
	if ok, err := s.CanTrash(relPath); err != nil {
		return err
	} else if !ok {
		return ErrNoTrash
	}
	return withCOM(func() error { return trash(p) })
}

// absPath — абсолютный путь файла: оболочке Windows нужен полный путь.
func (s *FS) absPath(relPath string) (string, error) {
	p, err := s.path(relPath)
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

// checkDeletable проверяет, что файл есть и его можно удалить: открытие с
// правом DELETE не удаётся, если другой процесс держит файл без
// FILE_SHARE_DELETE, — тогда удаление (и перемещение в корзину) не пройдёт.
func checkDeletable(p string) error {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return err
	}
	h, err := windows.CreateFile(name, windows.DELETE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		if isBusy(err) {
			return fmt.Errorf("%w: %v", ErrBusy, err)
		}
		return &os.PathError{Op: "delete", Path: p, Err: err}
	}
	windows.CloseHandle(h)
	return nil
}

func trash(p string) error {
	// pFrom — список путей, каждый завершается нулём, весь список — ещё одним
	from, err := windows.UTF16FromString(p)
	if err != nil {
		return err
	}
	from = append(from, 0)
	op := shFileOpStruct{
		wFunc:  foDelete,
		pFrom:  &from[0],
		fFlags: fofAllowUndo | fofNoConfirmation | fofSilent | fofNoErrorUI | fofWantNukeWarning,
	}
	r, _, _ := procSHFileOperationW.Call(uintptr(unsafe.Pointer(&op)))
	runtime.KeepAlive(from)
	switch {
	case r == uintptr(errSharingViolation) || r == uintptr(errLockViolation):
		return fmt.Errorf("%w: %v", ErrBusy, syscall.Errno(r))
	case r != 0:
		// коды SHFileOperation частично совпадают с системными, частично свои (DE_*)
		return fmt.Errorf("moving to the Recycle Bin failed: code %#x", r)
	case op.fAnyOperationsAborted != 0:
		return errors.New("moving to the Recycle Bin was cancelled")
	}
	return nil
}

// withCOM выполняет fn в потоке с инициализированным COM (однопоточный
// апартамент) — этого требуют функции оболочки Windows.
func withCOM(fn func() error) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	switch err := windows.CoInitializeEx(0, windows.COINIT_APARTMENTTHREADED|windows.COINIT_DISABLE_OLE1DDE); err {
	case nil, sFalse: // S_FALSE — уже инициализирован в потоке, парный вызов всё равно нужен
		defer windows.CoUninitialize()
	case rpcEChangedMode: // поток уже в другом апартаменте — COM готов
	default:
		return fmt.Errorf("COM initialization: %w", err)
	}
	return fn()
}

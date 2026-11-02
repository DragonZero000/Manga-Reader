//go:build android

// Package devreport сохраняет отчёт тестов производительности dev-сборки и
// отдаёт его в системное меню «Поделиться» (Android: Kotlin DevReport через
// JNI; остальные платформы — файл в папке, без «Поделиться»).
package devreport

/*
#include <stdint.h>
#include <stdlib.h>
char *drSave(uintptr_t env, uintptr_t ctx, const char *tree, const char *name, const char *text);
char *drShare(uintptr_t env, uintptr_t ctx, const char *subject, const char *text);
*/
import "C"

import (
	"errors"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// CanShare — есть системное меню «Поделиться» (Android).
const CanShare = true

// DirName — скрытая папка отчётов в папке библиотеки (сканер её пропускает).
const DirName = ".perf"

// Save сохраняет text в файл name папки DirName дерева SAF dir (tree-URI
// папки библиотеки). Вызывать вне UI-потока.
func Save(dir, name, text string) error {
	return native(func(env, ctx C.uintptr_t) *C.char {
		ct, cn, cx := C.CString(dir), C.CString(name), C.CString(text)
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cn))
		defer C.free(unsafe.Pointer(cx))
		return C.drSave(env, ctx, ct, cn, cx)
	})
}

// Share открывает системное меню «Поделиться» с текстом отчёта.
func Share(subject, text string) error {
	return native(func(env, ctx C.uintptr_t) *C.char {
		cs, cx := C.CString(subject), C.CString(text)
		defer C.free(unsafe.Pointer(cs))
		defer C.free(unsafe.Pointer(cx))
		return C.drShare(env, ctx, cs, cx)
	})
}

func native(call func(env, ctx C.uintptr_t) *C.char) error {
	return driver.RunNative(func(c any) error {
		ac, ok := c.(*driver.AndroidContext)
		if !ok {
			return errors.New("no Android context")
		}
		if e := call(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx)); e != nil {
			defer C.free(unsafe.Pointer(e))
			return errors.New(C.GoString(e))
		}
		return nil
	})
}

//go:build android

package mobilebrowser

/*
#include <stdint.h>
#include <stdlib.h>
char *mbOpen(uintptr_t env, uintptr_t ctx, const char *url, const char *tree, const char *settings);
char *mbClear(uintptr_t env, uintptr_t ctx, const char *kinds);
*/
import "C"

import (
	"errors"
	"strings"
	"unicode/utf16"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// Supported — встроенный браузер есть (Android).
const Supported = true

// Open открывает экран браузера поверх читалки: url — в новой вкладке
// («» — показать текущую), tree — SAF-дерево папки библиотеки для загрузок.
func Open(url, tree string, s Settings) error {
	return jni(func(env, ctx C.uintptr_t) *C.char {
		cu, ct, cs := C.CString(url), C.CString(tree), C.CString(s.json())
		defer C.free(unsafe.Pointer(cu))
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cs))
		return C.mbOpen(env, ctx, cu, ct, cs)
	})
}

// Clear очищает данные браузера (ClearCookies, ClearHistory) сразу; по
// окончании браузер показывает сообщение «Очищено».
func Clear(kinds []string) error {
	return jni(func(env, ctx C.uintptr_t) *C.char {
		ck := C.CString(strings.Join(kinds, ","))
		defer C.free(unsafe.Pointer(ck))
		return C.mbClear(env, ctx, ck)
	})
}

func jni(fn func(env, ctx C.uintptr_t) *C.char) error {
	return driver.RunNative(func(c any) error {
		ac, ok := c.(*driver.AndroidContext)
		if !ok {
			return errors.New("нет контекста Android")
		}
		if e := fn(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx)); e != nil {
			defer C.free(unsafe.Pointer(e))
			return errors.New(C.GoString(e))
		}
		return nil
	})
}

// goMbDownloaded вызывается из GoBridge.nativeOnDownloaded (Kotlin) со
// строками в UTF-16.
//
//export goMbDownloaded
func goMbDownloaded(rel *C.uint16_t, relLen C.int, page *C.uint16_t, pageLen C.int) {
	downloaded(utf16String(rel, relLen), utf16String(page, pageLen))
}

func utf16String(p *C.uint16_t, n C.int) string {
	if p == nil || n <= 0 {
		return ""
	}
	return string(utf16.Decode(unsafe.Slice((*uint16)(unsafe.Pointer(p)), int(n))))
}

//go:build android

// Package display управляет частотой экрана окна приложения на Android
// (Kotlin DisplayRate через JNI). На остальных платформах — заглушка.
package display

/*
#include <stdint.h>
#include <stdlib.h>
char *dispApply(uintptr_t env, uintptr_t ctx, int max60);
*/
import "C"

import (
	"errors"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// Supported — частоту экрана можно ограничить (Android).
const Supported = true

// SetMax60 просит систему показывать окно приложения с частотой 60 Гц
// (on=false — частоту выбирает система). Применяется в UI-потоке Android
// асинхронно; ошибка — только если вызов не удался.
func SetMax60(on bool) error {
	return driver.RunNative(func(c any) error {
		ac, ok := c.(*driver.AndroidContext)
		if !ok {
			return errors.New("нет контекста Android")
		}
		v := C.int(0)
		if on {
			v = 1
		}
		if e := C.dispApply(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx), v); e != nil {
			defer C.free(unsafe.Pointer(e))
			return errors.New(C.GoString(e))
		}
		return nil
	})
}

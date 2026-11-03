//go:build android

// Package display управляет частотой экрана окна приложения на Android
// (Kotlin DisplayRate через JNI). На остальных платформах — заглушка.
package display

/*
#include <stdint.h>
#include <stdlib.h>
char *dispApply(uintptr_t env, uintptr_t ctx, int max60, float *maxRate);
*/
import "C"

import (
	"errors"
	"unsafe"

	"fyne.io/fyne/v2/driver"

	"mangareader/internal/ui/frameclock"
)

// Supported — частоту экрана можно ограничить (Android).
const Supported = true

// SetMax60 просит систему показывать окно приложения с частотой 60 Гц
// (on=false — частоту выбирает система, а на время движения драйвер просит
// наибольшую частоту экрана, frameclock.SetRateHint). Кадры в обоих случаях
// идут по vsync, так что 60 Гц нужно только для экономии батареи.
// Применяется в UI-потоке Android асинхронно; ошибка — только если вызов не
// удался.
func SetMax60(on bool) error {
	return driver.RunNative(func(c any) error {
		ac, ok := c.(*driver.AndroidContext)
		if !ok {
			return errors.New("no Android context")
		}
		v := C.int(0)
		if on {
			v = 1
		}
		var maxRate C.float
		if e := C.dispApply(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx), v, &maxRate); e != nil {
			defer C.free(unsafe.Pointer(e))
			frameclock.SetRateHint(0)
			return errors.New(C.GoString(e))
		}
		if on {
			frameclock.SetRateHint(0)
		} else {
			frameclock.SetRateHint(float64(maxRate))
		}
		return nil
	})
}

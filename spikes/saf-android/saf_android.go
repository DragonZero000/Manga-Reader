//go:build android

package main

/*
#cgo LDFLAGS: -landroid -llog
#include <stdint.h>
#include <stdlib.h>
int safTakePersistable(uintptr_t env, uintptr_t ctx, const char *uri, char **err);
int safIsPersisted(uintptr_t env, uintptr_t ctx, const char *uri);
char *safTreeDocId(uintptr_t env, const char *uri, char **err);
char *safListChildren(uintptr_t env, uintptr_t ctx, const char *tree, const char *docId, char **err);
int safOpenFd(uintptr_t env, uintptr_t ctx, const char *tree, const char *docId, char **err);
*/
import "C"

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

type entry struct {
	id, name, mime string
	size, mod      int64
}

func jni(fn func(env, ctx C.uintptr_t) error) error {
	return driver.RunNative(func(c any) error {
		ac, ok := c.(*driver.AndroidContext)
		if !ok {
			return errors.New("нет контекста Android")
		}
		return fn(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx))
	})
}

func cerr(e *C.char) error {
	if e == nil {
		return nil
	}
	defer C.free(unsafe.Pointer(e))
	return errors.New(C.GoString(e))
}

func takePersistable(uri string) error {
	return jni(func(env, ctx C.uintptr_t) error {
		cu := C.CString(uri)
		defer C.free(unsafe.Pointer(cu))
		var e *C.char
		C.safTakePersistable(env, ctx, cu, &e)
		return cerr(e)
	})
}

func isPersisted(uri string) (ok bool) {
	jni(func(env, ctx C.uintptr_t) error {
		cu := C.CString(uri)
		defer C.free(unsafe.Pointer(cu))
		ok = C.safIsPersisted(env, ctx, cu) == 1
		return nil
	})
	return ok
}

func treeDocID(uri string) (id string, err error) {
	err = jni(func(env, _ C.uintptr_t) error {
		cu := C.CString(uri)
		defer C.free(unsafe.Pointer(cu))
		var e *C.char
		r := C.safTreeDocId(env, cu, &e)
		if err := cerr(e); err != nil {
			return err
		}
		defer C.free(unsafe.Pointer(r))
		id = C.GoString(r)
		return nil
	})
	return id, err
}

func listChildren(tree, docID string) (out []entry, err error) {
	err = jni(func(env, ctx C.uintptr_t) error {
		ct, cd := C.CString(tree), C.CString(docID)
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cd))
		var e *C.char
		r := C.safListChildren(env, ctx, ct, cd, &e)
		if err := cerr(e); err != nil {
			return err
		}
		defer C.free(unsafe.Pointer(r))
		for _, rec := range strings.Split(C.GoString(r), "\x1e") {
			f := strings.Split(rec, "\x1f")
			if len(f) != 5 {
				continue
			}
			size, _ := strconv.ParseInt(f[3], 10, 64)
			mod, _ := strconv.ParseInt(f[4], 10, 64)
			out = append(out, entry{id: f[0], name: f[1], mime: f[2], size: size, mod: mod})
		}
		return nil
	})
	return out, err
}

func openFile(tree, docID, name string) (f *os.File, err error) {
	err = jni(func(env, ctx C.uintptr_t) error {
		ct, cd := C.CString(tree), C.CString(docID)
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cd))
		var e *C.char
		fd := C.safOpenFd(env, ctx, ct, cd, &e)
		if err := cerr(e); err != nil {
			return err
		}
		f = os.NewFile(uintptr(fd), name)
		return nil
	})
	return f, err
}

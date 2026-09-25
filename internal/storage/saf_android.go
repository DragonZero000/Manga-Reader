//go:build android

package storage

/*
#cgo LDFLAGS: -landroid -llog
#include <stdint.h>
#include <stdlib.h>
int safTakePersistable(uintptr_t env, uintptr_t ctx, const char *uri, char **err);
int safIsPersisted(uintptr_t env, uintptr_t ctx, const char *uri, int write);
char *safTreeDocId(uintptr_t env, const char *uri, char **err);
char *safListChildren(uintptr_t env, uintptr_t ctx, const char *tree, const char *docId, char **err);
int safOpenFd(uintptr_t env, uintptr_t ctx, const char *tree, const char *docId, char **err);
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unsafe"

	"fyne.io/fyne/v2/driver"
)

// dirMime — MIME-тип папки в DocumentsContract.
const dirMime = "vnd.android.document/directory"

// SAF — папка, выбранная пользователем через Storage Access Framework.
// Файлы открываются через дескриптор, выданный системой (без копирования).
type SAF struct {
	tree string // content://…/tree/…

	mu    sync.RWMutex
	files map[string]safFile // по относительному пути — из последнего обхода
}

type safFile struct {
	docID string
	size  int64
}

// NewSAF создаёт хранилище для дерева tree.
func NewSAF(tree string) *SAF { return &SAF{tree: tree, files: map[string]safFile{}} }

// TakePersistable делает разрешение на чтение и запись дерева постоянным.
// Вызывать сразу после выбора папки, пока действует временный доступ.
func TakePersistable(tree string) error {
	return jni(func(env, ctx C.uintptr_t) error {
		cu := C.CString(tree)
		defer C.free(unsafe.Pointer(cu))
		var e *C.char
		C.safTakePersistable(env, ctx, cu, &e)
		return cerr(e)
	})
}

// Name — папка для показа: «Download/manga» для «primary:Download/manga».
func (s *SAF) Name() string {
	id, err := treeDocID(s.tree)
	if err != nil || id == "" {
		return s.tree
	}
	vol, p, ok := strings.Cut(id, ":")
	if !ok {
		return id
	}
	if vol == "primary" {
		return p
	}
	return p + " (" + vol + ")"
}

func (s *SAF) Walk(ctx context.Context, fn func(Entry) error) error {
	if !isPersisted(s.tree) {
		return fmt.Errorf("%w: нет разрешения на папку", ErrUnavailable)
	}
	root, err := treeDocID(s.tree)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	files := map[string]safFile{}
	var walk func(docID, rel string, top bool) error
	walk = func(docID, rel string, top bool) error {
		children, err := listChildren(s.tree, docID)
		if err != nil {
			if top {
				return fmt.Errorf("%w: %v", ErrUnavailable, err)
			}
			return nil // недоступная подпапка — пропускаем
		}
		for _, c := range children {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p := c.name
			if rel != "" {
				p = rel + "/" + c.name
			}
			isDir := c.mime == dirMime
			if !isDir {
				// соответствие нужно уже в fn: сканер открывает архив сразу
				f := safFile{docID: c.id, size: c.size}
				files[p] = f
				s.mu.Lock()
				s.files[p] = f
				s.mu.Unlock()
			}
			if err := fn(Entry{RelPath: p, Size: c.size, ModTime: time.UnixMilli(c.mod), IsDir: isDir}); err != nil {
				return err
			}
			if isDir {
				if err := walk(c.id, p, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(root, "", true); err != nil {
		return err
	}
	// заменить целиком — удалённые файлы исчезают
	s.mu.Lock()
	s.files = files
	s.mu.Unlock()
	return nil
}

func (s *SAF) Open(relPath string) (File, error) {
	s.mu.RLock()
	f, ok := s.files[relPath]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("файл %q не найден в папке", relPath)
	}
	var file *os.File
	err := jni(func(env, ctx C.uintptr_t) error {
		ct, cd := C.CString(s.tree), C.CString(f.docID)
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cd))
		var e *C.char
		fd := C.safOpenFd(env, ctx, ct, cd, &e)
		if err := cerr(e); err != nil {
			return err
		}
		file = os.NewFile(uintptr(fd), relPath)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return NewOSFile(file, f.size), nil
}

// --- JNI ---

type child struct {
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

func isPersisted(uri string) bool { return persisted(uri, false) }

// HasWriteAccess — у приложения есть постоянный доступ на запись к дереву
// (папки, выбранные до появления браузера, открыты только на чтение).
func HasWriteAccess(tree string) bool { return persisted(tree, true) }

func persisted(uri string, write bool) (ok bool) {
	w := C.int(0)
	if write {
		w = 1
	}
	jni(func(env, ctx C.uintptr_t) error {
		cu := C.CString(uri)
		defer C.free(unsafe.Pointer(cu))
		ok = C.safIsPersisted(env, ctx, cu, w) == 1
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

func listChildren(tree, docID string) (out []child, err error) {
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
			out = append(out, child{id: f[0], name: f[1], mime: f[2], size: size, mod: mod})
		}
		return nil
	})
	return out, err
}

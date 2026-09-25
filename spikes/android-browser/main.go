// Спайк S-B1…S-B3: встроенный браузер на GeckoView в Gradle-сборке.
// Fyne-экран с кнопкой «Браузер» открывает Kotlin-экран BrowserActivity
// через JNI (Intent.setClassName), браузер возвращается в Fyne по кнопке
// «MangaReader»/«Назад»; вкладка живёт в объекте процесса. Загрузки
// браузер пишет в выбранную SAF-папку и сообщает в Go (GoBridge).
package main

/*
#include <stdint.h>
#include <stdlib.h>
char *openBrowser(uintptr_t env, uintptr_t ctx, const char *url, const char *tree);
*/
import "C"

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver"
	"fyne.io/fyne/v2/widget"
)

func init() {
	app.SetMetadata(fyne.AppMetadata{ID: "io.github.mangareader.spike.browser", Name: "Browser spike", Version: "0.1.0", Build: 1})
}

var (
	mu        sync.Mutex
	downloads []string
	onChange  func()
)

// goOnDownloaded вызывается из Kotlin (GoBridge.nativeOnDownloaded) из
// потока загрузки.
//
//export goOnDownloaded
func goOnDownloaded(name, page *C.char) {
	mu.Lock()
	downloads = append(downloads, fmt.Sprintf("%s ← %s", C.GoString(name), C.GoString(page)))
	f := onChange
	mu.Unlock()
	if f != nil {
		fyne.Do(f)
	}
}

func openBrowser(url, tree string) error {
	return driver.RunNative(func(c any) error {
		ac, ok := c.(*driver.AndroidContext)
		if !ok {
			return errors.New("нет контекста Android")
		}
		cu, ct := C.CString(url), C.CString(tree)
		defer C.free(unsafe.Pointer(cu))
		defer C.free(unsafe.Pointer(ct))
		if e := C.openBrowser(C.uintptr_t(ac.Env), C.uintptr_t(ac.Ctx), cu, ct); e != nil {
			defer C.free(unsafe.Pointer(e))
			return errors.New(C.GoString(e))
		}
		return nil
	})
}

func main() {
	a := app.NewWithID("io.github.mangareader.spike.browser")
	w := a.NewWindow("Browser spike")
	status := widget.NewLabel("читалка (Fyne)")
	status.Wrapping = fyne.TextWrapWord
	tree := a.Preferences().String("tree")
	folder := widget.NewLabel("папка: " + tree)
	folder.Wrapping = fyne.TextWrapBreak
	got := widget.NewLabel("загрузок: 0")
	got.Wrapping = fyne.TextWrapBreak
	onChange = func() {
		mu.Lock()
		text := fmt.Sprintf("загрузок: %d", len(downloads))
		for _, d := range downloads {
			text += "\n" + d
		}
		mu.Unlock()
		got.SetText(text)
	}
	open := func(url string) {
		if err := openBrowser(url, tree); err != nil {
			status.SetText("ошибка: " + err.Error())
		}
	}
	choose := widget.NewButton("Выбрать папку", func() {
		dialog.ShowFolderOpen(func(u fyne.ListableURI, err error) {
			if err == nil && u != nil {
				tree = u.String()
				a.Preferences().SetString("tree", tree)
				folder.SetText("папка: " + tree)
			}
		}, w)
	})
	w.SetContent(container.NewVBox(
		status,
		widget.NewButton("Браузер", func() { open("") }),
		widget.NewButton("Открыть тестовый сайт", func() { open("http://127.0.0.1:8765/g/535147/") }),
		widget.NewButton("Новостной сайт", func() { open("https://edition.cnn.com/") }),
		widget.NewButton("uBlock на AMO", func() { open("https://addons.mozilla.org/ru/android/addon/ublock-origin/") }),
		choose, folder, got,
	))
	w.ShowAndRun()
}

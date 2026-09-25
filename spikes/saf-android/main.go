// Спайк: выбор папки через диалог Fyne (SAF), постоянное разрешение,
// обход дерева с размером и датой, чтение zip через дескриптор файла.
// Собирается только под Android.
package main

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
)

const dirMime = "vnd.android.document/directory"

func main() {
	a := app.NewWithID("io.github.mangareader.safspike")
	w := a.NewWindow("SAF spike")
	out := widget.NewLabel("")
	out.Wrapping = fyne.TextWrapBreak
	report := func(s string) {
		log.Print("SPIKE " + s)
		fyne.Do(func() { out.SetText(out.Text + s + "\n") })
	}

	test := func(tree string) {
		go func() {
			report("tree=" + tree)
			report(fmt.Sprintf("persisted=%v", isPersisted(tree)))
			root, err := treeDocID(tree)
			if err != nil {
				report("FAIL treeDocId: " + err.Error())
				return
			}
			start := time.Now()
			var walk func(id, rel string)
			n := 0
			walk = func(id, rel string) {
				children, err := listChildren(tree, id)
				if err != nil {
					report("FAIL list " + rel + ": " + err.Error())
					return
				}
				for _, c := range children {
					path := strings.TrimPrefix(rel+"/"+c.name, "/")
					if c.mime == dirMime {
						walk(c.id, path)
						continue
					}
					n++
					report(fmt.Sprintf("file %s size=%d mod=%s", path, c.size,
						time.UnixMilli(c.mod).Format("2006-01-02 15:04")))
					if !strings.HasSuffix(strings.ToLower(c.name), ".zip") {
						continue
					}
					f, err := openFile(tree, c.id, c.name)
					if err != nil {
						report("FAIL open " + path + ": " + err.Error())
						continue
					}
					zr, err := zip.NewReader(f, c.size)
					if err != nil {
						report("FAIL zip " + path + ": " + err.Error())
						f.Close()
						continue
					}
					// читаем последний файл архива — нужен произвольный доступ
					last := zr.File[len(zr.File)-1]
					rc, err := last.Open()
					if err == nil {
						b, _ := io.ReadAll(rc)
						rc.Close()
						report(fmt.Sprintf("SPIKE OK zip %s: %d entries, last %s = %d bytes", path, len(zr.File), last.Name, len(b)))
					} else {
						report("FAIL read " + path + ": " + err.Error())
					}
					f.Close()
				}
			}
			walk(root, "")
			report(fmt.Sprintf("walk done: %d files in %v", n, time.Since(start)))
		}()
	}

	pick := widget.NewButton("Выбрать папку", func() {
		dialog.ShowFolderOpen(func(lu fyne.ListableURI, err error) {
			if err != nil || lu == nil {
				report(fmt.Sprintf("picker cancelled/err: %v", err))
				return
			}
			tree := lu.String()
			if err := takePersistable(tree); err != nil {
				report("FAIL takePersistable: " + err.Error())
			} else {
				report("takePersistable OK")
			}
			a.Preferences().SetString("tree", tree)
			test(tree)
		}, w)
	})
	again := widget.NewButton("Проверить сохранённую", func() {
		if tree := a.Preferences().String("tree"); tree != "" {
			test(tree)
		} else {
			report("нет сохранённой папки")
		}
	})
	w.SetContent(container.NewBorder(container.NewVBox(pick, again), nil, nil, nil, container.NewVScroll(out)))
	if tree := a.Preferences().String("tree"); tree != "" {
		test(tree) // после перезапуска — без повторного выбора
	}
	w.ShowAndRun()
}

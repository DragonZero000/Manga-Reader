// Спайк S2: работает ли modernc.org/sqlite (с FTS5) в Android .apk.
// Отдельный модуль — зависимость не попадает в основной go.mod.
package main

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"runtime"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	_ "modernc.org/sqlite"
)

func main() {
	a := app.NewWithID("io.github.mangareader.spike")
	w := a.NewWindow("SQLite spike")
	out := widget.NewLabel("нажмите «Проверить»")
	out.Wrapping = 3 // fyne.TextWrapWord

	run := func() {
		dir := a.Storage().RootURI().Path()
		res, err := check(filepath.Join(dir, "spike.db"))
		msg := fmt.Sprintf("%s/%s\n", runtime.GOOS, runtime.GOARCH)
		if err != nil {
			msg += "SPIKE FAIL: " + err.Error()
		} else {
			msg += "SPIKE OK: " + res
		}
		log.Print(msg)
		out.SetText(msg)
	}
	w.SetContent(container.NewVBox(widget.NewButton("Проверить", run), out))
	run() // сразу при запуске — результат виден в logcat
	w.ShowAndRun()
}

func check(path string) (string, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return "", fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	var version string
	if err := db.QueryRow(`select sqlite_version()`).Scan(&version); err != nil {
		return "", fmt.Errorf("version: %w", err)
	}
	stmts := []string{
		`drop table if exists titles`,
		`create virtual table titles using fts5(title, alt_title)`,
		`insert into titles values ('english name', 'japanease name'), ('другое название', '')`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			return "", fmt.Errorf("%s: %w", s, err)
		}
	}
	var found string
	if err := db.QueryRow(`select title from titles where titles match 'japan*'`).Scan(&found); err != nil {
		return "", fmt.Errorf("fts5 match: %w", err)
	}
	var cyr string
	if err := db.QueryRow(`select title from titles where titles match 'другое'`).Scan(&cyr); err != nil {
		return "", fmt.Errorf("fts5 match (кириллица): %w", err)
	}
	return fmt.Sprintf("sqlite %s, fts5 → %q, %q, db=%s", version, found, cyr, path), nil
}

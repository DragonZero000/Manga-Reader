//go:build sqlite_fts5

// Package catalog — каталог библиотеки в SQLite: разобранные галереи и
// ошибки (кэш сканера), индекс поиска, уменьшенные обложки и копия ссылок
// скачанных файлов. Каталог — восстанавливаемый кэш: повреждённый файл или
// другая версия схемы пересоздаются. Методы безопасны для вызова из разных
// горутин; из UI-потока каталог не вызывается.
package catalog

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"sync"

	sqlite3 "github.com/mattn/go-sqlite3"

	"mangareader/internal/model"
	"mangareader/internal/search"
)

// FileName — имя файла каталога.
const FileName = "library.db"

// schemaVersion — версия схемы; при изменении схемы увеличить.
const schemaVersion = 1

// userVersion — PRAGMA user_version: схема и правила нормализации поиска.
func userVersion() int { return schemaVersion*100 + search.NormVersion }

const driverName = "sqlite3_mangareader"

func init() {
	sql.Register(driverName, &sqlite3.SQLiteDriver{
		ConnectHook: func(c *sqlite3.SQLiteConn) error {
			// натуральный порядок ключей — как model.NaturalLess в MemIndex
			return c.RegisterCollation("NATSORT", func(a, b string) int {
				switch {
				case model.NaturalLess(a, b):
					return -1
				case model.NaturalLess(b, a):
					return 1
				}
				return 0
			})
		},
	})
}

var schema = []string{
	`CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT)`,
	`CREATE TABLE files(rel TEXT PRIMARY KEY, size INTEGER, mtime INTEGER,
		gallery BLOB, err_kind TEXT, err_text TEXT)`,
	`CREATE TABLE docs(key TEXT PRIMARY KEY, rel TEXT, title_sort TEXT, title_norm TEXT,
		scan_norm TEXT, ext_id INTEGER, pages INTEGER, favorites INTEGER, size INTEGER,
		uploaded INTEGER, added INTEGER)`,
	`CREATE VIRTUAL TABLE docs_fts USING fts5(key UNINDEXED, hay, tokenize='trigram')`,
	`CREATE TABLE tags(key TEXT, type TEXT, name TEXT, name_norm TEXT, PRIMARY KEY(key, type, name))`,
	`CREATE INDEX tags_norm ON tags(name_norm, type)`,
	`CREATE TABLE covers(key TEXT PRIMARY KEY, rel TEXT, w INTEGER, h INTEGER, jpeg BLOB)`,
	`CREATE INDEX covers_rel ON covers(rel)`,
	`CREATE TABLE links(rel TEXT PRIMARY KEY, url TEXT)`,
}

// dataTables — таблицы с данными одной папки библиотеки.
var dataTables = []string{"files", "docs", "docs_fts", "tags", "covers", "links"}

// Catalog — открытый каталог.
type Catalog struct {
	db   *sql.DB
	path string
	// fresh — каталог создан при этом открытии (пустой): ссылки нужно
	// перенести из links.json, библиотека сканируется целиком.
	fresh bool
	// writeMu сериализует пишущие транзакции (в SQLite писатель один).
	writeMu sync.Mutex
}

// Open открывает каталог path для папки библиотеки root. Повреждённый файл
// и другая версия схемы пересоздаются; другой root очищает данные.
func Open(path, root string) (*Catalog, error) {
	c, err := open(path)
	if err != nil {
		log.Printf("каталог %s: %v — создаю заново", path, err)
		if rmErr := removeFiles(path); rmErr != nil {
			return nil, fmt.Errorf("каталог %s: %v; удаление: %w", path, err, rmErr)
		}
		if c, err = open(path); err != nil {
			return nil, fmt.Errorf("каталог %s: %w", path, err)
		}
	}
	if err := c.setRoot(root); err != nil {
		c.Close()
		return nil, err
	}
	if err := c.checkIndex(); err != nil {
		log.Printf("каталог: проверка индекса: %v", err)
	}
	return c, nil
}

// open открывает файл, проверяет целостность и версию, создаёт схему в новом.
func open(path string) (*Catalog, error) {
	db, err := sql.Open(driverName, path+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	c := &Catalog{db: db, path: path}
	var check string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&check); err != nil {
		db.Close()
		return nil, err
	}
	if check != "ok" {
		db.Close()
		return nil, fmt.Errorf("повреждён: %s", check)
	}
	var ver, tables int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&ver); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master`).Scan(&tables); err != nil {
		db.Close()
		return nil, err
	}
	switch {
	case tables == 0:
		if err := c.create(); err != nil {
			db.Close()
			return nil, fmt.Errorf("создание схемы: %w", err)
		}
	case ver != userVersion():
		db.Close()
		return nil, fmt.Errorf("версия %d, нужна %d", ver, userVersion())
	}
	return c, nil
}

func (c *Catalog) create() error {
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, s := range schema {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	if _, err := tx.Exec(`PRAGMA user_version = ` + strconv.Itoa(userVersion())); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	c.fresh = true
	return nil
}

// removeFiles удаляет файл каталога вместе с журналом WAL.
func removeFiles(path string) error {
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

// Fresh сообщает, создан ли каталог при открытии (или очищен сменой папки).
func (c *Catalog) Fresh() bool { return c.fresh }

// setRoot: каталог другой папки очищается.
func (c *Catalog) setRoot(root string) error {
	var cur string
	err := c.db.QueryRow(`SELECT value FROM meta WHERE key = 'root'`).Scan(&cur)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("каталог: папка: %w", err)
	}
	if err == nil && cur == root {
		return nil
	}
	return c.Reset(root)
}

// Reset очищает данные каталога и привязывает его к папке root (смена папки
// библиотеки).
func (c *Catalog) Reset(root string) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range dataTables {
		if _, err := tx.Exec(`DELETE FROM ` + t); err != nil {
			return fmt.Errorf("каталог: очистка %s: %w", t, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO meta(key, value) VALUES('root', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`, root); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	c.fresh = true
	return nil
}

// Close закрывает каталог.
func (c *Catalog) Close() error { return c.db.Close() }

// write выполняет fn в пишущей транзакции.
func (c *Catalog) write(fn func(tx *sql.Tx) error) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	tx, err := c.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

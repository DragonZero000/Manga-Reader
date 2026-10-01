// Package userdata — пользовательские данные о произведениях в SQLite
// (user.db): записи произведений со стабильным идентификатором, сверка с
// результатами сканирования и перенос записей по отпечатку содержимого.
// В отличие от каталога (library.db), база не восстановима из архивов и
// никогда не пересоздаётся: схема меняется миграциями, повреждённый файл
// откладывается в резервную копию. Методы безопасны для вызова из разных
// горутин; из UI-потока база не вызывается.
package userdata

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	sqlite3 "github.com/mattn/go-sqlite3"
)

// FileName — имя файла пользовательских данных.
const FileName = "user.db"

// ErrNewerSchema — база создана более новой версией приложения. Файл не
// изменяется, пользовательские данные в этом запуске недоступны.
var ErrNewerSchema = errors.New("user data was created by a newer version of the app")

// errDamaged — файл не является базой SQLite или не прошёл проверку.
var errDamaged = errors.New("damaged")

// migration добавляет изменения схемы одной версии.
type migration func(tx *sql.Tx) error

// migrations — миграции по порядку; PRAGMA user_version — число применённых.
// Применённые миграции не изменяются: новые изменения — новой миграцией.
var migrations = []migration{
	// 1: записи произведений. Будущие таблицы ссылаются на works(uid)
	// с ON DELETE CASCADE: удаление записи удаляет и её данные.
	func(tx *sql.Tx) error {
		return execAll(tx,
			`CREATE TABLE works(
				uid         INTEGER PRIMARY KEY,
				root        TEXT NOT NULL,
				rel         TEXT NOT NULL,
				fingerprint TEXT NOT NULL,
				orphaned_at INTEGER,
				UNIQUE(root, rel))`,
			`CREATE INDEX works_fp ON works(root, fingerprint)`,
		)
	},
	// 2: свои и скрытые теги, служебные значения и эпоха базы.
	func(tx *sql.Tx) error {
		if err := execAll(tx,
			`CREATE TABLE user_tags(
				uid  INTEGER NOT NULL REFERENCES works(uid) ON DELETE CASCADE,
				kind INTEGER NOT NULL,
				type TEXT NOT NULL,
				name TEXT NOT NULL,
				seq  INTEGER NOT NULL,
				PRIMARY KEY(uid, kind, type, name))`,
			`CREATE TABLE meta(key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		); err != nil {
			return err
		}
		epoch, err := newEpoch()
		if err != nil {
			return err
		}
		_, err = tx.Exec(`INSERT INTO meta(key, value) VALUES('epoch', ?)`, epoch)
		return err
	},
	// 3: позиция чтения — одна строка на произведение.
	func(tx *sql.Tx) error {
		return execAll(tx,
			`CREATE TABLE progress(
				uid        INTEGER PRIMARY KEY REFERENCES works(uid) ON DELETE CASCADE,
				page       TEXT    NOT NULL,
				page_index INTEGER NOT NULL,
				total      INTEGER NOT NULL,
				finished   INTEGER NOT NULL,
				updated_at INTEGER NOT NULL)`,
		)
	},
}

func execAll(tx *sql.Tx, stmts ...string) error {
	for _, s := range stmts {
		if _, err := tx.Exec(s); err != nil {
			return fmt.Errorf("%s: %w", s, err)
		}
	}
	return nil
}

// Recovery — что сделано при открытии повреждённой базы.
type Recovery struct {
	// Backup — путь резервной копии повреждённого файла; «» — база была
	// в порядке.
	Backup string
	// Cause — чем был повреждён файл (для журнала).
	Cause error
}

// Store — открытая база пользовательских данных.
type Store struct {
	db *sql.DB
	// writeMu сериализует пишущие транзакции (в SQLite писатель один).
	writeMu sync.Mutex
}

// Open открывает базу path (создаёт, если её нет) и применяет недостающие
// миграции. Повреждённый файл переименовывается в резервную копию
// (Recovery.Backup) и создаётся новая база. Ошибка — база недоступна
// (в том числе ErrNewerSchema), приложение работает без пользовательских
// данных.
func Open(path string) (*Store, Recovery, error) {
	s, err := open(path, migrations)
	if err == nil {
		return s, Recovery{}, nil
	}
	if !errors.Is(err, errDamaged) {
		return nil, Recovery{}, fmt.Errorf("user data %s: %w", path, err)
	}
	backup, rnErr := backupFiles(path, time.Now())
	if rnErr != nil {
		return nil, Recovery{}, fmt.Errorf("user data %s: %v; moving to a backup: %w", path, err, rnErr)
	}
	s, err2 := open(path, migrations)
	if err2 != nil {
		return nil, Recovery{}, fmt.Errorf("user data %s: %v; creating a new one: %w", path, err, err2)
	}
	return s, Recovery{Backup: backup, Cause: err}, nil
}

// open открывает файл, проверяет целостность и применяет миграции migs.
func open(path string, migs []migration) (*Store, error) {
	db, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_synchronous=NORMAL&_busy_timeout=5000&_foreign_keys=1")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	if err := prepare(db, migs); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func prepare(db *sql.DB, migs []migration) error {
	var check string
	if err := db.QueryRow(`PRAGMA quick_check`).Scan(&check); err != nil {
		return classify(err)
	}
	if check != "ok" {
		return fmt.Errorf("%w: %s", errDamaged, check)
	}
	var ver int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&ver); err != nil {
		return classify(err)
	}
	switch {
	case ver > len(migs):
		return fmt.Errorf("%w: version %d, known %d", ErrNewerSchema, ver, len(migs))
	case ver == len(migs):
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return classify(err)
	}
	defer tx.Rollback()
	for i := ver; i < len(migs); i++ {
		if err := migs[i](tx); err != nil {
			return fmt.Errorf("migration %d: %w", i+1, classify(err))
		}
	}
	if _, err := tx.Exec(`PRAGMA user_version = ` + strconv.Itoa(len(migs))); err != nil {
		return classify(err)
	}
	return classify(tx.Commit())
}

// classify помечает ошибки повреждённого файла как errDamaged.
func classify(err error) error {
	var se sqlite3.Error
	if errors.As(err, &se) && (se.Code == sqlite3.ErrNotADB || se.Code == sqlite3.ErrCorrupt) {
		return fmt.Errorf("%w: %v", errDamaged, err)
	}
	return err
}

// backupFiles переименовывает path и его журнал (-wal, -shm) в
// path.broken-<ГГГГММДД-ЧЧММСС> и возвращает путь копии.
func backupFiles(path string, now time.Time) (string, error) {
	backup := path + ".broken-" + now.Format("20060102-150405")
	for n := 2; exists(backup); n++ {
		backup = path + ".broken-" + now.Format("20060102-150405") + "-" + strconv.Itoa(n)
	}
	if err := os.Rename(path, backup); err != nil {
		return "", err
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if err := os.Rename(path+suffix, backup+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return backup, nil
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// Close закрывает базу.
func (s *Store) Close() error { return s.db.Close() }

// write выполняет fn в пишущей транзакции.
func (s *Store) write(fn func(tx *sql.Tx) error) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

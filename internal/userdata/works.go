package userdata

import (
	"database/sql"
	"errors"
	"log"
	"time"
)

// Записи произведений. root — ключ папки библиотеки (на ПК — папка
// библиотеки приложения, на Android — tree-URI SAF), rel — путь файла
// относительно неё со слешами '/', fp — отпечаток содержимого
// (model.Gallery.Fingerprint; «» — неизвестен).

// Ensure возвращает идентификатор записи произведения, создавая её при
// первом сохранении данных. Файл на месте: отметка «сирота» снимается,
// непустой отпечаток обновляется.
func (s *Store) Ensure(root, rel, fp string) (uid int64, err error) {
	err = s.write(func(tx *sql.Tx) error {
		err := tx.QueryRow(`SELECT uid FROM works WHERE root = ? AND rel = ?`, root, rel).Scan(&uid)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			res, err := tx.Exec(`INSERT INTO works(root, rel, fingerprint) VALUES(?, ?, ?)`, root, rel, fp)
			if err != nil {
				return err
			}
			uid, err = res.LastInsertId()
			return err
		case err != nil:
			return err
		}
		_, err = tx.Exec(`UPDATE works SET orphaned_at = NULL,
			fingerprint = CASE WHEN ? <> '' THEN ? ELSE fingerprint END WHERE uid = ?`, fp, fp, uid)
		return err
	})
	return uid, err
}

// Lookup находит запись произведения; ok = false — записи нет.
func (s *Store) Lookup(root, rel string) (uid int64, ok bool, err error) {
	err = s.db.QueryRow(`SELECT uid FROM works WHERE root = ? AND rel = ?`, root, rel).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return uid, err == nil, err
}

// MarkOrphan помечает запись сиротой с временем now (файл удалён через
// приложение). Уже помеченная запись сохраняет прежнее время.
func (s *Store) MarkOrphan(root, rel string, now time.Time) error {
	return s.write(func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE works SET orphaned_at = ?
			WHERE root = ? AND rel = ? AND orphaned_at IS NULL`, now.UnixNano(), root, rel)
		return err
	})
}

type work struct {
	uid      int64
	rel      string
	fp       string
	orphaned bool
}

// Reconcile сверяет записи папки root с файлами успешного сканирования
// files (путь → отпечаток; «» — файл есть, но не разобран): найденные
// теряют отметку «сирота», новый файл с тем же отпечатком, что у ровно
// одной пропавшей записи, забирает её, остальные пропавшие помечаются
// сиротами (кроме случая, когда в папке нет ни одного файла), сироты старше
// retention удаляются (0 — бессрочно). Всё — одной транзакцией.
func (s *Store) Reconcile(root string, files map[string]string, now time.Time, retention time.Duration) error {
	var moved, orphaned, deleted int64
	err := s.write(func(tx *sql.Tx) error {
		recs, err := loadWorks(tx, root)
		if err != nil {
			return err
		}
		// 1. файл на месте
		known := make(map[string]bool, len(recs))
		var missing []work
		for _, r := range recs {
			known[r.rel] = true
			fp, ok := files[r.rel]
			if !ok {
				missing = append(missing, r)
				continue
			}
			if r.orphaned || (fp != "" && fp != r.fp) {
				if _, err := tx.Exec(`UPDATE works SET orphaned_at = NULL,
					fingerprint = CASE WHEN ? <> '' THEN ? ELSE fingerprint END WHERE uid = ?`,
					fp, fp, r.uid); err != nil {
					return err
				}
			}
		}

		// 2. перенос по отпечатку: ровно одна пропавшая запись и ровно один новый файл
		fresh := map[string][]string{}
		for rel, fp := range files {
			if fp != "" && !known[rel] {
				fresh[fp] = append(fresh[fp], rel)
			}
		}
		lost := map[string][]int{} // отпечаток → индексы в missing
		for i, r := range missing {
			if r.fp != "" {
				lost[r.fp] = append(lost[r.fp], i)
			}
		}
		movedIdx := map[int]bool{}
		for fp, idx := range lost {
			if len(idx) != 1 || len(fresh[fp]) != 1 {
				continue
			}
			r := missing[idx[0]]
			if _, err := tx.Exec(`UPDATE works SET rel = ?, orphaned_at = NULL WHERE uid = ?`,
				fresh[fp][0], r.uid); err != nil {
				return err
			}
			log.Printf("user data: %s moved to %s", r.rel, fresh[fp][0])
			movedIdx[idx[0]] = true
			moved++
		}

		// 3. новые сироты — только если сканирование нашло хоть один файл
		if len(files) > 0 {
			for i, r := range missing {
				if movedIdx[i] || r.orphaned {
					continue
				}
				if _, err := tx.Exec(`UPDATE works SET orphaned_at = ? WHERE uid = ?`, now.UnixNano(), r.uid); err != nil {
					return err
				}
				orphaned++
			}
		}

		// 4. срок хранения сирот истёк
		if retention > 0 {
			res, err := tx.Exec(`DELETE FROM works WHERE root = ? AND orphaned_at IS NOT NULL AND orphaned_at < ?`,
				root, now.Add(-retention).UnixNano())
			if err != nil {
				return err
			}
			deleted, _ = res.RowsAffected()
		}
		return nil
	})
	if err == nil && moved+orphaned+deleted > 0 {
		log.Printf("user data: %d moved, %d orphaned, %d deleted", moved, orphaned, deleted)
	}
	return err
}

func loadWorks(tx *sql.Tx, root string) ([]work, error) {
	rows, err := tx.Query(`SELECT uid, rel, fingerprint, orphaned_at IS NOT NULL FROM works WHERE root = ?`, root)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var recs []work
	for rows.Next() {
		var r work
		if err := rows.Scan(&r.uid, &r.rel, &r.fp, &r.orphaned); err != nil {
			return nil, err
		}
		recs = append(recs, r)
	}
	return recs, rows.Err()
}

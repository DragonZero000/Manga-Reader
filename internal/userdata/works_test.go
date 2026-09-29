package userdata

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

const day = 24 * time.Hour

// get возвращает запись (root, rel); uid = 0 — записи нет.
func get(t *testing.T, s *Store, root, rel string) (w struct {
	uid    int64
	fp     string
	orphan int64 // UnixNano; 0 — не сирота
}) {
	t.Helper()
	var orphan *int64
	err := s.db.QueryRow(`SELECT uid, fingerprint, orphaned_at FROM works WHERE root = ? AND rel = ?`,
		root, rel).Scan(&w.uid, &w.fp, &orphan)
	if err != nil && !strings.Contains(err.Error(), "no rows") {
		t.Fatal(err)
	}
	if orphan != nil {
		w.orphan = *orphan
	}
	return w
}

// dump — все записи: «root|rel|fp|orphan», где orphan — «-» или сколько
// дней назад запись помечена сиротой относительно now.
func dump(t *testing.T, s *Store, now time.Time) []string {
	t.Helper()
	rows, err := s.db.Query(`SELECT root, rel, fingerprint, orphaned_at FROM works`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var root, rel, fp string
		var orphan *int64
		if err := rows.Scan(&root, &rel, &fp, &orphan); err != nil {
			t.Fatal(err)
		}
		o := "-"
		if orphan != nil {
			o = fmt.Sprintf("%dd", now.Sub(time.Unix(0, *orphan))/day)
		}
		out = append(out, strings.Join([]string{root, rel, fp, o}, "|"))
	}
	sort.Strings(out)
	return out
}

type seed struct {
	root, rel, fp string
	orphan        time.Duration // сколько назад помечена сиротой; 0 — не сирота
}

func TestReconcile(t *testing.T) {
	const root = "app:manga"
	for _, tc := range []struct {
		name      string
		seeds     []seed
		files     map[string]string
		retention time.Duration
		want      []string
	}{
		{
			name:  "переименование при закрытом приложении",
			seeds: []seed{{root, "a.zip", "A", 0}},
			files: map[string]string{"b.zip": "A", "x.zip": "X"},
			want:  []string{root + "|b.zip|A|-"},
		},
		{
			name:  "переименование при работающем приложении",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "c.zip", "C", 0}},
			files: map[string]string{"sub/b.zip": "A", "c.zip": "C"},
			want:  []string{root + "|c.zip|C|-", root + "|sub/b.zip|A|-"},
		},
		{
			name:  "перенос сироты прошлых сканов",
			seeds: []seed{{root, "a.zip", "A", 5 * day}},
			files: map[string]string{"b.zip": "A"},
			want:  []string{root + "|b.zip|A|-"},
		},
		{
			name:  "дубликаты: два новых файла",
			seeds: []seed{{root, "a.zip", "A", 0}},
			files: map[string]string{"b.zip": "A", "c.zip": "A"},
			want:  []string{root + "|a.zip|A|0d"},
		},
		{
			name:  "дубликаты: две пропавшие записи",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "a2.zip", "A", 0}},
			files: map[string]string{"b.zip": "A"},
			want:  []string{root + "|a.zip|A|0d", root + "|a2.zip|A|0d"},
		},
		{
			name:  "файл с тем же отпечатком уже имеет запись",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "b.zip", "A", 0}},
			files: map[string]string{"b.zip": "A"},
			want:  []string{root + "|a.zip|A|0d", root + "|b.zip|A|-"},
		},
		{
			name:  "изменён meta.json — отпечаток тот же",
			seeds: []seed{{root, "a.zip", "A", 0}},
			files: map[string]string{"a.zip": "A"},
			want:  []string{root + "|a.zip|A|-"},
		},
		{
			name:  "изменены страницы — отпечаток обновлён",
			seeds: []seed{{root, "a.zip", "A", 0}},
			files: map[string]string{"a.zip": "A2"},
			want:  []string{root + "|a.zip|A2|-"},
		},
		{
			name:  "ошибочный или занятый файл считается найденным",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "b.zip", "B", 3 * day}},
			files: map[string]string{"a.zip": "", "b.zip": ""},
			want:  []string{root + "|a.zip|A|-", root + "|b.zip|B|-"},
		},
		{
			name:  "файл без отпечатка не забирает запись",
			seeds: []seed{{root, "a.zip", "A", 0}},
			files: map[string]string{"b.zip": ""},
			want:  []string{root + "|a.zip|A|0d"},
		},
		{
			name:  "пустая папка — новых сирот нет",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "b.zip", "B", 0}},
			files: map[string]string{},
			want:  []string{root + "|a.zip|A|-", root + "|b.zip|B|-"},
		},
		{
			name:      "пустая папка — истёкшие сироты удаляются",
			seeds:     []seed{{root, "a.zip", "A", 0}, {root, "b.zip", "B", 31 * day}},
			files:     map[string]string{},
			retention: 30 * day,
			want:      []string{root + "|a.zip|A|-"},
		},
		{
			name:  "маленькая библиотека",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "b.zip", "B", 0}},
			files: map[string]string{"a.zip": "A"},
			want:  []string{root + "|a.zip|A|-", root + "|b.zip|B|0d"},
		},
		{
			name:  "уже помеченная сирота сохраняет время",
			seeds: []seed{{root, "a.zip", "A", 0}, {root, "b.zip", "B", 7 * day}},
			files: map[string]string{"a.zip": "A"},
			want:  []string{root + "|a.zip|A|-", root + "|b.zip|B|7d"},
		},
		{
			name:      "возвращение файла",
			seeds:     []seed{{root, "a.zip", "A", 10 * day}},
			files:     map[string]string{"a.zip": "A"},
			retention: 30 * day,
			want:      []string{root + "|a.zip|A|-"},
		},
		{
			name:      "истёк срок",
			seeds:     []seed{{root, "a.zip", "A", 31 * day}, {root, "b.zip", "B", 29 * day}},
			files:     map[string]string{"x.zip": "X"},
			retention: 30 * day,
			want:      []string{root + "|b.zip|B|29d"},
		},
		{
			name:  "бессрочно",
			seeds: []seed{{root, "a.zip", "A", 365 * day}},
			files: map[string]string{"x.zip": "X"},
			want:  []string{root + "|a.zip|A|365d"},
		},
		{
			name: "записи другой папки не трогаются",
			seeds: []seed{
				{"tree:A", "a.zip", "A", 400 * day}, {"tree:A", "b.zip", "B", 0}, {"tree:A", "c.zip", "C", 0},
				{root, "a.zip", "A", 0},
			},
			files:     map[string]string{"a.zip": "A", "x.zip": "B"},
			retention: day,
			want: []string{
				root + "|a.zip|A|-",
				"tree:A|a.zip|A|400d", "tree:A|b.zip|B|-", "tree:A|c.zip|C|-",
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openTest(t, filepath.Join(t.TempDir(), FileName))
			now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
			for _, sd := range tc.seeds {
				if _, err := s.Ensure(sd.root, sd.rel, sd.fp); err != nil {
					t.Fatal(err)
				}
				if sd.orphan > 0 {
					if err := s.MarkOrphan(sd.root, sd.rel, now.Add(-sd.orphan)); err != nil {
						t.Fatal(err)
					}
				}
			}
			if err := s.Reconcile(root, tc.files, now, tc.retention); err != nil {
				t.Fatal(err)
			}
			got := dump(t, s, now)
			if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
				t.Fatalf("записи:\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}

// Перенос сохраняет идентификатор записи (данные ссылаются на uid).
func TestReconcileKeepsUID(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	uid, err := s.Ensure("r", "a.zip", "A")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile("r", map[string]string{"sub/b.zip": "A"}, time.Now(), 30*day); err != nil {
		t.Fatal(err)
	}
	if got, ok, _ := s.Lookup("r", "sub/b.zip"); !ok || got != uid {
		t.Fatalf("uid после переноса %d, был %d", got, uid)
	}
	if _, ok, _ := s.Lookup("r", "a.zip"); ok {
		t.Fatal("прежний путь остался")
	}
}

func TestMarkOrphan(t *testing.T) {
	s := openTest(t, filepath.Join(t.TempDir(), FileName))
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	if _, err := s.Ensure("r", "a.zip", "A"); err != nil {
		t.Fatal(err)
	}

	// удалён последний файл: папка пуста, но запись помечена
	if err := s.MarkOrphan("r", "a.zip", now); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile("r", map[string]string{}, now, 30*day); err != nil {
		t.Fatal(err)
	}
	if w := get(t, s, "r", "a.zip"); w.orphan != now.UnixNano() {
		t.Fatalf("после удаления: %+v", w)
	}

	// повторная пометка не сдвигает время
	if err := s.MarkOrphan("r", "a.zip", now.Add(day)); err != nil {
		t.Fatal(err)
	}
	if w := get(t, s, "r", "a.zip"); w.orphan != now.UnixNano() {
		t.Fatalf("повторная пометка сдвинула время: %+v", w)
	}

	// нет записи — ничего не происходит
	if err := s.MarkOrphan("r", "missing.zip", now); err != nil {
		t.Fatal(err)
	}

	// восстановление из корзины на следующий день — под тем же или другим именем
	if err := s.Reconcile("r", map[string]string{"a (1).zip": "A"}, now.Add(day), 30*day); err != nil {
		t.Fatal(err)
	}
	if w := get(t, s, "r", "a (1).zip"); w.uid == 0 || w.orphan != 0 {
		t.Fatalf("после восстановления: %+v", w)
	}
	if err := s.MarkOrphan("r", "a (1).zip", now.Add(2*day)); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile("r", map[string]string{"a (1).zip": "A"}, now.Add(3*day), 30*day); err != nil {
		t.Fatal(err)
	}
	if w := get(t, s, "r", "a (1).zip"); w.orphan != 0 {
		t.Fatalf("восстановлен под прежним именем: %+v", w)
	}
}

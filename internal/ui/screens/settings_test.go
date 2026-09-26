package screens

import (
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/storage"
)

func newTestSettings(t *testing.T, st storage.Settings) *Settings {
	t.Helper()
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, st)
	return NewSettings(a, a.NewWindow("t"), svc, func(string) {}, nil, nil)
}

func TestDisplayCardAbsentWithoutSupport(t *testing.T) {
	old := displaySupported
	displaySupported = false
	t.Cleanup(func() { displaySupported = old })
	if s := newTestSettings(t, storage.NewMemSettings()); s.DisplayCheck() != nil {
		t.Fatal("без поддержки раздела «Экран» быть не должно")
	}
}

func TestDisplayCard(t *testing.T) {
	oldSup, oldSet := displaySupported, setMax60
	var applied []bool
	displaySupported = true
	setMax60 = func(on bool) error { applied = append(applied, on); return nil }
	t.Cleanup(func() { displaySupported, setMax60 = oldSup, oldSet })

	st := storage.NewMemSettings()
	s := newTestSettings(t, st)
	c := s.DisplayCheck()
	if c == nil || !c.Checked {
		t.Fatal("по умолчанию «Ограничить 60 Гц» включено")
	}
	c.SetChecked(false) // как нажатие пользователя
	if st.String(app.KeyDisplayMax60, "") != "0" || len(applied) != 1 || applied[0] {
		t.Fatalf("выключение: настройка %q, применено %v", st.String(app.KeyDisplayMax60, ""), applied)
	}
	// после перезапуска флажок восстанавливается
	if newTestSettings(t, st).DisplayCheck().Checked {
		t.Fatal("выключенный флажок не восстановился")
	}
}

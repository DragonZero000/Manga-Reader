package ui

import (
	"path/filepath"
	"strings"
	"testing"

	"fyne.io/fyne/v2/test"

	"mangareader/internal/app"
	"mangareader/internal/library"
	"mangareader/internal/storage"
)

// Повреждённые пользовательские данные: при запуске — уведомление с именем
// резервной копии.
func TestUserDataRecoveredToast(t *testing.T) {
	a := test.NewTempApp(t)
	svc := app.NewForTest(library.NewDirSource(t.TempDir(), nil), nil, storage.NewMemSettings())
	svc.UserDataBackup = filepath.Join(t.TempDir(), "user.db.broken-20260930-142501")
	s := NewShell(a, svc)
	s.Toast.do = serial
	waitFor(t, func() bool { return isVisible(s.Toast.Layer()) })
	if txt := labelText(s.Toast); !strings.Contains(txt, "user.db.broken-20260930-142501") || !strings.Contains(txt, "повреждены") {
		t.Fatalf("уведомление %q", txt)
	}
}

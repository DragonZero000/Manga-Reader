package details

import (
	"os"
	"testing"

	"mangareader/internal/i18n"
)

// Тесты проверяют русские подписи: язык интерфейса — русский.
func TestMain(m *testing.M) {
	i18n.Init("ru")
	os.Exit(m.Run())
}

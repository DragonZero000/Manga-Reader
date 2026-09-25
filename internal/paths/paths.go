// Package paths определяет папку приложения на ПК (портативный режим).
// Пакет не зависит от UI.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppID — идентификатор приложения (совпадает с FyneApp.toml; нужен на Android).
const AppID = "io.github.mangareader.app"

// LibraryDirName — папка с архивами внутри папки приложения.
const LibraryDirName = "manga"

// SettingsFileName — файл настроек в папке приложения.
const SettingsFileName = "settings.json"

// AppDir — папка приложения: каталог исполняемого файла. При запуске через
// «go run» и «go test» исполняемый файл лежит в каталоге сборки Go
// («…/go-build…/») — тогда используется текущая рабочая папка.
// Просто «внутри TEMP» не считается: приложение могут запустить, например,
// прямо из распакованного во временную папку архива.
func AppDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return os.Getwd()
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return appDirFor(exe, os.Getwd)
}

// appDirFor — логика AppDir с подставляемыми зависимостями (для тестов).
func appDirFor(exe string, getwd func() (string, error)) (string, error) {
	if isGoBuild(exe) {
		return getwd()
	}
	return filepath.Dir(exe), nil
}

// isGoBuild — исполняемый файл собран «go run»/«go test» во временном каталоге сборки.
func isGoBuild(exe string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Clean(exe)), "/") {
		if strings.HasPrefix(strings.ToLower(seg), "go-build") {
			return true
		}
	}
	return false
}

// EnsureDir создаёт каталог вместе с родительскими, если его нет.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("создание папки %s: %w", dir, err)
	}
	return nil
}

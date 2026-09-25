package browser

import (
	"errors"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// mozillaKey — где Firefox хранит служебные значения, в том числе с путём
// установки (Launcher, PreXULSkeletonUISettings, DllPrefetchExperiment,
// Default Browser Agent) и ключ Installer\<хеш пути>.
const mozillaKey = `Software\Mozilla\Firefox`

// installerKeys — подключи HKCU\Software\Mozilla\Firefox\Installer.
func installerKeys() []string {
	return subkeys(registry.CURRENT_USER, mozillaKey+`\Installer`)
}

// CleanRegistry убирает записи, которые Firefox из firefoxDir оставил в
// HKCU\Software\Mozilla\Firefox: значения, имя которых начинается с пути
// firefoxDir, и новые пустые подключи Installer (появившиеся после before;
// при before == nil они не трогаются). Значения других установок Firefox
// не затрагиваются. Возвращает число удалённых записей.
func CleanRegistry(firefoxDir string, before []string) (int, error) {
	return cleanRegistry(registry.CURRENT_USER, mozillaKey, firefoxDir, before)
}

func cleanRegistry(root registry.Key, path, firefoxDir string, before []string) (int, error) {
	dir := filepath.Clean(firefoxDir)
	n, err := cleanValues(root, path, dir)
	if before != nil {
		known := map[string]bool{}
		for _, k := range before {
			known[strings.ToLower(k)] = true
		}
		inst := path + `\Installer`
		for _, k := range subkeys(root, inst) {
			if known[strings.ToLower(k)] || !emptyKey(root, inst+`\`+k) {
				continue
			}
			if e := registry.DeleteKey(root, inst+`\`+k); e == nil {
				n++
			} else {
				err = errors.Join(err, e)
			}
		}
	}
	return n, err
}

// cleanValues рекурсивно удаляет значения, имя которых — путь внутри dir
// (dir, dir\…, dir|…).
func cleanValues(root registry.Key, path, dir string) (int, error) {
	k, err := registry.OpenKey(root, path, registry.READ|registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer k.Close()
	names, err := k.ReadValueNames(-1)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names {
		if !underDir(name, dir) {
			continue
		}
		if e := k.DeleteValue(name); e == nil {
			n++
		} else {
			err = errors.Join(err, e)
		}
	}
	for _, sub := range subkeys(root, path) {
		m, e := cleanValues(root, path+`\`+sub, dir)
		n += m
		err = errors.Join(err, e)
	}
	return n, err
}

// underDir — имя значения относится к установке dir: сам путь или путь,
// продолжающийся «\» или «|» (так Firefox формирует имена значений).
func underDir(name, dir string) bool {
	if len(name) < len(dir) || !strings.EqualFold(name[:len(dir)], dir) {
		return false
	}
	rest := name[len(dir):]
	return rest == "" || rest[0] == '\\' || rest[0] == '|'
}

func subkeys(root registry.Key, path string) []string {
	k, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadSubKeyNames(-1)
	return names
}

func emptyKey(root registry.Key, path string) bool {
	k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	info, err := k.Stat()
	return err == nil && info.SubKeyCount == 0 && info.ValueCount == 0
}

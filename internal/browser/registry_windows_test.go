package browser

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"golang.org/x/sys/windows/registry"
)

// testKey создаёт временный ключ HKCU\Software\MangaReaderTest-* и удаляет
// его после теста.
func testKey(t *testing.T) string {
	t.Helper()
	path := fmt.Sprintf(`Software\MangaReaderTest-%d`, time.Now().UnixNano())
	t.Cleanup(func() { deleteTree(registry.CURRENT_USER, path) })
	return path
}

func deleteTree(root registry.Key, path string) {
	for _, sub := range subkeys(root, path) {
		deleteTree(root, path+`\`+sub)
	}
	registry.DeleteKey(root, path)
}

func setValues(t *testing.T, path string, names ...string) {
	t.Helper()
	k, _, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	defer k.Close()
	for _, n := range names {
		if err := k.SetDWordValue(n, 1); err != nil {
			t.Fatal(err)
		}
	}
}

func values(t *testing.T, path string) []string {
	t.Helper()
	k, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE)
	if err != nil {
		return nil
	}
	defer k.Close()
	names, _ := k.ReadValueNames(-1)
	sort.Strings(names)
	return names
}

func TestCleanRegistry(t *testing.T) {
	root := testKey(t)
	ours := `C:\App\browser\firefox`
	other := `C:\Program Files\Firefox Developer Edition`
	setValues(t, root+`\Launcher`, ours+`\firefox.exe|Launcher`, `c:\app\BROWSER\firefox\firefox.exe|Image`, other+`\firefox.exe|Launcher`)
	setValues(t, root+`\Default Browser Agent`, ours+`|DisableTelemetry`, other+`|DisableTelemetry`, "CurrentDefault")
	setValues(t, root+`\PreXULSkeletonUISettings`, ours+`2\firefox.exe|Progress`) // другая папка с тем же префиксом
	setValues(t, root+`\Installer\OLDHASH`)
	before := subkeys(registry.CURRENT_USER, root+`\Installer`)
	setValues(t, root+`\Installer\NEWHASH`)
	setValues(t, root+`\Installer\NEWFULL`, "x") // новый, но не пустой — не наш

	n, err := cleanRegistry(registry.CURRENT_USER, root, ours, before)
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Errorf("удалено %d, ожидалось 4 (3 значения + ключ NEWHASH)", n)
	}
	if got := values(t, root+`\Launcher`); len(got) != 1 || got[0] != other+`\firefox.exe|Launcher` {
		t.Errorf("Launcher: %v", got)
	}
	if got := values(t, root+`\Default Browser Agent`); len(got) != 2 {
		t.Errorf("Default Browser Agent: %v", got)
	}
	if got := values(t, root+`\PreXULSkeletonUISettings`); len(got) != 1 {
		t.Errorf("значение другой папки удалено: %v", got)
	}
	inst := subkeys(registry.CURRENT_USER, root+`\Installer`)
	sort.Strings(inst)
	if fmt.Sprint(inst) != "[NEWFULL OLDHASH]" {
		t.Errorf("Installer: %v", inst)
	}

	// без списка «до» ключи Installer не трогаются
	setValues(t, root+`\Installer\ANOTHER`)
	if _, err := cleanRegistry(registry.CURRENT_USER, root, ours, nil); err != nil {
		t.Fatal(err)
	}
	if len(subkeys(registry.CURRENT_USER, root+`\Installer`)) != 3 {
		t.Error("при before == nil ключи Installer удаляться не должны")
	}
	// нет ключа — не ошибка
	if n, err := cleanRegistry(registry.CURRENT_USER, root+`\нет`, ours, nil); n != 0 || err != nil {
		t.Errorf("нет ключа: %d %v", n, err)
	}
}

// Команда android-build собирает Android-пакет MangaReader через Gradle:
// Go-часть — в libmangareader.so для каждой архитектуры (clang из NDK 27+),
// Java-классы Fyne — из модуля Fyne той версии, что в go.mod, затем
// gradlew assembleDebug/assembleRelease в папке android/ и копия APK в dist/.
//
//	go run ./tools/android-build [-abis arm64-v8a,armeabi-v7a] [-release [-require-signing]] [-tags frameprobe]
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"

	"mangareader/internal/appversion"
)

// minSdk — минимальная версия Android (API 30 = Android 11); clang из NDK
// выбирается под неё.
const minSdk = 30

// minNDK — минимальная версия NDK (выравнивание по 16 КБ, свежий clang).
const minNDK = 27

// abi — соответствие архитектуры Android настройкам Go и clang из NDK.
type abi struct {
	goarch, goarm, clang string
}

var abis = map[string]abi{
	"arm64-v8a":   {"arm64", "", "aarch64-linux-android"},
	"armeabi-v7a": {"arm", "7", "armv7a-linux-androideabi"},
	"x86_64":      {"amd64", "", "x86_64-linux-android"},
	"x86":         {"386", "", "i686-linux-android"},
}

func main() {
	abiList := flag.String("abis", "arm64-v8a", "архитектуры через запятую или пробел")
	release := flag.Bool("release", false, "release-сборка (ключ из MANGAREADER_KEYSTORE…)")
	requireSigning := flag.Bool("require-signing", false, "с -release: ошибка, если ключ не задан (для релизов)")
	out := flag.String("o", filepath.Join("dist", "mangareader.apk"), "куда положить APK")
	extraTags := flag.String("tags", "", "дополнительные теги сборки через запятую (например, frameprobe)")
	flag.Parse()
	log.SetFlags(0)
	if err := checkSigning(*release, *requireSigning, os.Getenv("MANGAREADER_KEYSTORE")); err != nil {
		log.Fatal("android-build: ", err)
	}
	if err := run(splitABIs(*abiList), *release, *out, buildTags(*extraTags)); err != nil {
		log.Fatal("android-build: ", err)
	}
}

// checkSigning проверяет флаги подписи до сборки: релиз проекта не должен
// выйти неподписанным — такой APK не ставится поверх предыдущего.
func checkSigning(release, require bool, keystore string) error {
	if require && !release {
		return errors.New("флаг -require-signing имеет смысл только с -release")
	}
	if require && keystore == "" {
		return errors.New("не задан ключ подписи: нужны MANGAREADER_KEYSTORE, MANGAREADER_KEYSTORE_PASSWORD, MANGAREADER_KEY_ALIAS, MANGAREADER_KEY_PASSWORD")
	}
	return nil
}

func run(list []string, release bool, out, tags string) error {
	if len(list) == 0 {
		return errors.New("не заданы архитектуры")
	}
	for _, a := range list {
		if _, ok := abis[a]; !ok {
			return fmt.Errorf("неизвестная архитектура %q (есть: %s)", a, strings.Join(knownABIs(), ", "))
		}
	}
	ver, err := appversion.Read("FyneApp.toml")
	if err != nil {
		return err
	}
	version, build := ver.String(), ver.Code()
	sdk, err := androidHome()
	if err != nil {
		return err
	}
	ndk, err := findNDK(sdk, os.Getenv("ANDROID_NDK_HOME"))
	if err != nil {
		return err
	}
	jdk, err := findJDK(os.Getenv("JAVA_HOME"), androidStudioJBR())
	if err != nil {
		return err
	}
	log.Printf("версия %s (%d), архитектуры %s", version, build, strings.Join(list, ", "))
	log.Printf("NDK %s, JDK %s", ndk, jdk)

	src := filepath.Join("android", "app", "src", "main")
	if err := copyFyneJava(filepath.Join(src, "java", "org", "golang", "app")); err != nil {
		return err
	}
	if err := copyFile("Icon.png", filepath.Join(src, "res", "mipmap-xxxhdpi", "ic_launcher.png")); err != nil {
		return fmt.Errorf("иконка: %w", err)
	}
	// лицензии — в assets/ APK (копии генерируются, в репозитории их нет)
	for _, f := range []string{"LICENSE", "THIRD_PARTY_NOTICES.md"} {
		if err := copyFile(f, filepath.Join(src, "assets", f)); err != nil {
			return fmt.Errorf("лицензии: %w", err)
		}
	}
	jni := filepath.Join(src, "jniLibs")
	if err := os.RemoveAll(jni); err != nil { // без библиотек прежних архитектур
		return err
	}
	for _, a := range list {
		if err := buildLib(ndk, a, filepath.Join(jni, a), version, build, tags); err != nil {
			return err
		}
	}

	task, apk := "assembleDebug", filepath.Join("app", "build", "outputs", "apk", "debug", "app-debug.apk")
	if release {
		task, apk = "assembleRelease", filepath.Join("app", "build", "outputs", "apk", "release", "app-release.apk")
		if os.Getenv("MANGAREADER_KEYSTORE") == "" {
			apk = filepath.Join("app", "build", "outputs", "apk", "release", "app-release-unsigned.apk")
			log.Print("MANGAREADER_KEYSTORE не задан — release-APK будет без подписи")
		}
	}
	args := []string{task, "--console=plain",
		"-PmangareaderAbis=" + strings.Join(list, ","), "-PversionName=" + version, "-PversionCode=" + strconv.Itoa(build)}
	// абсолютный путь: при NoDefaultCurrentDirectoryInExePath cmd.exe не
	// запускает программы из текущей папки по имени
	name := "gradlew"
	if runtime.GOOS == "windows" {
		name = "gradlew.bat"
	}
	gradlew, err := filepath.Abs(filepath.Join("android", name))
	if err != nil {
		return err
	}
	cmd := exec.Command(gradlew, args...)
	cmd.Dir = "android"
	cmd.Env = append(os.Environ(), "JAVA_HOME="+jdk)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	log.Printf("gradle %s", task)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gradle %s: %w", task, err)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	if err := copyFile(filepath.Join("android", apk), out); err != nil {
		return err
	}
	log.Printf("готово: %s", out)
	return nil
}

// buildTags — теги сборки библиотеки: migrated_fynedo и дополнительные
// (через запятую или пробел), без повторов.
func buildTags(extra string) string {
	tags := []string{"migrated_fynedo"}
	for _, t := range splitABIs(extra) {
		if !slices.Contains(tags, t) {
			tags = append(tags, t)
		}
	}
	return strings.Join(tags, ",")
}

func splitABIs(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' || r == ';' })
}

func knownABIs() []string {
	var out []string
	for k := range abis {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func androidHome() (string, error) {
	for _, k := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(k); v != "" {
			return v, nil
		}
	}
	if runtime.GOOS == "windows" {
		if d := filepath.Join(os.Getenv("LOCALAPPDATA"), "Android", "Sdk"); dirExists(d) {
			return d, nil
		}
	}
	return "", errors.New("не найден Android SDK: задайте ANDROID_HOME (Android Studio → Settings → Android SDK → Android SDK Location)")
}

// findNDK — ANDROID_NDK_HOME или самая новая версия из <sdk>/ndk не ниже minNDK.
func findNDK(sdk, env string) (string, error) {
	hint := fmt.Sprintf("установите NDK %d или новее: Android Studio → Settings → Android SDK → SDK Tools → NDK (Side by side)", minNDK)
	if env != "" {
		if ndkMajor(filepath.Base(env)) < minNDK {
			return "", fmt.Errorf("ANDROID_NDK_HOME=%s старее NDK %d; %s", env, minNDK, hint)
		}
		return env, nil
	}
	entries, _ := os.ReadDir(filepath.Join(sdk, "ndk"))
	best, bestVer := "", []int(nil)
	for _, e := range entries {
		v := versionParts(e.Name())
		if !e.IsDir() || len(v) == 0 || v[0] < minNDK {
			continue
		}
		if bestVer == nil || lessVersion(bestVer, v) {
			best, bestVer = filepath.Join(sdk, "ndk", e.Name()), v
		}
	}
	if best == "" {
		return "", errors.New("не найден NDK: " + hint)
	}
	return best, nil
}

func ndkMajor(name string) int {
	if v := versionParts(name); len(v) > 0 {
		return v[0]
	}
	return 0
}

func versionParts(s string) []int {
	var out []int
	for _, p := range strings.Split(s, ".") {
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func lessVersion(a, b []int) bool {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

// findJDK — JAVA_HOME, если это JDK 17+, иначе JBR из Android Studio.
func findJDK(javaHome, jbr string) (string, error) {
	if javaHome != "" && jdkMajor(javaHome) >= 17 {
		return javaHome, nil
	}
	if jbr != "" && jdkMajor(jbr) >= 17 {
		return jbr, nil
	}
	return "", fmt.Errorf("нужен JDK 17+ (JAVA_HOME=%q не подходит, JBR Android Studio не найден): установите Android Studio или задайте JAVA_HOME", javaHome)
}

var reJavaVersion = regexp.MustCompile(`JAVA_VERSION="(\d+)`)

// jdkMajor — основная версия JDK по файлу release в его папке (0 — не JDK).
func jdkMajor(dir string) int {
	data, err := os.ReadFile(filepath.Join(dir, "release"))
	if err != nil {
		return 0
	}
	m := reJavaVersion.FindSubmatch(data)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(string(m[1]))
	if n == 1 { // JDK 8: "1.8.0"
		return 8
	}
	return n
}

func androidStudioJBR() string {
	var candidates []string
	switch runtime.GOOS {
	case "windows":
		candidates = []string{
			filepath.Join(os.Getenv("ProgramFiles"), "Android", "Android Studio", "jbr"),
			filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Android Studio", "jbr"),
		}
	case "darwin":
		candidates = []string{"/Applications/Android Studio.app/Contents/jbr/Contents/Home"}
	default:
		candidates = []string{"/opt/android-studio/jbr", filepath.Join(os.Getenv("HOME"), "android-studio", "jbr")}
	}
	for _, c := range candidates {
		if dirExists(c) {
			return c
		}
	}
	return ""
}

// copyFyneJava копирует Java-классы Fyne из модуля той версии, что в go.mod.
func copyFyneJava(dst string) error {
	dir, err := moduleDir("fyne.io/fyne/v2")
	if err != nil {
		return err
	}
	src := filepath.Join(dir, "internal", "driver", "mobile", "app")
	for _, name := range []string{"GoNativeActivity.java", "FyneNotificationReceiver.java"} {
		if err := copyFile(filepath.Join(src, name), filepath.Join(dst, name)); err != nil {
			return fmt.Errorf("Java-классы Fyne: %w", err)
		}
	}
	return nil
}

// moduleDir — папка модуля той версии, что в go.mod. «go list -m» на чистом
// кэше модулей (CI) возвращает пустой Dir, а «go mod download» сначала скачивает.
func moduleDir(path string) (string, error) {
	out, err := exec.Command("go", "mod", "download", "-json", path).Output()
	if err != nil {
		return "", fmt.Errorf("модуль %s: %w", path, err)
	}
	return parseModuleDir(path, out)
}

func parseModuleDir(path string, data []byte) (string, error) {
	var m struct{ Dir, Error string }
	if err := json.Unmarshal(data, &m); err != nil {
		return "", fmt.Errorf("модуль %s: %w", path, err)
	}
	if m.Error != "" {
		return "", fmt.Errorf("модуль %s: %s", path, m.Error)
	}
	if m.Dir == "" {
		return "", fmt.Errorf("модуль %s: go mod download не сообщил папку", path)
	}
	return m.Dir, nil
}

// buildLib собирает libmangareader.so для архитектуры a в каталог dir.
func buildLib(ndk, a, dir, version string, build int, tags string) error {
	cfg := abis[a]
	cc, err := clangPath(ndk, cfg.clang)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	lib := filepath.Join(dir, "libmangareader.so")
	ldflags := fmt.Sprintf("-s -w -X main.version=%s -X main.build=%d -extldflags=-Wl,-z,max-page-size=16384", version, build)
	cmd := exec.Command("go", "build", "-buildmode=c-shared", "-tags", tags, "-ldflags", ldflags, "-o", lib, "./cmd/mangareader")
	cmd.Env = append(os.Environ(), "CGO_ENABLED=1", "GOOS=android", "GOARCH="+cfg.goarch, "CC="+cc)
	if cfg.goarm != "" {
		cmd.Env = append(cmd.Env, "GOARM="+cfg.goarm)
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	log.Printf("go build %s", a)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build %s: %w", a, err)
	}
	os.Remove(filepath.Join(dir, "libmangareader.h")) // заголовок c-shared не нужен в APK
	return nil
}

func clangPath(ndk, prefix string) (string, error) {
	host := map[string]string{"windows": "windows-x86_64", "darwin": "darwin-x86_64", "linux": "linux-x86_64"}[runtime.GOOS]
	name := fmt.Sprintf("%s%d-clang", prefix, minSdk)
	if runtime.GOOS == "windows" {
		name += ".cmd"
	}
	p := filepath.Join(ndk, "toolchains", "llvm", "prebuilt", host, "bin", name)
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("в NDK нет %s: %w", name, err)
	}
	return p, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

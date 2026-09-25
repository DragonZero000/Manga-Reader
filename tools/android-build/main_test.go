package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitABIs(t *testing.T) {
	got := strings.Join(splitABIs("arm64-v8a, armeabi-v7a x86_64"), "|")
	if got != "arm64-v8a|armeabi-v7a|x86_64" {
		t.Fatalf("%s", got)
	}
}

func TestCheckSigning(t *testing.T) {
	cases := []struct {
		release, require bool
		keystore         string
		wantErr          bool
	}{
		{false, false, "", false},
		{true, false, "", false}, // make build-android-release без ключа — неподписанный APK
		{true, true, "k.jks", false},
		{true, true, "", true},
		{false, true, "k.jks", true}, // -require-signing без -release
	}
	for _, c := range cases {
		if err := checkSigning(c.release, c.require, c.keystore); (err != nil) != c.wantErr {
			t.Errorf("release=%v require=%v keystore=%q: %v", c.release, c.require, c.keystore, err)
		}
	}
}

func TestParseModuleDir(t *testing.T) {
	if d, err := parseModuleDir("m", []byte(`{"Path":"m","Version":"v1.0.0","Dir":"/cache/m@v1.0.0"}`)); err != nil || d != "/cache/m@v1.0.0" {
		t.Errorf("%q %v", d, err)
	}
	for _, in := range []string{`{"Path":"m"}`, `{"Path":"m","Error":"not found"}`, `not json`} {
		if _, err := parseModuleDir("m", []byte(in)); err == nil {
			t.Errorf("%s: ожидалась ошибка", in)
		}
	}
}

func TestFindNDK(t *testing.T) {
	sdk := t.TempDir()
	for _, v := range []string{"21.4.7075529", "27.3.13750724", "27.10.1", "readme"} {
		os.MkdirAll(filepath.Join(sdk, "ndk", v), 0o755)
	}
	got, err := findNDK(sdk, "")
	if err != nil || filepath.Base(got) != "27.10.1" {
		t.Fatalf("самый новый NDK ≥27: %s %v", got, err)
	}
	if _, err := findNDK(sdk, filepath.Join(sdk, "ndk", "21.4.7075529")); err == nil {
		t.Error("ANDROID_NDK_HOME со старым NDK — ошибка")
	}
	old := t.TempDir()
	os.MkdirAll(filepath.Join(old, "ndk", "21.4.7075529"), 0o755)
	if _, err := findNDK(old, ""); err == nil || !strings.Contains(err.Error(), "SDK Tools") {
		t.Errorf("только старый NDK — подсказка: %v", err)
	}
}

func TestFindJDK(t *testing.T) {
	jdk := func(ver string) string {
		d := t.TempDir()
		os.WriteFile(filepath.Join(d, "release"), []byte("IMPLEMENTOR=\"x\"\nJAVA_VERSION=\""+ver+"\"\n"), 0o644)
		return d
	}
	j11, j21, j8 := jdk("11.0.30"), jdk("21.0.4"), jdk("1.8.0_402")
	if got, err := findJDK(j11, j21); err != nil || got != j21 {
		t.Errorf("JAVA_HOME=11 → JBR 21: %s %v", got, err)
	}
	if got, err := findJDK(j21, ""); err != nil || got != j21 {
		t.Errorf("JAVA_HOME=21: %s %v", got, err)
	}
	if _, err := findJDK(j8, ""); err == nil {
		t.Error("JDK 8 без JBR — ошибка")
	}
	if jdkMajor(j8) != 8 {
		t.Errorf("1.8 → 8: %d", jdkMajor(j8))
	}
}

func TestKnownABIs(t *testing.T) {
	if err := run([]string{"mips"}, false, "x.apk"); err == nil || !strings.Contains(err.Error(), "arm64-v8a") {
		t.Fatalf("неизвестная архитектура: %v", err)
	}
}

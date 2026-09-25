// Package appversion читает версию приложения из FyneApp.toml — единственного
// источника версии — и вычисляет из неё номер сборки Android (versionCode).
// Пакет не зависит от UI и сторонних библиотек.
package appversion

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// Ограничения частей версии: versionCode = Major*10000 + Minor*100 + Patch
// растёт вместе с версией, только пока Minor и Patch меньше 100, и не должен
// превышать предел Android (2100000000).
const (
	maxMajor = 2099
	maxPart  = 99
)

// Version — версия вида MAJOR.MINOR.PATCH.
type Version struct {
	Major, Minor, Patch int
}

var (
	reVersion = regexp.MustCompile(`(?m)^\s*Version\s*=\s*"([^"]*)"`)
	reFormat  = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`)
)

// Parse разбирает строку версии. Допускается только MAJOR.MINOR.PATCH без
// ведущих нулей и суффиксов, MINOR и PATCH — не больше 99, MAJOR — не больше 2099.
func Parse(s string) (Version, error) {
	m := reFormat.FindStringSubmatch(s)
	if m == nil {
		return Version{}, fmt.Errorf("версия %q: ожидается MAJOR.MINOR.PATCH, например 1.2.3 (без суффиксов и ведущих нулей)", s)
	}
	var p [3]int
	for i := range p {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("версия %q: %w", s, err)
		}
		p[i] = n
	}
	v := Version{p[0], p[1], p[2]}
	if v.Major > maxMajor || v.Minor > maxPart || v.Patch > maxPart {
		return Version{}, fmt.Errorf("версия %q: MAJOR не больше %d, MINOR и PATCH не больше %d", s, maxMajor, maxPart)
	}
	return v, nil
}

// Read читает поле Version из FyneApp.toml по пути path.
func Read(path string) (Version, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Version{}, fmt.Errorf("метаданные: %w (запускайте из корня проекта)", err)
	}
	m := reVersion.FindSubmatch(data)
	if m == nil {
		return Version{}, fmt.Errorf("в %s нет поля Version", path)
	}
	v, err := Parse(string(m[1]))
	if err != nil {
		return Version{}, fmt.Errorf("%s: %w", path, err)
	}
	return v, nil
}

// String — версия в виде MAJOR.MINOR.PATCH.
func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Code — номер сборки Android (versionCode): Major*10000 + Minor*100 + Patch.
func (v Version) Code() int {
	return v.Major*10000 + v.Minor*100 + v.Patch
}

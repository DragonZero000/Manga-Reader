// Package i18n — перевод интерфейса: тексты из встроенных файлов
// locales/<язык>.json, выбор языка, формы множественного числа и форматы
// дат, чисел и размеров. Язык задаётся один раз при запуске (Init); до этого
// действует English. Пакет не зависит от виджетов.
//
// Файл переводов — JSON в формате go-i18n: «ключ → текст» или «ключ →
// {"one": …, "few": …, "many": …, "other": …}» для форм множественного числа.
// Подстановки — шаблоны {{.Имя}}. Ключ "_name" — название языка на нём самом.
package i18n

import (
	"embed"
	"encoding/json"
	"errors"
	"log"
	"path"
	"sort"
	"strings"
	"sync/atomic"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// Fallback — язык по умолчанию: его тексты показываются, если в выбранном
// языке нет ключа или язык системы не поддерживается.
const Fallback = "en"

// Auto — значение настройки языка «как в системе».
const Auto = "auto"

// keyName — ключ с названием языка на нём самом.
const keyName = "_name"

//go:embed locales/*.json
var localesFS embed.FS

var (
	bundle *goi18n.Bundle
	// codes — языки из файлов переводов, по алфавиту.
	codes []string
	// current — язык и локализатор; меняются только в Init.
	current atomic.Pointer[state]
)

type state struct {
	lang string
	loc  *goi18n.Localizer
}

func init() {
	bundle = goi18n.NewBundle(language.English)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)
	files, err := localesFS.ReadDir("locales")
	if err != nil {
		panic("i18n: " + err.Error())
	}
	for _, f := range files {
		name := "locales/" + f.Name()
		data, err := localesFS.ReadFile(name)
		if err != nil {
			panic("i18n: " + err.Error())
		}
		if _, err := bundle.ParseMessageFileBytes(data, name); err != nil {
			panic("i18n: " + name + ": " + err.Error())
		}
		codes = append(codes, strings.TrimSuffix(f.Name(), path.Ext(f.Name())))
	}
	sort.Strings(codes)
	Init(Fallback)
}

// system — системная локаль, переданная в Start ("ru-RU").
var system atomic.Value

// Start задаёт язык при запуске по настройке setting (код языка или Auto) и
// системной локали systemLocale (см. Resolve) и запоминает системную локаль.
func Start(setting, systemLocale string) {
	system.Store(systemLocale)
	Init(Resolve(setting, systemLocale))
}

// SystemLocale — системная локаль, переданная в Start ("" — не задана).
func SystemLocale() string {
	s, _ := system.Load().(string)
	return s
}

// Init задаёт язык интерфейса: код из Available (например, "ru"); неизвестный
// код — Fallback. Вызывается при запуске до создания окна (и в тестах).
func Init(lang string) {
	if !IsAvailable(lang) {
		lang = Fallback
	}
	current.Store(&state{lang: lang, loc: goi18n.NewLocalizer(bundle, lang, Fallback)})
}

// Lang — текущий язык интерфейса.
func Lang() string { return current.Load().lang }

// Language — язык из файлов переводов.
type Language struct {
	Code string // "en", "ru"
	Name string // название на самом языке: "English", "Русский"
}

// Available — языки, для которых есть файл переводов.
func Available() []Language {
	out := make([]Language, 0, len(codes))
	for _, c := range codes {
		out = append(out, Language{Code: c, Name: localize(goi18n.NewLocalizer(bundle, c), keyName, nil, nil)})
	}
	return out
}

// IsAvailable сообщает, есть ли файл переводов для языка lang.
func IsAvailable(lang string) bool {
	for _, c := range codes {
		if c == lang {
			return true
		}
	}
	return false
}

// Resolve выбирает язык интерфейса: явно выбранный в настройке setting
// (если он есть среди переводов); при Auto или пустой настройке — основной
// язык системной локали system ("ru-RU" → "ru"), если он есть среди
// переводов; иначе Fallback.
func Resolve(setting, system string) string {
	if setting != "" && setting != Auto {
		if IsAvailable(setting) {
			return setting
		}
		return Fallback
	}
	if base := baseLanguage(system); IsAvailable(base) {
		return base
	}
	return Fallback
}

// baseLanguage — основной язык локали: "ru-RU" → "ru", "en_GB" → "en".
func baseLanguage(locale string) string {
	tag, err := language.Parse(strings.ReplaceAll(locale, "_", "-"))
	if err != nil {
		return ""
	}
	base, _ := tag.Base()
	return base.String()
}

// T — текст по ключу id на текущем языке. kv — пары «имя, значение» для
// подстановок {{.Имя}}.
func T(id string, kv ...any) string {
	return localize(current.Load().loc, id, data(kv), nil)
}

// TIn — текст по ключу id на языке lang, а не на текущем (например,
// подсказка на только что выбранном языке).
func TIn(lang, id string, kv ...any) string {
	return localize(goi18n.NewLocalizer(bundle, lang, Fallback), id, data(kv), nil)
}

// N — текст с формой множественного числа для count; в шаблоне число
// доступно как {{.Count}}, kv — дополнительные подстановки.
func N(id string, count int, kv ...any) string {
	d := data(kv)
	d["Count"] = count
	return localize(current.Load().loc, id, d, count)
}

func data(kv []any) map[string]any {
	d := make(map[string]any, len(kv)/2+1)
	for i := 0; i+1 < len(kv); i += 2 {
		if k, ok := kv[i].(string); ok {
			d[k] = kv[i+1]
		}
	}
	return d
}

func localize(loc *goi18n.Localizer, id string, d map[string]any, count any) string {
	cfg := &goi18n.LocalizeConfig{MessageID: id, TemplateData: d}
	if count != nil {
		cfg.PluralCount = count
	}
	msg, err := loc.Localize(cfg)
	if msg != "" {
		// ключа нет в выбранном языке — msg уже на Fallback
		return msg
	}
	var nf *goi18n.MessageNotFoundErr
	if errors.As(err, &nf) {
		log.Printf("i18n: no text for key %q", id)
	} else if err != nil {
		log.Printf("i18n: %s: %v", id, err)
	}
	return id
}

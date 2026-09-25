// Package browser — встроенный браузер на основе портативного Firefox ESR
// (Windows): настройки профиля, встроенное расширение и мост к приложению,
// запуск, привязка окна к главному окну приложения, уборка следов.
// Пакет не зависит от виджетов.
package browser

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Очистки данных, выполняемые Firefox при следующем запуске.
const (
	ClearCookies = "cookies"
	ClearHistory = "history"
)

// sanitizeItems — что очищает Firefox для каждой очистки.
var sanitizeItems = map[string][]string{
	ClearCookies: {"cookies", "offlineApps", "cache"},
	ClearHistory: {"history", "formdata", "downloads"},
}

// Config — всё, из чего генерируются настройки профиля.
type Config struct {
	FirefoxDir  string // папка firefox.exe
	ProfileDir  string // профиль
	DownloadDir string // папка загрузок (папка библиотеки)
	Home        string // домашняя страница; «» — стартовая страница Firefox
	Clear       []string
}

// LegacyPoliciesPath — policies.json прежних версий: корпоративные политики
// заставляют Firefox писать «Ваш браузер управляется Вашей организацией»,
// поэтому файл удаляется перед запуском.
func (c Config) LegacyPoliciesPath() string {
	return filepath.Join(c.FirefoxDir, "distribution", "policies.json")
}

// Раскладка шапки: навигация, адрес, вкладки (полоса скрыта стилем),
// закладки, загрузки, расширения, кнопка «MangaReader».
// tabbrowser-tabs нельзя убрать из TabsToolbar — панель скрывается стилем.
var uiState = `{"placements":{"widget-overflow-fixed-list":[],"unified-extensions-area":[],` +
	`"nav-bar":["back-button","forward-button","stop-reload-button","urlbar-container","alltabs-button","new-tab-button",` +
	`"bookmarks-menu-button","downloads-button","unified-extensions-button","` + extensionWidget + `"],` +
	`"toolbar-menubar":["menubar-items"],"TabsToolbar":["tabbrowser-tabs"],"vertical-tabs":[],"PersonalToolbar":["personal-bookmarks"]},` +
	`"seen":["` + extensionWidget + `"],"dirtyAreaCache":["nav-bar","TabsToolbar","PersonalToolbar"],"currentVersion":24,"newElementCount":0}`

type pref struct {
	name  string
	value any
}

// UserJS — содержимое user.js: применяется при каждом запуске браузера.
// Корпоративные политики не используются (см. LegacyPoliciesPath).
func UserJS(c Config) []byte {
	prefs := []pref{
		// userChrome.css, шапка, системный заголовок окна (полоса вкладок скрыта)
		{"toolkit.legacyUserProfileCustomizations.stylesheets", true},
		{"browser.uiCustomization.state", uiState},
		{"browser.tabs.inTitlebar", 0},
		{"browser.tabs.warnOnClose", false},
		{"browser.toolbars.bookmarks.visibility", "never"},
		// всегда тёмная тема
		{"extensions.activeThemeID", "firefox-compact-dark@mozilla.org"},
		{"layout.css.prefers-color-scheme.content-override", 0},
		{"browser.theme.content-theme", 0},
		{"browser.theme.toolbar-theme", 0},
		// загрузки в папку библиотеки без вопросов, файлы не открываются
		{"browser.download.folderList", 2},
		{"browser.download.dir", c.DownloadDir},
		{"browser.download.useDownloadDir", true},
		{"browser.download.always_ask_before_handling_new_types", false},
		{"browser.download.open_pdf_internally", false},
		// встроенное расширение MangaReader из папки профиля (неподписанное — ESR)
		{"xpinstall.signatures.required", false},
		{"extensions.autoDisableScopes", 0},
		{"extensions.enabledScopes", 15},
		// без обновлений, телеметрии, исследований и браузера по умолчанию
		{"app.update.disabledForTesting", true},
		{"app.update.auto", false},
		{"datareporting.policy.dataSubmissionEnabled", false},
		{"datareporting.healthreport.uploadEnabled", false},
		{"toolkit.telemetry.enabled", false},
		{"toolkit.telemetry.unified", false},
		{"app.shield.optoutstudies.enabled", false},
		{"app.normandy.enabled", false},
		{"browser.shell.checkDefaultBrowser", false},
		{"default-browser-agent.enabled", false},
		// без приветственных окон, условий использования и рекомендаций
		{"browser.preonboarding.enabled", false},
		{"browser.aboutwelcome.enabled", false},
		{"startup.homepage_welcome_url", ""},
		{"startup.homepage_welcome_url.additional", ""},
		{"browser.startup.homepage_override.mstone", "ignore"},
		{"browser.messaging-system.whatsNewPanel.enabled", false},
		{"browser.newtabpage.activity-stream.asrouter.userprefs.cfr.addons", false},
		{"browser.newtabpage.activity-stream.asrouter.userprefs.cfr.features", false},
		// без спонсорского контента
		{"extensions.pocket.enabled", false},
		{"browser.newtabpage.activity-stream.showSponsored", false},
		{"browser.newtabpage.activity-stream.showSponsoredTopSites", false},
		{"browser.newtabpage.activity-stream.feeds.section.topstories", false},
		{"browser.urlbar.suggest.quicksuggest.sponsored", false},
		{"browser.urlbar.suggest.quicksuggest.nonsponsored", false},
		// ничего вне папки приложения
		{"browser.cache.disk.parent_directory", c.ProfileDir},
		{"browser.startup.preXulSkeletonUI", false},
	}
	if c.Home != "" {
		prefs = append(prefs, pref{"browser.startup.homepage", c.Home}, pref{"browser.startup.page", 1})
	}
	var b strings.Builder
	b.WriteString("// Создаётся MangaReader при каждом запуске браузера — изменения будут перезаписаны.\n")
	for _, p := range prefs {
		fmt.Fprintf(&b, "user_pref(%q, %s);\n", p.name, jsValue(p.value))
	}
	if items := pendingItems(c.Clear); len(items) > 0 {
		pending, _ := json.Marshal([]map[string]any{{"id": "mangareader", "itemsToClear": items, "options": map[string]any{}}})
		fmt.Fprintf(&b, "user_pref(%q, %s);\n", "privacy.sanitize.pending", jsValue(string(pending)))
	}
	return []byte(b.String())
}

// pendingItems — объединённый список очищаемого для запрошенных очисток.
func pendingItems(clear []string) []string {
	set := map[string]bool{}
	for _, c := range clear {
		for _, it := range sanitizeItems[c] {
			set[it] = true
		}
	}
	items := make([]string, 0, len(set))
	for it := range set {
		items = append(items, it)
	}
	sort.Strings(items)
	return items
}

// jsValue — значение настройки в синтаксисе prefs.js (строки — в кавычках JS).
func jsValue(v any) string {
	switch v := v.(type) {
	case string:
		data, _ := json.Marshal(v) // экранирование JSON подходит для строк JS
		return string(data)
	default:
		return fmt.Sprint(v)
	}
}

// UserChrome — userChrome.css: скрыть панели вкладок и закладок.
func UserChrome() []byte {
	return []byte(`/* Создаётся MangaReader — изменения будут перезаписаны. */
#TabsToolbar,
#PersonalToolbar {
  visibility: collapse !important;
}
`)
}

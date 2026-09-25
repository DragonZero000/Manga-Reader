package io.github.mangareader.app.browser

import org.json.JSONObject
import java.net.URLEncoder

/** Настройки браузера: приходят из Go (раздел «Браузер» настроек) при каждом открытии. */
data class BrowserSettings(
    val home: String = "",
    val search: String = DEFAULT_SEARCH,
    val toolbarTop: Boolean = false,
) {
    /** Домашняя страница; пусто — пустая вкладка. */
    fun homeUrl(): String = home.ifBlank { "about:blank" }

    /** Адрес для строки ввода: адрес сайта как есть, иначе — поиск. */
    fun urlFor(input: String): String {
        val text = input.trim()
        if (Regex("^[a-zA-Z][a-zA-Z0-9+.-]*:").containsMatchIn(text) && !text.contains(' ')) return text
        if (!text.contains(' ') && Regex("""^[^\s/]+\.[^\s/]{2,}(/.*)?$""").matches(text)) return "https://$text"
        return search.replace("%s", URLEncoder.encode(text, "UTF-8"))
    }

    companion object {
        const val DEFAULT_SEARCH = "https://www.google.com/search?q=%s"

        fun parse(json: String?): BrowserSettings? {
            if (json.isNullOrBlank()) return null
            return runCatching {
                val o = JSONObject(json)
                BrowserSettings(
                    home = o.optString("home"),
                    search = o.optString("search").ifBlank { DEFAULT_SEARCH },
                    toolbarTop = o.optString("toolbar") == "top",
                )
            }.getOrNull()
        }
    }
}

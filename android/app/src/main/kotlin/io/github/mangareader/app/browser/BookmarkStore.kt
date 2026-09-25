package io.github.mangareader.app.browser

import android.content.Context
import org.json.JSONArray
import org.json.JSONObject
import java.io.File

/** Закладки браузера: JSON в личной папке приложения (files/browser/bookmarks.json). */
class BookmarkStore(ctx: Context) {
    data class Bookmark(val title: String, val url: String)

    private val file = File(ctx.filesDir, "browser/bookmarks.json")
    private val items = mutableListOf<Bookmark>()

    init {
        runCatching {
            val arr = JSONArray(file.readText())
            for (i in 0 until arr.length()) {
                val o = arr.getJSONObject(i)
                items += Bookmark(o.optString("title"), o.getString("url"))
            }
        }
    }

    @Synchronized
    fun list(): List<Bookmark> = items.toList()

    @Synchronized
    fun contains(url: String) = items.any { it.url == url }

    /** Добавляет страницу или убирает её, если она уже в закладках; возвращает, есть ли она теперь. */
    @Synchronized
    fun toggle(title: String, url: String): Boolean {
        val had = items.removeAll { it.url == url }
        if (!had) items.add(0, Bookmark(title.ifBlank { url }, url))
        save()
        return !had
    }

    @Synchronized
    fun remove(url: String) {
        if (items.removeAll { it.url == url }) save()
    }

    private fun save() {
        val arr = JSONArray()
        items.forEach { arr.put(JSONObject().put("title", it.title).put("url", it.url)) }
        file.parentFile?.mkdirs()
        val tmp = File(file.parentFile, "bookmarks.json.tmp")
        tmp.writeText(arr.toString())
        tmp.renameTo(file)
    }
}

package io.github.mangareader.app.browser

import android.content.ContentResolver
import android.content.Context
import android.net.Uri
import android.provider.DocumentsContract
import android.util.Log
import org.mozilla.geckoview.WebResponse
import java.util.concurrent.Executors

/**
 * Загрузки в корень папки библиотеки (SAF-дерево): файл пишется как
 * «имя.part» и переименовывается по завершении; при совпадении имени —
 * «имя (1).zip». По завершении Go получает путь и адрес страницы.
 */
object DownloadManager {
    private const val TAG = "MangaBrowser"

    enum class State { RUNNING, DONE, FAILED }

    class Item(val name: String, val page: String) {
        @Volatile var state = State.RUNNING
        @Volatile var done = 0L
        @Volatile var total = -1L
    }

    private val io = Executors.newFixedThreadPool(2)
    private val lock = Any()
    private val items = mutableListOf<Item>()
    var onChange: (() -> Unit)? = null

    fun list(): List<Item> = synchronized(lock) { items.toList() }

    fun active(): Int = synchronized(lock) { items.count { it.state == State.RUNNING } }

    fun clearFinished() = synchronized(lock) { items.removeAll { it.state != State.RUNNING } }

    /**
     * Начать загрузку ответа движка; page — страница, где нажали «Скачать».
     * onFinish вызывается ровно один раз по окончании (успех или ошибка) из
     * потока загрузки.
     */
    fun start(ctx: Context, response: WebResponse, page: String, tree: String, onFinish: () -> Unit = {}) {
        val app = ctx.applicationContext
        if (tree.isBlank()) {
            BrowserEngine.toast("Папка библиотеки не выбрана")
            response.body?.close()
            onFinish()
            return
        }
        io.execute {
            try {
                run(app, response, page, Uri.parse(tree))
            } finally {
                onFinish()
            }
        }
    }

    private fun run(ctx: Context, response: WebResponse, page: String, tree: Uri) {
        val cr = ctx.contentResolver
        var part: Uri? = null
        var item: Item? = null
        try {
            val parent = DocumentsContract.buildDocumentUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
            // выбор свободного имени и создание .part — под общей блокировкой,
            // чтобы параллельные загрузки не взяли одно имя
            synchronized(lock) {
                val taken = childNames(cr, tree) + items.filter { it.state == State.RUNNING }.map { it.name }
                val name = uniqueName(fileName(response), taken)
                part = DocumentsContract.createDocument(cr, parent, "application/octet-stream", "$name.part")
                    ?: error("не удалось создать $name.part")
                item = Item(name, page).also { items.add(0, it) }
            }
            val it = item!!
            it.total = contentLength(response)
            changed(ctx)
            val body = response.body ?: error("нет тела ответа")
            body.use { input ->
                cr.openOutputStream(part!!)!!.use { out ->
                    val buf = ByteArray(64 * 1024)
                    var lastReport = 0L
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        out.write(buf, 0, n)
                        it.done += n
                        if (it.done - lastReport >= 512 * 1024) {
                            lastReport = it.done
                            changed(ctx)
                        }
                    }
                }
            }
            DocumentsContract.renameDocument(cr, part!!, it.name) ?: error("не удалось переименовать ${it.name}.part")
            it.state = State.DONE
            Log.i(TAG, "скачано ${it.name} (${it.done} байт) со страницы $page")
            GoBridge.nativeOnDownloaded(it.name, page)
            BrowserEngine.toast("Скачано: ${it.name}")
        } catch (e: Exception) {
            Log.e(TAG, "загрузка ${response.uri}", e)
            item?.state = State.FAILED
            part?.let { runCatching { DocumentsContract.deleteDocument(cr, it) } }
            BrowserEngine.toast("Не удалось скачать ${item?.name ?: response.uri}")
        } finally {
            changed(ctx)
        }
    }

    private fun changed(ctx: Context) {
        DownloadService.update(ctx)
        onChange?.invoke()
    }

    private fun contentLength(r: WebResponse): Long =
        header(r, "Content-Length")?.toLongOrNull() ?: -1L

    private fun header(r: WebResponse, name: String): String? =
        r.headers.entries.firstOrNull { it.key.equals(name, true) }?.value

    /** Имя файла: из Content-Disposition, иначе — последний сегмент адреса. */
    internal fun fileName(r: WebResponse): String {
        val cd = header(r, "Content-Disposition").orEmpty()
        val fromHeader = Regex("""filename\*=(?:UTF-8'')?([^;]+)""", RegexOption.IGNORE_CASE).find(cd)?.groupValues?.get(1)
            ?: Regex("""filename="?([^";]+)"?""", RegexOption.IGNORE_CASE).find(cd)?.groupValues?.get(1)
        val raw = fromHeader?.let { Uri.decode(it.trim().trim('"')) }
            ?: Uri.parse(r.uri).lastPathSegment
        return sanitize(raw)
    }

    internal fun sanitize(name: String?): String {
        val clean = name.orEmpty().replace(Regex("""[\\/:*?"<>|\u0000-\u001f]"""), "_").trim().trimStart('.')
        return clean.ifBlank { "download.bin" }.take(200)
    }

    internal fun uniqueName(name: String, taken: Collection<String>): String {
        val lower = taken.map { it.lowercase() }.toSet()
        fun free(n: String) = n.lowercase() !in lower && "$n.part".lowercase() !in lower
        if (free(name)) return name
        val dot = name.lastIndexOf('.').takeIf { it > 0 } ?: name.length
        val base = name.substring(0, dot)
        val ext = name.substring(dot)
        var i = 1
        while (!free("$base ($i)$ext")) i++
        return "$base ($i)$ext"
    }

    private fun childNames(cr: ContentResolver, tree: Uri): List<String> {
        val children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, DocumentsContract.getTreeDocumentId(tree))
        val out = mutableListOf<String>()
        cr.query(children, arrayOf(DocumentsContract.Document.COLUMN_DISPLAY_NAME), null, null, null)?.use { c ->
            while (c.moveToNext()) out += c.getString(0)
        }
        return out
    }
}

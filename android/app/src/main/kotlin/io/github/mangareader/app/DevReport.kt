package io.github.mangareader.app

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.provider.DocumentsContract
import android.util.Log

/**
 * Отчёт тестов производительности dev-сборки (Go: internal/devreport):
 * сохранение в скрытую папку `.perf` выбранной папки библиотеки (SAF) и
 * системное меню «Поделиться» с текстом отчёта.
 */
object DevReport {
    private const val TAG = "MangaDevReport"
    private const val DIR = ".perf"

    /** Сохраняет [text] в файл [name] папки `.perf` дерева [tree]; "" — успех, иначе текст ошибки. */
    @JvmStatic
    fun save(ctx: Context, tree: String, name: String, text: String): String {
        return try {
            val cr = ctx.contentResolver
            val treeUri = Uri.parse(tree)
            val rootId = DocumentsContract.getTreeDocumentId(treeUri)
            val dirId = findChild(ctx, treeUri, rootId, DIR) ?: run {
                val parent = DocumentsContract.buildDocumentUriUsingTree(treeUri, rootId)
                val created = DocumentsContract.createDocument(cr, parent, DocumentsContract.Document.MIME_TYPE_DIR, DIR)
                    ?: error("could not create $DIR")
                DocumentsContract.getDocumentId(created)
            }
            val dir = DocumentsContract.buildDocumentUriUsingTree(treeUri, dirId)
            val file = DocumentsContract.createDocument(cr, dir, "text/plain", name) ?: error("could not create $name")
            cr.openOutputStream(file)!!.use { it.write(text.toByteArray(Charsets.UTF_8)) }
            ""
        } catch (e: Exception) {
            Log.w(TAG, "saving the report", e)
            e.toString()
        }
    }

    /** Системное меню «Поделиться» с текстом отчёта. */
    @JvmStatic
    fun share(ctx: Context, subject: String, text: String) {
        val activity = ctx as? Activity
        if (activity == null) {
            Log.w(TAG, "share: no Activity ($ctx)")
            return
        }
        activity.runOnUiThread {
            try {
                val send = Intent(Intent.ACTION_SEND).apply {
                    type = "text/plain"
                    putExtra(Intent.EXTRA_SUBJECT, subject)
                    putExtra(Intent.EXTRA_TEXT, text)
                }
                activity.startActivity(Intent.createChooser(send, subject))
            } catch (e: Exception) {
                Log.w(TAG, "share", e)
            }
        }
    }

    /** id документа-ребёнка [name] в папке [parentId] или null. */
    private fun findChild(ctx: Context, tree: Uri, parentId: String, name: String): String? {
        val children = DocumentsContract.buildChildDocumentsUriUsingTree(tree, parentId)
        val cols = arrayOf(DocumentsContract.Document.COLUMN_DOCUMENT_ID, DocumentsContract.Document.COLUMN_DISPLAY_NAME)
        ctx.contentResolver.query(children, cols, null, null, null)?.use { c ->
            while (c.moveToNext()) {
                if (c.getString(1) == name) return c.getString(0)
            }
        }
        return null
    }
}

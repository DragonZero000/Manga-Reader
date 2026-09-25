package io.github.mangareader.spike.browser

import android.content.Context
import android.net.Uri
import android.provider.DocumentsContract
import android.util.Log
import org.mozilla.geckoview.GeckoResult
import org.mozilla.geckoview.GeckoRuntime
import org.mozilla.geckoview.GeckoRuntimeSettings
import org.mozilla.geckoview.WebExtension
import org.mozilla.geckoview.WebExtensionController
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.WebResponse
import java.util.concurrent.Executors

// Движок и вкладка живут в процессе, а не в экране: закрытие экрана
// браузера не теряет страницу и историю.
object BrowserEngine {
    private const val TAG = "MangaBrowser"
    private var runtime: GeckoRuntime? = null
    private var session: GeckoSession? = null
    private val io = Executors.newSingleThreadExecutor()
    var tree: String = ""
    var url: String = ""
        private set
    var canGoBack = false
        private set
    var onChange: (() -> Unit)? = null

    fun runtime(ctx: Context): GeckoRuntime =
        runtime ?: GeckoRuntime.create(
            ctx.applicationContext,
            GeckoRuntimeSettings.Builder()
                .preferredColorScheme(GeckoRuntimeSettings.COLOR_SCHEME_DARK)
                .consoleOutput(false)
                .build(),
        ).also { rt ->
            runtime = rt
            val ext = rt.webExtensionController
            // без этого фоновые скрипты расширений не запускаются
            ext.enableExtensionProcessSpawning()
            // спайк: установку разрешаем без диалога и пишем в лог
            ext.setPromptDelegate(object : WebExtensionController.PromptDelegate {
                override fun onInstallPromptRequest(
                    extension: WebExtension,
                    permissions: Array<String>,
                    origins: Array<String>,
                    dataCollectionPermissions: Array<String>,
                ): GeckoResult<WebExtension.PermissionPromptResponse> {
                    Log.i(TAG, "установка ${extension.metaData.name}: разрешения ${permissions.toList()} сайты ${origins.size}")
                    return GeckoResult.fromValue(WebExtension.PermissionPromptResponse(true, false, false))
                }
            })
            ext.list().accept { list ->
                Log.i(TAG, "расширения: ${list?.map { "${it.metaData.name} ${it.metaData.version} enabled=${it.metaData.enabled}" }}")
                list?.forEach { watchAction(it) }
            }
        }

    fun session(ctx: Context): GeckoSession {
        session?.let { return it }
        val app = ctx.applicationContext
        val s = GeckoSession()
        s.navigationDelegate = object : GeckoSession.NavigationDelegate {
            override fun onLocationChange(
                session: GeckoSession,
                url: String?,
                perms: MutableList<GeckoSession.PermissionDelegate.ContentPermission>,
                hasUserGesture: Boolean,
            ) {
                this@BrowserEngine.url = url ?: ""
                onChange?.invoke()
            }

            override fun onCanGoBack(session: GeckoSession, canGoBack: Boolean) {
                this@BrowserEngine.canGoBack = canGoBack
            }
        }
        // всё, что движок отдаёт на скачивание (ссылка, перенаправление,
        // POST, blob), приходит сюда уже выполненным запросом
        s.contentDelegate = object : GeckoSession.ContentDelegate {
            override fun onExternalResponse(session: GeckoSession, response: WebResponse) {
                val type = response.headers.entries.firstOrNull { it.key.equals("Content-Type", true) }?.value.orEmpty()
                Log.i(TAG, "внешний ответ ${response.uri} type=$type")
                if (type.startsWith("application/x-xpinstall") || response.uri.substringBefore('?').endsWith(".xpi")) {
                    runtime(app).webExtensionController.install(response.uri).accept(
                        { e -> Log.i(TAG, "установлено: ${e?.metaData?.name}"); e?.let { watchAction(it) } },
                        { t -> Log.e(TAG, "установка не удалась", t) },
                    )
                    return
                }
                val page = url // страница, где нажали «Скачать»
                io.execute { save(app, response, page) }
            }
        }
        s.open(runtime(ctx))
        s.loadUri("about:blank")
        session = s
        runtime(ctx).webExtensionController.list().accept { list -> list?.forEach { watchAction(it) } }
        return s
    }

    // значок кнопки расширения (у uBlock — число заблокированных запросов)
    private val actionDelegate = object : WebExtension.ActionDelegate {
        override fun onBrowserAction(extension: WebExtension, session: GeckoSession?, action: WebExtension.Action) {
            Log.i(TAG, "значок ${extension.metaData.name}: «${action.badgeText}» (${if (session == null) "по умолчанию" else url})")
        }
    }

    private fun watchAction(ext: WebExtension) {
        ext.setActionDelegate(actionDelegate)
        session?.webExtensionController?.setActionDelegate(ext, actionDelegate)
    }

    private fun save(ctx: Context, response: WebResponse, page: String) {
        val cr = ctx.contentResolver
        try {
            val name = fileName(response)
            val treeUri = Uri.parse(tree)
            val parent = DocumentsContract.buildDocumentUriUsingTree(treeUri, DocumentsContract.getTreeDocumentId(treeUri))
            val part = DocumentsContract.createDocument(cr, parent, "application/octet-stream", "$name.part")
                ?: error("createDocument вернул null")
            var total = 0L
            response.body?.use { input ->
                cr.openOutputStream(part)!!.use { out -> total = input.copyTo(out) }
            } ?: error("нет тела ответа")
            val done = DocumentsContract.renameDocument(cr, part, name)
            Log.i(TAG, "сохранено $name ($total байт) → $done, страница $page, uri ${response.uri}")
            GoBridge.nativeOnDownloaded(name, page)
        } catch (e: Exception) {
            Log.e(TAG, "загрузка ${response.uri}", e)
        }
    }

    // имя файла: из Content-Disposition, иначе — последний сегмент пути
    private fun fileName(r: WebResponse): String {
        val cd = r.headers.entries.firstOrNull { it.key.equals("Content-Disposition", true) }?.value.orEmpty()
        Regex("""filename\*?=(?:UTF-8'')?"?([^";]+)"?""", RegexOption.IGNORE_CASE).find(cd)?.let {
            return Uri.decode(it.groupValues[1]).replace(Regex("""[\\/:*?"<>|]"""), "_")
        }
        return Uri.parse(r.uri).lastPathSegment?.takeIf { it.isNotBlank() } ?: "download.bin"
    }
}

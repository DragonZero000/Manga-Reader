package io.github.mangareader.app.browser

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.util.Log
import android.widget.Toast
import org.mozilla.geckoview.GeckoResult
import org.mozilla.geckoview.GeckoRuntime
import org.mozilla.geckoview.GeckoRuntimeSettings
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.StorageController
import org.mozilla.geckoview.WebExtension
import org.mozilla.geckoview.WebExtensionController
import org.mozilla.geckoview.WebResponse

/** Вкладка: сессия движка и её состояние для панели и списка вкладок. */
class Tab(val session: GeckoSession) {
    @Volatile var title = ""
    @Volatile var url = ""
    @Volatile var canGoBack = false
    @Volatile var canGoForward = false
    @Volatile var progress = 100
}

/**
 * Движок браузера на процесс: GeckoRuntime, вкладки, расширения, закладки.
 * Экран браузера только подключает активную вкладку, поэтому вкладки
 * сохраняются, пока жив процесс (закрытие экрана, «MangaReader»).
 * Все методы — в главном потоке.
 */
object BrowserEngine {
    private const val TAG = "MangaBrowser"
    private val main = Handler(Looper.getMainLooper())

    /** Экран браузера: обновления для панели и диалоги. */
    interface Listener {
        fun onTabsChanged()
        fun onTabUpdated(tab: Tab)
        fun onInstallPrompt(name: String, permissions: List<String>, sites: Int, decide: (Boolean) -> Unit)
        fun onDownloadStarted()
    }

    private lateinit var app: Context
    private var runtime: GeckoRuntime? = null
    lateinit var bookmarks: BookmarkStore
        private set

    val tabs = mutableListOf<Tab>()
    var active = -1
        private set
    var settings = BrowserSettings()
    var tree = ""
    var listener: Listener? = null

    fun init(ctx: Context) {
        if (runtime != null) return
        app = ctx.applicationContext
        bookmarks = BookmarkStore(app)
        val rt = GeckoRuntime.create(
            app,
            GeckoRuntimeSettings.Builder()
                .preferredColorScheme(GeckoRuntimeSettings.COLOR_SCHEME_DARK)
                .consoleOutput(false)
                .aboutConfigEnabled(false)
                .build(),
        )
        runtime = rt
        val ext = rt.webExtensionController
        // без этого фоновые скрипты расширений не запускаются (спайк S-B3)
        ext.enableExtensionProcessSpawning()
        ext.setPromptDelegate(object : WebExtensionController.PromptDelegate {
            override fun onInstallPromptRequest(
                extension: WebExtension,
                permissions: Array<String>,
                origins: Array<String>,
                dataCollectionPermissions: Array<String>,
            ): GeckoResult<WebExtension.PermissionPromptResponse> {
                val result = GeckoResult<WebExtension.PermissionPromptResponse>()
                val decide = { ok: Boolean -> result.complete(WebExtension.PermissionPromptResponse(ok, false, false)) }
                val l = listener
                if (l == null) decide(false)
                else l.onInstallPrompt(extension.metaData.name ?: extension.id, permissions.toList(), origins.size, decide)
                return result
            }
        })
    }

    fun runtime(): GeckoRuntime = runtime ?: error("BrowserEngine.init не вызван")

    fun activeTab(): Tab? = tabs.getOrNull(active)

    /** Новая вкладка с адресом (null — домашняя страница). */
    fun newTab(url: String?): Tab {
        val tab = Tab(GeckoSession())
        setup(tab)
        tab.session.open(runtime())
        tab.session.loadUri(url ?: settings.homeUrl())
        tabs += tab
        active = tabs.lastIndex
        listener?.onTabsChanged()
        return tab
    }

    fun select(i: Int) {
        if (i !in tabs.indices || i == active) return
        active = i
        listener?.onTabsChanged()
    }

    fun close(i: Int) {
        if (i !in tabs.indices) return
        tabs.removeAt(i).session.close()
        if (tabs.isEmpty()) {
            active = -1
            newTab(null) // закрыта последняя — новая с домашней страницей
            return
        }
        if (active >= tabs.size || i < active) active = (active - 1).coerceAtLeast(0)
        listener?.onTabsChanged()
    }

    private fun setup(tab: Tab) {
        val s = tab.session
        s.navigationDelegate = object : GeckoSession.NavigationDelegate {
            override fun onLocationChange(
                session: GeckoSession,
                url: String?,
                perms: MutableList<GeckoSession.PermissionDelegate.ContentPermission>,
                hasUserGesture: Boolean,
            ) {
                tab.url = url.orEmpty()
                listener?.onTabUpdated(tab)
            }

            override fun onCanGoBack(session: GeckoSession, canGoBack: Boolean) {
                tab.canGoBack = canGoBack
                listener?.onTabUpdated(tab)
            }

            override fun onCanGoForward(session: GeckoSession, canGoForward: Boolean) {
                tab.canGoForward = canGoForward
                listener?.onTabUpdated(tab)
            }

            // target=_blank, window.open — новая вкладка; сессию открывает движок
            override fun onNewSession(session: GeckoSession, uri: String): GeckoResult<GeckoSession> {
                val t = Tab(GeckoSession())
                setup(t)
                t.url = uri
                tabs += t
                active = tabs.lastIndex
                main.post { listener?.onTabsChanged() }
                return GeckoResult.fromValue(t.session)
            }
        }
        s.contentDelegate = object : GeckoSession.ContentDelegate {
            override fun onTitleChange(session: GeckoSession, title: String?) {
                tab.title = title.orEmpty()
                listener?.onTabUpdated(tab)
            }

            override fun onCloseRequest(session: GeckoSession) {
                close(tabs.indexOf(tab))
            }

            // всё, что движок отдаёт на скачивание: ссылка, перенаправление, POST, blob
            override fun onExternalResponse(session: GeckoSession, response: WebResponse) {
                val type = response.headers.entries.firstOrNull { it.key.equals("Content-Type", true) }?.value.orEmpty()
                if (type.startsWith("application/x-xpinstall") || response.uri.substringBefore('?').endsWith(".xpi")) {
                    response.body?.close()
                    install(response.uri)
                    return
                }
                listener?.onDownloadStarted()
                DownloadManager.start(app, response, tab.url, tree)
            }
        }
        s.progressDelegate = object : GeckoSession.ProgressDelegate {
            override fun onProgressChange(session: GeckoSession, progress: Int) {
                tab.progress = progress
                listener?.onTabUpdated(tab)
            }
        }
    }

    private fun install(uri: String) {
        runtime().webExtensionController.install(uri).accept(
            { e -> toast("Расширение установлено: ${e?.metaData?.name ?: ""}") },
            { t -> Log.w(TAG, "установка расширения $uri", t); toast("Расширение не установлено") },
        )
    }

    /** Очистка: «cookies» — cookies, данные сайтов, кэш; «history» — история вкладок и список загрузок. */
    fun clearData(kinds: List<String>) {
        var flags = 0L
        if ("cookies" in kinds) flags = flags or StorageController.ClearFlags.COOKIES or
            StorageController.ClearFlags.DOM_STORAGES or StorageController.ClearFlags.ALL_CACHES or
            StorageController.ClearFlags.AUTH_SESSIONS
        if ("history" in kinds) {
            tabs.forEach { it.session.purgeHistory() }
            DownloadManager.clearFinished()
        }
        if (flags == 0L) {
            toast("Очищено")
            return
        }
        runtime().storageController.clearData(flags).accept(
            { toast("Очищено") },
            { t -> Log.w(TAG, "очистка", t); toast("Не удалось очистить: ${t?.message}") },
        )
    }

    fun toast(text: String) {
        main.post { Toast.makeText(app, text, Toast.LENGTH_SHORT).show() }
    }
}

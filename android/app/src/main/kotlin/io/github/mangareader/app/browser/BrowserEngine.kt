package io.github.mangareader.app.browser

import android.content.Context
import android.os.Handler
import android.os.Looper
import android.os.SystemClock
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

/**
 * Вкладка: сессия движка и её состояние для панели и списка вкладок.
 * session == null — вкладка выгружена: сессия закрыта, процесс контента
 * освобождён, а адрес, история и позиция лежат в state.
 */
class Tab {
    var session: GeckoSession? = null
        internal set
    /** Последнее состояние сессии (история, адрес, прокрутка) для восстановления. */
    var state: GeckoSession.SessionState? = null
        internal set
    /** Идущие загрузки, начатые с этой вкладки: пока > 0, вкладка не выгружается. */
    var downloads = 0
        internal set
    /** Когда вкладка последний раз была активной (elapsedRealtime). */
    var lastUsed = 0L
        internal set
    @Volatile var title = ""
    @Volatile var url = ""
    @Volatile var canGoBack = false
    @Volatile var canGoForward = false
    @Volatile var progress = 100

    val loaded: Boolean get() = session != null
}

/**
 * Движок браузера на процесс: GeckoRuntime, вкладки, расширения, закладки.
 * Экран браузера только подключает активную вкладку, поэтому вкладки
 * сохраняются, пока жив процесс (закрытие экрана, «MangaReader»).
 * Неактивные вкладки выгружаются (enforce): на экране браузера загружены
 * активная и последняя использованная, после ухода с экрана — только
 * активная; выгруженная восстанавливается из сохранённого состояния.
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
    /** Экран браузера на переднем плане (BrowserActivity между onStart и onStop). */
    var foreground = false
        private set

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

    /** Сессия активной вкладки; выгруженная вкладка сначала восстанавливается. */
    fun activeSession(): GeckoSession? = activeTab()?.let { ensureLoaded(it) }

    /** Новая вкладка с адресом (null — домашняя страница). */
    fun newTab(url: String?): Tab {
        val tab = Tab()
        tab.url = url ?: settings.homeUrl()
        tabs += tab
        makeActive(tabs.lastIndex)
        ensureLoaded(tab)
        enforce()
        listener?.onTabsChanged()
        return tab
    }

    fun select(i: Int) {
        if (i !in tabs.indices || i == active) return
        makeActive(i)
        ensureLoaded(tabs[i])
        enforce()
        listener?.onTabsChanged()
    }

    fun close(i: Int) {
        if (i !in tabs.indices) return
        val tab = tabs.removeAt(i)
        tab.session?.close()
        tab.session = null
        if (tabs.isEmpty()) {
            active = -1
            newTab(null) // закрыта последняя — новая с домашней страницей
            return
        }
        if (active >= tabs.size || i < active) active = (active - 1).coerceAtLeast(0)
        activeTab()?.lastUsed = SystemClock.elapsedRealtime()
        enforce()
        listener?.onTabsChanged()
    }

    /** Экран браузера показан (true) или скрыт (false): активность вкладок и выгрузка. */
    fun setForeground(on: Boolean) {
        foreground = on
        enforce()
    }

    /**
     * Сессия вкладки: загруженная — как есть, выгруженная — новая сессия из
     * сохранённого состояния (страница загружается заново, история та же).
     */
    fun ensureLoaded(tab: Tab): GeckoSession {
        tab.session?.let { return it }
        val s = GeckoSession()
        setup(tab, s)
        tab.session = s
        s.open(runtime())
        val st = tab.state
        if (st != null) s.restoreState(st) else s.loadUri(tab.url.ifBlank { settings.homeUrl() })
        Log.i(TAG, "вкладка загружена${if (st != null) " из состояния" else ""}: ${tab.url}")
        return s
    }

    private fun makeActive(i: Int) {
        val now = SystemClock.elapsedRealtime()
        activeTab()?.lastUsed = now // уходим с неё — она «последняя»
        active = i
        tabs[i].lastUsed = now
    }

    /**
     * Политика выгрузки: активная вкладка загружена всегда; на экране
     * браузера — ещё последняя использованная; вкладки с загрузками не
     * трогаются. Активна (отрисовка, высокий приоритет) только показанная.
     */
    fun enforce() {
        val cur = activeTab()
        val keep = mutableSetOf<Tab>()
        cur?.let { keep += it }
        if (foreground) {
            tabs.filter { it !== cur && it.loaded }.maxByOrNull { it.lastUsed }?.let { keep += it }
        }
        for (tab in tabs) {
            if (tab !in keep && tab.downloads == 0) unload(tab)
            val s = tab.session ?: continue
            if (!s.isOpen) continue // сессию из onNewSession откроет GeckoView; enforce повторит attach()
            val shown = tab === cur && foreground
            s.setActive(shown)
            s.setPriorityHint(if (shown) GeckoSession.PRIORITY_HIGH else GeckoSession.PRIORITY_DEFAULT)
        }
    }

    /** Выгрузить вкладку: состояние остаётся в tab.state, сессия закрывается. */
    private fun unload(tab: Tab) {
        val s = tab.session ?: return
        tab.session = null
        s.close()
        Log.i(TAG, "вкладка выгружена: ${tab.url}")
    }

    /** Процесс вкладки завершён системой или упал: вкладка — выгруженная. */
    private fun lost(tab: Tab, session: GeckoSession, why: String) {
        if (session !== tab.session) return
        Log.w(TAG, "процесс вкладки $why: ${tab.url}")
        tab.session = null
        runCatching { session.close() }
        if (tab === activeTab() && foreground) listener?.onTabsChanged() // экран восстановит её
    }

    private fun setup(tab: Tab, s: GeckoSession) {
        s.navigationDelegate = object : GeckoSession.NavigationDelegate {
            override fun onLocationChange(
                session: GeckoSession,
                url: String?,
                perms: MutableList<GeckoSession.PermissionDelegate.ContentPermission>,
                hasUserGesture: Boolean,
            ) {
                if (session !== tab.session) return
                tab.url = url.orEmpty()
                listener?.onTabUpdated(tab)
            }

            override fun onCanGoBack(session: GeckoSession, canGoBack: Boolean) {
                if (session !== tab.session) return
                tab.canGoBack = canGoBack
                listener?.onTabUpdated(tab)
            }

            override fun onCanGoForward(session: GeckoSession, canGoForward: Boolean) {
                if (session !== tab.session) return
                tab.canGoForward = canGoForward
                listener?.onTabUpdated(tab)
            }

            // target=_blank, window.open — новая вкладка; сессию открывает движок
            override fun onNewSession(session: GeckoSession, uri: String): GeckoResult<GeckoSession> {
                val t = Tab()
                val ns = GeckoSession()
                setup(t, ns)
                t.session = ns
                t.url = uri
                tabs += t
                makeActive(tabs.lastIndex)
                main.post {
                    enforce()
                    listener?.onTabsChanged()
                }
                return GeckoResult.fromValue(ns)
            }
        }
        s.contentDelegate = object : GeckoSession.ContentDelegate {
            override fun onTitleChange(session: GeckoSession, title: String?) {
                if (session !== tab.session) return
                tab.title = title.orEmpty()
                listener?.onTabUpdated(tab)
            }

            override fun onCloseRequest(session: GeckoSession) {
                if (session === tab.session) close(tabs.indexOf(tab))
            }

            override fun onKill(session: GeckoSession) = lost(tab, session, "завершён системой")

            override fun onCrash(session: GeckoSession) = lost(tab, session, "упал")

            // всё, что движок отдаёт на скачивание: ссылка, перенаправление, POST, blob
            override fun onExternalResponse(session: GeckoSession, response: WebResponse) {
                val type = response.headers.entries.firstOrNull { it.key.equals("Content-Type", true) }?.value.orEmpty()
                if (type.startsWith("application/x-xpinstall") || response.uri.substringBefore('?').endsWith(".xpi")) {
                    response.body?.close()
                    install(response.uri)
                    return
                }
                listener?.onDownloadStarted()
                // пока идёт загрузка, вкладку не выгружаем: поток ответа — из её сессии
                tab.downloads++
                DownloadManager.start(app, response, tab.url, tree) {
                    main.post {
                        tab.downloads--
                        enforce()
                    }
                }
            }
        }
        s.progressDelegate = object : GeckoSession.ProgressDelegate {
            override fun onProgressChange(session: GeckoSession, progress: Int) {
                if (session !== tab.session) return
                tab.progress = progress
                listener?.onTabUpdated(tab)
            }

            // свежее состояние — для восстановления после выгрузки
            override fun onSessionStateChange(session: GeckoSession, sessionState: GeckoSession.SessionState) {
                if (session === tab.session) tab.state = sessionState
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
            // загруженные — средствами движка; у выгруженных история в состоянии
            tabs.forEach { t -> t.session?.purgeHistory() ?: run { t.state = null } }
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

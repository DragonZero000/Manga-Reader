package io.github.mangareader.app.browser

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.os.Build
import android.os.Bundle
import android.util.TypedValue
import android.view.Gravity
import android.view.View
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.LinearLayout
import android.widget.PopupMenu
import android.widget.ScrollView
import android.widget.Switch
import android.widget.TextView
import org.mozilla.geckoview.GeckoSession
import org.mozilla.geckoview.GeckoView
import org.mozilla.geckoview.WebExtension
import org.mozilla.geckoview.WebExtensionController

/**
 * Экран браузера поверх читалки (та же задача приложения). Показывает
 * активную вкладку BrowserEngine; «MangaReader» и «Назад» на первой странице
 * возвращают в читалку, вкладки при этом сохраняются (неактивные —
 * выгружаются движком и восстанавливаются при показе).
 */
class BrowserActivity : Activity(), BrowserEngine.Listener, BrowserToolbar.Actions {
    private lateinit var gecko: GeckoView
    private lateinit var toolbar: BrowserToolbar
    private var toolbarTop = false
    private var attached: GeckoSession? = null // сессия, подключённая к виду

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        BrowserEngine.init(this)
        gecko = GeckoView(this)
        toolbar = BrowserToolbar(this).also { it.actions = this }
        window.statusBarColor = BrowserToolbar.BG
        window.navigationBarColor = BrowserToolbar.BG
        apply(intent, created = true)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        setIntent(intent)
        apply(intent, created = false)
    }

    /** Настройки, папка и адрес из Intent читалки. */
    private fun apply(intent: Intent?, created: Boolean) {
        BrowserSettings.parse(intent?.getStringExtra("settings"))?.let { BrowserEngine.settings = it }
        intent?.getStringExtra("tree")?.takeIf { it.isNotBlank() }?.let { BrowserEngine.tree = it }
        if (created || toolbarTop != BrowserEngine.settings.toolbarTop) layout()
        val url = intent?.getStringExtra("url").orEmpty()
        when {
            url.isNotBlank() -> BrowserEngine.newTab(url)
            BrowserEngine.tabs.isEmpty() -> BrowserEngine.newTab(null)
        }
        // адрес обработан: при пересоздании экрана не открывать его снова
        intent?.removeExtra("url")
    }

    private fun layout() {
        toolbarTop = BrowserEngine.settings.toolbarTop
        (gecko.parent as? LinearLayout)?.removeAllViews()
        val root = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            fitsSystemWindows = true
            setBackgroundColor(BrowserToolbar.BG)
        }
        val web = LinearLayout.LayoutParams(MATCH_PARENT, 0, 1f)
        val bar = LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT)
        if (toolbarTop) {
            root.addView(toolbar, bar)
            root.addView(gecko, web)
        } else {
            root.addView(gecko, web)
            root.addView(toolbar, bar)
        }
        setContentView(root)
    }

    override fun onStart() {
        super.onStart()
        BrowserEngine.listener = this
        DownloadManager.onChange = null
        BrowserEngine.setForeground(true)
        attach()
    }

    override fun onStop() {
        BrowserEngine.listener = null
        gecko.releaseSession()
        attached = null
        // «MangaReader», «Назад», сворачивание: активная — неактивна, остальные выгружаются
        BrowserEngine.setForeground(false)
        super.onStop()
    }

    /**
     * Подключить активную вкладку к виду: выгруженная сначала
     * восстанавливается (сессию из onNewSession движок открывает сам).
     */
    private fun attach() {
        val tab = BrowserEngine.activeTab() ?: return
        val s = BrowserEngine.ensureLoaded(tab)
        if (s !== attached) {
            if (!s.isOpen) {
                gecko.postDelayed({ attach() }, 50)
                return
            }
            gecko.releaseSession()
            gecko.setSession(s)
            attached = s
            BrowserEngine.enforce() // сессия открыта — активность и приоритет
        }
        refresh()
    }

    private fun refresh() {
        val tab = BrowserEngine.activeTab()
        toolbar.show(tab, BrowserEngine.tabs.size, tab != null && BrowserEngine.bookmarks.contains(tab.url))
    }

    // --- BrowserEngine.Listener ---

    override fun onTabsChanged() = runOnUiThread { attach() }

    override fun onTabUpdated(tab: Tab) = runOnUiThread { if (tab === BrowserEngine.activeTab()) refresh() }

    override fun onInstallPrompt(name: String, permissions: List<String>, sites: Int, decide: (Boolean) -> Unit) = runOnUiThread {
        val perms = buildList {
            addAll(permissions)
            if (sites > 0) add("доступ к данным сайтов: $sites")
        }.joinToString("\n• ", prefix = "• ")
        AlertDialog.Builder(this)
            .setTitle("Добавить «$name»?")
            .setMessage("Расширению нужны разрешения:\n$perms")
            .setPositiveButton("Добавить") { _, _ -> decide(true) }
            .setNegativeButton("Отмена") { _, _ -> decide(false) }
            .setOnCancelListener { decide(false) }
            .show()
    }

    override fun onDownloadStarted() = runOnUiThread {
        // уведомление о загрузке (Android 13+ спрашивает разрешение); загрузка идёт и без него
        if (Build.VERSION.SDK_INT >= 33 &&
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), 1)
        }
    }

    // --- BrowserToolbar.Actions ---

    override fun onAddress(text: String) {
        if (text.isBlank()) return
        BrowserEngine.activeSession()?.loadUri(BrowserEngine.settings.urlFor(text))
    }

    override fun onReload() {
        BrowserEngine.activeSession()?.reload()
    }

    override fun onBack() {
        BrowserEngine.activeSession()?.goBack()
    }

    override fun onForward() {
        BrowserEngine.activeSession()?.goForward()
    }

    override fun onReader() = showReader()

    override fun onBookmark() {
        val tab = BrowserEngine.activeTab() ?: return
        if (tab.url.isBlank() || tab.url == "about:blank") return
        val added = BrowserEngine.bookmarks.toggle(tab.title, tab.url)
        BrowserEngine.toast(if (added) "Добавлено в закладки" else "Удалено из закладок")
        refresh()
    }

    override fun onMenu(anchor: View) {
        PopupMenu(this, anchor).apply {
            menu.add("Новая вкладка").setOnMenuItemClickListener { BrowserEngine.newTab(null); true }
            menu.add("Закладки").setOnMenuItemClickListener { showBookmarks(); true }
            menu.add("Загрузки").setOnMenuItemClickListener { showDownloads(); true }
            menu.add("Расширения").setOnMenuItemClickListener { showExtensions(); true }
            show()
        }
    }

    override fun onTabs() {
        val list = listDialogBody()
        val dialog = AlertDialog.Builder(this)
            .setTitle("Вкладки: ${BrowserEngine.tabs.size}")
            .setView(ScrollView(this).apply { addView(list) })
            .setPositiveButton("Новая вкладка") { _, _ -> BrowserEngine.newTab(null) }
            .setNegativeButton("Закрыть", null)
            .create()
        BrowserEngine.tabs.forEachIndexed { i, tab ->
            list.addView(row(
                tab.title.ifBlank { "Новая вкладка" }, tab.url, i == BrowserEngine.active,
                onOpen = { BrowserEngine.select(i); dialog.dismiss() },
                onClose = { BrowserEngine.close(i); dialog.dismiss(); onTabs() },
            ))
        }
        dialog.show()
    }

    private fun showBookmarks() {
        val list = listDialogBody()
        val items = BrowserEngine.bookmarks.list()
        val dialog = AlertDialog.Builder(this)
            .setTitle("Закладки")
            .setView(ScrollView(this).apply { addView(list) })
            .setNegativeButton("Закрыть", null)
            .create()
        if (items.isEmpty()) list.addView(note("Закладок пока нет — нажмите ☆ на странице"))
        items.forEach { b ->
            list.addView(row(
                b.title, b.url, false,
                onOpen = { BrowserEngine.activeSession()?.loadUri(b.url); dialog.dismiss() },
                onClose = { BrowserEngine.bookmarks.remove(b.url); dialog.dismiss(); showBookmarks(); refresh() },
            ))
        }
        dialog.show()
    }

    private fun showDownloads() {
        val list = listDialogBody()
        val items = DownloadManager.list()
        if (items.isEmpty()) list.addView(note("Загрузок пока нет. Файлы сохраняются в папку библиотеки"))
        items.forEach { d ->
            val status = when (d.state) {
                DownloadManager.State.RUNNING ->
                    if (d.total > 0) "скачивается: ${d.done * 100 / d.total}%" else "скачивается: ${d.done / 1024} КБ"
                DownloadManager.State.DONE -> "готово — в библиотеке"
                DownloadManager.State.FAILED -> "не удалось скачать"
            }
            list.addView(row(d.name, status, false, onOpen = null, onClose = null))
        }
        AlertDialog.Builder(this)
            .setTitle("Загрузки")
            .setView(ScrollView(this).apply { addView(list) })
            .setNegativeButton("Закрыть", null)
            .show()
    }

    private fun showExtensions() {
        val controller = BrowserEngine.runtime().webExtensionController
        controller.list().accept { exts ->
            runOnUiThread {
                val list = listDialogBody()
                val builder = AlertDialog.Builder(this)
                    .setTitle("Расширения")
                    .setView(ScrollView(this).apply { addView(list) })
                    .setPositiveButton("Найти расширения") { _, _ ->
                        BrowserEngine.newTab("https://addons.mozilla.org/ru/android/")
                    }
                    .setNegativeButton("Закрыть", null)
                val dialog = builder.create()
                val user = exts.orEmpty().filter { !it.isBuiltIn }
                if (user.isEmpty()) list.addView(note("Расширений нет. Установите их с addons.mozilla.org кнопкой «Добавить в Firefox»"))
                user.forEach { e -> list.addView(extensionRow(controller, e) { dialog.dismiss(); showExtensions() }) }
                dialog.show()
            }
        }
    }

    private fun extensionRow(controller: WebExtensionController, e: WebExtension, reload: () -> Unit): View {
        val name = e.metaData.name ?: e.id
        val row = LinearLayout(this).apply {
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(20), dp(10), dp(12), dp(10))
        }
        row.addView(text("$name ${e.metaData.version}", 16f, Color.WHITE), LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
        row.addView(Switch(this).apply {
            isChecked = e.metaData.enabled
            setOnCheckedChangeListener { _, on ->
                val r = if (on) controller.enable(e, WebExtensionController.EnableSource.USER)
                else controller.disable(e, WebExtensionController.EnableSource.USER)
                r.accept({ runOnUiThread(reload) }, { runOnUiThread(reload) })
            }
        })
        row.addView(text("✕", 20f, Color.LTGRAY).apply {
            setPadding(dp(16), dp(4), dp(8), dp(4))
            setOnClickListener {
                AlertDialog.Builder(this@BrowserActivity)
                    .setTitle("Удалить «$name»?")
                    .setPositiveButton("Удалить") { _, _ -> controller.uninstall(e).accept({ runOnUiThread(reload) }, { runOnUiThread(reload) }) }
                    .setNegativeButton("Отмена", null)
                    .show()
            }
        })
        return row
    }

    // --- системная «Назад» и возврат в читалку ---

    @Deprecated("системная «Назад»")
    override fun onBackPressed() {
        val tab = BrowserEngine.activeTab()
        if (tab != null && tab.canGoBack) BrowserEngine.activeSession()?.goBack() else finish()
    }

    /** «MangaReader»: читалка — на передний план, браузер остаётся под ней. */
    private fun showReader() {
        startActivity(
            Intent().setClassName(this, "org.golang.app.GoNativeActivity")
                .addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT),
        )
    }

    // --- элементы диалогов ---

    private fun listDialogBody() = LinearLayout(this).apply {
        orientation = LinearLayout.VERTICAL
        setPadding(0, dp(8), 0, dp(8))
    }

    private fun row(title: String, subtitle: String, current: Boolean, onOpen: (() -> Unit)?, onClose: (() -> Unit)?): View {
        val row = LinearLayout(this).apply {
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(20), dp(10), dp(12), dp(10))
            if (current) setBackgroundColor(Color.argb(60, 143, 143, 255))
        }
        val texts = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            addView(text(title, 16f, Color.WHITE).apply { maxLines = 2 })
            if (subtitle.isNotBlank()) addView(text(subtitle, 13f, Color.LTGRAY).apply { maxLines = 1 })
            if (onOpen != null) setOnClickListener { onOpen() }
        }
        row.addView(texts, LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
        if (onClose != null) {
            row.addView(text("✕", 20f, Color.LTGRAY).apply {
                setPadding(dp(16), dp(4), dp(8), dp(4))
                setOnClickListener { onClose() }
            })
        }
        return row
    }

    private fun note(s: String) = text(s, 15f, Color.LTGRAY).apply { setPadding(dp(20), dp(12), dp(20), dp(12)) }

    private fun text(s: String, sp: Float, color: Int) = TextView(this).apply {
        text = s
        setTextColor(color)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, sp)
    }

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()
}

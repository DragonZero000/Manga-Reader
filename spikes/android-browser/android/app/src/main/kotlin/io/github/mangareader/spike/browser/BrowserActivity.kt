package io.github.mangareader.spike.browser

import android.app.Activity
import android.content.Intent
import android.graphics.Color
import android.os.Bundle
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.Button
import android.widget.LinearLayout
import android.widget.TextView
import org.mozilla.geckoview.GeckoView

class BrowserActivity : Activity() {
    private lateinit var gecko: GeckoView
    private lateinit var address: TextView

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        address = TextView(this).apply { setTextColor(Color.WHITE); textSize = 14f; setPadding(24, 24, 24, 24) }
        val toReader = Button(this).apply { text = "MangaReader"; setOnClickListener { showReader() } }
        val bar = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            setBackgroundColor(Color.rgb(28, 27, 34))
            addView(address, LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f))
            addView(toReader, LinearLayout.LayoutParams(WRAP_CONTENT, WRAP_CONTENT))
        }
        gecko = GeckoView(this)
        setContentView(LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            fitsSystemWindows = true
            addView(bar, LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT))
            addView(gecko, LinearLayout.LayoutParams(MATCH_PARENT, 0, 1f))
        })
        load(intent)
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        load(intent)
    }

    private fun load(intent: Intent?) {
        intent?.getStringExtra("tree")?.let { if (it.isNotEmpty()) BrowserEngine.tree = it }
        val url = intent?.getStringExtra("url")
        if (!url.isNullOrEmpty()) BrowserEngine.session(this).loadUri(url)
    }

    override fun onStart() {
        super.onStart()
        BrowserEngine.onChange = { runOnUiThread { address.text = BrowserEngine.url } }
        gecko.setSession(BrowserEngine.session(this))
        address.text = BrowserEngine.url
    }

    override fun onStop() {
        gecko.releaseSession()
        BrowserEngine.onChange = null
        super.onStop()
    }

    @Deprecated("системная «Назад»")
    override fun onBackPressed() {
        if (BrowserEngine.canGoBack) BrowserEngine.session(this).goBack() else finish()
    }

    // «MangaReader»: читалку — на передний план, браузер остаётся под ней
    private fun showReader() {
        startActivity(
            Intent().setClassName(this, "org.golang.app.GoNativeActivity")
                .addFlags(Intent.FLAG_ACTIVITY_REORDER_TO_FRONT),
        )
    }
}

package io.github.mangareader.app.browser

import android.content.Context
import android.graphics.Color
import android.graphics.Typeface
import android.graphics.drawable.GradientDrawable
import android.text.InputType
import android.util.TypedValue
import android.view.Gravity
import android.view.KeyEvent
import android.view.View
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputMethodManager
import android.widget.EditText
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView

/**
 * Панель браузера (по мотивам Firefox для Android): адрес/поиск и «обновить»,
 * ниже — назад, вперёд, вкладки с числом, «MangaReader», закладка, меню.
 * Экран кладёт её сверху или снизу по настройке.
 */
class BrowserToolbar(ctx: Context) : LinearLayout(ctx) {
    interface Actions {
        fun onAddress(text: String)
        fun onReload()
        fun onBack()
        fun onForward()
        fun onTabs()
        fun onReader()
        fun onBookmark()
        fun onMenu(anchor: View)
    }

    var actions: Actions? = null

    private val address = EditText(ctx)
    private val progress = ProgressBar(ctx, null, android.R.attr.progressBarStyleHorizontal)
    private val back = button("←") { actions?.onBack() }
    private val forward = button("→") { actions?.onForward() }
    private val tabs = button("1") { actions?.onTabs() }
    private val star = button("☆") { actions?.onBookmark() }
    private var editing = false

    init {
        orientation = VERTICAL
        setBackgroundColor(BG)
        address.apply {
            setSingleLine()
            inputType = InputType.TYPE_TEXT_VARIATION_URI or InputType.TYPE_CLASS_TEXT
            imeOptions = EditorInfo.IME_ACTION_GO
            hint = "Адрес или поиск"
            setTextColor(Color.WHITE)
            setHintTextColor(Color.GRAY)
            setTextSize(TypedValue.COMPLEX_UNIT_SP, 15f)
            background = GradientDrawable().apply { setColor(FIELD); cornerRadius = dp(20).toFloat() }
            setPadding(dp(14), dp(8), dp(14), dp(8))
            setSelectAllOnFocus(true)
            setOnFocusChangeListener { _, has -> editing = has }
            setOnEditorActionListener { _, id, ev ->
                if (id == EditorInfo.IME_ACTION_GO || ev?.keyCode == KeyEvent.KEYCODE_ENTER) {
                    actions?.onAddress(text.toString())
                    clearFocus()
                    hideKeyboard()
                    true
                } else false
            }
        }
        val row1 = LinearLayout(ctx).apply {
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(8), dp(6), dp(4), dp(2))
            addView(address, LayoutParams(0, LayoutParams.WRAP_CONTENT, 1f))
            addView(button("⟳") { actions?.onReload() }, LayoutParams(dp(48), dp(44)))
        }
        progress.apply { max = 100; visibility = View.GONE }
        val row2 = LinearLayout(ctx).apply {
            gravity = Gravity.CENTER_VERTICAL
            setPadding(dp(4), 0, dp(4), dp(4))
            val reader = button("MangaReader") { actions?.onReader() }
            for (v in listOf(back, forward, tabs, reader, star, button("⋮") { actions?.onMenu(it) })) {
                // «MangaReader» — шире остальных, чтобы подпись не переносилась
                addView(v, LayoutParams(0, dp(44), if (v === reader) 2f else 1f))
            }
        }
        tabs.background = GradientDrawable().apply { setStroke(dp(2), Color.WHITE); cornerRadius = dp(6).toFloat() }
        addView(row1)
        addView(progress, LayoutParams(LayoutParams.MATCH_PARENT, dp(3)))
        addView(row2)
    }

    /** Обновить панель по вкладке. */
    fun show(tab: Tab?, tabCount: Int, bookmarked: Boolean) {
        if (!editing) address.setText(tab?.url?.takeIf { it != "about:blank" }.orEmpty())
        back.isEnabled = tab?.canGoBack == true
        back.alpha = if (back.isEnabled) 1f else 0.35f
        forward.isEnabled = tab?.canGoForward == true
        forward.alpha = if (forward.isEnabled) 1f else 0.35f
        tabs.text = if (tabCount > 99) "∞" else tabCount.toString()
        star.text = if (bookmarked) "★" else "☆"
        val p = tab?.progress ?: 100
        progress.visibility = if (p in 1..99) View.VISIBLE else View.GONE
        progress.progress = p
    }

    private fun button(label: String, onClick: (View) -> Unit) = TextView(context).apply {
        text = label
        gravity = Gravity.CENTER
        setTextColor(Color.WHITE)
        setTextSize(TypedValue.COMPLEX_UNIT_SP, if (label.length > 2) 13f else 20f)
        if (label.length > 2) typeface = Typeface.DEFAULT_BOLD
        setSingleLine()
        isClickable = true
        isFocusable = true
        val out = TypedValue()
        context.theme.resolveAttribute(android.R.attr.selectableItemBackgroundBorderless, out, true)
        setBackgroundResource(out.resourceId)
        setOnClickListener(onClick)
    }

    private fun hideKeyboard() {
        context.getSystemService(InputMethodManager::class.java).hideSoftInputFromWindow(windowToken, 0)
    }

    private fun dp(v: Int) = (v * resources.displayMetrics.density).toInt()

    companion object {
        val BG = Color.rgb(28, 27, 34)
        val FIELD = Color.rgb(43, 42, 51)
    }
}

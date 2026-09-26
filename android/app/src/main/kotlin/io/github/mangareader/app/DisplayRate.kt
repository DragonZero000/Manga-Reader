package io.github.mangareader.app

import android.app.Activity
import android.content.Context
import android.util.Log
import kotlin.math.abs

/**
 * Частота экрана окна приложения (Go: internal/display). Fyne рисует по
 * таймеру 60 Гц, поэтому на экране 120 Гц кадры выходят неровными; режим
 * дисплея 60 Гц убирает худший случай. Окно BrowserActivity не затрагивается.
 */
object DisplayRate {
    private const val TAG = "MangaDisplay"

    /** max60 — просить 60 Гц; false — частоту выбирает система. */
    @JvmStatic
    fun apply(ctx: Context, max60: Boolean) {
        val activity = ctx as? Activity
        if (activity == null) {
            Log.w(TAG, "частота экрана: нет Activity ($ctx)")
            return
        }
        activity.runOnUiThread {
            try {
                val window = activity.window
                val lp = window.attributes
                lp.preferredDisplayModeId = 0
                lp.preferredRefreshRate = 0f
                if (max60) {
                    val display = activity.display
                    val cur = display?.mode
                    // тот же размер, частота ближе всего к 60 Гц
                    val mode = display?.supportedModes
                        ?.filter { cur != null && it.physicalWidth == cur.physicalWidth && it.physicalHeight == cur.physicalHeight }
                        ?.minByOrNull { abs(it.refreshRate - 60f) }
                    if (mode != null && abs(mode.refreshRate - 60f) < 1f) {
                        lp.preferredDisplayModeId = mode.modeId
                    } else {
                        lp.preferredRefreshRate = 60f // режима 60 Гц с этим разрешением нет
                    }
                }
                window.attributes = lp
                Log.i(TAG, "частота экрана: max60=$max60 mode=${lp.preferredDisplayModeId} rate=${lp.preferredRefreshRate}")
            } catch (e: Exception) {
                Log.w(TAG, "частота экрана", e)
            }
        }
    }
}

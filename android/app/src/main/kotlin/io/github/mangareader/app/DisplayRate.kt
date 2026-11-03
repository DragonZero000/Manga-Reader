package io.github.mangareader.app

import android.app.Activity
import android.content.Context
import android.util.Log
import kotlin.math.abs

/**
 * Частота экрана окна приложения (Go: internal/display). Кадры идут по vsync
 * выбранной частоты (Choreographer), поэтому 60 Гц нужно только для экономии
 * батареи: «Ограничить 60 Гц» выбирает режим дисплея 60 Гц. Без ограничения
 * частоту выбирает система, а на время движения драйвер просит наибольшую
 * частоту (ANativeWindow_setFrameRate) — её возвращает apply. Окно
 * BrowserActivity не затрагивается.
 */
object DisplayRate {
    private const val TAG = "MangaDisplay"

    /**
     * max60 — просить 60 Гц; false — частоту выбирает система. Возвращает
     * наибольшую частоту режимов дисплея с текущим разрешением (0 — неизвестна).
     */
    @JvmStatic
    fun apply(ctx: Context, max60: Boolean): Float {
        val activity = ctx as? Activity
        if (activity == null) {
            Log.w(TAG, "display refresh rate: no Activity ($ctx)")
            return 0f
        }
        val maxRate = try {
            val display = activity.display
            val cur = display?.mode
            display?.supportedModes
                ?.filter { cur != null && it.physicalWidth == cur.physicalWidth && it.physicalHeight == cur.physicalHeight }
                ?.maxOfOrNull { it.refreshRate } ?: 0f
        } catch (e: Exception) {
            Log.w(TAG, "display modes", e)
            0f
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
                Log.i(TAG, "display refresh rate: max60=$max60 mode=${lp.preferredDisplayModeId} rate=${lp.preferredRefreshRate}")
            } catch (e: Exception) {
                Log.w(TAG, "display refresh rate", e)
            }
        }
        return maxRate
    }
}

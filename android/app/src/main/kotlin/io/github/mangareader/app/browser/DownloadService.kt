package io.github.mangareader.app.browser

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.IBinder

/**
 * Foreground service на время загрузок: уведомление с прогрессом не даёт
 * Android прервать загрузку, когда пользователь ушёл в читалку или свернул
 * приложение. Останавливается, когда активных загрузок нет.
 */
class DownloadService : Service() {
    companion object {
        private const val CHANNEL = "downloads"
        private const val ID = 1
        @Volatile private var running = false

        /** Запустить, обновить или остановить службу по состоянию загрузок. */
        fun update(ctx: Context) {
            val app = ctx.applicationContext
            if (DownloadManager.active() > 0) {
                if (!running) {
                    running = true
                    app.startForegroundService(Intent(app, DownloadService::class.java))
                } else {
                    app.getSystemService(NotificationManager::class.java).notify(ID, build(app))
                }
            } else if (running) {
                running = false
                app.stopService(Intent(app, DownloadService::class.java))
            }
        }

        private fun build(ctx: Context): Notification {
            val nm = ctx.getSystemService(NotificationManager::class.java)
            if (nm.getNotificationChannel(CHANNEL) == null) {
                nm.createNotificationChannel(NotificationChannel(CHANNEL, "Загрузки", NotificationManager.IMPORTANCE_LOW))
            }
            val active = DownloadManager.list().filter { it.state == DownloadManager.State.RUNNING }
            val first = active.firstOrNull()
            val b = Notification.Builder(ctx, CHANNEL)
                .setSmallIcon(android.R.drawable.stat_sys_download)
                .setOngoing(true)
                .setOnlyAlertOnce(true)
                .setContentTitle(if (active.size > 1) "Скачивание: ${active.size} файла" else "Скачивание ${first?.name ?: ""}")
            if (first != null && first.total > 0) {
                val pct = (first.done * 100 / first.total).toInt().coerceIn(0, 100)
                b.setProgress(100, pct, false).setContentText("${first.name} — $pct%")
            } else {
                b.setProgress(0, 0, true)
            }
            return b.build()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startForeground(ID, build(this), ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
        if (DownloadManager.active() == 0) {
            running = false
            stopSelf()
        }
        return START_NOT_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null
}

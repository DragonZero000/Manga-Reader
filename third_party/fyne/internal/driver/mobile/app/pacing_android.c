//go:build android

/* mangareader: темп кадров Android (third_party/fyne/PATCHES.md).
 *
 * Кадровые колбэки AChoreographer и смена частоты экрана — на потоке ввода
 * (runInputQueue, у него есть ALooper); время показа кадров —
 * EGL_ANDROID_get_frame_timestamps на потоке GL (mainUI). */

#include <android/choreographer.h>
#include <android/looper.h>
#include <android/native_window.h>
#include <EGL/egl.h>
#include <EGL/eglext.h>
#include <stdatomic.h>
#include <stdint.h>
#include <string.h>
#include <time.h>
#include "_cgo_export.h"

extern EGLDisplay display;
extern EGLSurface surface;

static ALooper* pacingLooper;
static AChoreographer* pacingChoreographer;
static atomic_int vsyncWanted;
static int64_t vsyncPostedAt; /* заказан колбэк (0 — нет); только поток ввода */

/* Колбэк, не пришедший за столько нс, заказывается снова. */
#define VSYNC_LOST_NS 250000000LL

static PFNEGLGETNEXTFRAMEIDANDROIDPROC getNextFrameId;
static PFNEGLGETFRAMETIMESTAMPSANDROIDPROC getFrameTimestamps;
static EGLSurface timestampsSurface; /* поверхность с включёнными метками */

int64_t pacingMonoNow() {
	struct timespec ts;
	clock_gettime(CLOCK_MONOTONIC, &ts);
	return (int64_t)ts.tv_sec * 1000000000LL + ts.tv_nsec;
}

static void pacingFrameCallback(int64_t frameTimeNanos, void* data) {
	vsyncPostedAt = 0;
	onPacingVsync(frameTimeNanos);
}

static void pacingRefreshCallback(int64_t vsyncPeriodNanos, void* data) {
	onPacingPeriod(vsyncPeriodNanos);
}

/* Поток ввода, после ALooper_prepare. */
void pacingInitLooper(ALooper* looper) {
	pacingLooper = looper;
	pacingChoreographer = AChoreographer_getInstance();
	if (pacingChoreographer != NULL) {
		AChoreographer_registerRefreshRateCallback(pacingChoreographer, pacingRefreshCallback, NULL);
	}
}

/* Любой поток: заказать кадровый колбэк на ближайший vsync. */
void pacingRequestVsync() {
	atomic_store(&vsyncWanted, 1);
	if (pacingLooper != NULL) {
		ALooper_wake(pacingLooper);
	}
}

/* Поток ввода, после каждого ALooper_pollOnce. */
void pacingPoll() {
	if (!atomic_exchange(&vsyncWanted, 0) || pacingChoreographer == NULL) {
		return;
	}
	int64_t now = pacingMonoNow();
	if (vsyncPostedAt != 0 && now - vsyncPostedAt < VSYNC_LOST_NS) {
		return;
	}
	vsyncPostedAt = now;
	AChoreographer_postFrameCallback64(pacingChoreographer, pacingFrameCallback, NULL);
}

/* Поток GL: поверхность создана или уничтожена — метки включаются заново. */
void pacingSurfaceChanged() {
	timestampsSurface = EGL_NO_SURFACE;
}

/* Поток GL: включить метки времени кадров на текущей поверхности (1 — есть). */
int pacingEnableTimestamps() {
	if (surface == EGL_NO_SURFACE) {
		return 0;
	}
	if (timestampsSurface == surface) {
		return 1;
	}
	if (getNextFrameId == NULL) {
		const char* ext = eglQueryString(display, EGL_EXTENSIONS);
		if (ext == NULL || strstr(ext, "EGL_ANDROID_get_frame_timestamps") == NULL) {
			return 0;
		}
		getNextFrameId = (PFNEGLGETNEXTFRAMEIDANDROIDPROC)eglGetProcAddress("eglGetNextFrameIdANDROID");
		getFrameTimestamps = (PFNEGLGETFRAMETIMESTAMPSANDROIDPROC)eglGetProcAddress("eglGetFrameTimestampsANDROID");
	}
	if (getNextFrameId == NULL || getFrameTimestamps == NULL) {
		return 0;
	}
	if (!eglSurfaceAttrib(display, surface, EGL_TIMESTAMPS_ANDROID, EGL_TRUE)) {
		return 0;
	}
	timestampsSurface = surface;
	return 1;
}

/* Поток GL, перед eglSwapBuffers: номер кадра EGL (-1 — меток нет). */
int64_t pacingNextFrameId() {
	EGLuint64KHR id = 0;
	if (timestampsSurface == EGL_NO_SURFACE || timestampsSurface != surface) {
		return -1;
	}
	if (!getNextFrameId(display, surface, &id)) {
		return -1;
	}
	return (int64_t)id;
}

/* Поток GL: время показа кадра id (CLOCK_MONOTONIC, нс); -2 — ещё не
 * известно, -1 — не будет известно. */
int64_t pacingPresentTime(int64_t id) {
	static const EGLint names[] = {EGL_DISPLAY_PRESENT_TIME_ANDROID};
	EGLnsecsANDROID v = EGL_TIMESTAMP_INVALID_ANDROID;
	if (timestampsSurface == EGL_NO_SURFACE || timestampsSurface != surface) {
		return -1;
	}
	if (!getFrameTimestamps(display, surface, (EGLuint64KHR)id, 1, names, &v)) {
		return -1;
	}
	return v;
}

/* Поток GL: частота окна (0 — выбирает система). */
int32_t pacingSetFrameRate(ANativeWindow* window, float rate) {
	return ANativeWindow_setFrameRate(window, rate, ANATIVEWINDOW_FRAME_RATE_COMPATIBILITY_DEFAULT);
}

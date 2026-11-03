//go:build android

// mangareader: темп кадров Android (third_party/fyne/PATCHES.md).

package app

/*
#cgo LDFLAGS: -lnativewindow

#include <android/looper.h>
#include <android/native_window.h>
#include <stdint.h>

int32_t pacingSetFrameRate(ANativeWindow* window, float rate);
int64_t pacingMonoNow();
void pacingInitLooper(ALooper* looper);
void pacingRequestVsync();
void pacingPoll();
void pacingSurfaceChanged();
int pacingEnableTimestamps();
int64_t pacingNextFrameId();
int64_t pacingPresentTime(int64_t id);
*/
import "C"

import (
	"log"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2/internal/pacing"
)

// presentWait — сколько кадров ждать точного времени показа от EGL; потом
// время показа оценивается по SwapBuffers и сетке vsync.
const presentWait = 8

var (
	// lastVsync — последняя метка vsync от Choreographer (CLOCK_MONOTONIC, нс).
	lastVsync atomic.Int64
	// driving — кадры драйвера идут по Choreographer (Vsync вызван).
	driving atomic.Bool
	// vsyncs — метки vsync для цикла драйвера; полон — метка заменяется.
	vsyncs = make(chan time.Time, 1)
	// frameRates — запросы ANativeWindow_setFrameRate для потока GL.
	frameRates = make(chan float32, 1)
)

//export onPacingVsync
func onPacingVsync(ns C.int64_t) {
	lastVsync.Store(int64(ns))
	if !driving.Load() {
		return
	}
	t := monoTime(int64(ns))
	select {
	case vsyncs <- t:
	default:
		select {
		case <-vsyncs:
		default:
		}
		select {
		case vsyncs <- t:
		default:
		}
	}
}

// Vsync — метки vsync (время кадрового колбэка Choreographer) для цикла
// драйвера; кадр заказывается RequestVsync. С первого вызова кадровые
// колбэки идут в канал.
func Vsync() <-chan time.Time {
	driving.Store(true)
	return vsyncs
}

// RequestVsync заказывает кадровый колбэк на ближайший vsync (любой поток).
func RequestVsync() { C.pacingRequestVsync() }

// SetFrameRate просит систему показывать окно с частотой rate, Гц (0 —
// частоту выбирает система). Применяется в потоке GL; повторный запрос
// заменяет ещё не применённый.
func SetFrameRate(rate float32) {
	select {
	case frameRates <- rate:
	default:
		select {
		case <-frameRates:
		default:
		}
		select {
		case frameRates <- rate:
		default:
		}
	}
}

// applyFrameRate — ANativeWindow_setFrameRate для окна w (поток GL). Ошибка
// пишется в журнал один раз.
func applyFrameRate(w *C.ANativeWindow, rate float32) {
	if w == nil {
		return
	}
	if r := C.pacingSetFrameRate(w, C.float(rate)); r != 0 && !frameRateFailed {
		frameRateFailed = true
		log.Printf("pacing: ANativeWindow_setFrameRate(%g): error %d", rate, int(r))
	}
}

var frameRateFailed bool // только поток GL

//export onPacingPeriod
func onPacingPeriod(ns C.int64_t) {
	if ns > 0 {
		pacing.SetPeriod(time.Duration(ns))
	}
}

// monoTime переводит CLOCK_MONOTONIC (нс) во время Go.
func monoTime(ns int64) time.Time {
	return time.Now().Add(time.Duration(ns - int64(C.pacingMonoNow())))
}

// pendingPresent — кадр, время показа которого ещё не известно.
type pendingPresent struct {
	id    uint64
	egl   int64 // номер кадра EGL; -1 — меток нет
	start int64 // перед eglSwapBuffers, CLOCK_MONOTONIC
	age   int   // кадров с тех пор
}

// Только поток GL (mainUI).
var (
	frameSeq     uint64
	presentQueue []pendingPresent
	presented    []Presented // буфер PublishResult.Presented
)

// surfaceChanged — поверхность EGL создана или уничтожена: прежние номера
// кадров EGL недействительны.
func surfaceChanged() {
	C.pacingSurfaceChanged()
	for i := range presentQueue {
		presentQueue[i].egl = -1
	}
}

// swapStart — начало eglSwapBuffers (CLOCK_MONOTONIC); только поток GL.
var swapStart int64

// beforeSwap — номер следующего кадра EGL, если замер идёт и метки есть.
func beforeSwap() int64 {
	if !pacing.Observed() {
		return -1
	}
	swapStart = int64(C.pacingMonoNow())
	if C.pacingEnableTimestamps() == 0 {
		return -1
	}
	return int64(C.pacingNextFrameId())
}

// afterSwap — время кадра для driver/pacing: номер, время SwapBuffers и
// кадры, время показа которых стало известно (по порядку).
func afterSwap(egl int64) PublishResult {
	if !pacing.Observed() {
		return PublishResult{}
	}
	swapped := int64(C.pacingMonoNow())
	frameSeq++
	presentQueue = append(presentQueue, pendingPresent{id: frameSeq, egl: egl, start: swapStart})
	presented = presented[:0]
	n := 0
	for ; n < len(presentQueue); n++ {
		p := presentQueue[n]
		t := int64(-1)
		if p.egl >= 0 {
			t = int64(C.pacingPresentTime(C.int64_t(p.egl)))
		}
		if t == -2 && p.age < presentWait {
			break
		}
		if t > 0 {
			pacing.SetPresentSource(pacing.PresentEGL)
		} else {
			t = gridPresent(p.start)
		}
		presented = append(presented, Presented{ID: p.id, Present: monoTime(t)})
	}
	presentQueue = presentQueue[:copy(presentQueue, presentQueue[n:])]
	for i := range presentQueue {
		presentQueue[i].age++
	}
	if !driving.Load() {
		C.pacingRequestVsync() // сетка vsync для оценки следующих кадров
	}
	return PublishResult{FrameID: frameSeq, Swapped: monoTime(swapped), Presented: presented}
}

// gridPresent — первый vsync после отправки кадра (submitted, CLOCK_MONOTONIC,
// нс): возврат eglSwapBuffers бывает у самой границы vsync, и оценка по нему
// прыгала бы на соседний vsync. Без сетки vsync — submitted.
func gridPresent(submitted int64) int64 {
	v := lastVsync.Load()
	if v == 0 {
		return submitted
	}
	return pacing.NextVsync(time.Unix(0, submitted), time.Unix(0, v), pacing.Period()).UnixNano()
}

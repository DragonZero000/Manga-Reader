// mangareader: темп кадров Windows — vsync монитора окна, время показа по
// DWM (third_party/fyne/PATCHES.md).

package glfw

import (
	"encoding/binary"
	"fmt"
	"log"
	"syscall"
	"time"
	"unsafe"

	"github.com/go-gl/glfw/v3.4/glfw"

	"fyne.io/fyne/v2/internal/pacing"
)

const (
	// vsyncAhead — таймер цикла чуть чаще vsync: такт ждёт в канале тикера,
	// пока SwapBuffers ждёт vsync, и цикл держится на vsync.
	vsyncAhead = 1.1
	// idleAfter — без кадров дольше этого цикл только опрашивает события.
	idleAfter = 250 * time.Millisecond
	// idleTick — период таймера в простое.
	idleTick = time.Second / 60
	// vsyncOffFrames — столько кадров подряд без ожидания vsync — vsync
	// выключен в драйвере.
	vsyncOffFrames = 30
)

var (
	dwmapi                      = syscall.NewLazyDLL("dwmapi.dll")
	dwmGetCompositionTimingInfo = dwmapi.NewProc("DwmGetCompositionTimingInfo")
	dwmFlush                    = dwmapi.NewProc("DwmFlush")
	queryPerformanceCounter     = kernel32.NewProc("QueryPerformanceCounter")
	queryPerformanceFrequency   = kernel32.NewProc("QueryPerformanceFrequency")
)

// DWM_TIMING_INFO упакована (pshpack1): поля читаются по смещениям.
const (
	dwmTimingInfoSize     = 292
	dwmQpcRefreshPeriodAt = 12
	dwmQpcVBlankAt        = 28
)

// vsyncCheck — проверка, ждёт ли SwapBuffers vsync. Только главный поток.
var vsyncCheck struct {
	frames int           // кадров подряд: друг за другом и SwapBuffers < ½ периода
	gaps   time.Duration // сумма промежутков между ними
	off    bool          // vsync выключен: ждём композицию DWM после SwapBuffers
}

func init() {
	if _, _, ok := dwmVBlank(); ok {
		pacing.SetPresentSource(pacing.PresentDWM)
	}
}

// enableVsync включает vsync для контекста текущего окна. Поток контекста.
func enableVsync() {
	glfw.SwapInterval(1)
	if !vsyncCheck.off {
		pacing.SetMode(pacing.ModeVsync)
	}
}

// resetVsyncCheck начинает проверку vsync заново (окно на другом мониторе).
func resetVsyncCheck() {
	vsyncCheck.frames, vsyncCheck.gaps = 0, 0
	if vsyncCheck.off {
		vsyncCheck.off = false
		pacing.SetMode(pacing.ModeVsync)
	}
}

// checkVsync: SwapBuffers с start по end. Если vsyncOffFrames кадров подряд
// шли друг за другом, SwapBuffers возвращался быстрее половины периода и
// кадры шли чаще vsync, vsync выключен в драйвере: кадр после SwapBuffers
// ждёт композиции DWM (waitVblank), без DWM — таймер с частотой монитора.
func checkVsync(start, end time.Time) {
	if vsyncCheck.off {
		return
	}
	p := pacing.Period()
	gap := end.Sub(lastSwap)
	if lastSwap.IsZero() || gap > 2*p || end.Sub(start) >= p/2 {
		vsyncCheck.frames, vsyncCheck.gaps = 0, 0
		return
	}
	vsyncCheck.frames++
	vsyncCheck.gaps += gap
	if vsyncCheck.frames < vsyncOffFrames {
		return
	}
	avg := vsyncCheck.gaps / time.Duration(vsyncCheck.frames)
	vsyncCheck.frames, vsyncCheck.gaps = 0, 0
	if avg >= p*95/100 {
		return // кадры не чаще vsync: SwapBuffers его ждёт
	}
	vsyncCheck.off = true
	how := "waiting for DWM composition"
	if dwmFlush.Find() == nil {
		pacing.SetMode(pacing.ModeDWM)
	} else {
		pacing.SetMode(pacing.ModeTimer)
		how = fmt.Sprintf("a %.0f Hz timer", pacing.RefreshRate())
	}
	log.Printf("pacing: SwapBuffers does not wait for vsync (frames %.1f ms apart, vsync %.1f ms), switching to %s",
		float64(avg)/float64(time.Millisecond), float64(p)/float64(time.Millisecond), how)
}

// waitVblank: если vsync выключен в драйвере, кадр ждёт ближайшей композиции
// DWM (DwmFlush) — она идёт по vblank монитора. Без DWM — не ждёт, темп
// держит таймер с частотой монитора.
func waitVblank() {
	if vsyncCheck.off && dwmFlush.Find() == nil {
		dwmFlush.Call()
	}
}

// tickPeriod — период таймера цикла: в простое 1/60 с, при движении чуть
// чаще vsync (vsyncAhead) или, если vsync выключен, период монитора.
func tickPeriod(now time.Time) time.Duration {
	if lastSwap.IsZero() || now.Sub(lastSwap) > idleAfter {
		return idleTick
	}
	p := pacing.Period()
	if vsyncCheck.off && dwmFlush.Find() != nil {
		return p
	}
	return time.Duration(float64(p) / vsyncAhead)
}

// frameTimeAt — время кадра, начатого в now: vblank, на котором он будет
// показан; без DWM — now. Такт начинается сразу после SwapBuffers, у самой
// границы vblank V, и кадр покажется на V+P: берётся первый vblank после
// now + P/2, иначе время кадра прыгало бы между V и V+P.
func frameTimeAt(now time.Time) time.Time {
	vblank, period, ok := dwmVBlank()
	if !ok {
		return now
	}
	return pacing.NextVsync(now.Add(period/2), vblank, period)
}

// dwmVBlank — vblank композиции DWM (ближайший, может быть в будущем) и её
// период; ok=false — DWM недоступен.
func dwmVBlank() (vblank time.Time, period time.Duration, ok bool) {
	if dwmGetCompositionTimingInfo.Find() != nil {
		return time.Time{}, 0, false
	}
	var info [dwmTimingInfoSize]byte
	binary.LittleEndian.PutUint32(info[:], dwmTimingInfoSize)
	if hr, _, _ := dwmGetCompositionTimingInfo.Call(0, uintptr(unsafe.Pointer(&info[0]))); hr != 0 {
		return time.Time{}, 0, false
	}
	var now, freq int64
	queryPerformanceCounter.Call(uintptr(unsafe.Pointer(&now)))
	queryPerformanceFrequency.Call(uintptr(unsafe.Pointer(&freq)))
	at := time.Now()
	qpcPeriod := int64(binary.LittleEndian.Uint64(info[dwmQpcRefreshPeriodAt:]))
	qpcVBlank := int64(binary.LittleEndian.Uint64(info[dwmQpcVBlankAt:]))
	if freq <= 0 || qpcPeriod <= 0 || qpcVBlank == 0 {
		return time.Time{}, 0, false
	}
	toDur := func(q int64) time.Duration { return time.Duration(float64(q) * float64(time.Second) / float64(freq)) }
	return at.Add(toDur(qpcVBlank - now)), toDur(qpcPeriod), true
}

// presentAfter — первый vblank DWM после отправки кадра (submitted); без DWM — submitted.
func presentAfter(submitted time.Time) time.Time {
	vblank, period, ok := dwmVBlank()
	if !ok {
		return submitted
	}
	return pacing.NextVsync(submitted, vblank, period)
}

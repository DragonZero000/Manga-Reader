//go:build frameprobe

package reader

import (
	"log"
	"time"
)

// flipStart — время начала последнего перелистывания (замер задержки).
var flipStart time.Time

// probeFlip отмечает начало перелистывания. Вызывается из UI-потока.
func probeFlip() { flipStart = time.Now() }

// probeShown пишет в журнал время от перелистывания до показа страницы;
// cached — страница взята из кэша без декодирования.
func probeShown(cached bool) {
	if flipStart.IsZero() {
		return
	}
	src := "decoded"
	if cached {
		src = "cached"
	}
	log.Printf("flipprobe: page shown in %s (%s)", time.Since(flipStart).Round(time.Millisecond), src)
	flipStart = time.Time{}
}

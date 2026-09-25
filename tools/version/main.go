// Команда version печатает версию приложения из FyneApp.toml — для Makefile
// и CI, чтобы версия задавалась в одном месте.
//
//	go run ./tools/version         # 0.1.0
//	go run ./tools/version -code   # 100 (versionCode Android)
package main

import (
	"flag"
	"fmt"
	"log"

	"mangareader/internal/appversion"
)

func main() {
	code := flag.Bool("code", false, "печатать номер сборки Android (versionCode)")
	path := flag.String("f", "FyneApp.toml", "файл метаданных")
	flag.Parse()
	log.SetFlags(0)
	v, err := appversion.Read(*path)
	if err != nil {
		log.Fatal("version: ", err)
	}
	if *code {
		fmt.Println(v.Code())
		return
	}
	fmt.Println(v)
}

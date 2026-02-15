package main

import (
	"github.com/biisal/fast-stream-bot/config"
)

func main() {
	printLogo("v1.0.0")
	cfg := config.MustLoad("")
	flags := perseFlags()
	if err := mount(cfg, flags); err != nil {
		panic(err)
	}
}

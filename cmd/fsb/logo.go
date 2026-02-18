package main

import (
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/fatih/color"
)

func getVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		return info.Main.Version
	}
	return "(unknown)"
}

func printLogo(version string) {
	if version == "" {
		version = getVersion()
	}

	gitLink := color.HiGreenString("https://github.com/biisal/fast-stream-bot")
	versionStr := color.HiYellowString("You are using version %s", version)

	logo := fmt.Sprintf(`
█▀▀ ▄▀█ █▀ ▀█▀ ▄▄ █▀ ▀█▀ █▀█ █▀▀ ▄▀█ █▀▄▀█ ▄▄ █▄▄ █▀█ ▀█▀
█▀░ █▀█ ▄█ ░█░ ░░ ▄█ ░█░ █▀▄ ██▄ █▀█ █░▀░█ ░░ █▄█ █▄█ ░█░
Thanks for using
⭐ Star on GitHub: %s
%s
`, gitLink, versionStr)
	slog.Info(logo)
}

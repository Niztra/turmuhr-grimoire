//go:build !windows

package main

import (
	"os/exec"
	"runtime"
)

func initConsole() {}

func openBrowser(url string) {
	if runtime.GOOS == "darwin" {
		exec.Command("open", url).Start()
		return
	}
	exec.Command("xdg-open", url).Start()
}

package main

import (
	"os/exec"
	"syscall"
)

// Umlaute im Konsolenfenster richtig anzeigen
func initConsole() {
	k := syscall.NewLazyDLL("kernel32.dll")
	k.NewProc("SetConsoleOutputCP").Call(65001)
}

func openBrowser(url string) { exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start() }

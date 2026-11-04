//go:build windows

package terminal

import (
	"os"

	"golang.org/x/sys/windows"
)

// enableVT makes a Windows console read the escape sequences written to it.
func enableVT(out *os.File) func() {
	h := windows.Handle(out.Fd())
	var mode uint32
	if windows.GetConsoleMode(h, &mode) != nil {
		return func() {}
	}
	_ = windows.SetConsoleMode(h, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING)
	return func() { _ = windows.SetConsoleMode(h, mode) }
}

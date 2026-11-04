package terminal

import (
	"fmt"
	"io"
	"os"
	"sync"

	"golang.org/x/term"
)

// IsTerminal says in and out are a terminal, for the live region and raw keys.
func IsTerminal(in, out *os.File) bool {
	return term.IsTerminal(int(in.Fd())) && term.IsTerminal(int(out.Fd()))
}

// Raw puts the terminal in raw mode with bracketed paste. The caller defers restore
// (safe to call more than once), which is what leaves the terminal usable on every way
// out, a panic included.
func Raw(in, out *os.File) (restore func(), err error) {
	endVT := enableVT(out)
	old, err := term.MakeRaw(int(in.Fd()))
	if err != nil {
		endVT()
		return nil, fmt.Errorf("terminal: raw mode: %w", err)
	}
	_, _ = io.WriteString(out, pasteOn)
	var once sync.Once
	return func() {
		once.Do(func() {
			_, _ = io.WriteString(out, pasteOff)
			_ = term.Restore(int(in.Fd()), old)
			endVT()
		})
	}, nil
}

// ColsOf is the width of terminal f now, for Config.Cols; 80 when it can't say.
func ColsOf(f *os.File) func() int {
	return func() int {
		if w, _, err := term.GetSize(int(f.Fd())); err == nil && w > 0 {
			return w
		}
		return 80
	}
}

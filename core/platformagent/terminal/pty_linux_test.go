//go:build linux

package terminal

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
	"golang.org/x/term"

	"github.com/enowx/enowx/core/platformagent/record"
)

// openPTY is a terminal's two ends.
func openPTY(t *testing.T) (master, slave *os.File) {
	t.Helper()
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skip("no PTY here: ", err)
	}
	t.Cleanup(func() { master.Close() })
	if err := unix.IoctlSetPointerInt(int(master.Fd()), unix.TIOCSPTLCK, 0); err != nil {
		t.Fatal(err)
	}
	n, err := unix.IoctlGetInt(int(master.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatal(err)
	}
	slave, err = os.OpenFile(fmt.Sprintf("/dev/pts/%d", n), os.O_RDWR, 0)
	if err != nil {
		t.Skip("no PTY here: ", err)
	}
	t.Cleanup(func() { slave.Close() })
	return master, slave
}

// In a real terminal: raw keys reach it without a line discipline in between, paste
// mode is on, and everything is as it was afterwards.
func TestInARealTerminal(t *testing.T) {
	master, slave := openPTY(t)
	before, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if err := unix.IoctlSetWinsize(int(slave.Fd()), unix.TIOCSWINSZ, &unix.Winsize{Row: 24, Col: 72}); err != nil {
		t.Fatal(err)
	}
	if !IsTerminal(slave, slave) {
		t.Fatal("not a terminal")
	}
	if got := ColsOf(slave)(); got != 72 {
		t.Fatalf("cols %d", got)
	}
	restore, err := Raw(slave, slave)
	if err != nil {
		t.Fatal(err)
	}
	defer restore()

	out := &lockedBuffer{}
	go func() { _, _ = io.Copy(out, master) }()

	r := &fakeRunner{mode: record.ModeDefault}
	got := make(chan string, 1)
	r.onTurn = func(ctx context.Context, text string) error {
		got <- text
		return nil
	}
	tm := New(Config{In: slave, Out: slave, Interactive: true, Cols: ColsOf(slave)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- tm.Run(ctx, r) }()

	waitFor(t, "the prompt", func() bool { return strings.Contains(out.String(), "❯ ") }, out.String)
	if !strings.Contains(out.String(), pasteOn) {
		t.Fatalf("paste mode isn't on: %q", out.String())
	}
	if _, err := master.Write([]byte("\x1b[200~two\nlines\x1b[201~")); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := master.Write([]byte("\r")); err != nil {
		t.Fatal(err)
	}
	select {
	case text := <-got:
		if text != "two\nlines" {
			t.Fatalf("message %q", text)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("no turn; output: %q", out.String())
	}
	if _, err := master.Write([]byte("\x04")); err != nil { // Ctrl+D, raw: to the program
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("Run didn't end; output: %q", out.String())
	}

	restore()
	after, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(before) != fmt.Sprint(after) {
		t.Fatalf("the terminal isn't as it was:\n%v\n%v", before, after)
	}
	waitFor(t, "paste mode off", func() bool { return strings.Contains(out.String(), pasteOff) }, out.String)
}

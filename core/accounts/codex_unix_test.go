//go:build unix

package accounts

import (
	"os"

	"golang.org/x/sys/unix"
)

func tryWriterLock(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

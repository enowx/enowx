//go:build !linux && !darwin && !windows

package procstat

import "errors"

func processes() ([]Proc, error) { return nil, errors.ErrUnsupported }

func process(pid int) (Proc, error) { return Proc{}, errors.ErrUnsupported }

func memory() (total, used uint64) { return 0, 0 }

// Env isn't read on this system.
func Env(pid int, key string) string { return "" }

// Args aren't read on this system.
func Args(pid int) []string { return nil }

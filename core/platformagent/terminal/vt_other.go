//go:build !windows

package terminal

import "os"

// enableVT has nothing to do: terminals here read escape sequences as they are.
func enableVT(*os.File) func() { return func() {} }

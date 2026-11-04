//go:build !linux

package platformagent

// HardenProcess does nothing here: no simple call does what Linux's does. On macOS
// PT_DENY_ATTACH stops a debugger but not ps reading the environment, and it kills a
// process already being debugged. docs/claude-platform.md says what remains.
func HardenProcess() error { return nil }

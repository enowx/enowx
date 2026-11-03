//go:build windows

package session

// marked leaves argv as it is: there is no /bin/sh to name. A session's processes are
// told by their environment (EnvSession), which procstat reads on Windows.
func marked(_ string, argv []string) []string { return argv }

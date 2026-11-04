//go:build !windows

package tools

// foldEnvName leaves name: environment names are case-sensitive here.
func foldEnvName(name string) string { return name }

//go:build !windows

package accounts

func sameEnvName(a, b string) bool { return a == b }

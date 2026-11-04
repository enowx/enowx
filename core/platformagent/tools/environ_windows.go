//go:build windows

package tools

import "strings"

// foldEnvName folds name: Windows ignores the case of environment names.
func foldEnvName(name string) string { return strings.ToUpper(name) }

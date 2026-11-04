package accounts

import "strings"

func sameEnvName(a, b string) bool { return strings.EqualFold(a, b) }

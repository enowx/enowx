package main

import _ "embed"

// MyGo uses the same source for the executable's fallback notification icon.
//
//go:embed resources/icon.png
var applicationIcon []byte

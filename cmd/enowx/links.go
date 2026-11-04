package main

import (
	"log"
	"os"
	"strings"

	"github.com/egoist/mygo"

	"github.com/enowx/enowx/core/designshare"
)

// enowx:// links reach the app differently per system. macOS sends them to the
// running app by Apple Events. Windows and Linux start a new process with the link
// as its argument, which hands it to the first one by MyGo's single-instance lock.
// Only a launch that brings a link stops there: enowX can otherwise run twice, for
// another server, as it always could.

// bringsLink says this launch's arguments hold a link.
func bringsLink(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(strings.ToLower(a), designshare.LinkScheme+"://") {
			return true
		}
	}
	return false
}

// takeLinks sets up what links need. It says false when this launch's link was
// handed to the app already running, and this one has nothing else to do. An app
// with a home of its own (make dev, a test) neither holds the lock nor registers.
func takeLinks() bool {
	if os.Getenv("ENOWX_HOME") != "" {
		return true
	}
	if !mygo.App.RequestSingleInstanceLock() && bringsLink(os.Args[1:]) {
		return false
	}
	// The installers register the scheme; an install that updated itself from
	// before it existed never ran one again.
	if mygo.App.IsPackaged() && !mygo.App.IsURLSchemeRegistered(designshare.LinkScheme) {
		if err := mygo.App.RegisterURLScheme(designshare.LinkScheme); err != nil {
			log.Printf("enowx: links: %v", err)
		}
	}
	return true
}

package ui

import (
	"os"
	"path/filepath"
	"strings"
)

// bundleOutsideApplications says an app bundle at exe runs from somewhere it may not
// stay: a download, a backup, a disk image. Not a build under development, which has
// a data folder of its own, nor a program with no bundle.
func bundleOutsideApplications(exe, home, enowxHome string) bool {
	i := strings.Index(exe, ".app/")
	if i < 0 || enowxHome != "" {
		return false
	}
	folder := filepath.Dir(exe[:i+len(".app")])
	return folder != "/Applications" && folder != filepath.Join(home, "Applications")
}

// warnBundlePlace says once, on the first connection, that the app runs from
// outside Applications.
func (a *App) warnBundlePlace() {
	if a.bundleWarned {
		return
	}
	a.bundleWarned = true
	exe, err := os.Executable()
	home, herr := os.UserHomeDir()
	if err != nil || herr != nil || !bundleOutsideApplications(exe, home, os.Getenv("ENOWX_HOME")) {
		return
	}
	a.status = "enowX is running from " + filepath.Dir(exe[:strings.Index(exe, ".app/")+len(".app")]) + ". Move it to Applications: if it moves later, agents reach it there."
	a.notice = a.status
}

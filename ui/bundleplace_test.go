package ui

import "testing"

// Only an installed app's bundle kept somewhere else is warned of.
func TestBundleOutsideApplications(t *testing.T) {
	const home = "/Users/ada"
	for exe, want := range map[string]bool{
		"/Applications/enowX.app/Contents/MacOS/enowX":               false,
		home + "/Applications/enowX.app/Contents/MacOS/enowX":        false,
		home + "/.enowx-app/app-backup/old.app/Contents/MacOS/enowX": true,
		"/Volumes/enowX/enowX.app/Contents/MacOS/enowX":              true,
		"/usr/local/bin/enowxd":                                      false,
	} {
		if got := bundleOutsideApplications(exe, home, ""); got != want {
			t.Errorf("%s: %v", exe, got)
		}
	}
	if bundleOutsideApplications("/tmp/build/enowX.app/Contents/MacOS/enowX", home, home+"/.enowx-dev") {
		t.Error("a build under development was warned of")
	}
}

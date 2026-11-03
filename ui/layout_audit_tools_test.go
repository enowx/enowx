package ui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/androidview"
)

// stubAndroidSDK is an SDK whose adb has an emulator online, whose log says a
// few things, and whose emulator has another virtual device stopped.
func stubAndroidSDK(t *testing.T) string {
	root := t.TempDir()
	write := func(rel, script string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write("platform-tools/adb", `
case "$*" in
  "devices -l") printf 'List of devices attached\nemulator-5554 device product:x model:sdk device:emu\n\n' ;;
  *"emu avd name"*) printf 'Pixel_8_emulator_with_a_name_too_long_for_a_row_of_the_side_panel\nOK\n' ;;
  *"ro.build.version.sdk"*) echo 34 ;;
  *logcat*) printf '10-25 14:13:45.123  4242  4250 W View: slow frame, 22 ms\n10-25 14:13:46.123  4242  4250 E enowX: a line long enough to have to wrap inside the frame of the log, whatever the width of the window it shows in happens to be at the time\n'; sleep 2 ;;
esac
`)
	write("emulator/emulator", `
case "$*" in
  "-list-avds") printf 'Pixel_8_emulator_with_a_name_too_long_for_a_row_of_the_side_panel\nTablet\n' ;;
  "-accel-check") echo ok ;;
  *) sleep 2 ;;
esac
`)
	return root
}

var auditUpstream = sync.OnceValue(func() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Long", "a value long enough to have to wrap inside the frame of the viewer, whatever the width of the window it shows in happens to be at the time, and then some more")
		fmt.Fprintf(w, `{"users":[{"id":1,"name":"Ada"}],"path":%q}`, r.URL.Path)
	}))
})

// auditToolPages are the Traffic and Android tools past their first page, and
// what they open in the center: a real exchange through the audit's daemon, and
// a device of the stub SDK.
func auditToolPages() []auditPage {
	session := func(a *App, tool string) proto.SessionInfo {
		a.page = ""
		a.selectSession(a.sessions[0])
		a.openPanel(tool)
		return a.sessions[0]
	}
	exchange := func(a *App, tab string) {
		s := session(a, panelTraffic)
		sent, err := a.client.TrafficSend(proto.TrafficSendParams{Method: "GET", URL: auditUpstream().URL + "/a/long/path/that/runs/inside/the/frame/of/the/viewer/and/of/the/row/in/the/side/panel", Session: s.ID})
		if err != nil {
			panic(err)
		}
		a.trafficOf().Apply(sent)
		a.openCenter(s, centerExchange+fmt.Sprintf("%d %s", sent.ID, "a long name"))
	}
	return []auditPage{
		{name: "traffic-rules", open: func(a *App) { session(a, panelTraffic); a.trafficOf().ShowPage("Rules") }},
		{name: "traffic-intercept", open: func(a *App) { session(a, panelTraffic); a.trafficOf().ShowPage("Intercept") }},
		{name: "traffic-requests", open: func(a *App) { session(a, panelTraffic); a.trafficOf().ShowPage("Requests") }},
		{name: "traffic-exchange", open: func(a *App) { exchange(a, "") }},
		{name: "android-logcat", open: func(a *App) {
			a.openCenter(session(a, panelAndroid), centerAndroid+androidview.ViewLogcat)
		}},
		{name: "android-screen", open: func(a *App) {
			a.openCenter(session(a, panelAndroid), centerAndroid+androidview.ViewScreen)
		}},
	}
}

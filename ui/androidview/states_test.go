package androidview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/wrapcheck"
)

// TestAndroidStates draws each state of the tool the design lists: what it must
// say, and that nothing sticks out of what wraps it, in the panel's 320. Hosts
// have no client: what the status says is all it draws.
func TestAndroidStates(t *testing.T) {
	mac := Host{ID: "mac", Name: "this Mac", Session: true, Local: true}
	lab := Host{ID: "lab", Name: "enowxlabs"}
	full := proto.AndroidStatus{SDK: "/sdk", ADB: true, Emulator: true, Accel: true}
	devices := func(v *View) {
		st := full
		st.Devices = []proto.AndroidDevice{
			{Serial: "emulator-5554", Kind: "emulator", Name: "Pixel 8 emulator", State: "device", API: "34"},
			{Serial: "tv1", Kind: "phone", Name: "Living room TV with a rather long name of its own", State: "device", API: "33", TV: true},
			{Serial: "a1", Kind: "phone", Name: "Galaxy S23", State: "unauthorized"},
			{Serial: "a2", Kind: "phone", Name: "Old phone", State: "offline"},
			{Kind: "emulator", Name: "Booting", State: "starting"},
		}
		st.AVDs = []string{"Pixel 8 emulator", "Booting", "Tablet"}
		v.status["mac"] = st
		v.command = "flutter run -d $ANDROID_SERIAL"
		v.project = proto.AndroidProject{Kind: "flutter", Package: "com.enowx.app.with.a.rather.long.package.name"}
	}
	logging := func(v *View) {
		v.logging = true
		v.logFor = "mac/emulator-5554|com.enowx.app.with.a.rather.long.package.name"
		v.log = []proto.AndroidLogLine{{Time: "09:00:00.000", Level: "I", Tag: "ActivityManager", Message: "Start proc com.enowx.app"},
			{Time: "09:00:00.100", Level: "D", Tag: "enowX", Message: "connected to this Mac"},
			{Time: "09:00:00.200", Level: "W", Tag: "View", Message: "slow frame, 22 ms"},
			{Time: "09:00:00.300", Level: "E", Tag: "enowX", Message: strings.Repeat("a long line of the log that wraps ", 8)}}
	}
	states := []struct {
		name   string
		set    func(*View)
		hosts  []Host
		center string
		wants  []string
	}{
		{name: "no-devices", set: func(v *View) { v.status["mac"] = full }, wants: []string{"No devices: start an emulator after Set up Android, or connect a phone.", "No device online", "Connect a phone"}},
		{name: "devices", set: devices, wants: []string{"Pixel 8 emulator", "Running · API 34 · this Mac", "Android TV · API 33 · this Mac", "Allow USB debugging on the phone · this Mac", "Offline · this Mac", "Starting… · this Mac", "Stopped · this Mac", "Start", "Run app", "Flutter app · com.enowx", "Screen", "Logcat", "Catch its traffic", "Stop", "Connect a phone"}},
		{name: "caught-busy", set: func(v *View) {
			devices(v)
			v.caught["mac/emulator-5554"] = &caught{}
			v.busy, v.notice = "Catching Pixel 8 emulator · this Mac's traffic…", "Catching Pixel 8 emulator · this Mac's traffic…"
		}, wants: []string{"Stop catching", "Catching Pixel 8 emulator · this Mac's traffic…"}},
		{name: "host-error", hosts: []Host{mac, lab}, set: func(v *View) {
			v.status["mac"] = proto.AndroidStatus{}
			v.errs["lab"] = "The host can't be reached right now"
		}, wants: []string{"enowxlabs: The host can't be reached right now", "this Mac has no Android SDK yet.", "Set up Android"}},
		{name: "slow", set: func(v *View) {
			st := full
			st.Accel, st.AccelWhy, st.AVDs = false, "KVM isn't available to this user", []string{"Tablet"}
			v.status["mac"] = st
		}, wants: []string{"this Mac's emulator is slow: KVM isn't available to this user"}},
		{name: "setup-running", set: func(v *View) {
			v.setup = &localSetup{}
			v.setup.out.WriteString("Downloading the command line tools…\nAccept? (y/N)\n")
		}, wants: []string{"Setting Android up on this machine", "Accept the licenses shown"}},
		{name: "setup-failed", set: func(v *View) {
			v.setup = &localSetup{done: true, err: "android: sdkmanager failed: exit status 1"}
		}, wants: []string{"android: sdkmanager failed: exit status 1", "Close"}},
		{name: "connect", hosts: []Host{mac, lab}, set: func(v *View) {
			v.status["mac"] = full
			v.connect = connectForm{open: true, err: "adb: failed to connect"}
		}, wants: []string{"Connect a phone or a TV", "Through", "Address", "Pairing address", "Pairing code", "adb: failed to connect", "Connect"}},
		{name: "connecting", set: func(v *View) {
			v.status["mac"] = full
			v.connect = connectForm{open: true, busy: true, address: "192.168.1.24:5555"}
		}, wants: []string{"Connecting…"}},
		{name: "screen-offline", center: ViewScreen, set: func(v *View) { v.status["mac"] = full }, wants: []string{"The device's screen shows here once it runs."}},
		{name: "screen-closed", center: ViewScreen, set: devices, wants: []string{"Pixel 8 emulator · this Mac · screen", "Show its screen"}},
		{name: "screen-connecting", center: ViewScreen, set: func(v *View) {
			devices(v)
			v.screen = &screen{key: "mac/emulator-5554"}
		}, wants: []string{"Connecting to the device…", "Back", "Home", "Recent apps", "Volume down", "Power", "Close the screen"}},
		{name: "screen-cannot-decode", center: ViewScreen, set: func(v *View) {
			devices(v)
			v.screen = &screen{key: "mac/emulator-5554", err: "This system's web view can't decode video: Pop out shows it in a window of its own."}
		}, wants: []string{"can't decode video"}},
		{name: "screen-portrait", center: ViewScreen, set: func(v *View) {
			devices(v)
			v.screen = &screen{key: "mac/emulator-5554", view: stubView{}, width: 540, height: 1200}
		}, wants: []string{"stub screen"}},
		{name: "screen-tv", center: ViewScreen, set: func(v *View) {
			devices(v)
			v.screen = &screen{key: "mac/emulator-5554", view: stubView{}, width: 1920, height: 1080, tv: true}
		}, wants: []string{"stub screen", "OK", "Left", "Up", "Down", "Right", "Menu"}},
		{name: "logcat-offline", center: ViewLogcat, set: func(v *View) { v.status["mac"] = full }, wants: []string{"The device's log shows here once it runs."}},
		{name: "logcat", center: ViewLogcat, set: func(v *View) { devices(v); logging(v) }, wants: []string{"Only com.enowx", "All", "Warn", "slow frame, 22 ms"}},
		{name: "logcat-no-package", center: ViewLogcat, set: func(v *View) {
			devices(v)
			logging(v)
			v.project.Package, v.logFor = "", "mac/emulator-5554|"
		}, wants: []string{"Start proc com.enowx.app"}},
		{name: "logcat-filtered-empty", center: ViewLogcat, set: func(v *View) { devices(v); logging(v); v.search = "nothing like this" }, wants: []string{"Search the log"}},
	}
	for _, st := range states {
		t.Run(st.name, func(t *testing.T) {
			v := New()
			if st.set != nil {
				st.set(v)
			}
			hosts := st.hosts
			if hosts == nil {
				hosts = []Host{mac}
			}
			wanted := func() bool { return false } // the first answer stops following
			o := Options{Hosts: hosts, Update: func(fn func()) { fn() }, Current: func() string { return st.center }}
			tt := ui.NewTester(func(c *ui.Context) {
				c.SetTheme(theme.Window())
				ui.Row(c).Width(1200).Height(720).Background(theme.Frame).TextColor(theme.Text).AlignItems(ui.Stretch).Children(func() {
					ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
						switch st.center {
						case ViewScreen:
							v.DrawScreen(c, o, wanted)
						case ViewLogcat:
							v.DrawLogcat(c, o, wanted)
						}
					})
					ui.Column(c).Width(320).Shrink(0).Background(theme.Panel).Children(func() { v.Draw(c, o, wanted) })
				})
			}, 1200, 720)
			for range 4 {
				tt.Frame()
			}
			for _, want := range st.wants {
				if !hasTextPart(tt, want) {
					t.Errorf("no %q in %q", want, tt.Texts())
				}
			}
			if out := wrapcheck.OutOfParents(tt); len(out) > 0 {
				t.Errorf("out of their parents:\n%s", strings.Join(out, "\n"))
			}
			shotTo(t, tt, "android-"+st.name+".png")
		})
	}
}

// hasTextPart says whether some text drawn holds part.
func hasTextPart(tt *ui.Tester, part string) bool {
	for _, text := range tt.Texts() {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

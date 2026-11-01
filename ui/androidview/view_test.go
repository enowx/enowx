package androidview

import (
	"image/png"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/daemon"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The tool lists the host's devices and virtual devices, reads the
// project's app, runs it on a device in a tab, and shows the device's
// log for the app.
func TestAndroidTool(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	root := t.TempDir()
	write := func(rel, script string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte("#!/bin/sh\n"+script), 0o755)
	}
	write("platform-tools/adb", `
case "$*" in
  "devices -l") printf 'List of devices attached\nemulator-5554 device product:x model:sdk device:emu\n\n' ;;
  *"emu avd name"*) printf 'enowX_Phone\nOK\n' ;;
  *"ro.build.version.sdk"*) echo 35 ;;
  *"pidof com.example.app"*) echo 4242 ;;
  *logcat*) printf '10-25 14:13:45.123  4242  4250 I flutter: Hello from the app\n'; sleep 30 ;;
esac
`)
	write("emulator/emulator", `
case "$*" in
  "-list-avds") printf 'enowX_Phone\nTablet\n' ;;
  "-accel-check") echo ok ;;
  *) sleep 30 ;;
esac
`)
	t.Setenv("ANDROID_HOME", root)
	srv, err := daemon.NewServer(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	a, b := net.Pipe()
	go srv.ServeConn(a)
	cl, err := client.New(b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cl.Close()
	project := t.TempDir()
	os.WriteFile(filepath.Join(project, "pubspec.yaml"), []byte("name: app\nflutter:\n"), 0o644)
	os.MkdirAll(filepath.Join(project, "android", "app"), 0o755)
	os.WriteFile(filepath.Join(project, "android", "app", "build.gradle"), []byte(`android { defaultConfig { applicationId "com.example.app" } }`), 0o644)
	if _, err := cl.AddProject(project); err != nil {
		t.Fatal(err)
	}
	sess, err := cl.CreateSession(proto.SessionCreateParams{Project: project, Agent: "shell", BranchMode: proto.BranchNone})
	if err != nil {
		t.Fatal(err)
	}
	queue := make(chan func(), 256)
	v := New()
	defer v.Close()
	var opened, front string
	wanted := func() bool { return true }
	o := Options{Hosts: []Host{{ID: "local", Name: "This PC", Client: cl, Session: true, Local: true}}, Session: sess,
		Update: func(fn func()) { queue <- fn }, OpenTab: func(id string) { opened = id },
		OpenCenter: func(view string) { front = view }, Current: func() string { return front }}
	// What the list opens shows beside it, as the center does.
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Row(c).Width(1000).Height(760).Background(theme.Background).TextColor(theme.Text).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(320).Shrink(0).Children(func() { v.Draw(c, o, wanted) })
			ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				switch front {
				case ViewScreen:
					v.DrawScreen(c, o, wanted)
				case ViewLogcat:
					v.DrawLogcat(c, o, wanted)
				}
			})
		})
	}, 1000, 760)
	drain := func(until func() bool, what string) {
		t.Helper()
		deadline := time.Now().Add(10 * time.Second)
		for !until() {
			select {
			case fn := <-queue:
				fn()
			case <-time.After(30 * time.Millisecond):
			}
			tt.Frame()
			if time.Now().After(deadline) {
				t.Fatalf("never: %s (%q)", what, tt.Texts())
			}
		}
	}
	drain(func() bool {
		return tt.HasText("enowX_Phone") && tt.HasText("Running · API 35 · This PC") && tt.HasText("Tablet") && tt.HasText("Stopped · This PC")
	}, "the devices")
	drain(func() bool {
		return strings.Contains(v.command, "flutter run") && v.project.Package == "com.example.app"
	}, "the project")
	if err := tt.Click("Logcat"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return tt.HasText("Hello from the app") }, "the app's log")
	v.command = `echo "running on $ANDROID_SERIAL"`
	tt.Frame()
	if err := tt.Click("Run app"); err != nil {
		t.Fatal(err)
	}
	drain(func() bool { return opened != "" }, "the run tab")
	drain(func() bool {
		text, _ := cl.Screen(opened)
		return strings.Contains(text, "running on emulator-5554")
	}, "the app running")
	shotTo(t, tt, "android-tool.png")

	// Its screen: first what shows it, then shown, with the device's buttons.
	if err := tt.Click("Screen"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Show its screen") {
		t.Fatalf("nothing to show the screen with: %q", tt.Texts())
	}
	v.screen = &screen{key: v.picked, width: 540, height: 1200, view: stubView{}}
	tt.Frame()
	if !tt.HasText("stub screen") {
		t.Fatalf("no screen: %q", tt.Texts())
	}
	shotTo(t, tt, "android-screen.png")

	// A TV's: its remote under it.
	v.screen = &screen{key: v.picked, width: 1920, height: 1080, view: stubView{}, tv: true}
	tt.Frame()
	if !tt.HasText("OK") {
		t.Fatalf("no remote: %q", tt.Texts())
	}
	shotTo(t, tt, "android-tv.png")
	v.screen = &screen{key: v.picked}
	tt.Frame()
	if !tt.HasText("Connecting to the device…") {
		t.Fatalf("not connecting: %q", tt.Texts())
	}
	v.CloseScreen()
	v.connect.open = true
	tt.Frame()
	tt.Frame()
	shotTo(t, tt, "android-connect.png")
}

// stubView stands in for the web view a screen shows in.
type stubView struct{}

func (stubView) Show(c *ui.Context) ui.Element {
	return ui.Column(c).Center().Background(ui.RGBA(30, 60, 90, 1)).Children(func() {
		ui.Text(c, "stub screen").TextColor(theme.Text)
	})
}
func (stubView) Close() {}

func shotTo(t *testing.T, tt *ui.Tester, name string) {
	dir := os.Getenv("SHOTS")
	if dir == "" {
		return
	}
	f, err := os.Create(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

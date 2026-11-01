// Package androidview is the side panel's Android tool: the devices of
// the session's host and of this machine, the session's app run on one,
// its screen shown here, and its log (docs/plans/mobile.md).
package androidview

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/url"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/android"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// refreshEvery is how often the devices are asked again while the tool
// shows; logEvery how often the log.
const (
	refreshEvery = 3 * time.Second
	logEvery     = 800 * time.Millisecond
	logKept      = 3000
)

// Host is a host whose devices the tool shows.
type Host struct {
	ID, Name string
	Client   *client.Client
	// Session is true for the session's host, where its app runs.
	Session bool
	// Local is this machine, where windows and scrcpy show.
	Local bool
}

// Options connect the tool to the app.
type Options struct {
	Hosts   []Host
	Session proto.SessionInfo
	Update  func(func())
	// OpenTab shows a tab the tool made (setup, the app running).
	OpenTab func(id string)
	// ForwardPort forwards a port of the session's host to this machine,
	// for a device here to reach the session's dev server.
	ForwardPort func(port int)
	// Local is this machine's daemon, connected when a device of another
	// host's screen shows here; nil when none can be.
	Local func() (*client.Client, error)
	// NewScreenView makes a web view loading url and hands it to done on
	// the main thread, for a screen shown in the tool; nil without web
	// views, when screens open in scrcpy's window.
	NewScreenView func(url string, done func(ScreenView, error))
	// OpenCenter shows the device's screen or its log (ViewScreen, ViewLogcat)
	// in the center; Current is the one in front there, "" for neither.
	OpenCenter func(view string)
	Current    func() string
}

// What the tool opens in the center.
const (
	ViewScreen = "screen"
	ViewLogcat = "logcat"
)

// View is the tool. It belongs to the main thread; what it asks hosts
// reports through Options.Update.
type View struct {
	status       map[string]proto.AndroidStatus // by host ID
	errs         map[string]string
	polling      bool
	wanted       func() bool
	picked       string // host ID + "/" + serial, or + "/avd:" + name
	project      proto.AndroidProject
	projDir      string
	command      string
	notice       string
	busy         string
	log          []proto.AndroidLogLine
	logAfter     int64
	logFor       string
	logging      bool
	appOnly      bool
	level        int
	search       string
	logList      ui.ListState
	connect      connectForm
	setup        *localSetup
	bridges      map[string]*client.ADBBridge // by picked device, for runs across hosts
	screens      map[string]*exec.Cmd
	caught       map[string]*caught // devices whose traffic the session's host catches
	screen       *screen            // the device's screen shown in the center
	pickedName   string             // as last worked out, for what has no Options
	pickedOnline bool
	// devicesAsked is when another tool last listed the devices.
	devicesAsked time.Time
}

// caught is a device sending its HTTP through the session's host's
// interceptor: the source it is, the port of its host it sends to, and
// the forward that port is when its host is another.
type caught struct {
	source string
	port   int
	ln     net.Listener
	host   Host
	serial string
}

type connectForm struct {
	open, busy          bool
	address, pair, code string
	host                string // the ID of the host whose adb connects
	err                 string
}

// New returns a tool with nothing read.
func New() *View {
	v := &View{status: map[string]proto.AndroidStatus{}, errs: map[string]string{}, appOnly: true,
		bridges: map[string]*client.ADBBridge{}, screens: map[string]*exec.Cmd{}, caught: map[string]*caught{}}
	v.logList.FollowEnd = true
	return v
}

// Close stops the bridges and the screens the tool started.
func (v *View) Close() {
	for _, b := range v.bridges {
		b.Close()
	}
	for _, c := range v.screens {
		if c.Process != nil {
			c.Process.Kill()
		}
	}
	for _, c := range v.caught {
		if c.ln != nil {
			c.ln.Close()
		}
	}
	if v.screen != nil {
		v.screen.close()
	}
}

// follow asks the hosts for their devices while the tool shows.
func (v *View) follow(o Options, wanted func() bool) {
	v.wanted = wanted
	if v.polling {
		return
	}
	v.polling = true
	go func() {
		for {
			type got struct {
				st  proto.AndroidStatus
				err error
			}
			res := map[string]got{}
			for _, h := range o.Hosts {
				if h.Client == nil {
					continue
				}
				st, err := h.Client.AndroidStatus()
				res[h.ID] = got{st, err}
			}
			keep := make(chan bool, 1)
			o.Update(func() {
				if v.wanted == nil || !v.wanted() {
					v.polling = false
					keep <- false
					return
				}
				keep <- true
				for id, r := range res {
					if r.err != nil {
						v.errs[id] = client.Explain(r.err)
						continue
					}
					delete(v.errs, id)
					v.status[id] = r.st
				}
			})
			if !<-keep {
				return
			}
			time.Sleep(refreshEvery)
		}
	}()
}

// device is a device of the list: its host and itself.
type device struct {
	host Host
	dev  proto.AndroidDevice
	avd  string // a virtual device not running
}

func (d device) key() string {
	if d.avd != "" {
		return d.host.ID + "/avd:" + d.avd
	}
	return d.host.ID + "/" + d.dev.Serial
}

func (d device) label() string {
	name := d.dev.Name
	if d.avd != "" {
		name = d.avd
	}
	return name + " · " + d.host.Name
}

// devices lists every host's devices, then its virtual devices not
// running.
func (v *View) devices(o Options) []device {
	var out []device
	for _, h := range o.Hosts {
		st := v.status[h.ID]
		running := map[string]bool{}
		for _, d := range st.Devices {
			if d.Serial == "" { // starting
				running[d.Name] = true
				out = append(out, device{host: h, dev: d, avd: d.Name})
				continue
			}
			if d.Kind == "emulator" {
				running[d.Name] = true
			}
			out = append(out, device{host: h, dev: d})
		}
		for _, avd := range st.AVDs {
			if !running[avd] {
				out = append(out, device{host: h, avd: avd, dev: proto.AndroidDevice{Kind: "emulator", Name: avd, State: "stopped"}})
			}
		}
	}
	return out
}

func (v *View) pickedDevice(o Options) (device, bool) {
	d, ok := v.findPicked(o)
	v.pickedName, v.pickedOnline = "", false
	if ok {
		v.pickedName, v.pickedOnline = d.dev.Name, d.dev.State == "device"
	}
	return d, ok
}

func (v *View) findPicked(o Options) (device, bool) {
	devs := v.devices(o)
	if v.picked == "" {
		// The first online device, else the first one.
		for _, d := range devs {
			if d.dev.State == "device" {
				v.picked = d.key()
				return d, true
			}
		}
		if len(devs) > 0 {
			v.picked = devs[0].key()
			return devs[0], true
		}
		return device{}, false
	}
	for _, d := range devs {
		if d.key() == v.picked {
			return d, true
		}
	}
	return device{}, false
}

// Draw shows the tool: the devices, the app to run on one, and the screen and
// the log, which open in the center (DrawScreen, DrawLogcat).
func (v *View) Draw(c *ui.Context, o Options, wanted func() bool) {
	v.follow(o, wanted)
	v.readProject(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(0, theme.S3, theme.S3, theme.S3).Gap(theme.S2).Children(func() {
		v.noticeBanner(c)
		if v.setup != nil {
			v.setupView(c, o)
			return
		}
		ui.Scroll(c.Key("tool")).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
				v.hostNotes(c, o)
				v.deviceRows(c, o)
				v.runRow(c, o)
				v.viewRows(c, o)
				v.actionRow(c, o)
			})
		})
	})
	v.connectSheet(c, o)
}

// noticeBanner says what an action is doing, or how it went, until dismissed.
func (v *View) noticeBanner(c *ui.Context) {
	if v.notice == "" {
		return
	}
	widget.Banner(c, theme.TextSoft, v.notice, func() {
		if widget.IconButton(c, icon.Close, "Dismiss", false).Clicked() {
			v.notice = ""
		}
	})
}

// hostNotes says what a host lacks: the SDK, acceleration.
func (v *View) hostNotes(c *ui.Context, o Options) {
	note := func(key, text string, col ui.Color) {
		ui.Text(c.Key(key), text).FontSize(theme.TextUI).TextColor(col).Padding(0, theme.S3)
	}
	for _, h := range o.Hosts {
		if err := v.errs[h.ID]; err != "" {
			note("e"+h.ID, h.Name+": "+err, theme.Error)
			continue
		}
		st, ok := v.status[h.ID]
		if !ok {
			continue
		}
		switch {
		case st.SDK == "" || !st.ADB || !st.Emulator:
			ui.Row(c.Key("s" + h.ID)).FillWidth().Gap(theme.S2).PaddingX(theme.S3).Children(func() {
				ui.Text(c, h.Name+" has no Android SDK yet.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Grow(1).Basis(0).MinWidth(0)
				if widget.Button(c, widget.Secondary, nil, "Set up Android", "").Disabled(v.busy != "").Clicked() {
					v.startSetup(o, h)
				}
			})
		case !st.Accel && len(st.AVDs) > 0:
			note("a"+h.ID, h.Name+"'s emulator is slow: "+st.AccelWhy, theme.Warning)
		}
	}
}

// deviceState is how a device stands, in a few words, and its dot: online done,
// starting working, waiting on the user or gone amber, else idle.
func deviceState(d device) (string, ui.Color) {
	where := " · " + d.host.Name
	api := ""
	if d.dev.API != "" {
		api = " · API " + d.dev.API
	}
	switch d.dev.State {
	case "device":
		what := "Running"
		if d.dev.TV {
			what = "Android TV"
		}
		return what + api + where, theme.Done
	case "starting":
		return "Starting…" + where, theme.Working
	case "unauthorized":
		return "Allow USB debugging on the phone" + where, theme.Warning
	case "offline":
		return "Offline" + where, theme.Warning
	}
	return "Stopped" + where, theme.Idle
}

// deviceRows lists the devices, each with what it can do first: a virtual one
// stopped starts, one online runs the app.
func (v *View) deviceRows(c *ui.Context, o Options) {
	devs := v.devices(o)
	v.pickedDevice(o)
	if len(devs) == 0 {
		ui.Text(c, "No devices: start an emulator after Set up Android, or connect a phone.").FontSize(theme.TextUI).TextColor(theme.TextSoft).Padding(theme.S1, theme.S3)
		return
	}
	ui.Column(c).FillWidth().Children(func() {
		for _, x := range devs {
			sub, col := deviceState(x)
			name := x.dev.Name
			if x.avd != "" {
				name = x.avd
			}
			it := widget.Item{Dot: col, Hollow: x.dev.State != "device", Title: name, Sub: sub, On: x.key() == v.picked, Label: x.label()}
			row := widget.ItemRow(c.Key(x.key()), it, func() {
				switch {
				case x.avd != "" && x.dev.State == "stopped":
					if widget.Button(c, widget.Secondary, nil, "Start", "").Disabled(v.busy != "").Clicked() {
						v.act(o, "Starting "+x.avd+"…", func() error { return x.host.Client.AndroidStart(x.avd, x.host.Local) })
					}
				case x.dev.State == "device":
					can := strings.TrimSpace(v.command) != "" && v.busy == ""
					if widget.Button(c, widget.Secondary, nil, "Run app", "").Disabled(!can).Clicked() {
						v.run(o, x)
					}
				}
			})
			if row.Clicked() {
				v.picked = x.key()
				v.log, v.logAfter, v.logFor = nil, 0, ""
			}
		}
	})
}

// viewRows open the picked device's screen and log in the center.
func (v *View) viewRows(c *ui.Context, o Options) {
	d, ok := v.pickedDevice(o)
	online := ok && d.dev.State == "device"
	sub, col := "No device online", theme.Idle
	if online {
		sub, col = d.label(), theme.Done
	}
	current := ""
	if o.Current != nil {
		current = o.Current()
	}
	ui.Column(c).FillWidth().Children(func() {
		for _, r := range []struct{ view, title string }{{ViewScreen, "Screen"}, {ViewLogcat, "Logcat"}} {
			if widget.ItemRow(c.Key(r.view), widget.Item{Dot: col, Hollow: !online, Title: r.title, Sub: sub, On: current == r.view}, nil).Clicked() && o.OpenCenter != nil {
				o.OpenCenter(r.view)
			}
		}
	})
}

// actionRow is what else can be done: with the picked device, and connecting
// another.
func (v *View) actionRow(c *ui.Context, o Options) {
	d, ok := v.pickedDevice(o)
	ui.Row(c).FillWidth().Wrap().Gap(theme.S2).Children(func() {
		if ok && d.dev.State == "device" {
			if c0 := v.caught[d.key()]; c0 != nil {
				if widget.Button(c, widget.Secondary, nil, "Stop catching", "").Disabled(v.busy != "").Clicked() {
					v.release(o, d, c0)
				}
			} else if widget.Button(c, widget.Secondary, nil, "Catch its traffic", "").Disabled(v.busy != "").Clicked() {
				v.catch(o, d)
			}
			if d.dev.Kind == "emulator" && widget.Button(c, widget.Secondary, nil, "Stop", "").Tooltip("Shut the emulator down").Disabled(v.busy != "").Clicked() {
				serial := d.dev.Serial
				v.act(o, "", func() error { return d.host.Client.AndroidStop(proto.AndroidDeviceParams{Serial: serial}) })
			}
		}
		if widget.Button(c, widget.Secondary, nil, "Connect a phone", "").Tooltip("Connect a phone or a TV over Wi-Fi, or make a virtual TV").Clicked() {
			v.connect.open = true
		}
	})
}

// act runs fn off the main thread, saying what it does.
func (v *View) act(o Options, doing string, fn func() error) {
	v.busy, v.notice = doing, doing
	go func() {
		err := fn()
		o.Update(func() {
			v.busy = ""
			if err != nil {
				v.notice = client.Explain(err)
			} else if v.notice == doing {
				v.notice = ""
			}
		})
	}()
}

// readProject asks the session's host what the session's app is.
func (v *View) readProject(o Options) {
	dir := o.Session.Dir
	if dir == v.projDir || dir == "" {
		return
	}
	v.projDir = dir
	var h *client.Client
	for _, x := range o.Hosts {
		if x.Session {
			h = x.Client
		}
	}
	if h == nil {
		return
	}
	go func() {
		p, err := h.AndroidProject(dir)
		o.Update(func() {
			if err == nil && v.projDir == dir {
				v.project = p
				if v.command == "" {
					v.command = p.Command
				}
			}
		})
	}()
}

// runRow is the app the devices' Run app runs, and the command that does.
func (v *View) runRow(c *ui.Context, o Options) {
	ui.Column(c).FillWidth().Gap(theme.S1).PaddingX(theme.S3).Children(func() {
		what := "The session's app"
		if v.project.Kind != "" {
			what = map[string]string{"flutter": "Flutter", "expo": "Expo", "react-native": "React Native", "capacitor": "Capacitor", "nativephp": "NativePHP", "gradle": "Android (Gradle)"}[v.project.Kind] + " app"
		}
		if v.project.Package != "" {
			what += " · " + v.project.Package
		}
		widget.Running(c, what, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
		widget.Input(c, &v.command, "The command that runs it, $ANDROID_SERIAL its device").FillWidth().Font(theme.MonoFont)
	})
}

// run runs the session's app on d in a new tab, bridging ADB when d is
// on another host than the session.
func (v *View) run(o Options, d device) {
	var session Host
	for _, h := range o.Hosts {
		if h.Session {
			session = h
		}
	}
	if session.Client == nil {
		return
	}
	params := proto.AndroidRunParams{Session: o.Session.ID, Serial: d.dev.Serial, Command: strings.TrimSpace(v.command)}
	key := d.key()
	kind := v.project.Kind
	v.act(o, "Starting the app on "+d.label()+"…", func() error {
		if d.host.ID != session.ID {
			b, err := client.BridgeADB(session.Client, d.host.Client)
			if err != nil {
				return err
			}
			params.ADB = b.Addr
			o.Update(func() {
				if old := v.bridges[key]; old != nil {
					old.Close()
				}
				v.bridges[key] = b
			})
			// The phone reaches Metro on its own host: bring it here.
			if (kind == "expo" || kind == "react-native") && o.ForwardPort != nil {
				o.Update(func() { o.ForwardPort(8081) })
			}
		}
		run, err := session.Client.AndroidRun(params)
		if err != nil {
			return err
		}
		o.Update(func() {
			if run.Package != "" {
				v.project.Package = run.Package
			}
			if o.OpenTab != nil {
				o.OpenTab(run.Tab)
			}
		})
		return nil
	})
}

// connectSheet connects a phone over Wi-Fi, as a dialog over the window.
func (v *View) connectSheet(c *ui.Context, o Options) {
	f := &v.connect
	if !f.open {
		return
	}
	// Through the session's host's adb by default; another host's, as this
	// machine's, when the device is near it rather than the server.
	if f.host == "" {
		for _, h := range o.Hosts {
			if h.Session {
				f.host = h.ID
			}
		}
	}
	var on Host
	for _, h := range o.Hosts {
		if h.ID == f.host || on.Client == nil {
			on = h
		}
	}
	widget.FormSheet(c, &f.open, 520, "Connect a phone or a TV", "Over Wi-Fi, through a host's adb: the device must be on a network that host reaches. On a phone: Settings → Developer options → Wireless debugging, whose address goes here. On an Android TV: Developer options → Network debugging, then its address with :5555, no pairing.", func() {
		if len(o.Hosts) > 1 {
			widget.Field(c, "Through", func() {
				names := make([]string, len(o.Hosts))
				cur := 0
				for i, h := range o.Hosts {
					names[i] = h.Name
					if h.ID == on.ID {
						cur = i
					}
				}
				ui.Row(c).Children(func() {
					if i := widget.Segmented(c, names, cur, nil); i >= 0 {
						f.host = o.Hosts[i].ID
					}
				})
			})
		}
		widget.Field(c, "Address", func() {
			widget.Input(c, &f.address, "192.168.1.5:41823").FillWidth().Font(theme.MonoFont)
		})
		ui.Column(c).FillWidth().Gap(theme.S2).Children(func() {
			ui.Text(c, "The first time, pair it: Pair device with pairing code shows these.").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
					widget.Field(c, "Pairing address", func() { widget.Input(c, &f.pair, "192.168.1.5:37099").FillWidth().Font(theme.MonoFont) })
				})
				ui.Column(c).Width(140).Children(func() {
					widget.Field(c, "Pairing code", func() { widget.Input(c, &f.code, "6 digits").FillWidth().Font(theme.MonoFont) })
				})
			})
		})
		if f.err != "" {
			ui.Text(c, f.err).FontSize(theme.TextUI).TextColor(theme.Error)
		}
		var session Host
		for _, h := range o.Hosts {
			if h.Session {
				session = h
			}
		}
		if session.Client != nil {
			ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "No TV at hand? An emulator of one, on "+session.Name+".").FontSize(theme.TextUI).TextColor(theme.TextSoft).Grow(1).Basis(0).MinWidth(0)
				if widget.Button(c, widget.Secondary, nil, "Make a virtual TV", "").Disabled(v.busy != "").Clicked() {
					f.open = false
					v.act(o, "Setting up a virtual Android TV in a tab…", func() error {
						tab, err := session.Client.AndroidSetupWith(proto.AndroidSetupParams{Session: o.Session.ID, TV: true})
						if err == nil && o.OpenTab != nil {
							o.Update(func() { o.OpenTab(tab.ID) })
						}
						return err
					})
				}
			})
		}
	}, func() {
		label := "Connect"
		if f.busy {
			label = "Connecting…"
		}
		if widget.PrimaryButton(c, nil, label, "").Disabled(strings.TrimSpace(f.address) == "" || f.busy).Clicked() {
			p := proto.AndroidConnectParams{Address: strings.TrimSpace(f.address), PairAddress: strings.TrimSpace(f.pair), Code: strings.TrimSpace(f.code)}
			f.busy, f.err = true, ""
			go func() {
				err := on.Client.AndroidConnect(p)
				o.Update(func() {
					f.busy = false
					if err != nil {
						f.err = client.Explain(err)
						return
					}
					v.connect = connectForm{}
					v.notice = "Connected: it shows in the list once adb sees it."
				})
			}()
		}
	})
}

// showScreen shows d's screen in a scrcpy window here: directly for a
// device of this machine, through the bridge for another host's.
func (v *View) showScreen(o Options, d device) {
	key := d.key()
	if cmd := v.screens[key]; cmd != nil && cmd.ProcessState == nil {
		v.notice = "Its screen is open already."
		return
	}
	v.act(o, "Opening "+d.label()+"'s screen…", func() error {
		sc, err := android.InstallScrcpy(context.Background(), io.Discard)
		if err != nil {
			return err
		}
		p := android.ScreenParams{Serial: d.dev.Serial, Title: d.label()}
		if sdk, ok := android.FindSDK(); ok {
			p.SDKADB = sdk.ADB()
		}
		var cleanup []func()
		if !d.host.Local {
			if o.Local == nil {
				return fmt.Errorf("android: this machine's daemon isn't reachable to show another host's device")
			}
			lc, err := o.Local()
			if err != nil {
				return err
			}
			b, err := client.BridgeADB(lc, d.host.Client)
			if err != nil {
				return err
			}
			cleanup = append(cleanup, b.Close)
			port := 27200 + int(time.Now().UnixNano()%700)
			ln, err := client.ForwardADBPort(d.host.Client, port)
			if err != nil {
				b.Close()
				return err
			}
			cleanup = append(cleanup, func() { ln.Close() })
			p.ADBServer, p.TunnelPort = b.Addr, port
		}
		cmd := sc.Command(p)
		if err := cmd.Start(); err != nil {
			for _, f := range cleanup {
				f()
			}
			return fmt.Errorf("android: scrcpy: %w", err)
		}
		o.Update(func() { v.screens[key] = cmd })
		go func() {
			cmd.Wait()
			for _, f := range cleanup {
				f()
			}
		}()
		return nil
	})
}

// DrawLogcat is the picked device's log, in the center, the app's lines by
// default, on the ground code stays dark on.
func (v *View) DrawLogcat(c *ui.Context, o Options, wanted func() bool) {
	v.follow(o, wanted)
	v.readProject(o)
	d, ok := v.pickedDevice(o)
	online := ok && d.dev.State == "device"
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		title := "Logcat"
		if ok {
			title = d.label() + " · logcat"
		}
		widget.ViewerBar(c, icon.Android, title, func() {
			if online && widget.IconButton(c, icon.Trash, "Clear", false).Clicked() {
				v.log = nil
			}
		})
		if online {
			v.logFilters(c)
		}
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Background(widget.CodeGround).Children(func() {
			if !online {
				groundNote(c, "The device's log shows here once it runs.")
				return
			}
			v.readLog(o, d)
			lines := v.shownLines()
			ui.List(c.Key("log"), &v.logList, len(lines), func(i int) { logLine(c, lines[i]) }).Grow(1).FillWidth()
		})
	})
}

// logFilters narrow the log: the app's, a level, words.
func (v *View) logFilters(c *ui.Context) {
	ui.Row(c.Key("filters")).FillWidth().Shrink(0).Padding(theme.S2, theme.S4).Gap(theme.S2).BorderWidth(0, 0, 0.5, 0).BorderColor(theme.Border).Children(func() {
		if v.project.Package != "" {
			ui.Row(c).MinWidth(0).Shrink(1).Clip().Children(func() {
				widget.Checkbox(c, &v.appOnly, "Only "+v.project.Package)
			})
		}
		ui.Spacer(c)
		if i := widget.Segmented(c, []string{"All", "Info", "Warn", "Error"}, v.level, nil); i >= 0 {
			v.level = i
		}
		ui.Row(c).Width(180).MinWidth(100).Shrink(1).Children(func() {
			widget.SearchField(c, &v.search, "Search the log", icon.Search)
		})
	})
}

// logLine is a line of the log: warnings and errors stand out by their tag,
// verbose and debug lines are quiet.
func logLine(c *ui.Context, l proto.AndroidLogLine) {
	head, body := widget.CodeDim, widget.CodeText
	switch l.Level {
	case "W":
		head = widget.CodeWarn
	case "E", "F", "A":
		head, body = widget.CodeError, widget.CodeError
	case "V", "D":
		body = widget.CodeDim
	}
	ui.Row(c).FillWidth().Padding(1, theme.S4).Children(func() {
		ui.RichText(c,
			ui.Span{Text: l.Time + " ", Color: widget.CodeDim},
			ui.Span{Text: l.Level + " " + l.Tag + ": ", Color: head},
			ui.Span{Text: l.Message, Color: body},
		).Font(theme.MonoFont).FontSize(theme.TextUI).Grow(1).Basis(0).MinWidth(0).Selectable()
	})
}

// groundNote says something on the code's ground.
func groundNote(c *ui.Context, text string) {
	ui.Column(c).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
		ui.Text(c, text).FontSize(theme.TextUI).TextColor(widget.CodeDim).TextAlign(ui.Center)
	})
}

func (v *View) shownLines() []proto.AndroidLogLine {
	minLevel := map[int]string{1: "IWEFA", 2: "WEFA", 3: "EFA"}[v.level]
	q := strings.ToLower(v.search)
	var out []proto.AndroidLogLine
	for _, l := range v.log {
		if minLevel != "" && !strings.Contains(minLevel, l.Level) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(l.Tag+" "+l.Message), q) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// readLog asks the device's host for new lines now and then.
func (v *View) readLog(o Options, d device) {
	pkg := ""
	if v.appOnly {
		pkg = v.project.Package
	}
	key := d.key() + "|" + pkg
	if v.logFor != key {
		v.logFor, v.log, v.logAfter = key, nil, 0
	}
	if v.logging {
		return
	}
	v.logging = true
	h, serial, after := d.host.Client, d.dev.Serial, v.logAfter
	go func() {
		l, err := h.AndroidLogcat(proto.AndroidLogcatParams{Serial: serial, Package: pkg, After: after})
		time.Sleep(logEvery)
		o.Update(func() {
			v.logging = false
			if err != nil || v.logFor != key {
				return
			}
			v.log = append(v.log, l.Lines...)
			if len(v.log) > logKept {
				v.log = slices.Clone(v.log[len(v.log)-logKept:])
			}
			v.logAfter = l.Last
		})
	}()
}

// localSetup is Android being set up on this machine, by the app itself:
// the setup's output, and the licenses answered here.
type localSetup struct {
	mu      sync.Mutex
	out     strings.Builder
	answers *io.PipeWriter
	done    bool
	err     string
}

func (s *localSetup) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.out.Write(p)
	return len(p), nil
}

func (s *localSetup) finished() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}

func (s *localSetup) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.out.String()
	if len(t) > 20000 {
		t = t[len(t)-20000:]
	}
	return t
}

// startSetup sets Android up on host h: in a tab of the session on the
// session's host, else here in the app, its output shown in the tool.
func (v *View) startSetup(o Options, h Host) {
	if h.Session {
		v.act(o, "Setting Android up in a tab…", func() error {
			tab, err := h.Client.AndroidSetup(o.Session.ID)
			if err == nil && o.OpenTab != nil {
				o.Update(func() { o.OpenTab(tab.ID) })
			}
			return err
		})
		return
	}
	s := &localSetup{}
	r, w := io.Pipe()
	s.answers = w
	v.setup = s
	go func() {
		err := android.Setup(context.Background(), r, s)
		s.mu.Lock()
		s.done = true
		if err != nil {
			s.err = err.Error()
		}
		s.mu.Unlock()
		o.Update(func() {})
	}()
	// The output shows as it comes.
	go func() {
		for !s.finished() {
			time.Sleep(time.Second)
			o.Update(func() {})
		}
	}()
}

func (v *View) setupView(c *ui.Context, o Options) {
	s := v.setup
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Gap(theme.S2).Children(func() {
		ui.Text(c, "Setting Android up on this machine").FontSize(theme.TextUI).FontWeight(500)
		widget.LogView(c, s.text(), 360)
		s.mu.Lock()
		done, failed := s.done, s.err
		s.mu.Unlock()
		if failed != "" {
			ui.Text(c, failed).FontSize(theme.TextUI).TextColor(theme.Error)
		}
		ui.Row(c).FillWidth().Gap(theme.S2).Justify(ui.End).Children(func() {
			if !done && widget.Button(c, widget.Secondary, nil, "Accept the licenses shown", "").Clicked() {
				go io.WriteString(s.answers, strings.Repeat("y\n", 12))
			}
			if done && widget.PrimaryButton(c, nil, "Close", "").Clicked() {
				v.setup = nil
			}
		})
	})
}

// catch sends device d's HTTP through the session's host's interceptor:
// a source there, its port reached from d's host (forwarded here when d
// is on this machine and the session on a server), and d's proxy and
// trust set by its host.
func (v *View) catch(o Options, d device) {
	var session Host
	for _, h := range o.Hosts {
		if h.Session {
			session = h
		}
	}
	if session.Client == nil {
		return
	}
	key, serial := d.key(), d.dev.Serial
	v.act(o, "Catching "+d.label()+"'s traffic…", func() error {
		t, err := session.Client.TrafficAttach(proto.TrafficAttachParams{Kind: proto.TargetAndroid, Session: o.Session.ID, Device: serial, Label: d.label()})
		if err != nil {
			return err
		}
		ca, err := session.Client.TrafficCA()
		if err != nil {
			return err
		}
		c := &caught{source: t.Source, host: d.host, serial: serial}
		if d.host.ID == session.ID {
			u, err := url.Parse(t.Proxy)
			if err != nil {
				return err
			}
			c.port, _ = strconv.Atoi(u.Port())
		} else {
			ln, err := session.Client.ForwardTrafficSource(t.Source)
			if err != nil {
				return err
			}
			c.ln, c.port = ln, ln.Addr().(*net.TCPAddr).Port
		}
		res, err := d.host.Client.AndroidProxy(proto.AndroidProxyParams{Serial: serial, Port: c.port, CA: ca.PEM})
		if err != nil {
			if c.ln != nil {
				c.ln.Close()
			}
			session.Client.TrafficDetach(t.Source)
			return err
		}
		o.Update(func() {
			v.caught[key] = c
			v.notice = "Its traffic shows in Traffic. " + res.Note
		})
		return nil
	})
}

func (v *View) release(o Options, d device, c *caught) {
	var session Host
	for _, h := range o.Hosts {
		if h.Session {
			session = h
		}
	}
	key := d.key()
	delete(v.caught, key)
	v.act(o, "", func() error {
		c.host.Client.AndroidProxy(proto.AndroidProxyParams{Serial: c.serial, Port: c.port, Off: true})
		if c.ln != nil {
			c.ln.Close()
		}
		if session.Client != nil {
			return session.Client.TrafficDetach(c.source)
		}
		return nil
	})
}

// Device is a device the tool knows, for other tools: Traffic catches
// one's traffic from its Intercept page.
type Device struct {
	Key, Label string
	Caught     bool
}

// Devices are the online devices of o's hosts, read again while they
// are asked for, every frame another tool shows them.
func (v *View) Devices(o Options) []Device {
	v.devicesAsked = time.Now()
	if !v.polling {
		v.follow(o, func() bool { return time.Since(v.devicesAsked) < 2*refreshEvery })
	}
	var out []Device
	for _, d := range v.devices(o) {
		if d.dev.State == "device" {
			out = append(out, Device{Key: d.key(), Label: d.label(), Caught: v.caught[d.key()] != nil})
		}
	}
	return out
}

// Catch sends device key's traffic through the session's interceptor, or
// stops it.
func (v *View) Catch(o Options, key string, on bool) {
	for _, d := range v.devices(o) {
		if d.key() != key {
			continue
		}
		if c := v.caught[key]; c != nil && !on {
			v.release(o, d, c)
		} else if c == nil && on {
			v.catch(o, d)
		}
		return
	}
}

// PickedName is the name of the device picked, "" before there is one; for what
// the center's tab of its screen says.
func (v *View) PickedName() string { return v.pickedName }

// CloseScreen stops showing the device's screen: its tab closed.
func (v *View) CloseScreen() {
	if v.screen != nil {
		v.screen.close()
		v.screen = nil
	}
}

// Online says whether the device picked is, for the dot of the tabs that show it.
func (v *View) Online() bool { return v.pickedOnline }

// Notice is what the tool last said, as how a device's traffic is caught
// and what is left to do.
func (v *View) Notice() string { return v.notice }

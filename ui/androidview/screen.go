package androidview

import (
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/android"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A device's screen shows in a center tab: its host sends it as scrcpy's
// video (android.mirror), a loopback server of the app hands it to a web
// view, whose page decodes it with WebCodecs and sends touches and keys
// back. Pop out opens it in scrcpy's own window instead.

//go:embed screen.html
var screenPage []byte

// ScreenView is the web view showing a screen's page; the app makes it.
type ScreenView interface {
	Show(c *ui.Context) ui.Element
	Close()
}

// screen is a device's screen shown. Its fields belong to the main
// thread, but for the server's, which only read what it was made with.
type screen struct {
	key    string
	label  string
	serial string
	host   Host
	tunnel *client.Tunnel
	ln     net.Listener
	view   ScreenView
	width  int // the video's, as the page last said
	height int
	err    string
	tv     bool // an Android TV: a remote's D-pad under it
}

func (s *screen) close() {
	if s.view != nil {
		s.view.Close()
	}
	if s.ln != nil {
		s.ln.Close()
	}
	if s.tunnel != nil {
		s.tunnel.Close()
	}
}

// openScreen shows d's screen in the tool.
func (v *View) openScreen(o Options, d device) {
	if v.screen != nil {
		v.screen.close()
		v.screen = nil
	}
	if o.NewScreenView == nil {
		v.showScreen(o, d) // no web view here: scrcpy's window
		return
	}
	s := &screen{key: d.key(), label: d.label(), serial: d.dev.Serial, host: d.host, tv: d.dev.TV}
	v.screen = s
	v.act(o, "Showing "+d.label()+"'s screen…", func() error {
		t, err := d.host.Client.AndroidMirror(proto.AndroidMirrorParams{Serial: s.serial, MaxSize: 1280})
		if err != nil {
			o.Update(func() {
				if v.screen == s {
					v.screen = nil
				}
			})
			return err
		}
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Close()
			return err
		}
		var tok [12]byte
		rand.Read(tok[:])
		base := "/" + hex.EncodeToString(tok[:]) + "/"
		go http.Serve(ln, s.handler(o, v, t, base))
		url := "http://" + ln.Addr().String() + base
		if s.tv {
			url += "?tv=1" // Enter is the remote's OK
		}
		o.Update(func() {
			if v.screen != s {
				t.Close()
				ln.Close()
				return
			}
			s.tunnel, s.ln = t, ln
			o.NewScreenView(url, func(sv ScreenView, err error) {
				if err != nil || v.screen != s {
					if sv != nil {
						sv.Close()
					}
					if err != nil {
						v.notice = "The screen can't show here: " + err.Error()
					}
					return
				}
				s.view = sv
			})
		})
		return nil
	})
}

// handler serves the page, the video (once) and the input, under base, a
// path no other page can guess.
func (s *screen) handler(o Options, v *View, t *client.Tunnel, base string) http.Handler {
	h := s.host.Client
	serial := s.serial
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rest, ok := strings.CutPrefix(r.URL.Path, base)
		if !ok {
			http.NotFound(w, r)
			return
		}
		switch rest {
		case "":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(screenPage)
		case "video":
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Header().Set("Cache-Control", "no-store")
			copyFlushing(w, t)
		case "input":
			var in proto.AndroidMirrorInput
			if json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&in) != nil {
				http.Error(w, "bad input", http.StatusBadRequest)
				return
			}
			switch in.Kind {
			case "size":
				o.Update(func() { s.width, s.height = in.Width, in.Height })
			case "unsupported":
				o.Update(func() { s.err = "This system's web view can't decode video: Pop out shows it in a window of its own." })
			default:
				in.Serial = serial
				if err := h.AndroidMirrorInput(in); err != nil {
					http.Error(w, err.Error(), http.StatusBadGateway)
					return
				}
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}

// copyFlushing copies the video to the page as it comes.
func copyFlushing(w http.ResponseWriter, r io.Reader) {
	f, _ := w.(http.Flusher)
	buf := make([]byte, 64<<10)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return
			}
			if f != nil {
				f.Flush()
			}
		}
		if err != nil {
			return
		}
	}
}

// DrawScreen is the picked device's screen, in the center, or what shows it; its
// bar holds the device's buttons.
func (v *View) DrawScreen(c *ui.Context, o Options, wanted func() bool) {
	v.follow(o, wanted)
	d, ok := v.pickedDevice(o)
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
		title := "Screen"
		if ok {
			title = d.label() + " · screen"
		}
		widget.ViewerBar(c, icon.Android, title, func() { v.screenKeys(c, o) })
		v.noticeBanner(c)
		ui.Column(c).Grow(1).FillWidth().MinHeight(0).Background(theme.CodeGround).Children(func() {
			switch {
			case ok && v.screen != nil && v.screen.key == d.key():
				v.screenView(c, o, d)
			case !ok || d.dev.State != "device":
				groundNote(c, "The device's screen shows here once it runs.")
			default:
				ui.Column(c).Grow(1).FillWidth().Center().Gap(theme.S3).Padding(theme.S5).Children(func() {
					ui.Text(c, "Use "+d.label()+" from here: touch, type, its buttons.").FontSize(theme.TextUI).TextColor(widget.CodeDim).TextAlign(ui.Center)
					if widget.PrimaryButton(c, nil, "Show its screen", "").Disabled(v.busy != "").Clicked() {
						v.openScreen(o, d)
					}
				})
			}
		})
	})
}

// screenView is the screen shown, in the device's shape: as big as the room
// lets it be, which the frame never outgrows; the page keeps the shape inside,
// black around.
func (v *View) screenView(c *ui.Context, o Options, d device) {
	s := v.screen
	_, winH := c.Size()
	room := ui.Column(c).Grow(1).FillWidth().MinHeight(0).Padding(theme.S3).Gap(theme.S2).Center().Clip()
	bounds := room.LayoutBounds()
	h := max(240, bounds.H-2*theme.S3)
	if bounds.H == 0 {
		h = max(240, winH-360) // the first frame, before the room is known
	}
	if s.tv {
		h -= theme.ControlHeight + theme.S4 + theme.S2 // the remote under it
	}
	w := h * 9 / 19.5
	if s.width > 0 && s.height > 0 {
		w = h * float32(s.width) / float32(s.height)
	}
	if bounds.W > 0 && w > bounds.W-2*theme.S3 {
		h *= (bounds.W - 2*theme.S3) / w
		w = bounds.W - 2*theme.S3
	}
	room.Children(func() {
		ui.Column(c).Size(w, h).Shrink(0).Padding(theme.S2).Center().Radius(theme.RadiusPanel).Background(ui.Hex("#090A10")).
			InsetShadow(0, 0, 0, 0.5, ui.RGBA(255, 255, 255, 0.2)).Children(func() {
			switch {
			case s.err != "":
				ui.Text(c, s.err).FontSize(theme.TextUI).TextColor(widget.CodeDim).TextAlign(ui.Center).Padding(theme.S4)
			case s.view != nil:
				s.view.Show(c).FillWidth().FillHeight()
			default:
				ui.Text(c, "Connecting to the device…").FontSize(theme.TextUI).TextColor(widget.CodeDim).TextAlign(ui.Center)
			}
		})
		if s.tv {
			v.remote(c, s)
		}
	})
}

// remote is a TV's remote under its screen: the D-pad and OK, Back, Home
// and Menu; the keyboard's arrows and Enter do the same in the screen.
func (v *View) remote(c *ui.Context, s *screen) {
	key := func(svg *ui.SVG, label string, code int) {
		if widget.IconButton(c, svg, label, false).Clicked() {
			go s.host.Client.AndroidMirrorInput(proto.AndroidMirrorInput{Serial: s.serial, Kind: "key", Key: code})
		}
	}
	ui.Row(c).Shrink(0).Padding(theme.S1, theme.S2).Gap(theme.S1).Radius(theme.RadiusControl).Background(theme.Panel).AlignItems(ui.Center).Children(func() {
		key(icon.ArrowLeft, "Left", android.KeyLeft)
		key(icon.ArrowUp, "Up", android.KeyUp)
		if widget.Button(c, widget.Secondary, nil, "OK", "").Tooltip("The remote's OK (Enter)").Clicked() {
			go s.host.Client.AndroidMirrorInput(proto.AndroidMirrorInput{Serial: s.serial, Kind: "key", Key: android.KeyCenter})
		}
		key(icon.ArrowDown, "Down", android.KeyDown)
		key(icon.ArrowRight, "Right", android.KeyRight)
		ui.Box(c).Width(1).Height(18).Background(theme.Border)
		key(icon.List, "Menu", android.KeyMenu)
	})
}

// screenKeys are the device's buttons, in the viewer's bar while its screen
// shows: always in view, whatever the height.
func (v *View) screenKeys(c *ui.Context, o Options) {
	s := v.screen
	d, ok := v.pickedDevice(o)
	if s == nil || !ok || s.key != d.key() {
		return
	}
	key := func(svg *ui.SVG, label string, code int) {
		if widget.IconButton(c, svg, label, false).Clicked() {
			go s.host.Client.AndroidMirrorInput(proto.AndroidMirrorInput{Serial: s.serial, Kind: "key", Key: code})
		}
	}
	key(icon.ArrowLeft, "Back", android.KeyBack)
	key(icon.Home, "Home", android.KeyHome)
	key(icon.List, "Recent apps", android.KeyAppSwitch)
	key(icon.VolumeDown, "Volume down", android.KeyVolumeDown)
	key(icon.VolumeUp, "Volume up", android.KeyVolumeUp)
	key(icon.Power, "Power", android.KeyPower)
	ui.Box(c).Width(1).Height(18).Shrink(0).Background(theme.Border)
	if widget.IconButton(c, icon.External, "Pop out: scrcpy's window, at full speed", false).Clicked() {
		s.close()
		v.screen = nil
		v.showScreen(o, d)
	}
	if widget.IconButton(c, icon.Close, "Close the screen", false).Clicked() {
		s.close()
		v.screen = nil
	}
}

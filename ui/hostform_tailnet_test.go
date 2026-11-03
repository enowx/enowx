package ui

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
	"golang.org/x/crypto/ssh"
	"tailscale.com/net/netns"
	"tailscale.com/tailcfg"
	"tailscale.com/tsnet"
	"tailscale.com/tstest/integration"
	"tailscale.com/tstest/integration/testcontrol"
	"tailscale.com/types/logger"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/ui/theme"
)

// Adding a host with "Through enowX's own tailnet" on saves it so, and
// connecting reaches the server over enowX's tailnet device, the server
// being on the tailnet only.
func TestAddHostThroughOwnTailnet(t *testing.T) {
	netns.SetEnabled(false)
	defer netns.SetEnabled(true)
	derpMap := integration.RunDERPAndSTUN(t, logger.Discard, "127.0.0.1")
	control := &testcontrol.Server{DERPMap: derpMap, DNSConfig: &tailcfg.DNSConfig{Proxied: true},
		MagicDNSDomain: "tail-scale.ts.net", Logf: logger.Discard}
	control.HTTPTestServer = httptest.NewUnstartedServer(control)
	control.HTTPTestServer.Start()
	defer control.HTTPTestServer.Close()
	t.Setenv(hostconn.ControlEnv, control.HTTPTestServer.URL)
	home := t.TempDir()
	t.Setenv("HOME", home)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	server := &tsnet.Server{Dir: filepath.Join(t.TempDir(), "server"), Hostname: "devbox",
		ControlURL: control.HTTPTestServer.URL, Ephemeral: true, Logf: logger.Discard}
	defer server.Close()
	status, err := server.Up(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := server.Listen("tcp", ":22")
	if err != nil {
		t.Fatal(err)
	}
	signedIn := make(chan string, 4)
	go acceptSSH(t, ln, signedIn)

	store, err := hosts.Open(filepath.Join(home, "hosts.json"))
	if err != nil {
		t.Fatal(err)
	}
	a := New(Options{Hosts: store})
	posted := make(chan func(), 64)
	a.onMain = func(fn func()) { posted <- fn }
	a.addHost = addHostForm{open: true, address: "root@" + status.TailscaleIPs[0].String(), name: "devbox"}
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Column(c).Width(640).Height(900).Background(theme.Background).Children(func() { a.addHostDialog(c) })
	}, 640, 900)
	frame := func() {
		for {
			select {
			case fn := <-posted:
				fn()
				continue
			default:
			}
			break
		}
		a.answerTrust(true) // the server's key is new
		tt.Frame()
	}
	frame()
	if !tt.HasText("Through enowX's own tailnet") {
		t.Fatalf("no tailnet option: %q", tt.Texts())
	}
	// The switch sits at the dialog's right edge, centered on the label
	// and its description.
	r, _ := tt.Find("Through enowX's own tailnet")
	for x := r.X + r.W; x < r.X+700 && !a.addHost.tailnet; x += 12 {
		tt.ClickAt(x, r.Y+10)
		frame()
	}
	if !a.addHost.tailnet {
		t.Fatal("the tailnet switch didn't turn on")
	}
	shot(t, tt, "host-form-tailnet.png")
	if err := tt.Click("Add and connect"); err != nil {
		t.Fatal(err)
	}
	frame()
	list := store.List()
	if len(list) != 1 || !list[0].Tailnet {
		t.Fatalf("hosts %+v", list)
	}
	deadline := time.Now().Add(time.Minute)
	for {
		select {
		case user := <-signedIn:
			if user != "root" {
				t.Fatalf("signed in as %q", user)
			}
			return
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("server never reached; connecting %q, error %q", a.connecting, a.connErr)
		}
		frame()
		time.Sleep(50 * time.Millisecond)
	}
}

// acceptSSH tells signedIn who signed in and fails every command: the
// test only asks whether the server was reached.
func acceptSSH(t *testing.T, ln net.Listener, signedIn chan<- string) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Error(err)
		return
	}
	cfg := &ssh.ServerConfig{NoClientAuth: true}
	cfg.AddHostKey(signer)
	for {
		nc, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			conn, chans, reqs, err := ssh.NewServerConn(nc, cfg)
			if err != nil {
				return
			}
			select {
			case signedIn <- conn.User():
			default:
			}
			go ssh.DiscardRequests(reqs)
			for nch := range chans {
				_ = nch.Reject(ssh.Prohibited, "no shell here")
			}
		}()
	}
}

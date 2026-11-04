package ui

import (
	"fmt"
	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/widget"
	"regexp"
	"strings"
	"time"
)

// trustPrompt asks the user whether a server's host key is the one they
// expect; the connecting goroutine waits on answer.
type trustPrompt struct {
	addr        string
	fingerprint string
	answer      chan bool
}

// hostOptions are how a connection made while connection gen is the
// app's asks the user: to trust a host, for a password, a user to run as,
// a key's passphrase.
func (a *App) hostOptions(gen int) hostconn.Options {
	return hostconn.Options{
		DaemonCommand: a.opts.DaemonCommand,
		TrustHost:     func(addr, fingerprint string) bool { return a.askTrust(gen, addr, fingerprint) },
		Password:      a.password,
		ProviderToken: a.providerToken,
		RunAs:         func(h hosts.Host, users []hostconn.ServerUser) (hosts.Host, error) { return a.askRunAs(gen, h, users) },
		Passphrase:    func(path string) (string, error) { return a.askPassphrase(gen, path) },
		TailnetLogin: func(url string) {
			a.onMain(func() {
				if gen == a.gen && a.connecting != "" {
					a.connecting, a.connectLink = "Sign enowX in to your tailnet in the browser…", url
				}
			})
		},
	}
}

// connect drops the current connection and connects to h in the
// background. Sessions keep running in the daemon left behind.
func (a *App) connect(h hosts.Host) {
	a.closeGOATabs()
	// As saved now: a retry or a link may hold a copy from before the user
	// chose whom to run as, which would ask again and undo it.
	if saved, ok := a.storedHost(h.ID); ok {
		h = saved
	}
	// The same host coming back: what the user had open stays.
	back := a.client == nil && a.retryHost == h.ID && a.host.ID == h.ID
	maximized, form, picker := a.maximized, a.newSession.open, a.addProject
	switch {
	case back:
		a.diag.Event("conn.connecting", "host", h.ID, "name", h.Name, "retry", a.retrying)
	case a.host.ID != "" && a.host.ID != h.ID:
		a.diag.Event("host.switched", "from", a.host.ID, "to", h.ID, "name", h.Name)
		a.diag.Event("conn.connecting", "host", h.ID, "name", h.Name)
	default:
		a.diag.Event("conn.connecting", "host", h.ID, "name", h.Name)
	}
	a.disconnect()
	if back {
		a.maximized, a.newSession.open, a.addProject = maximized, form, picker
	}
	a.gen++
	gen := a.gen
	a.host = h
	a.connectByApp = false // the timer and the app's start say so after
	a.syncWatches()        // the host left is watched now, this one shown
	a.connecting, a.connErr, a.connFail, a.status, a.connectLink = "Connecting to "+h.Name+"…", "", nil, "", ""
	if h.ID == hosts.LocalID {
		a.connecting = "Starting…"
	}
	opts := a.hostOptions(gen)
	opts.OnEvent = func(ev proto.Event) { a.handleEvent(gen, ev) }
	a.daemonBehindOptions(&opts, h, gen)
	opts.Progress = func(s string) {
		a.onMain(func() {
			if gen == a.gen && a.connecting != "" {
				a.connecting = s + "…"
			}
		})
	}
	// What the server says while signing in shows, a link to open with it:
	// Tailscale SSH waits for the user to check in a browser.
	opts.Banner = func(msg string) error {
		a.onMain(func() {
			if gen == a.gen && a.connecting != "" {
				a.connecting, a.connectLink = strings.TrimSpace(msg), linkIn(msg)
			}
		})
		return nil
	}
	openProject := ""
	if gen == 1 {
		openProject = a.opts.OpenProject
	}
	began := time.Now()
	go func() {
		c, err := hostconn.Connect(h, opts)
		if err != nil {
			a.diag.Event("conn.failed", "host", h.ID, "name", h.Name, "took", time.Since(began), "err", err)
		}
		a.onMain(func() {
			if gen != a.gen {
				if c != nil {
					c.Close()
				}
				return
			}
			a.connecting = ""
			if err != nil {
				if a.retrying > 0 && a.retryHost == h.ID {
					a.retryLater(h, err)
					return
				}
				a.connErr, a.connFail = err.Error(), err
				a.promptSignIn(h, err)
				return
			}
			a.diag.Event("conn.connected", "host", h.ID, "name", h.Name, "conn", c.ID(), "took", time.Since(began), "afterTries", a.retrying)
			if a.retrying > 0 {
				a.diag.Event("conn.reconnected", "host", h.ID, "name", h.Name, "conn", c.ID(), "afterTries", a.retrying)
				a.notify("Reconnected to " + h.Name)
			}
			a.retrying, a.retryHost = 0, ""
			a.client = c
			a.spacesConnected(c)
			a.warnBundlePlace()
			a.onboardingCheck(c)
			a.pollUsage(c, gen)
			a.pollCredits(c, gen)
			a.pollConflicts(c, gen)
			go a.refresh(c, gen, openProject)
			go a.checkDrift(c, h)
			go a.watchDisconnect(c, h, gen)
			go func() { _ = c.ServeBrowsers() }() // an older daemon has no enowx browser
			go func() { _ = c.ServeApp() }()
			a.vaultConnected(c)
			go a.followDaemonBehind(c, h, gen)
		})
	}()
}

func (a *App) watchDisconnect(c *client.Client, h hosts.Host, gen int) {
	<-c.Done()
	a.logClosed("main", h, c)
	a.onMain(func() {
		if gen == a.gen && a.client == c {
			a.client = nil
			// The sessions keep running there: connect again by itself,
			// as the network comes back.
			a.retrying, a.retryHost = 0, a.host.ID
			a.retryLater(a.host, nil)
		}
	})
}

// retryLater connects to h again after a while, longer each time up to
// retryMax, until it answers or the user goes elsewhere; err is why the
// last try failed.
func (a *App) retryLater(h hosts.Host, err error) {
	a.retrying++
	wait := min(time.Second<<min(a.retrying-1, 5), retryMax)
	a.connErr = "Disconnected from " + h.Name + ". Its sessions keep running; connecting again in " + fmt.Sprint(wait.Round(time.Second)) + "…"
	if err != nil {
		a.connErr += " (" + client.Explain(err) + ")"
	}
	a.diag.Event("conn.retry", "host", h.ID, "name", h.Name, "try", a.retrying, "wait", wait, "err", err)
	attempt := a.retrying
	time.AfterFunc(wait, func() {
		a.onMain(func() {
			if a.retrying != attempt || a.retryHost != h.ID || a.host.ID != h.ID || a.client != nil {
				return // connected meanwhile, or the user went elsewhere
			}
			a.connect(h)
			a.connectByApp = true
		})
	})
}

// retryMax is the longest wait between tries to connect again.
const retryMax = 30 * time.Second

// disconnect closes the connection and forgets its state.
func (a *App) disconnect() {
	for _, p := range a.panes {
		p.close()
	}
	a.stopForwards("")
	a.closeAppWindows("")
	if a.androidView != nil {
		a.androidView.Close() // its bridges go with the connection
		a.androidView = nil
	}
	a.local.close()
	a.sshRefused.Store(false)
	a.portOffers = nil
	if a.client != nil {
		a.client.Close()
		a.client = nil
	}
	if a.trust != nil {
		a.trust.answer <- false
		a.trust = nil
	}
	a.cancelRunAs()
	a.answerPassphrase(false)
	a.signIn = nil
	a.closeSync()
	a.agents, a.projects, a.sessions, a.pipelines = nil, nil, nil, nil
	a.daemonInfo = proto.DaemonInfo{}
	a.panes, a.removed = map[string]*pane{}, map[string]bool{}
	a.project, a.selected, a.maximized = "", "", ""
	a.newSession.open = false
	a.addProject.Close()
	a.restoredView, a.deleting, a.menu = false, nil, widget.ContextMenu{}
}

// askTrust runs on the connecting goroutine and waits for the user.
func (a *App) askTrust(gen int, addr, fingerprint string) bool {
	answer := make(chan bool, 1)
	a.onMain(func() {
		if gen != a.gen {
			answer <- false
			return
		}
		a.trust = &trustPrompt{addr: addr, fingerprint: fingerprint, answer: answer}
	})
	return <-answer
}

func (a *App) answerTrust(yes bool) {
	if a.trust != nil {
		a.trust.answer <- yes
		a.trust = nil
	}
}

func (a *App) reloadHosts() {
	if a.opts.Hosts != nil {
		a.hostList = a.opts.Hosts.List()
	}
	a.syncWatches()
}

// allHosts is this machine followed by the stored servers, plus the
// current host when it came from the command line and isn't stored.
func (a *App) allHosts() []hosts.Host {
	out := append([]hosts.Host{LocalHost()}, a.hostList...)
	for _, h := range out {
		if h.ID == a.host.ID {
			return out
		}
	}
	if a.host.ID != "" {
		out = append(out, a.host)
	}
	return out
}

var linkPattern = regexp.MustCompile(`https?://[^\s"'<>]+`)

// linkIn is the first link in a server's message, "" for none.
func linkIn(msg string) string { return linkPattern.FindString(msg) }

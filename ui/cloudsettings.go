package ui

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/textfmt"
)

// Settings › Cloud: the connected host's sign-in to enowX's cloud
// (docs/cloud.md), through its daemon like everything else: signing in
// with a code the browser approves, the account's devices, and its teams
// (cloudteams.go). The rail's account entry (railaccount.go) signs in too.
// Nothing reaches the cloud before the user signs in.

const sectionCloud = "Cloud"

// cloudSettings is the section's state for the connected host, read when
// it shows. It belongs to the main thread.
type cloudSettings struct {
	host     string
	st       proto.CloudStatus
	loaded   bool
	busy     string // what is under way, shown
	err      string
	approve  string // a code typed, to approve another device
	opened   string // the code whose page was opened, once
	devices  []api.Device
	teams    cloudTeams
	tab      int           // the tab shown once signed in, of cloudTabs
	recovery recoveryState // the Devices tab's recovery key (recovery.go)
	// full are the teams with their members, latest the last of their logs, as
	// far as the user may read them.
	full   map[string]api.Team
	latest []cloudRecord
}

// cloudTabs split what a signed-in host has in the cloud.
var cloudTabs = []string{"Teams", "Devices"}

func (a *App) cloudOf() *cloudSettings {
	if a.cloudSet == nil || a.cloudSet.host != a.host.ID {
		a.cloudSet = &cloudSettings{host: a.host.ID}
		a.loadCloud()
	}
	return a.cloudSet
}

// loadCloud reads the host's sign-in, and when signed in its devices and
// teams.
func (a *App) loadCloud() {
	cs, cl := a.cloudSet, a.client
	if cs == nil || cl == nil {
		return
	}
	go func() {
		st, err := cl.CloudStatus()
		var devices []api.Device
		var teams []api.Team
		var full map[string]api.Team
		var latest []cloudRecord
		if err == nil && st.SignedIn {
			if e := cl.CloudCall("GET", api.PathDevices, nil, &devices); e != nil {
				err = e
			}
			if e := cl.CloudCall("GET", api.PathTeams, nil, &teams); e != nil && err == nil {
				err = e
			}
			full, latest = cloudTeamsInFull(cl, teams)
		}
		a.onMain(func() {
			cs.loaded = true
			cs.err = ""
			if err != nil {
				cs.err = client.Explain(err)
			}
			cs.setStatus(st)
			cs.devices, cs.teams.list, cs.full, cs.latest = devices, teams, full, latest
		})
	}()
}

func (cs *cloudSettings) setStatus(st proto.CloudStatus) {
	cs.st = st
	if st.Err != "" {
		cs.err = st.Err
	}
}

// cloudEvent takes the host's sign-in as it changes; it runs on the
// client's goroutine.
func (a *App) cloudEvent(ev proto.Event) {
	if ev.Name != proto.EventCloudChanged {
		return
	}
	var st proto.CloudStatus
	if json.Unmarshal(ev.Data, &st) != nil {
		return
	}
	a.onMain(func() {
		cs := a.cloudSet
		if cs == nil || cs.host != a.host.ID {
			return
		}
		was := cs.st.SignedIn
		cs.setStatus(st)
		if st.SignedIn && !was {
			a.loadCloud() // signed in just now: its devices and teams
		}
		if !st.SignedIn {
			cs.devices, cs.teams, cs.full, cs.latest = nil, cloudTeams{}, nil, nil
		}
	})
}

// cloudDo runs fn off the main thread, then done on it, showing what is
// under way and the error if any.
func (a *App) cloudDo(what string, fn func(cl *client.Client) error, done func()) {
	cs, cl := a.cloudSet, a.client
	if cs == nil || cl == nil || cs.busy != "" {
		return
	}
	cs.busy, cs.err = what, ""
	go func() {
		err := fn(cl)
		a.onMain(func() {
			cs.busy = ""
			if err != nil {
				cs.err = client.Explain(err)
				return
			}
			if done != nil {
				done()
			}
		})
	}()
}

// cloudSignIn starts signing the connected host in, as the app when it is
// this computer.
func (a *App) cloudSignIn() {
	cs := a.cloudOf()
	kind := proto.CloudDeviceHost
	if a.host.ID == hosts.LocalID {
		kind = proto.CloudDeviceApp
	}
	name := a.host.Name
	a.cloudDo("Asking for a code…", func(cl *client.Client) error {
		st, err := cl.CloudSignIn(proto.CloudSignInParams{Kind: kind, Name: name})
		a.onMain(func() { cs.setStatus(st) })
		return err
	}, nil)
}

// cloudSignOut asks, then signs the connected host out.
func (a *App) cloudSignOut() {
	a.confirm("Sign "+a.host.Name+" out?",
		"Its teams' spaces leave Notes and Canvas until you sign in again, and it stops taking the teams' changes. Your own notes stay.",
		"Sign out", a.cloudSignOutNow)
}

func (a *App) cloudSignOutNow() {
	cs := a.cloudOf()
	a.cloudDo("Signing out…", func(cl *client.Client) error {
		st, err := cl.CloudSignOut()
		a.onMain(func() { cs.setStatus(st); cs.devices, cs.teams, cs.full, cs.latest = nil, cloudTeams{}, nil, nil })
		return err
	}, nil)
}

// cloudCancel drops a sign-in waiting for its code.
func (a *App) cloudCancel() {
	cs := a.cloudOf()
	a.cloudDo("", func(cl *client.Client) error {
		st, err := cl.CloudCancel()
		a.onMain(func() { cs.setStatus(st) })
		return err
	}, nil)
}

// approveDevice shows the device waiting behind code, and signs it in to
// the account once the user confirms: a code someone else sent would sign
// their device in as the user, team keys included.
func (a *App) approveDevice(cs *cloudSettings, code string) {
	var pending api.PendingDevice
	a.cloudDo("Looking up "+code+"…", func(cl *client.Client) error {
		return cl.CloudCall("GET", strings.Replace(api.PathDevicePending, "{code}", url.PathEscape(code), 1), nil, &pending)
	}, func() {
		kind := "the enowX app"
		if pending.Kind == api.DeviceHost {
			kind = "a host's daemon"
		}
		msg := "“" + pending.Name + "”, " + kind + ", " + deviceOrigin(pending) + ", signs in to your account and gets your teams' keys. Approve it only if you are signing it in yourself, just now."
		if pending.From != "" && !pending.SameNetwork {
			msg = "It started from another network than this host's. " + msg
		}
		a.confirm("Sign “"+pending.Name+"” in to your account?", msg, "Approve", func() {
			a.cloudDo("Approving "+code+"…", func(cl *client.Client) error {
				return cl.CloudCall("POST", api.PathDeviceApprove, api.DeviceApproveParams{UserCode: code}, nil)
			}, func() { cs.approve = ""; a.loadCloud() })
		})
	})
}

// deviceOrigin says when and where a device's sign-in started.
func deviceOrigin(d api.PendingDevice) string {
	out := "started " + textfmt.Age(d.Created, time.Now()) + " ago"
	if d.From != "" {
		out += " from " + d.From
		if d.Place != "" && d.Place != "XX" {
			out += " (" + d.Place + ")"
		}
	}
	return out
}

// openCloudPage opens the page approving the pending code, once per code
// unless again.
func (a *App) openCloudPage(again bool) {
	cs := a.cloudOf()
	p := cs.st.Pending
	if p == nil || p.VerifyURLComplete == "" || (!again && cs.opened == p.UserCode) {
		return
	}
	cs.opened = p.UserCode
	link := p.VerifyURLComplete
	if !sameSite(link, cs.st.URL) {
		// Only the cloud's own page: not another site, scheme or app.
		a.status = "The cloud asked to open an address of another site; not opened"
		return
	}
	go func() { _ = mygo.Shell.OpenExternal(link) }()
}

// sameSite says link is a page of the cloud at base: same scheme and
// host, http or https only.
func sameSite(link, base string) bool {
	l, err1 := url.Parse(link)
	b, err2 := url.Parse(base)
	if err1 != nil || err2 != nil || (l.Scheme != "https" && l.Scheme != "http") {
		return false
	}
	return l.Scheme == b.Scheme && strings.EqualFold(l.Host, b.Host)
}

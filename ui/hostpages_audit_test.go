package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/core/setup"
	"github.com/enowx/enowx/core/tailnet"
	"github.com/enowx/enowx/core/vps"
	"github.com/enowx/enowx/hostconn"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/syncview"
)

// The Hosts, Set up and Sync environment pages with sample hosts in every
// state, for the layout audit: other hosts aren't reachable from a test, so
// what the app knows of them is filled in.

var auditLabs = hosts.Host{ID: "labs", Name: "enowxlabs", Address: "root@enowxlabs", RunAs: "enowdev", Provider: "hetzner", ProviderID: "1"}

// auditHosts saves sample hosts and what their watches would say.
func auditHosts(t *testing.T, a *App) {
	t.Helper()
	store, err := hosts.Open(filepath.Join(t.TempDir(), "hosts.json"))
	if err != nil {
		t.Fatal(err)
	}
	a.opts.Hosts = store
	usage := proto.HostUsage{CPUs: 8, CPU: 31, MemTotal: 16 << 30, MemUsed: 9 << 30, DiskTotal: 200 << 30, DiskUsed: 40 << 30}
	sample := []struct {
		h  hosts.Host
		st hostStatus
	}{
		{auditLabs, hostStatus{state: hostOnline, usage: usage, sessions: []proto.SessionInfo{{ID: "s"}}}},
		{hosts.Host{ID: "studio", Name: "studio-mac", Address: "enowdev@studio-mac", Kind: hosts.KindDevice}, hostStatus{state: hostOnline, usage: usage}},
		{hosts.Host{ID: "pi", Name: "pi-lab", Address: "pi@pi-lab"}, hostStatus{state: hostNeedsYou}},
		{hosts.Host{ID: "old", Name: "a server with a rather long name for its card", Address: "root@old-vps"}, hostStatus{state: hostOffline, err: "ssh: connect to host old-vps port 22: operation timed out, and a reason long enough to take the two lines it may have and then some more of it"}},
	}
	a.watches = map[string]*hostWatch{}
	a.hostList = nil
	for _, s := range sample {
		saved, err := store.Add(s.h) // it gets an ID of its own
		if err != nil {
			t.Fatal(err)
		}
		if s.h.ID == auditLabs.ID {
			auditLabs.ID = saved.ID
		}
		a.hostList = append(a.hostList, saved)
		a.watches[saved.ID] = &hostWatch{host: saved, status: s.st, info: proto.DaemonInfo{Version: "0.5.0"}}
	}
	a.peers, a.peersLoaded = []tailnet.Peer{
		{HostName: "nas-box", IP: "100.74.12.8", OS: "linux", Online: true},
		{HostName: "old-laptop", IP: "100.81.3.22", OS: "macOS"},
	}, true
	a.hostsPageShown = true
}

func auditSetupSteps() []setup.Step {
	return []setup.Step{
		{ID: "packages", Title: "System packages: git, curl, build-essential", Detail: "all here", Done: true, Script: "apt-get install -y git curl"},
		{ID: "mise", Title: "mise, which installs the toolchains", Detail: "2025.1", Done: true, Script: "curl https://mise.run | sh"},
		{ID: "tool:go", Title: "Go 1.27", Detail: "go1.27.3", Done: true, Script: "mise use -g go@1.27"},
		{ID: "tool:node", Title: "Node 24 with a name long enough to run in its row of the card", Detail: "not here", Script: "mise use -g node@24"},
		{ID: "git", Title: "Git user: enowdev <dev@example.com>", Detail: "enowdev", Done: true, Script: "git config --global user.name enowdev"},
		{ID: "gh", Title: "GitHub CLI (gh)", Detail: "2.82", Done: true, Script: "apt-get install gh"},
		{ID: "gh-auth", Title: "Sign gh in to GitHub", Detail: "signed in", Done: true, Asks: true, Script: "gh auth login"},
		{ID: "agent:claude", Title: "Claude Code", Detail: "not here", Script: "npm i -g @anthropic-ai/claude-code"},
		{ID: "agent:codex", Title: "Codex", Detail: "not here", Script: "npm i -g @openai/codex"},
		{ID: "signin:claude", Title: "Sign Claude Code in", Detail: "a code to enter in a browser", Asks: true, Script: "claude login"},
	}
}

func auditSetup(a *App, commands bool) {
	a.host = auditLabs
	p := &setupPage{host: a.host, from: setupFromDevice, scanned: true, steps: auditSetupSteps(), picked: map[string]bool{}}
	for _, s := range p.steps {
		p.picked[s.ID] = !s.Done
	}
	if commands {
		p.commands = map[string]bool{"Toolchains": true, "Agents": true}
	}
	a.setup, a.page = p, pageSetup
}

func auditEnv() (proto.EnvScan, proto.EnvScan) {
	item := func(kind, name, hash string, secrets ...string) proto.EnvItem {
		return proto.EnvItem{ID: "claude/" + kind + "/" + name, Agent: "claude", Kind: kind, Name: name, Hash: hash, Secrets: secrets}
	}
	local := proto.EnvScan{Items: []proto.EnvItem{
		item(proto.EnvSkill, "concise-context", "2"), item(proto.EnvSkill, "credential-guard", "1"), item(proto.EnvSkill, "handoff", "1"),
		item(proto.EnvMCP, "enowx", "1"), item(proto.EnvMCP, "coolify", "1", "COOLIFY_TOKEN"), item(proto.EnvMCP, "enowxcord", "1"),
		item(proto.EnvSubagent, "code-reviewer.md", "1"), item(proto.EnvCommand, "release.md", "1"),
		item(proto.EnvMemory, "CLAUDE.md", "2"), item(proto.EnvGit, "enowdev", "1"),
	}}
	remote := proto.EnvScan{Items: []proto.EnvItem{
		item(proto.EnvSkill, "concise-context", "1"), item(proto.EnvSkill, "credential-guard", "1"), item(proto.EnvSkill, "handoff", "1"),
		item(proto.EnvMCP, "enowx", "1"), item(proto.EnvMCP, "enowxcord", "1"), item(proto.EnvSubagent, "code-reviewer.md", "1"),
		item(proto.EnvMemory, "CLAUDE.md", "1"), item(proto.EnvGit, "enowdev", "1"),
	}}
	return local, remote
}

func auditSync(a *App, showSame bool) {
	a.host = auditLabs
	local, remote := auditEnv()
	a.sync = syncview.Loaded(syncview.Options{Host: auditLabs, Local: LocalHost(), AgentName: a.agentName}, local, remote)
	if showSame {
		a.sync.ShowSame(true)
	}
	a.page = pageSync
}

// TestLayoutAuditHosts draws them as TestLayoutAudit draws the others.
func TestLayoutAuditHosts(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	auditPagesOn(t, h, hostAuditPages(t), func() { closeHostAudit(h) })
}

// auditPagesOn draws pages at every size, with a clean start before each, and
// fails on what layoutProblems finds.
func auditPagesOn(t *testing.T, h *notesHarness, pages []auditPage, reset func()) {
	t.Helper()
	only := os.Getenv("AUDIT")
	var problems []string
	for _, p := range pages {
		if only != "" && !strings.Contains(p.name, only) {
			continue
		}
		for _, size := range auditSizes {
			closeAudit(h.app)
			reset()
			p.open(h.app)
			tt := ui.NewTester(func(c *ui.Context) { h.app.view(c) }, size[0], size[1])
			h.tt = tt
			for range 6 {
				h.settle("", func() bool { return true })
			}
			name := fmt.Sprintf("%s-%dx%d", p.name, size[0], size[1])
			shot(t, tt, filepath.Join("audit", name+".png"))
			for _, pr := range layoutProblems(tt, float32(size[0]), float32(size[1]), p.fill) {
				problems = append(problems, name+": "+pr)
			}
		}
	}
	if len(problems) > 0 {
		t.Errorf("%d layout problems:\n%s", len(problems), strings.Join(problems, "\n"))
	}
}

// hostAuditPages are the hosts' pages and the states of a connection.
func hostAuditPages(t *testing.T) []auditPage {
	return []auditPage{
		{name: "hosts-all", fill: true, open: func(a *App) { auditHosts(t, a); a.page = pageHosts }},
		{name: "hosts-remote", fill: true, open: func(a *App) {
			auditHosts(t, a)
			a.host = auditLabs
			a.drift = map[string]int{auditLabs.ID: 2}
			a.page = pageHosts
		}},
		{name: "setup-page", fill: true, open: func(a *App) { auditSetup(a, false) }},
		{name: "setup-commands", fill: true, open: func(a *App) { auditSetup(a, true) }},
		{name: "setup-scanning", fill: true, open: func(a *App) {
			a.host = auditLabs
			a.setup, a.page = &setupPage{host: a.host, scanning: true}, pageSetup
		}},
		{name: "sync-page", fill: true, open: func(a *App) { auditSync(a, true) }},
		{name: "sync-changed", fill: true, open: func(a *App) { auditSync(a, false) }},
		{name: "dialog-host-tailnet", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			a.addHost = addHostForm{open: true}
			a.peers = append(a.peers, tailnet.Peer{HostName: "a device with a name too long for its row of the dialog, which has to run", IP: "100.2.3.4", OS: "linux"})
		}},
		{name: "dialog-host-address", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			a.addHost = addHostForm{open: true, manual: true, err: "Run sessions as: use lowercase letters, digits, - and _, starting with a letter."}
		}},
		{name: "dialog-host-root", open: func(a *App) {
			auditHosts(t, a)
			a.page, a.peers = pageHosts, nil
			a.peersErr = "Tailscale isn't installed: install it to see your tailnet's devices here."
			a.addHost = addHostForm{open: true, asRoot: true, device: true}
		}},
		{name: "dialog-host-edit", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			h := auditLabs
			h.PasswordAuth = true
			a.openEditHost(h)
		}},
		{name: "dialog-trust", open: func(a *App) {
			auditHosts(t, a)
			a.trust = &trustPrompt{addr: "pi-lab:22", fingerprint: "SHA256:sample-fingerprint-not-real (ED25519)", answer: make(chan bool, 1)}
		}},
		{name: "dialog-sign-in", open: func(a *App) {
			auditHosts(t, a)
			a.signIn = &signInPrompt{host: auditLabs, refusal: &transport.AuthError{Addr: "enowxlabs:22", User: "root", TriedPassword: true}, user: "root", remember: true, err: "Type the password."}
		}},
		{name: "dialog-sign-in-keys", open: func(a *App) {
			auditHosts(t, a)
			a.signIn = &signInPrompt{host: auditLabs, refusal: &transport.AuthError{Addr: "enowxlabs:22", User: "root", Allowed: []string{"publickey"}}, user: "root"}
		}},
		{name: "dialog-passphrase", open: func(a *App) {
			auditHosts(t, a)
			a.passphrase = &passphrasePrompt{path: "/Users/enowdev/.ssh/id_ed25519_and_a_name_long_enough_that_it_has_to_wrap_in_the_box_it_shows_in", answer: make(chan passphraseAnswer, 1)}
		}},
		{name: "dialog-run-as", open: func(a *App) {
			auditHosts(t, a)
			a.runAs = &runAsPrompt{host: auditLabs, user: "enowx", answer: make(chan runAsAnswer, 1), err: "Use lowercase letters, digits, - and _, starting with a letter."}
		}},
		{name: "dialog-run-as-users", open: func(a *App) {
			auditHosts(t, a)
			a.runAs = &runAsPrompt{host: auditLabs, choice: "deploy", answer: make(chan runAsAnswer, 1), users: []hostconn.ServerUser{{Name: "deploy", Home: "/home/deploy"}, {Name: "enowdev", Home: "/home/enowdev"}}}
		}},
		{name: "dialog-vps-regions", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			regions := []vps.Option{{ID: "hel", Name: "Helsinki"}, {ID: "fsn", Name: "Falkenstein"}}
			sizes := []vps.Option{{ID: "cx22", Name: "CX22", Detail: "2 CPU, 4 GB, and a detail long enough to run in its field", Monthly: 4.5}}
			a.creating = &vpsForm{open: true, provider: vps.Providers[0].ID, token: "sample", loaded: vps.Providers[0].ID, regions: regions, region: "hel", sizes: sizes, size: "cx22", name: "enowx-2", key: "/Users/enowdev/.ssh/id_ed25519.pub", remember: true,
				busy: "Waiting for the server to start", err: "The provider said no, and a reason long enough to have to wrap inside its notice in the dialog, whatever its width."}
		}},
		{name: "dialog-vps-no-key", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			a.creating = &vpsForm{open: true, provider: vps.Providers[0].ID, token: "sample", fromVault: true, loaded: vps.Providers[0].ID, regions: []vps.Option{{ID: "hel", Name: "Helsinki"}}, region: "hel", name: "enowx-2"}
		}},
		{name: "dialog-vps-existing", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			var box string
			for _, p := range vps.Providers {
				if vps.NeedsCertificate(p.ID) {
					box = p.ID
				}
			}
			a.creating = &vpsForm{open: true, provider: box, token: "sample", loaded: box, regions: []vps.Option{{ID: "a", Name: "Here"}}, region: "a", listed: true, existing: true, key: "/Users/enowdev/.ssh/id_ed25519.pub", pick: "s",
				servers: []vps.Server{{ID: "s", Name: "a box", Status: "suspended", Size: "4 vCPUs, 8 GB"}}, busy: "Reading where to sign in…"}
		}},
		{name: "dialog-vps-delete", open: func(a *App) {
			auditHosts(t, a)
			a.page = pageHosts
			h := auditLabs
			a.deletingServer = &h
		}},
		{name: "connecting", open: func(a *App) {
			a.host, a.client, a.page, a.connecting, a.connectLink = auditLabs, nil, "", "Tailscale wants you to sign in to reach it", "https://login.tailscale.com/a/b"
		}},
		{name: "connect-failed", open: func(a *App) {
			a.host, a.client, a.page, a.connErr = auditLabs, nil, "", "ssh: handshake failed: unable to authenticate"
			a.connFail = &transport.AuthError{Addr: "enowxlabs:22", User: "root"}
		}},
		{name: "disconnected", open: func(a *App) {
			a.host, a.client, a.page, a.connErr = auditLabs, nil, "", "Disconnected from enowxlabs. Its sessions keep running; connecting again in 8s…"
		}},
	}
}

// closeHostAudit gives the harness its local host back.
func closeHostAudit(h *notesHarness) {
	a := h.app
	a.host = hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	a.client, a.setup, a.sync, a.connecting, a.connectLink, a.connErr, a.connFail = h.cl, nil, nil, "", "", "", nil
	a.trust, a.signIn, a.passphrase, a.runAs, a.deletingServer = nil, nil, nil, nil, nil
}

// The Create a server dialog's drop-downs open within the window, each option
// running inside its row.
func TestVPSOptionPicker(t *testing.T) {
	h := newNotesHarness(t)
	seedAudit(t, h)
	a := h.app
	auditHosts(t, a)
	a.page = pageHosts
	long := "2 CPU, 4 GB, and a detail long enough to run in its row of the list"
	a.creating = &vpsForm{open: true, provider: vps.Providers[0].ID, token: "sample", loaded: vps.Providers[0].ID, name: "enowx-2", region: "hel", size: "cx22",
		regions: []vps.Option{{ID: "hel", Name: "Helsinki"}, {ID: "fsn", Name: "Falkenstein"}},
		sizes:   []vps.Option{{ID: "cx22", Name: "CX22", Detail: long, Monthly: 4.5}, {ID: "cx32", Name: "CX32", Detail: "4 CPU, 8 GB", Monthly: 8}}}
	tt := ui.NewTester(func(c *ui.Context) { a.view(c) }, 1280, 800)
	h.tt = tt
	tt.Advance(time.Second)
	r, ok := tt.Find("Helsinki")
	if !ok {
		t.Fatalf("no Helsinki: %q", tt.Texts())
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	tt.Advance(time.Second)
	h.settle("the regions offered", func() bool { return tt.HasText("Falkenstein") })
	shot(t, tt, filepath.Join("audit", "vps-picker.png"))
	for _, pr := range layoutProblems(tt, 1280, 800, false) {
		t.Error(pr)
	}
	r, ok = tt.Find("Falkenstein")
	if !ok {
		t.Fatalf("no Falkenstein: %q", tt.Texts())
	}
	tt.ClickAt(r.X+r.W/2, r.Y+r.H/2)
	h.settle("the region picked", func() bool { return a.creating.region == "fsn" })
}

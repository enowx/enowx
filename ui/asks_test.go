package ui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/hosts"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/transport"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/vaultview"
)

// asksHarness is the app's asks over a composer the user is typing into.
type asksHarness struct {
	*notesHarness
	typed string
}

func newAsksHarness(t *testing.T) *asksHarness {
	t.Helper()
	h := &asksHarness{notesHarness: newNotesHarness(t)}
	info, err := h.cl.Info()
	if err != nil {
		t.Fatal(err)
	}
	h.app.daemonInfo, h.app.host = info, hosts.Host{ID: hosts.LocalID, Name: "this Mac"}
	h.tt = ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Width(1000).Height(760).Background(theme.Background).TextColor(theme.Text).Children(func() {
			ui.TextArea(c, &h.typed).Label("Message").Width(400).Height(120)
			h.app.asksView(c)
			h.app.passphraseDialog(c)
			h.app.confirmDialog(c)
		})
	}, 1000, 760)
	h.tt.Frame()
	return h
}

func (h *asksHarness) focusComposer() {
	h.t.Helper()
	if err := h.tt.Click("Message"); err != nil {
		h.t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Frame()
	if !h.tt.Focused("Message") {
		h.t.Fatal("the composer has no keyboard to begin with")
	}
}

// settled runs frames long enough for anything that would move the focus.
func (h *asksHarness) settled() {
	n := 0
	h.settle("frames", func() bool { n++; return n > 8 })
}

func (h *asksHarness) hostAsk(run *string) {
	h.t.Helper()
	info, _ := h.cl.Info()
	params, _ := json.Marshal(proto.SessionRef{ID: "nope"})
	raw, _ := json.Marshal(proto.HostCallParams{Host: info.ID, Method: proto.MethodSessionKill, Params: params, From: "asker"})
	h.app.hostCallAsked(info.ID, proto.AppRequest{Params: raw}, func(_ any, err error) {
		if err == nil || !strings.Contains(err.Error(), "the user said no") {
			*run = "ran"
		} else {
			*run = "refused"
		}
	}, func() {})
}

// askKind is one kind of ask: raising it, whether it was approved.
type askKind struct {
	name, title, approve string
	raise                func()
	approved             func() bool
}

func (h *asksHarness) askKinds() []askKind {
	a, cl := h.app, h.cl
	event := func(name string, v any) proto.Event {
		data, _ := json.Marshal(v)
		return proto.Event{Name: name, Data: data}
	}
	host, vault, hibernate := "", 0, false
	gone := func(left func() int) func() bool { return func() bool { return left() == 0 } }
	var trust chan bool
	var phrase chan passphraseAnswer
	var runAs chan runAsAnswer
	var signIn *signInPrompt
	return []askKind{
		{"host control", "Let a session do this?", "Allow",
			func() { host = ""; h.hostAsk(&host) }, func() bool { return host == "ran" }},
		{"takeover", "bob asks to take over", "Let them type", func() {
			a.presenceEvent(event(proto.EventTakeoverAsked, proto.TakeoverAsk{Session: "s1", Visitor: proto.Visitor{Account: "b", Name: "bob"}}))()
		}, gone(func() int { return len(a.presenceOf().asks) })},
		{"vault", "Keep this?", "Allow", func() {
			vault = 0
			a.vaultQueue(&vaultRequestAsk{req: proto.VaultRequest{ID: "v1"}, ask: vaultview.Ask{Title: "Keep this?"}, done: func(n int) { vault = n }})
		}, func() bool { return vault != 0 }},
		{"feature", "Turn on traffic2?", "Turn on everywhere", func() {
			a.featureRequested(cl, "this Mac", proto.FeatureRequest{ID: "traffic2", RequestID: "r1"})
		}, gone(func() int { return len(a.featAsks) })},
		{"hibernate", "“asker” asks to hibernate", "Hibernate", func() {
			hibernate = false
			a.hibernateAsks = append(a.hibernateAsks, &hibernateForm{asker: "asker", until: askUntil(hostAskLife), reply: func(yes bool) { hibernate = yes }})
		}, func() bool { return hibernate }},
		{"decision", "it seemed to loop", "Let it go on", func() {
			a.decideEvent(event(proto.EventDecision, proto.Decision{ID: "d1", Session: "s1", Why: "Same test.", Acted: proto.ActedPaused}))
		}, gone(func() int { return len(a.decisions) })},
		{"account", "hit its usage limit", "Switch and go on", func() {
			a.accountEvent(event(proto.EventAccountFallback, proto.AccountFallback{Session: "s1", From: "Work", To: "Spare", Ask: true}))
		}, gone(func() int { return len(a.acctAsks) })},
		{"rag", "Keep this as a memory of the project?", "Save memory", func() {
			a.ragEvent(event(proto.EventRAGOffer, proto.RAGRememberParams{RAGRef: proto.RAGRef{Project: "/x", Session: "s1"}, Text: "We chose Go."}))
		}, gone(func() int { return len(a.ragOffers) })},
		{"trust", "Trust this server?", "Trust and connect", func() {
			trust = make(chan bool, 1)
			a.connectByApp, a.trust = true, &trustPrompt{addr: "h:22", fingerprint: "SHA256:x", answer: trust}
		}, func() bool { return len(trust) == 1 && <-trust }},
		{"run as", "Run agents as a regular user?", "Create user and connect", func() {
			runAs = make(chan runAsAnswer, 1)
			a.connectByApp, a.runAs = true, &runAsPrompt{host: a.host, user: defaultRunAs, answer: runAs}
		}, func() bool { return len(runAs) == 1 }},
		{"passphrase", "Unlock your SSH key", "Unlock and connect", func() {
			phrase = make(chan passphraseAnswer, 1)
			a.connectByApp, a.passphrase = true, &passphrasePrompt{path: "/k", answer: phrase}
		}, func() bool { return len(phrase) == 1 && (<-phrase).err == nil }},
		{"sign in", "Sign in to h", "Sign in", func() {
			// With no password typed it stops short of connecting, saying so.
			signIn = &signInPrompt{host: hosts.Host{ID: "h", Name: "h", Address: "root@h"}, refusal: &transport.AuthError{Addr: "h:22", User: "root"}, user: "root"}
			a.connectByApp, a.signIn = true, signIn
		}, func() bool {
			if signIn.err == "" {
				return false
			}
			a.signIn = nil
			return true
		}},
	}
}

// Each kind of ask arriving leaves the composer every character and the
// keyboard, Enter and Space approve nothing, and a click, or Tab then Enter,
// does. A password or text field in the card waits its turn.
func TestTypingSurvivesEveryAsk(t *testing.T) {
	h := newAsksHarness(t)
	for _, how := range []string{"click", "tab"} {
		for _, k := range h.askKinds() {
			t.Run(k.name+"/"+how, func(t *testing.T) {
				h.typed = ""
				h.tt.Frame()
				h.focusComposer()
				h.tt.Type("ab")
				k.raise()
				h.settle("the card", func() bool { return strings.Contains(strings.Join(h.tt.Texts(), " "), k.title) })
				h.settled()
				h.tt.Type("cd")
				h.tt.Key(0, ui.KeyEnter)
				h.tt.Key(0, ui.KeySpace)
				h.tt.Type("ef")
				h.settled()
				if got := strings.Join(strings.Fields(h.typed), ""); got != "abcdef" {
					t.Fatalf("typed %q", h.typed)
				}
				if !h.tt.Focused("Message") {
					t.Fatal("the composer lost the keyboard")
				}
				if k.approved() {
					t.Fatal("typing approved it")
				}
				if how == "click" {
					if err := h.tt.Click(k.approve); err != nil {
						t.Fatal(err)
					}
				} else {
					for i := 0; !h.tt.Focused(k.approve); i++ {
						if i > 12 {
							t.Fatal("Tab never reaches the approving button")
						}
						h.tt.Key(0, ui.KeyTab)
						h.tt.Frame()
						if i == 0 && h.tt.Focused(k.approve) {
							t.Fatal("the first Tab lands on the approving button")
						}
					}
					h.tt.Key(0, ui.KeyEnter)
				}
				h.settle("approved", k.approved)
				h.app.connectByApp = false
				h.settled()
				if how == "tab" && !h.tt.Focused("Message") {
					t.Fatal("the answer left the keyboard nowhere")
				}
			})
		}
	}
}

// Answering by keyboard doesn't leave the keyboard on the next ask's button:
// Enter again approves nothing.
func TestEnterAgainDoesntApproveTheNextAsk(t *testing.T) {
	h := newAsksHarness(t)
	h.focusComposer()
	first, second := "", ""
	h.hostAsk(&first)
	h.hostAsk(&second)
	h.settled()
	for i := 0; !h.tt.Focused("Allow"); i++ {
		if i > 6 {
			t.Fatal("Tab never reaches Allow")
		}
		h.tt.Key(0, ui.KeyTab)
		h.tt.Frame()
	}
	h.tt.Key(0, ui.KeyEnter)
	h.settle("the first answered", func() bool { return first == "ran" })
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.settled()
	if second != "" || len(h.app.hostAsks) != 1 {
		t.Fatalf("the next ask: %q, %d waiting", second, len(h.app.hostAsks))
	}
}

// A confirm the user opened keeps its focus and isn't replaced by an agent's
// question, which waits behind it.
func TestAskWaitsBehindTheUsersConfirm(t *testing.T) {
	h := newAsksHarness(t)
	done := false
	h.app.confirm("Remove it?", "It goes.", "Remove", func() { done = true })
	h.settled()
	if !h.tt.Focused("Remove") {
		t.Fatal("a confirm the user opened no longer focuses its button")
	}
	asked := ""
	h.hostAsk(&asked)
	h.settled()
	if !h.tt.Focused("Remove") || h.app.confirming == nil || !h.tt.HasText("Remove it?") {
		t.Fatal("an agent's question disturbed the user's confirm")
	}
	h.tt.Key(0, ui.KeyEnter)
	h.settle("the user's own answer", func() bool { return done })
	h.settled()
	if asked != "" || len(h.app.hostAsks) != 1 {
		t.Fatalf("the question: %q, %d waiting", asked, len(h.app.hostAsks))
	}
	if err := h.tt.Click("Allow"); err != nil {
		t.Fatal(err)
	}
	h.settle("then answered", func() bool { return asked == "ran" })
}

// Past the daemon's wait a card goes, refused: the yes can't run any more.
func TestLateAsksDoNothing(t *testing.T) {
	h := newAsksHarness(t)
	past := time.Now().Add(-time.Second)
	asked := ""
	h.hostAsk(&asked)
	h.app.hostAsks[0].until = past
	yes, called := false, false
	h.app.hibernateAsks = []*hibernateForm{{asker: "a", until: past, reply: func(y bool) { yes, called = y, true }}}
	answer := 0
	h.app.vaultQueue(&vaultRequestAsk{req: proto.VaultRequest{ID: "v"}, done: func(n int) { answer = n }})
	h.app.vlt.asks[0].until = past
	h.app.featureRequested(h.cl, "this Mac", proto.FeatureRequest{ID: "traffic", RequestID: "r"})
	h.app.featAsks[0].until = past
	h.settled()
	if asked != "refused" || !called || yes || answer == 0 || len(h.app.featAsks) != 0 || h.tt.HasText("Allow") {
		t.Fatalf("host %q, hibernate %v/%v, vault %d, features %d", asked, called, yes, answer, len(h.app.featAsks))
	}
}

// A passphrase asked on a connection the user started takes the keyboard as
// ever; on one the app started, only once they click into the field.
func TestPassphraseFocusFollowsWhoConnected(t *testing.T) {
	h := newAsksHarness(t)
	h.app.passphrase = &passphrasePrompt{path: "/k", answer: make(chan passphraseAnswer, 1)}
	h.settled()
	if !h.tt.Focused("Passphrase") {
		t.Fatal("the user's own connecting no longer focuses the field")
	}
	h.app.answerPassphrase(false)
	h.settled()

	h.focusComposer()
	h.app.connectByApp = true
	h.app.passphrase = &passphrasePrompt{path: "/k", answer: make(chan passphraseAnswer, 1)}
	h.settled()
	if !h.tt.Focused("Message") || h.tt.Focused("Passphrase") {
		t.Fatal("the app's own retry took the keyboard")
	}
	if err := h.tt.Click("Passphrase"); err != nil {
		t.Fatal(err)
	}
	h.tt.Frame()
	h.tt.Type("secret")
	h.tt.Frame()
	if h.app.passphrase.value != "secret" {
		t.Fatalf("typed %q", h.app.passphrase.value)
	}
}

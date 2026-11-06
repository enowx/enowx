package daemon

import (
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/enowx/enowx/core/session"
	"github.com/enowx/enowx/proto"
)

// restart asks for a restart of GOA, then what its agent started with: nil unless it did.
func (r *goaSwitchRig) restart(p proto.GOASwitchParams) (proto.GOASwitchResult, []string, error) {
	r.t.Helper()
	for _, ext := range []string{".args", ".env"} {
		_ = os.Remove(r.out + "/" + r.goa.ID + ext)
	}
	var res proto.GOASwitchResult
	if err := r.c.Call(proto.MethodGOARestart, p, &res); err != nil {
		return res, nil, err
	}
	if !res.Restarted {
		return res, nil, nil
	}
	args, _ := started(r.t, r.out, r.goa.ID)
	return res, args, nil
}

// GOA restarts on its agent and config, going on with its conversation by its ID, in the
// same session; it can't restart itself.
func TestGOARestartGoesOnWithItsConversation(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	if _, _, err := r.restart(proto.GOASwitchParams{From: r.goa.ID}); err == nil || !strings.Contains(err.Error(), "can't restart itself") {
		t.Fatalf("GOA restarted itself: %v", err)
	}
	res, args, err := r.restart(proto.GOASwitchParams{})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Restarted || res.Conversation != proto.GOAConversationSame || res.Session.ID != r.goa.ID {
		t.Fatalf("restart: %+v", res)
	}
	if i := slices.Index(args, "--resume"); i < 0 || i+1 >= len(args) || args[i+1] != goaSwitchConvo {
		t.Fatalf("not resumed by its ID: %q", args)
	}
}

// A GOA whose agent ended right after starting, again and again, is refused continuing for
// a while; the user's restart isn't.
func TestGOARestartAfterQuickExits(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	sess, err := r.srv.sessions.Get(r.goa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.srv.sessions.Replace(session.Spec{ID: r.goa.ID, Dir: r.goa.Dir, Command: []string{"sh", "-c", "exit 1"}, Meta: sess.Meta()}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "it ended", func() bool { s, err := r.srv.sessions.Get(r.goa.ID); return err == nil && s.Info().Exited })
	for range quickExitsMax {
		s, _ := r.srv.sessions.Get(r.goa.ID)
		if _, err := r.srv.sessions.Replace(session.Spec{ID: r.goa.ID, Dir: r.goa.Dir, Command: []string{"sh", "-c", "exit 1"}, Meta: s.Meta()}); err != nil {
			t.Fatal(err)
		}
		waitFor(t, "it ended", func() bool { s, err := r.srv.sessions.Get(r.goa.ID); return err == nil && s.Info().Exited })
	}
	res, _, err := r.restart(proto.GOASwitchParams{})
	if err != nil || !res.Restarted {
		t.Fatalf("restart after quick exits: %+v %v", res, err)
	}
}

// At work, a restart needs the user's yes; with it, it goes.
func TestGOARestartAtWorkAsksFirst(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	r.said()
	sess, err := r.srv.sessions.Get(r.goa.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.srv.sessions.SetState(sess, proto.StateWorking); err != nil {
		t.Fatal(err)
	}
	before := r.info()
	res, _, err := r.restart(proto.GOASwitchParams{})
	if err != nil || res.Restarted || !res.AtWork {
		t.Fatalf("at work, without force: %+v %v", res, err)
	}
	r.untouched(before)
	if res, _, err = r.restart(proto.GOASwitchParams{Force: true}); err != nil || !res.Restarted {
		t.Fatalf("with force: %+v %v", res, err)
	}
}

// A conversation it can't go on with is said, nothing done; a new one starts only when
// asked for, never by itself.
func TestGOARestartWithItsConversationGone(t *testing.T) {
	r := newGOASwitchRig(t, "claude")
	path := r.said()
	if res, _, err := r.restart(proto.GOASwitchParams{}); err != nil || !res.Restarted {
		t.Fatalf("first restart: %+v %v", res, err) // it records the conversation it is on
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	r.srv.transcripts.forget(r.goa.ID)
	before := r.info()
	res, _, err := r.restart(proto.GOASwitchParams{})
	if err != nil || res.Restarted || res.Unresumable == "" || !strings.Contains(res.Unresumable, goaSwitchConvo) {
		t.Fatalf("its conversation gone: %+v %v", res, err)
	}
	r.untouched(before)
	res, args, err := r.restart(proto.GOASwitchParams{NewConversation: true})
	if err != nil || !res.Restarted || res.Conversation != proto.GOAConversationNew {
		t.Fatalf("on a new conversation: %+v %v", res, err)
	}
	if slices.Contains(args, "--resume") || slices.Contains(args, "--continue") {
		t.Fatalf("a new conversation went on with one: %q", args)
	}
}

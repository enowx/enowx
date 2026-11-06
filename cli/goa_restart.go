package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// goaRestartUsage is `goa restart`'s line of help.
const goaRestartUsage = "usage: enowx-app goa restart [--check] [--force] [--new-conversation]"

// goaRestart restarts GOA on its agent and config, going on with its conversation
// (docs/goa.md). Never from GOA's own session: it ends the turn asking for it.
func (r runner) goaRestart(args []string) error {
	fs := flagSet("goa restart")
	check := fs.Bool("check", false, "")
	fresh := fs.Bool("new-conversation", false, "")
	force := fs.Bool("force", false, "")
	if words, err := parseAll(fs, args); err != nil || len(words) > 0 {
		return errors.New(goaRestartUsage)
	}
	p := proto.GOASwitchParams{Check: *check, NewConversation: *fresh, Force: *force, From: os.Getenv(EnvSession)}
	if !p.Check && os.Getenv(proto.EnvGOA) != "" {
		return errors.New("GOD of Agents can't restart itself, as that ends the turn it asks in: tell the user to restart it from GOA's header, or with `goa restart` in a terminal")
	}
	var res proto.GOASwitchResult
	var err error
	if p.Check {
		err = r.c.Call(proto.MethodGOARestart, p, &res)
	} else {
		err = r.guarded(proto.MethodGOARestart, p, &res) // an agent asking: the user's yes
	}
	if client.IsUnknownMethod(err) {
		return errors.New("this host's enowX is too old to restart GOD of Agents this way: update it, or change its config and back in Settings")
	}
	if err != nil {
		return err
	}
	if !p.Check && !res.Restarted {
		return goaRestartNeeds(res)
	}
	fmt.Fprintln(r.out, goaRestartText(res, p.Check))
	return nil
}

// goaRestartNeeds is the yes res lacks, nothing done.
func goaRestartNeeds(res proto.GOASwitchResult) error {
	if res.Unresumable != "" {
		return fmt.Errorf("GOD of Agents can't go on with its conversation (%s): restart it on a new one with --new-conversation", res.Unresumable)
	}
	return errors.New("GOD of Agents is at work, and restarting cuts that off: run it again with --force")
}

// goaRestartText says what a restart did, or would do.
func goaRestartText(res proto.GOASwitchResult, check bool) string {
	conversation := "its conversation goes on"
	switch {
	case res.Conversation == proto.GOAConversationNew && res.Unresumable != "":
		conversation = "on a new conversation, as it can't go on with its own (" + res.Unresumable + ")"
	case res.Conversation == proto.GOAConversationNew:
		conversation = "on a new conversation"
	}
	on := res.Agent + " on " + nonEmpty(res.Config, "the official config")
	if res.Restarted {
		return "GOD of Agents restarted: " + on + ", " + conversation
	}
	text := "It would restart: " + on + ", " + conversation
	if res.AtWork {
		text += ". It is at work, which that cuts off (--force)"
	}
	return text
}

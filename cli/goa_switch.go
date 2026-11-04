package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
)

// goaSwitchUsage is `goa switch`'s line of help.
const goaSwitchUsage = "usage: enowx-app goa switch [--agent A] [--config NAME|official] [--check] [--new-conversation] [--force]"

// goaSwitch shows or changes the agent and config GOA runs on (docs/goa.md). In GOA's own
// session it only reads: a change ends the turn asking for it, so it stays the user's.
func (r runner) goaSwitch(args []string) error {
	fs := flagSet("goa switch")
	agentID := fs.String("agent", "", "")
	config := fs.String("config", "", "")
	check := fs.Bool("check", false, "")
	fresh := fs.Bool("new-conversation", false, "")
	force := fs.Bool("force", false, "")
	if _, err := parseAll(fs, args); err != nil {
		return errors.New(goaSwitchUsage)
	}
	p := proto.GOASwitchParams{Agent: *agentID, Config: *config, Check: *check, NewConversation: *fresh, Force: *force, From: os.Getenv(EnvSession)}
	p.Check = p.Check || p.Agent == "" && p.Config == ""
	if !p.Check && os.Getenv(proto.EnvGOA) != "" {
		return errors.New("GOD of Agents can't change its own agent or config, as that ends the turn it asks in: tell the user to change it in Settings in GOA's header, or with `goa switch` in a terminal")
	}
	var res proto.GOASwitchResult
	var err error
	if p.Check {
		err = r.c.Call(proto.MethodGOASwitch, p, &res)
	} else {
		err = r.guarded(proto.MethodGOASwitch, p, &res) // an agent asking: the user's yes
	}
	if client.IsUnknownMethod(err) {
		return errors.New("this host's enowX is too old to change GOD of Agents' agent or config: update it. GOD of Agents is as it was")
	}
	if err != nil {
		return err
	}
	if !p.Check && !res.Same && !res.Restarted {
		return goaSwitchNeeds(res)
	}
	fmt.Fprintln(r.out, goaSwitchText(res, p.Check))
	if res.Note != "" {
		fmt.Fprintln(r.out, res.Note)
	}
	return nil
}

// goaSwitchNeeds is the yes res lacks, nothing done.
func goaSwitchNeeds(res proto.GOASwitchResult) error {
	if res.Conversation == proto.GOAConversationNew {
		return fmt.Errorf("on %s GOD of Agents starts a new conversation: the one it is on doesn't carry over. Run it again with --new-conversation if that is what the user wants (the board, its collection and rules stay)", res.Agent)
	}
	return errors.New("GOD of Agents is at work, and restarting cuts that off: run it again with --force if that is what the user wants")
}

// goaSwitchText says where res leaves GOA.
func goaSwitchText(res proto.GOASwitchResult, check bool) string {
	on := res.Agent + " on " + nonEmpty(res.Config, "the official config") + " (" + res.Level + ")"
	mode := "its agent's default permission mode"
	if res.Mode != "" {
		mode = "permission mode " + res.Mode
	}
	var conversation string
	switch res.Conversation {
	case proto.GOAConversationSame:
		conversation = "the same conversation goes on"
	case proto.GOAConversationNone:
		conversation = "it has no conversation yet"
	default:
		conversation = "a new conversation: the one it is on doesn't carry over"
	}
	switch {
	case res.Same:
		return "GOD of Agents runs on " + on + ", in " + mode
	case res.Restarted:
		return "GOD of Agents restarted: " + on + ", in " + mode + "; " + conversation
	case check:
		text := "It would restart: " + on + "; " + conversation
		if res.AtWork {
			text += ". It is at work, which that cuts off (--force)"
		}
		return text
	}
	return on
}

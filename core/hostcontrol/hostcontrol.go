// Package hostcontrol says what a session may do to sessions and settings
// of a host through host.call (docs/goa-tools.md): the methods it reaches,
// each at a level, and who may use each level. The asking host's daemon
// checks it first, and the app checks it again before it relays a request
// to another host, so neither alone opens a method the other refuses.
package hostcontrol

import (
	"fmt"
	"slices"

	"github.com/enowx/enowx/cloud/api"
	"github.com/enowx/enowx/proto"
)

// Level is how much a method changes.
type Level int

const (
	// None: the method can't be called through host.call.
	None Level = iota
	// Read only reads.
	Read
	// Change changes what can be changed back: a session started, a
	// prompt sent, a setting changed.
	Change
	// Confirm loses or publishes something (a session closed, a branch
	// pushed or merged, a secret sent to a host): the user says yes in
	// the app first when an agent asks.
	Confirm
)

func (l Level) String() string {
	switch l {
	case Read:
		return "read"
	case Change:
		return "change"
	case Confirm:
		return "confirm"
	}
	return "none"
}

// methods are what host.call reaches, at their level. Nothing of the
// vault, hooks, the cloud's sign-in or the daemon's own life is here: a
// credential's value is never read, and those changes stay the user's.
var methods = map[string]Level{
	proto.MethodDaemonInfo:           Read,
	proto.MethodAgentList:            Read,
	proto.MethodAccountList:          Read,
	proto.MethodSessionList:          Read,
	proto.MethodSessionScreen:        Read,
	proto.MethodSessionTranscript:    Read,
	proto.MethodSessionPeek:          Read,
	proto.MethodSessionCommands:      Read,
	proto.MethodSessionPorts:         Read,
	proto.MethodSessionAsk:           Read,
	proto.MethodProjectList:          Read,
	proto.MethodProjectRules:         Read,
	proto.MethodProjectConfig:        Read,
	proto.MethodProjectCloneStatus:   Read,
	proto.MethodReviewStatus:         Read,
	proto.MethodReviewDiff:           Read,
	proto.MethodDaemonSettings:       Read,
	proto.MethodSessionEnvList:       Read, // values masked
	proto.MethodHibernatePrefs:       Read,
	proto.MethodShellList:            Read,
	proto.MethodFeaturesList:         Read,
	proto.MethodFeaturesGuide:        Read,
	proto.MethodSessionCreate:        Change,
	proto.MethodSessionFanOut:        Change,
	proto.MethodSessionInput:         Change,
	proto.MethodSessionInterrupt:     Change,
	proto.MethodSessionAnswer:        Change,
	proto.MethodSessionMode:          Change,
	proto.MethodSessionRename:        Change,
	proto.MethodSessionNote:          Change,
	proto.MethodSessionContinue:      Change,
	proto.MethodSessionHibernate:     Change,
	proto.MethodSessionWake:          Change,
	proto.MethodSessionAutoHibernate: Change,
	proto.MethodAccountSwitch:        Change,
	proto.MethodProjectAdd:           Change,
	proto.MethodProjectClone:         Change,
	proto.MethodProjectRulesSave:     Change,
	proto.MethodProjectConfigSave:    Change,
	proto.MethodReviewCommit:         Change,
	proto.MethodDaemonSetSettings:    Change,
	proto.MethodShellSet:             Change,
	proto.MethodFeaturesSet:          Change,
	proto.MethodFeaturesLazy:         Change,
	proto.MethodFeaturesPreset:       Change,
	proto.MethodAppSettings:          Change,
	proto.MethodSessionKill:          Confirm,
	proto.MethodReviewPush:           Confirm,
	proto.MethodReviewPR:             Confirm,
	proto.MethodReviewMerge:          Confirm,
	proto.MethodReviewDiscard:        Confirm,
	proto.MethodProjectRemove:        Confirm,
	proto.MethodSessionEnvSet:        Confirm, // a secret's value, sent to the host
	proto.MethodProjectCloneCancel:   Change,
	proto.MethodHostControlLog:       Read,
}

// LevelOf is method's level, None when host.call doesn't reach it.
func LevelOf(method string) Level { return methods[method] }

// Methods are the methods host.call reaches, sorted.
func Methods() []string {
	var out []string
	for m := range methods {
		out = append(out, m)
	}
	slices.Sort(out)
	return out
}

// roleNeeds is the role in a team a member needs for each level, on a
// host a teammate shares.
var roleNeeds = map[Level]string{Read: api.RoleViewer, Change: api.RoleMember, Confirm: api.RoleMember}

var rank = map[string]int{api.RoleViewer: 1, api.RoleMember: 2, api.RoleAdmin: 3, api.RoleOwner: 4}

// Check says whether method may go through host.call to a host on which
// the user has role: "" for one of the user's own hosts, else their role
// in the team sharing it.
func Check(method, role string) (Level, error) {
	l := LevelOf(method)
	if l == None {
		return None, fmt.Errorf("control: %s can't be used on a host from a session", method)
	}
	if role == "" {
		return l, nil
	}
	if need := roleNeeds[l]; rank[role] < rank[need] {
		return l, fmt.Errorf("control: %s needs a %s of the team on that host, you're a %s", method, need, role)
	}
	return l, nil
}

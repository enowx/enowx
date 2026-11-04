package textfmt

import "github.com/enowx/enowx/proto"

// SessionRules says what a session's agent has of its rules, "" when none hold for it.
func SessionRules(r *proto.SessionRules) string {
	switch {
	case r == nil || r.For == "" || r.Current == "" && r.Version == "":
		return ""
	case r.Current == "":
		if r.Hook {
			return "rules removed, told with its next prompt"
		}
		return "rules removed, it still has " + r.Version
	case r.Version != r.Current:
		switch {
		case r.Hook:
			return "rules " + r.Current + " wait for its next prompt"
		case r.Version == "":
			return "rules " + r.Current + " not confirmed, it is told to read them"
		}
		return "rules changed to " + r.Current + ", it has " + r.Version
	case r.Again:
		return "rules " + r.Current + " given again with its next prompt"
	case r.Via == proto.RulesViaFile:
		return "rules " + r.Current + " in its guide file, not confirmed read"
	}
	return "rules " + r.Current + " arrived"
}

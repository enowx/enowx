package goa

import (
	"regexp"
	"strings"
)

// Why a worker's tool call is the user's to answer, not GOA's.
const (
	UrgentUndo    = "it can't be undone"
	UrgentMoney   = "it spends money"
	UrgentPublish = "it publishes"
	UrgentSecrets = "it touches secrets"
	UrgentUnsure  = "the judge isn't sure"
)

var urgentCommands = []struct {
	why string
	re  *regexp.Regexp
}{
	{UrgentPublish, regexp.MustCompile(`\bgit\s+push\b|\b(npm|pnpm|yarn|cargo|gem|poetry)\s+publish\b|\btwine\s+upload\b|\bdocker\s+push\b|\bgh\s+(release|pr\s+merge)\b|\b(vercel|netlify)\b.*--prod|\b(fly|flyctl)\s+deploy\b|\bkubectl\s+apply\b|\bterraform\s+apply\b`)},
	{UrgentUndo, regexp.MustCompile(`\brm\s+(-[a-zA-Z]*[rf]|--recursive|--force)|\bgit\s+(reset\s+--hard|clean\s+-[a-zA-Z]*f|branch\s+-D|checkout\s+--\s|restore\s)|\b(drop|truncate)\s+(table|database|schema)\b|\bdelete\s+from\b|\bmkfs|\bdd\s+if=|\bkubectl\s+delete\b|\bterraform\s+destroy\b|\bdocker\s+(system|volume)\s+prune\b|\bshred\b`)},
	{UrgentMoney, regexp.MustCompile(`\bstripe\b|\bpayment|\bpurchase|\bcheckout\s+session|\bbuy\b|\bbilling\b|\b(hcloud|doctl|vultr-cli)\s+server\s+create\b|\baws\s+ec2\s+run-instances\b`)},
	{UrgentSecrets, regexp.MustCompile(`(^|[\s/'"])\.env(\.|\b)|id_rsa|id_ed25519|\.pem\b|\.p12\b|credentials|\bsecrets?\b|\bpasswords?\b|\bapi[_-]?key|\bprintenv\b|\bcreds\s+(env|run|login)\b|\bsecurity\s+find-generic-password\b`)},
}

// Urgent says why a worker's tool call (tool, its command, the files it
// touches) is the user's to answer, "" when GOA and the judge may: what
// can't be undone, spends money, publishes, or touches secrets.
func Urgent(tool, command string, paths []string) string {
	text := strings.ToLower(command + " " + strings.Join(paths, " "))
	for _, u := range urgentCommands {
		if u.re.MatchString(text) {
			return u.why
		}
	}
	return ""
}

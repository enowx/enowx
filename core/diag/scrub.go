package diag

import (
	"regexp"
)

// What looks like a secret, in an error or a line of an older log.
var secrets = []struct {
	re   *regexp.Regexp
	with string
}{
	{regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?(-----END [A-Z0-9 ]*PRIVATE KEY-----|$)`), "[masked key]"},
	{regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]{6,}`), "$1 [masked]"},
	{regexp.MustCompile(`(?i)\b([a-z0-9_-]*(?:token|secret|password|passwd|pwd|api[_-]?key|apikey|authorization|cookie|credential|private[_-]?key|access[_-]?key)[a-z0-9_-]*)(["']?\s*[:=]\s*["']?)[^\s"',;&]+`), "$1$2[masked]"},
	{regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^/\s:@]*:)[^/\s@]+@`), "$1[masked]@"},
	{regexp.MustCompile(`\b(sk|pk|rk)-[A-Za-z0-9_-]{12,}`), "[masked]"},
	{regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{10,}|github_pat_[A-Za-z0-9_]{10,}|glpat-[A-Za-z0-9_-]{10,}|xox[abprs]-[A-Za-z0-9-]{10,})`), "[masked]"},
	{regexp.MustCompile(`\b(AKIA|ASIA)[A-Z0-9]{12,}`), "[masked]"},
	{regexp.MustCompile(`\bAIza[A-Za-z0-9_-]{20,}`), "[masked]"},
	{regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}(\.[A-Za-z0-9_-]+)?`), "[masked]"},
	// Long runs of one alphabet are keys more often than anything a log needs: IDs here are shorter.
	{regexp.MustCompile(`\b[A-Za-z0-9+/_-]{48,}={0,2}`), "[masked]"},
}

// Scrub masks what looks like a secret in text.
func Scrub(text string) string {
	for _, s := range secrets {
		text = s.re.ReplaceAllString(text, s.with)
	}
	return text
}

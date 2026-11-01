package credits

import (
	"bytes"
	"context"
	"os/exec"
	"regexp"
	"slices"
)

// claudeService matches the keychain items Claude Code keeps its login
// in: one per configuration folder, the default one unsuffixed.
var claudeServices = regexp.MustCompile(`"svce"<blob>="(Claude Code-credentials(?:-[0-9a-f]+)?)"`)

// claudeKeychain reads every Claude Code login in the macOS keychain.
// Listing the items reads their names only; reading one is what macOS may
// ask the user to allow.
func claudeKeychain(ctx context.Context) [][]byte {
	names := []string{"Claude Code-credentials"}
	if list, err := exec.CommandContext(ctx, "security", "dump-keychain").Output(); err == nil {
		for _, m := range claudeServices.FindAllSubmatch(list, -1) {
			if name := string(m[1]); !slices.Contains(names, name) && !skipped(ctx, name) {
				names = append(names, name)
			}
		}
	}
	var out [][]byte
	for _, name := range names {
		item, err := exec.CommandContext(ctx, "security", "find-generic-password", "-s", name, "-w").Output()
		if err == nil {
			out = append(out, bytes.TrimSpace(item))
		}
	}
	return out
}

// claudeKeychainItem reads keychain item service, if there is one.
func claudeKeychainItem(ctx context.Context, service string) [][]byte {
	item, err := exec.CommandContext(ctx, "security", "find-generic-password", "-s", service, "-w").Output()
	if err != nil {
		return nil
	}
	return [][]byte{bytes.TrimSpace(item)}
}

// skipped says keychain item service is an account's home's, read apart.
func skipped(ctx context.Context, service string) bool {
	homes, _ := ctx.Value(skipKey{}).([]Home)
	for _, h := range homes {
		if h.Agent == "claude" && claudeService(h.Path) == service {
			return true
		}
	}
	return false
}

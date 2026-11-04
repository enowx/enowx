package platformview

import (
	"fmt"
	"math"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// KeyState is what the device knows of a key for the agent.
type KeyState int

const (
	KeyHave    KeyState = iota // the vault has one
	KeyMissing                 // it has none, or there is no vault yet
	KeyLocked                  // it is locked: there may be one
)

// Welcome is what the agent's chat says before its first message.
type Welcome struct {
	Name          string // the agent's
	Model, Effort string // what it runs on, as they read
	Budget        float64
	Key           KeyState
	OpenKeys      func()
}

// welcomeWidth is how wide its words run, in a chat that leaves it.
const welcomeWidth = 460

// Draw draws it, in the room it is given.
func (w Welcome) Draw(c *ui.Context) {
	ui.Column(c).FillWidth().AlignItems(ui.Center).Padding(2*theme.S5, 0, 0, 0).Children(func() {
		ui.Column(c).FillWidth().MaxWidth(welcomeWidth).Gap(theme.S3).Children(func() {
			ui.Row(c).FillWidth().Gap(theme.S3).AlignItems(ui.Center).Children(func() {
				widget.Avatar(c, icon.Agent("claude-platform"), 36, theme.Text)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					ui.Row(c).FillWidth().MinWidth(0).Children(func() {
						widget.Running(c, w.Name, widget.RunStyle{Size: theme.TextTitle, Weight: 600, Color: theme.Text})
					})
					quiet(c, "enowX's own coding agent, on Anthropic's API with your key.")
				})
			})
			w.facts(c)
			KeyNotice(c.Key("key"), w.Key, w.OpenKeys)
			quiet(c, "You pay Anthropic by the token at list prices. /budget sets what a session may spend before it asks; the figure beside the context shows what it has cost.")
		})
	})
}

// facts are the model, the effort and the budget, in rows: a long name runs in its own.
func (w Welcome) facts(c *ui.Context) {
	row := func(label, value string) {
		if value == "" {
			return
		}
		ui.Row(c.Key(label)).FillWidth().MinWidth(0).Gap(theme.S3).Children(func() {
			ui.Text(c, label).FontSize(theme.TextUI).TextColor(theme.TextSoft).SingleLine().Width(64).Shrink(0)
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, value, widget.RunStyle{Color: theme.Text})
			})
		})
	}
	ui.Column(c.Key("facts")).FillWidth().Gap(theme.S1).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).Background(theme.Sunken).Children(func() {
		row("Model", w.Model)
		row("Effort", w.Effort)
		if w.Budget > 0 {
			row("Budget", budgetText(w.Budget)+" a session")
		}
	})
}

// budgetText writes a budget: $10, $12.50.
func budgetText(usd float64) string {
	if usd == math.Trunc(usd) {
		return fmt.Sprintf("$%.0f", usd)
	}
	return fmt.Sprintf("$%.2f", usd)
}

// KeyNotice says the agent has no key to use, or may not, with the way to fix that;
// nothing with one.
func KeyNotice(c *ui.Context, state KeyState, open func()) {
	title, body, action := "", "", ""
	switch state {
	case KeyMissing:
		title, action = "No Anthropic API key yet", "Add an API key"
		body = "Claude Platform talks to Anthropic with your key, kept in your vault. Without one it can't answer, unless the host's own environment has one."
	case KeyLocked:
		title, action = "Your vault is locked", "Unlock it"
		body = "Claude Platform borrows its key from your vault, so unlock it to see the key or add one."
	default:
		return
	}
	widget.Notice(c, icon.Key, theme.Warning, title, body, func() {
		ui.Row(c.Key("act")).FillWidth().Padding(theme.S1, 0, 0, 0).Children(func() {
			if open != nil && widget.Button(c.Key("open-keys"), widget.Secondary, nil, action, "").Clicked() {
				open()
			}
		})
	})
}

// KeyRefused says a notice of the agent's is about its key: the API's 401, or no key to
// send it.
func KeyRefused(notice string) bool {
	n := strings.ToLower(notice)
	for _, s := range []string{"401", "authentication_error", "authentication error", "invalid x-api-key", "invalid api key", "no api key", "api key is missing"} {
		if strings.Contains(n, s) {
			return true
		}
	}
	return false
}

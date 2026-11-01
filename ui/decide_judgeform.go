package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/decide"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// judgeForm adds a judge of the user's: a decision model (one of the
// providers that serve them, or any System One address), a model API (its
// key from the vault) or a model server on the host.
type judgeForm struct {
	open                    bool
	kind, provider          string
	url, cred, model, label string
	preset                  string // a decision model's provider (decide.DecisionPresets' Label)
	openAI                  bool   // a custom decision address speaks OpenAI's Decisions, not System One
	// key is the key typed here (kept on the host alone), unless fromVault:
	// a vault credential (cred) instead.
	key       string
	fromVault bool
	err       string
}

var judgeKinds = []string{proto.JudgeDecision, proto.JudgeLocal, proto.JudgeAPI}

func (a *App) judgeFormSheet(c *ui.Context, st *decideState) {
	f := &st.adding
	if !f.open {
		return
	}
	widget.FormSheet(c, &f.open, 560, "Add a judge", "A decision model answers typed questions with probabilities, fast and cheap, and is asked first. A small chat model (Haiku, a mini or flash model, a 7–8B local one) is the fallback.", func() {
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		labels := []string{"A decision model", "A model server on " + a.host.Name, "A model API"}
		cur := max(0, indexOf(judgeKinds, f.kind))
		if i := widget.Segmented(c, labels, cur, nil); i >= 0 && judgeKinds[i] != f.kind {
			f.kind = judgeKinds[i]
			if f.kind == proto.JudgeAPI && f.provider == "" {
				f.provider = proto.ProviderOpenAI
			}
		}
		switch f.kind {
		case proto.JudgeDecision:
			a.decisionFields(c, f)
		case proto.JudgeAPI:
			widget.Field(c, "Provider", func() {
				hookSelect(c.Key("provider"), "Provider", &f.provider, providers, providerNames)
			})
			a.keyFields(c, "judge-key", &f.key, &f.cred, &f.fromVault, "sk-…")
			widget.Field(c, "Address (OpenAI-compatible; empty for the provider's own)", func() {
				widget.Input(c, &f.url, "https://api.example.com/v1").FillWidth().Font(theme.MonoFont)
			})
			widget.Field(c, "Model", func() { widget.Input(c, &f.model, "claude-haiku-5-5").FillWidth().Font(theme.MonoFont) })
		default:
			widget.Field(c, "Address (OpenAI-compatible)", func() {
				widget.Input(c, &f.url, "http://127.0.0.1:11434/v1").FillWidth().Font(theme.MonoFont)
			})
			widget.Field(c, "Model", func() { widget.Input(c, &f.model, "qwen3:8b").FillWidth().Font(theme.MonoFont) })
		}
		widget.Field(c, "Name (optional)", func() { widget.Input(c, &f.label, "").FillWidth() })
	}, func() {
		if widget.PrimaryButton(c, icon.Plus, "Add", "").Disabled(strings.TrimSpace(f.model) == "").Clicked() {
			if j, err := f.judge(); err != "" {
				f.err = err
			} else {
				a.addJudge(st, j)
			}
		}
	})
}

// decisionFields are a decision model's: its provider (an address and
// model filled in), the key, the model.
func (a *App) decisionFields(c *ui.Context, f *judgeForm) {
	opts := make([]string, len(decide.DecisionPresets))
	names := map[string]string{}
	for i, p := range decide.DecisionPresets {
		opts[i], names[p.Label] = p.Label, p.Label
	}
	if f.preset == "" {
		f.pick(decide.DecisionPresets[0])
	}
	was := f.preset
	hookSelect(c.Key("decision-provider"), "Provider", &f.preset, opts, names)
	if was != f.preset {
		for _, p := range decide.DecisionPresets {
			if p.Label == f.preset {
				f.pick(p)
			}
		}
	}
	local := f.provider == proto.ProviderLocal
	widget.Field(c, "System One address", func() {
		widget.Input(c, &f.url, "https://decisions.example.com/v1/systemone").FillWidth().Font(theme.MonoFont)
		if f.provider == proto.ProviderCloudflare {
			ui.Text(c, "{account} is the credential's account ID, {model} the model.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		if f.provider == proto.ProviderOpenRouter {
			ui.Text(c, "OpenRouter's Decisions speak System One; paste the address its docs give.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
	})
	if f.provider == "custom" {
		switchRowKey(c, "openai-shape", &f.openAI, "Speaks OpenAI's Decisions API", "Its input and questions list, not System One's state and questions map.")
	}
	if !local {
		a.keyFields(c, "decision-key", &f.key, &f.cred, &f.fromVault, "sk-…")
	}
	widget.Field(c, "Model", func() { widget.Input(c, &f.model, "jev-latest").FillWidth().Font(theme.MonoFont) })
}

// pick fills the form from decision model preset p.
func (f *judgeForm) pick(p proto.Judge) {
	f.preset, f.provider, f.url, f.model, f.openAI = p.Label, p.Provider, p.URL, p.Model, p.Protocol == decide.DecisionOpenAI
}

// judge is the judge the form adds, or why not.
func (f *judgeForm) judge() (proto.Judge, string) {
	j := proto.Judge{Kind: f.kind, Provider: f.provider, URL: strings.TrimSpace(f.url), Cred: f.cred, Model: strings.TrimSpace(f.model), Label: strings.TrimSpace(f.label)}
	if !f.fromVault {
		j.Cred = ""
	}
	switch f.kind {
	case proto.JudgeAPI:
		if j.Cred == "" && strings.TrimSpace(f.key) == "" && j.URL == "" {
			return j, "Type the API's key, or pick the vault's credential holding it."
		}
	case proto.JudgeDecision:
		if f.openAI {
			j.Protocol = decide.DecisionOpenAI
		}
		if j.URL == "" {
			return j, "Give the System One address."
		}
		if j.Provider == "custom" || j.Provider == "opper" || j.Provider == "ollama" {
			j.Provider = "" // known by its address
		}
	}
	return j, ""
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}

// keyFields are where a key comes from: typed here (kept on the host
// alone, never shown again) or a vault credential, which other hosts can
// use too.
func (a *App) keyFields(c *ui.Context, key string, typed, cred *string, fromVault *bool, hint string) {
	cur := 0
	if *fromVault {
		cur = 1
	}
	widget.Field(c, "Key", func() {
		if i := widget.Segmented(c.Key(key+"-from"), []string{"Typed here", "From the vault"}, cur, nil); i >= 0 {
			*fromVault = i == 1
		}
		if *fromVault {
			a.ragCredSelect(c, key, cred)
			return
		}
		widget.Input(c.Key(key), typed, hint).FillWidth().Password()
		ui.Text(c, "Kept on "+a.host.Name+" alone, readable by its user only; never synced or shown again.").FontSize(theme.TextLabel).TextColor(theme.Muted)
	})
}

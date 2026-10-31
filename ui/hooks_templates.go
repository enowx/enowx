package ui

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// hookTemplates are hooks to start from: the common ones, ready to save
// or change.
var hookTemplates = []struct {
	name string
	hook proto.Hook
}{
	{"Refuse dangerous commands", proto.Hook{Name: "Refuse dangerous commands", On: true, Event: proto.HookToolBefore, Action: proto.HookActionBlock,
		Tool:   `rm\s+-[a-z]*r[a-z]*f?\s+(/|~|\$HOME)(\s|$)|git\s+push\s.*--force|git\s+reset\s+--hard|DROP\s+(TABLE|DATABASE)`,
		Reason: "This command can't be undone: ask the user to run it."}},
	{"Have the judge approve commands", proto.Hook{Name: "Judge commands", On: true, Event: proto.HookToolBefore, Action: proto.HookActionJudge,
		Tool: `^(Bash|shell|exec_command)$`}},
	{"Trim long test and build output", proto.Hook{Name: "Trim long output", On: true, Event: proto.HookToolBefore, Action: proto.HookActionTrim,
		Text: `^\s*(npm|pnpm|yarn|bun)\s+(test|run\s+(test|build|lint)|install|ci)\b|^\s*go\s+(test|build|vet)\b|^\s*(make|cargo\s+(build|test)|pytest|mvn|gradle|\./gradlew)\b`}},
	{"Refuse pushes to main", proto.Hook{Name: "No pushes to main", On: true, Event: proto.HookGitBefore, Action: proto.HookActionBlock,
		Text: `push\b.*\b(main|master)\b`, Reason: "Push to a branch of its own and open a pull request."}},
	{"Format the code after a turn", proto.Hook{Name: "Format", On: true, Event: proto.HookFileChanged, Action: proto.HookActionRun, Background: true,
		Paths:   []string{"*.go", "*.ts", "*.tsx", "*.js", "*.css"},
		Command: `if [ -f go.mod ]; then gofmt -w .; fi; if [ -f package.json ]; then npx --no-install prettier --write . >/dev/null 2>&1; fi; exit 0`}},
	{"Project rules at the start of a session", proto.Hook{Name: "Project rules", On: true, Event: proto.HookSessionStart, Action: proto.HookActionRun,
		Command: `cat .enowx/rules.md AGENTS.md 2>/dev/null | head -c 6000`}},
	{"Notify when a turn is done", proto.Hook{Name: "Done", On: true, Event: proto.HookTurnDone, Action: proto.HookActionNotify, Kind: "done",
		Message: "{agent} finished in {project}"}},
	{"Push the work when a turn is done", proto.Hook{Name: "Auto push", On: true, Event: proto.HookTurnDone, Action: proto.HookActionEnowX, Background: true,
		Command: "project autopush . --now"}},
	{"Post to a channel when a turn is done", proto.Hook{Name: "Tell the channel", On: true, Event: proto.HookTurnDone, Action: proto.HookActionWebhook,
		Message: "{agent} finished a turn in {project} ({branch})"}},
}

// hookTemplateMenu starts a new hook from a template.
func (a *App) hookTemplateMenu(c *ui.Context, st *hooksState) {
	picked := ""
	sel := ui.SelectBase(c.Key("hook-templates"), &picked)
	widget.Trigger(sel.Trigger, sel.Open()).Width(190).Label("From a template").Children(func() {
		ui.Icon(c, icon.Wand).FontSize(13).TextColor(theme.TextSoft)
		ui.Text(c, "From a template").FontSize(12.5).SingleLine()
		ui.Spacer(c)
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(6, 0, 0, 0).Width(300))
		for _, t := range hookTemplates {
			item := sel.Item(t.name)
			widget.PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, t.name).FontSize(12.5) })
		}
	})
	for _, t := range hookTemplates {
		if t.name == picked && st.loaded {
			st.form.start(t.hook, hostPlace, true)
		}
	}
}

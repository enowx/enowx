package ui

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// hookForm adds a hook or changes one, in a dialog: as a form, its lists
// typed comma-separated, or as the hook's JSON.
type hookForm struct {
	open    bool
	hook    proto.Hook
	place   hookPlace
	adding  bool // a new hook, whose place can still be picked
	asJSON  bool
	json    string
	lists   map[string]*string // agents, projects, paths, branches, hosts
	numbers map[string]*string // max, every, timeout
	err     string
	testing bool
	tested  *proto.HookRun
}

func (f *hookForm) start(h proto.Hook, place hookPlace, adding bool) {
	*f = hookForm{open: true, hook: h, place: place, adding: adding}
	f.lists = map[string]*string{}
	for k, v := range map[string][]string{"agents": h.Agents, "projects": h.Projects, "paths": h.Paths, "branches": h.Branches, "hosts": h.Hosts} {
		s := strings.Join(v, ", ")
		f.lists[k] = &s
	}
	f.numbers = map[string]*string{}
	for k, v := range map[string]int{"max": h.Max, "every": h.Every, "timeout": h.Timeout} {
		s := ""
		if v > 0 {
			s = strconv.Itoa(v)
		}
		f.numbers[k] = &s
	}
}

// built is the form's hook, its text lists and numbers read, or the JSON
// typed.
func (f *hookForm) built() (proto.Hook, error) {
	if f.asJSON {
		var h proto.Hook
		if err := json.Unmarshal([]byte(f.json), &h); err != nil {
			return h, err
		}
		h.ID, h.Project, h.Session = f.hook.ID, "", ""
		return h, nil
	}
	h := f.hook
	h.Agents, h.Projects, h.Paths = splitList(*f.lists["agents"]), splitList(*f.lists["projects"]), splitList(*f.lists["paths"])
	h.Branches, h.Hosts = splitList(*f.lists["branches"]), splitList(*f.lists["hosts"])
	num := func(k string) int { n, _ := strconv.Atoi(strings.TrimSpace(*f.numbers[k])); return n }
	h.Max, h.Every, h.Timeout = num("max"), num("every"), num("timeout")
	h.Name = strings.TrimSpace(h.Name)
	return h, nil
}

// toggleJSON switches the form between its fields and the hook's JSON.
func (f *hookForm) toggleJSON() {
	if f.asJSON {
		h, err := f.built()
		if err != nil {
			f.err = "The JSON doesn't read: " + err.Error()
			return
		}
		f.start(h, f.place, f.adding)
		return
	}
	h, _ := f.built()
	h.ID, h.Project, h.Session = "", "", ""
	data, _ := json.MarshalIndent(h, "", "  ")
	f.asJSON, f.json, f.err = true, string(data), ""
}

func (a *App) hookFormSheet(c *ui.Context, st *hooksState) {
	f := &st.form
	if !f.open {
		return
	}
	title := "New hook"
	if !f.adding {
		title = "Edit hook"
	}
	widget.FormSheet(c, &f.open, 640, title, "Runs on "+a.host.Name+", for every agent that sees the event.", func() {
		if f.err != "" {
			widget.Notice(c, icon.Alert, theme.Error, f.err, "", nil)
		}
		ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
			if f.adding {
				a.hookPlacePicker(c, f)
			}
			ui.Spacer(c)
			mode := 0
			if f.asJSON {
				mode = 1
			}
			if i := widget.Segmented(c, []string{"Form", "JSON"}, mode, nil); i >= 0 && i != mode {
				f.toggleJSON()
			}
		})
		if f.asJSON {
			ui.TextAreaBase(c, &f.json).FillWidth().MinHeight(320).MaxHeight(520).Padding(9, 11).FontSize(theme.TextLabel).Font(theme.MonoFont).
				Label("The hook as JSON").Radius(theme.RadiusControl).Background(theme.Sunken).Border(1, theme.Border)
			ui.Text(c, "Fields as in docs/hooks.md: event, action, command, context, reason, find, replace, url, message, agents, projects, tool, paths, text, branches, hosts, any, not, once, max, every, hours, schedule, background, timeout.").
				FontSize(11).TextColor(theme.Muted)
		} else {
			a.hookFields(c, st, f)
		}
		if r := f.tested; r != nil {
			a.hookTestResult(c, r)
		}
	}, func() {
		if widget.Button(c, widget.Secondary, icon.Play, "Test", "").Disabled(f.testing).Clicked() {
			a.testHookForm(f)
		}
		if widget.PrimaryButton(c, icon.Check, "Save", "").Disabled(st.busy).Clicked() {
			h, err := f.built()
			if err != nil {
				f.err = "The JSON doesn't read: " + err.Error()
			} else {
				f.err = ""
				a.saveHook(st, f.place, h, false)
			}
		}
	})
}

// hookPlacePicker picks where a new hook is kept: this host, a project
// or a session.
func (a *App) hookPlacePicker(c *ui.Context, f *hookForm) {
	key := func(p hookPlace) string { return p.scope + "\x00" + p.dir + "\x00" + p.session }
	label := func(p hookPlace) string {
		switch p.scope {
		case proto.HookScopeProject:
			return "Project " + filepath.Base(p.dir)
		case proto.HookScopeSession:
			return "Session " + a.hookSessionName(p.session)
		}
		return "This host (synced)"
	}
	places := []hookPlace{hostPlace}
	for _, p := range a.projects {
		places = append(places, hookPlace{scope: proto.HookScopeProject, dir: p.Path})
	}
	for _, s := range a.sessions {
		if !s.Exited && s.Parent == "" && s.Agent != "" {
			places = append(places, hookPlace{scope: proto.HookScopeSession, session: s.ID})
		}
	}
	cur := key(f.place)
	sel := ui.SelectBase(c, &cur)
	widget.Trigger(sel.Trigger, sel.Open()).Width(260).Label("Kept").Children(func() {
		ui.Text(c, label(f.place)).FontSize(theme.TextUI).SingleLine().Shrink(1)
		ui.Spacer(c)
		ui.Icon(c, icon.ChevronDown).FontSize(12).TextColor(theme.Muted)
	})
	sel.Popup(func(panel ui.Element) {
		widget.Popup(panel.Margin(6, 0, 0, 0).Width(320))
		ui.Scroll(c).FillWidth().MaxHeight(320).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for _, p := range places {
					item := sel.Item(key(p))
					widget.PopupItem(item, item.Highlighted()).Children(func() { ui.Text(c, label(p)).FontSize(theme.TextUI) })
				}
			})
		})
	})
	for _, p := range places {
		if key(p) == cur {
			f.place = p
		}
	}
}

// testHookForm runs the form's hook on a sample event.
func (a *App) testHookForm(f *hookForm) {
	cl := a.client
	if cl == nil {
		return
	}
	h, err := f.built()
	if err != nil {
		f.err = "The JSON doesn't read: " + err.Error()
		return
	}
	h.On = true
	ev := proto.HookEvent{Event: h.Event, Session: "test", Agent: "claude", Prompt: "Fix the login redirect", Tool: "Bash", Command: "go test ./...",
		Paths: []string{"main.go"}, Op: "commit", Time: time.Now()}
	if p, ok := a.currentProject(); ok {
		ev.Dir, ev.Project = p.Path, p.Path
	}
	f.testing, f.tested = true, nil
	go func() {
		r, err := cl.TestHook(proto.HookTest{Hook: h, Event: ev})
		a.onMain(func() {
			f.testing = false
			if err != nil {
				f.err = client.Explain(err)
				return
			}
			f.err, f.tested = "", &r
		})
	}()
}

func (a *App) hookTestResult(c *ui.Context, r *proto.HookRun) {
	col, what := theme.Done, "Ran in "+strconv.FormatInt(r.Ms, 10)+" ms"
	switch {
	case r.Error != "":
		col, what = theme.Error, r.Error
	case r.Blocked:
		col, what = theme.Warning, "Refused"
	case r.Exit != 0:
		col, what = theme.Error, "Exited with "+strconv.Itoa(r.Exit)
	}
	ui.Column(c).FillWidth().Gap(4).Padding(8, 10).Radius(theme.RadiusControl).Background(theme.Sunken).Children(func() {
		ui.Text(c, "Test: "+what).FontSize(theme.TextLabel).TextColor(col)
		if out := strings.TrimSpace(r.Output); out != "" {
			ui.Text(c, out).Font(theme.MonoFont).FontSize(theme.TextLabel).TextColor(theme.TextSoft).MaxLines(8)
		}
	})
}

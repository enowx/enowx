package trafficview

import (
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/intercept"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// rulesState is the Rules page: the project's rules as read, and the one
// being edited.
type rulesState struct {
	project string
	loaded  bool
	loading bool
	saving  bool
	err     string
	list    []proto.TrafficRule
	filters []string // saved with the project
	edit    *ruleForm
	menus   map[string]*widget.ContextMenu // each row's, by rule
}

// ruleForm is a rule being written: its fields as typed.
type ruleForm struct {
	index int // in the list, -1 for a new rule
	rule  proto.TrafficRule

	methods, host, path, body string
	pathMode                  int
	action                    int
	status, headers, resBody  string
	times, delay, throttle    string
	target                    string
	keepHost                  bool
	reqSet, reqRemove         string
	resSet, resRemove         string
	resStatus                 string
	replaceFrom, replaceTo    string
	jsonPatch                 string
	fail                      int
	stage                     int
	messages                  string
	echo                      bool
	err                       string
}

var (
	pathModes  = []string{"prefix", "exact", "glob", "regex"}
	actions    = []string{"Mock", "Forward", "Transform", "Fail", "Pause", "Pass"}
	actionKind = []string{proto.ActionMock, proto.ActionForward, proto.ActionTransform, proto.ActionFail, proto.ActionBreakpoint, proto.ActionPass}
	fails      = []string{"status", "close", "reset", "timeout"}
	failLabels = []string{"503", "Close", "Reset", "Time out"}
	stages     = []string{"request", "response", "both"}
)

// projectOf is the project whose rules the page shows: the session's.
func projectOf(o Options) string { return o.Session.Project }

func (r *rulesState) load(o Options) {
	p := projectOf(o)
	if (r.loaded && r.project == p) || r.loading || o.Host == nil {
		return
	}
	r.loading, r.project = true, p
	h := o.Host
	go func() {
		rules, err := h.TrafficRules(p)
		o.Update(func() {
			r.loading, r.loaded = false, true
			if err != nil {
				r.err = client.Explain(err)
				return
			}
			r.err, r.list, r.filters = "", rules.Rules, rules.Filters
		})
	}()
}

func (r *rulesState) save(o Options, list []proto.TrafficRule, done func()) {
	r.saveAll(o, list, r.filters, done)
}

// saveAll saves the project's rules and saved filters.
func (r *rulesState) saveAll(o Options, list []proto.TrafficRule, filters []string, done func()) {
	r.saving = true
	h, p := o.Host, r.project
	go func() {
		saved, err := h.SetTrafficRules(proto.TrafficRules{Project: p, Rules: list, Filters: filters})
		o.Update(func() {
			r.saving = false
			if err != nil {
				r.err = client.Explain(err)
				return
			}
			r.err, r.list, r.filters = "", saved.Rules, saved.Filters
			if done != nil {
				done()
			}
		})
	}()
}

// startNew opens the form for a new rule, filled from rule.
func (r *rulesState) startNew(rule proto.TrafficRule, o Options) {
	r.load(o)
	f := formOf(rule)
	f.index = -1
	r.edit = f
}

// ruleFromExchange is a rule matching requests like ex: its method, host
// and path; a mock answering as it was answered, or a breakpoint.
func ruleFromExchange(ex proto.TrafficExchange, res proto.TrafficBody, kind string) proto.TrafficRule {
	path := ex.Path
	if u, err := url.Parse(ex.URL); err == nil {
		path = u.Path
	}
	r := proto.TrafficRule{Enabled: true, Match: proto.TrafficMatch{Methods: []string{ex.Method}, Host: ex.Host, Path: path, PathMode: "exact"}}
	r.Action.Kind = kind
	if kind == proto.ActionMock {
		r.Action.Status = ex.Status
		if r.Action.Status == 0 {
			r.Action.Status = 200
		}
		for _, h := range ex.ResHeaders {
			if strings.EqualFold(h.Name, "Content-Type") {
				r.Action.Headers = append(r.Action.Headers, h)
			}
		}
		if utf8.Valid(res.Data) && res.Encoding == "" {
			r.Action.Body = string(res.Data)
		}
	} else {
		r.Action.Break = "both"
	}
	return r
}

func formOf(r proto.TrafficRule) *ruleForm {
	a := r.Action
	f := &ruleForm{rule: r, methods: strings.Join(r.Match.Methods, ","), host: r.Match.Host, path: r.Match.Path, body: r.Match.Body,
		pathMode: max(0, slices.Index(pathModes, r.Match.PathMode)), action: max(0, slices.Index(actionKind, a.Kind)),
		resBody: a.Body, target: a.Target, keepHost: a.KeepHost, echo: a.Echo, messages: strings.Join(a.Messages, "\n")}
	if r.Match.PathMode == "" {
		f.pathMode = 0
	}
	if a.Kind == "" {
		f.action = 5
	}
	f.status = numText(a.Status)
	f.times, f.delay, f.throttle = numText(a.Times), numText(a.DelayMs), numText(a.Throttle)
	f.headers = headerLines(a.Headers)
	f.fail = max(0, slices.Index(fails, a.Fail))
	f.stage = max(0, slices.Index(stages, a.Break))
	if t := a.Request; t != nil {
		f.reqSet, f.reqRemove = headerLines(t.SetHeaders), strings.Join(t.RemoveHeaders, ", ")
	}
	if t := a.Response; t != nil {
		f.resSet, f.resRemove, f.resStatus = headerLines(t.SetHeaders), strings.Join(t.RemoveHeaders, ", "), numText(t.Status)
		if len(t.Replace) > 0 {
			f.replaceFrom, f.replaceTo = t.Replace[0].From, t.Replace[0].To
		}
		var lines []string
		for k, val := range t.JSON {
			lines = append(lines, k+" = "+val)
		}
		slices.Sort(lines)
		f.jsonPatch = strings.Join(lines, "\n")
	}
	return f
}

func numText(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func headerLines(hs []proto.Header) string {
	var lines []string
	for _, h := range hs {
		lines = append(lines, h.Name+": "+h.Value)
	}
	return strings.Join(lines, "\n")
}

func parseHeaderLines(s string) []proto.Header {
	var out []proto.Header
	for _, line := range strings.Split(s, "\n") {
		k, v, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(k) != "" {
			out = append(out, proto.Header{Name: strings.TrimSpace(k), Value: strings.TrimSpace(v)})
		}
	}
	return out
}

func parseNum(s, what string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s is a number", what)
	}
	return n, nil
}

// build makes the rule the form says.
func (f *ruleForm) build() (proto.TrafficRule, error) {
	r := f.rule
	r.Match.Host, r.Match.Path, r.Match.Body = strings.TrimSpace(f.host), strings.TrimSpace(f.path), f.body
	r.Match.Methods = nil
	for _, m := range strings.Split(f.methods, ",") {
		if m = strings.ToUpper(strings.TrimSpace(m)); m != "" {
			r.Match.Methods = append(r.Match.Methods, m)
		}
	}
	r.Match.PathMode = pathModes[f.pathMode]
	if r.Match.PathMode == "prefix" {
		r.Match.PathMode = ""
	}
	a := proto.TrafficAction{Kind: actionKind[f.action]}
	var err error
	if a.DelayMs, err = parseNum(f.delay, "Delay"); err != nil {
		return r, err
	}
	if a.Throttle, err = parseNum(f.throttle, "Throttle"); err != nil {
		return r, err
	}
	switch a.Kind {
	case proto.ActionMock:
		if a.Status, err = parseNum(f.status, "Status"); err != nil {
			return r, err
		}
		if a.Times, err = parseNum(f.times, "Times"); err != nil {
			return r, err
		}
		a.Headers, a.Body, a.Echo = parseHeaderLines(f.headers), f.resBody, f.echo
		if strings.TrimSpace(f.messages) != "" {
			a.Messages = strings.Split(strings.TrimSpace(f.messages), "\n")
		}
	case proto.ActionForward:
		a.Target, a.KeepHost = strings.TrimSpace(f.target), f.keepHost
	case proto.ActionTransform:
		a.Request = &proto.TrafficTransform{SetHeaders: parseHeaderLines(f.reqSet), RemoveHeaders: splitList(f.reqRemove)}
		a.Response = &proto.TrafficTransform{SetHeaders: parseHeaderLines(f.resSet), RemoveHeaders: splitList(f.resRemove)}
		if a.Response.Status, err = parseNum(f.resStatus, "Status"); err != nil {
			return r, err
		}
		if f.replaceFrom != "" {
			a.Response.Replace = []proto.TrafficReplace{{From: f.replaceFrom, To: f.replaceTo}}
		}
		for _, line := range strings.Split(f.jsonPatch, "\n") {
			k, val, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(k) != "" {
				if a.Response.JSON == nil {
					a.Response.JSON = map[string]string{}
				}
				a.Response.JSON[strings.TrimSpace(k)] = strings.TrimSpace(val)
			}
		}
	case proto.ActionFail:
		a.Fail = fails[f.fail]
	case proto.ActionBreakpoint:
		a.Break = stages[f.stage]
	}
	r.Action = a
	if r.Match.Host == "" && r.Match.Path == "" && len(r.Match.Methods) == 0 && r.Match.Body == "" {
		return r, fmt.Errorf("say what it matches: a host, a path, a method or a body")
	}
	return r, nil
}

func splitList(s string) []string {
	var out []string
	for _, x := range strings.Split(s, ",") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func (v *View) rulesPage(c *ui.Context, o Options) {
	r := &v.rules
	r.load(o)
	if r.project != projectOf(o) {
		r.loaded = false
	}
	ui.Column(c).Grow(1).FillWidth().MinHeight(0).Gap(theme.S2).Children(func() {
		ui.Row(c).FillWidth().Shrink(0).Gap(theme.S2).Children(func() {
			if widget.PrimaryButton(c, icon.Plus, "New rule", "").Clicked() {
				r.edit = formOf(proto.TrafficRule{Enabled: true, Action: proto.TrafficAction{Kind: proto.ActionMock, Status: 200}})
				r.edit.index = -1
			}
			where := "This host's rules"
			if r.project != "" {
				where = "Rules of " + baseName(r.project) + " (.enowx/traffic.json)"
			}
			ui.Row(c).Grow(1).Basis(0).MinWidth(0).Children(func() {
				widget.Running(c, where, widget.RunStyle{Size: theme.TextLabel, Color: theme.TextSoft})
			})
		})
		if r.err != "" {
			ui.Text(c, r.err).FontSize(theme.TextUI).TextColor(theme.Error)
		}
		if len(r.list) == 0 && r.loaded {
			note(c, "No rules: everything passes through. Mock on a request, or New rule, answers, changes, slows or pauses requests.", theme.TextSoft)
			return
		}
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				for i := range r.list {
					v.ruleRow(c.Key(r.list[i].ID), o, i)
				}
			})
		})
	})
}

// ruleRow is a rule: picking it edits it; beside it, moving it up or deleting
// it, and turning it on or off.
func (v *View) ruleRow(c *ui.Context, o Options, i int) {
	r := &v.rules
	rule := r.list[i]
	title := intercept.Describe(rule)
	if rule.Name != "" {
		title = rule.Name + ": " + title
	}
	it := widget.Item{Title: title, Sub: fmt.Sprintf("%d hits", rule.Hits), Mono: true}
	if !rule.Enabled {
		it.TitleColor = theme.TextSoft
	}
	if rule.Problem != "" {
		it.Sub, it.SubColor = rule.Problem, theme.Error
	}
	b := widget.ItemRow(c, it, func() {
		if r.menus == nil {
			r.menus = map[string]*widget.ContextMenu{}
		}
		m := r.menus[rule.ID]
		if m == nil {
			m = &widget.ContextMenu{}
			r.menus[rule.ID] = m
		}
		widget.MoreMenu(c, m, "rule", func(done func()) {
			if i > 0 && widget.MenuItemTip(c, icon.ArrowUp, "Move up", "Rules apply first to last", false) {
				list := slices.Clone(r.list)
				list[i-1], list[i] = list[i], list[i-1]
				r.save(o, list, nil)
				done()
			}
			if widget.MenuItem(c, icon.Pencil, "Edit", false) {
				r.edit = formOf(rule)
				r.edit.index = i
				done()
			}
			if widget.MenuItem(c, icon.Trash, "Delete", true) {
				r.save(o, slices.Delete(slices.Clone(r.list), i, i+1), nil)
				done()
			}
		})
		on := rule.Enabled
		if widget.Switch(c, &on, "Rule on") {
			list := slices.Clone(r.list)
			list[i].Enabled = on
			r.save(o, list, nil)
		}
	})
	if b.Clicked() {
		r.edit = formOf(rule)
		r.edit.index = i
	}
}

// ruleSheet is the rule being written, as a dialog over the tool.
func (v *View) ruleSheet(c *ui.Context, o Options) {
	r := &v.rules
	f := r.edit
	if f == nil {
		return
	}
	field := func(label string, value *string, placeholder string) {
		widget.Field(c, label, func() { widget.Input(c, value, placeholder).FillWidth() })
	}
	title := "New rule"
	if f.index >= 0 {
		title = "Edit rule"
	}
	where := "Saved in this host's rules."
	if r.project != "" {
		where = "Saved with " + baseName(r.project) + ", in .enowx/traffic.json. Rules apply first to last; the first that matches wins."
	}
	open := true
	sheet(c, &open, 560, title, where, func() {
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			ui.Text(c, "When a request matches").FontSize(theme.TextUI).FontWeight(500)
			field("Name (optional)", &f.rule.Name, "Login fails")
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				ui.Column(c).Width(120).Children(func() { field("Methods", &f.methods, "GET,POST") })
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { field("Host", &f.host, "api.example.com or *.example.com") })
			})
			ui.Row(c).FillWidth().Gap(theme.S2).AlignItems(ui.End).Children(func() {
				ui.Column(c).Grow(1).Basis(0).MinWidth(0).Children(func() { field("Path", &f.path, "/api/login") })
				if i := widget.Segmented(c, pathModes, f.pathMode, nil); i >= 0 {
					f.pathMode = i
				}
			})
			field("Body contains (optional)", &f.body, "")
		})
		ui.Column(c).FillWidth().Gap(theme.S3).Children(func() {
			ui.Text(c, "Then").FontSize(theme.TextUI).FontWeight(500)
			ui.Row(c).FillWidth().Children(func() {
				if i := widget.Segmented(c, actions, f.action, nil); i >= 0 {
					f.action = i
				}
			})
			switch actionKind[f.action] {
			case proto.ActionMock:
				ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
					ui.Column(c).Width(100).Children(func() { field("Status", &f.status, "200") })
					ui.Column(c).Width(100).Children(func() { field("Times", &f.times, "always") })
				})
				codeArea(c, "Headers (Name: value per line)", &f.headers, 44)
				codeArea(c, "Body", &f.resBody, 110)
				codeArea(c, "WebSocket: messages to send, one per line", &f.messages, 36)
				check(c, &f.echo, "WebSocket: echo what the client sends")
			case proto.ActionForward:
				field("To the origin", &f.target, "https://staging.example.com")
				check(c, &f.keepHost, "Keep the Host header")
			case proto.ActionTransform:
				codeArea(c, "Request: set headers", &f.reqSet, 36)
				field("Request: remove headers", &f.reqRemove, "Authorization, Cookie")
				codeArea(c, "Response: set headers", &f.resSet, 36)
				field("Response: remove headers", &f.resRemove, "")
				field("Response: status", &f.resStatus, "unchanged")
				widget.FieldCols(c,
					func() { field("Response: replace", &f.replaceFrom, "") },
					func() { field("with", &f.replaceTo, "") })
				codeArea(c, "Response JSON: field = value per line", &f.jsonPatch, 36)
			case proto.ActionFail:
				ui.Row(c).FillWidth().Children(func() {
					if i := widget.Segmented(c, failLabels, f.fail, nil); i >= 0 {
						f.fail = i
					}
				})
			case proto.ActionBreakpoint:
				ui.Row(c).Gap(theme.S2).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "Pause the").FontSize(theme.TextUI).TextColor(theme.TextSoft)
					if i := widget.Segmented(c, stages, f.stage, nil); i >= 0 {
						f.stage = i
					}
				})
			}
			ui.Row(c).FillWidth().Gap(theme.S2).Children(func() {
				ui.Column(c).Width(120).Children(func() { field("Delay (ms)", &f.delay, "0") })
				ui.Column(c).Width(150).Children(func() { field("Throttle (bytes/s)", &f.throttle, "off") })
			})
			errorText(c, f.err)
			errorText(c, r.err)
		})
	}, func() {
		label := "Save rule"
		if r.saving {
			label = "Saving…"
		}
		if widget.PrimaryButton(c, nil, label, "").Disabled(r.saving).Clicked() {
			rule, err := f.build()
			if err != nil {
				f.err = err.Error()
				return
			}
			list := slices.Clone(r.list)
			if f.index >= 0 && f.index < len(list) {
				list[f.index] = rule
			} else {
				list = append([]proto.TrafficRule{rule}, list...)
			}
			r.save(o, list, func() { r.edit = nil })
		}
	})
	if !open {
		r.edit = nil
	}
}

func baseName(p string) string {
	p = strings.TrimRight(p, `/\`)
	if i := strings.LastIndexAny(p, `/\`); i >= 0 {
		return p[i+1:]
	}
	return p
}

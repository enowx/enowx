package ui

import (
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/client"
	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/textfmt"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// pushFields are a project's auto push settings as the user types them.
type pushFields struct {
	interval, branch, remote, base string
}

func pushFieldsOf(st proto.AutopushStatus) pushFields {
	return pushFields{interval: formatEvery(projectlinks.Interval(st.AutopushSettings)), branch: st.Branch, remote: st.Remote, base: st.Base}
}

// linksPush is the project's auto pushes: on or off, how, how they went.
func (a *App) linksPush(c *ui.Context, f *linksForm) {
	if !f.project.IsGit {
		widget.Notice(c, icon.Alert, theme.Muted, "Not a git repository", "Auto pushes keep a repository's work on its remote.", nil)
		return
	}
	if !f.pushRead {
		widget.Spinner(c, 14)
		return
	}
	st := &f.push
	elsewhere := st.On && st.Host != "" && st.Host != a.daemonInfo.ID
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, "Push the work on its own").FontSize(theme.TextUI).FontWeight(600).TextColor(theme.TextStrong)
			ui.Text(c, "Commits what is in the folder to a branch of its own and pushes it, never touching your branch, staged files or working tree. .env files and keys never go, and a push carrying secrets stops.").
				FontSize(11.5).TextColor(theme.Muted)
		})
		on := st.On
		if widget.Switch(c, &on, "Auto push") {
			st.On = on
			if on && st.Host == "" {
				st.Host = a.daemonInfo.ID
			}
			a.savePush(f)
		}
	})
	if st.Visibility == "public" {
		widget.Notice(c, icon.Alert, theme.Warning, "This repository is public",
			"Anyone can see the branch's work in progress, half done as it may be.", nil)
	}
	if elsewhere {
		widget.Notice(c, icon.Server, theme.Muted, "Pushed by "+st.HostName,
			"One host pushes a project, so two never push over each other.", func() {
				if widget.GhostButton(c, "Push from "+a.host.Name+" instead").Clicked() {
					st.Host = a.daemonInfo.ID
					a.savePush(f)
				}
			})
	}
	widget.Field(c, "Every", func() { widget.Input(c, &f.pushText.interval, "30m").Width(120) })
	widget.Field(c, "To the branch", func() {
		ui.Column(c).FillWidth().Gap(5).Children(func() {
			widget.Input(c, &f.pushText.branch, "enowx/autosync").FillWidth()
			ui.Text(c, "Only this branch is ever pushed: never main, master or the branch checked out.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		})
	})
	widget.Field(c, "Of the remote", func() { widget.Input(c, &f.pushText.remote, "origin").Width(160) })
	ui.Column(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
		if widget.Checkbox(c, &st.PR, "Open a pull request of it") && st.PR {
			ui.Text(c, "With gh, signed in.").FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		if st.PR {
			widget.Input(c, &f.pushText.base, "into: the repository's default branch").FillWidth()
		}
	})
	a.pushOutcome(c, f)
	ui.Row(c).FillWidth().Gap(8).Children(func() {
		busy := f.busy["push"] != ""
		b := widget.Button(c, widget.Secondary, icon.ArrowUp, "Push now", "").Disabled(busy || elsewhere)
		if b.Clicked() && !busy && !elsewhere {
			a.pushNow(f)
		}
		if busy {
			widget.Spinner(c, 13)
		}
		ui.Spacer(c)
		if widget.PrimaryButton(c, icon.Check, "Save", "").Clicked() {
			a.savePush(f)
		}
	})
}

// pushOutcome says how the last push went, with the branch's page.
func (a *App) pushOutcome(c *ui.Context, f *linksForm) {
	st := f.push
	now := time.Now()
	ui.Column(c).FillWidth().Gap(4).Children(func() {
		switch {
		case st.Err != "":
			widget.Notice(c, icon.Alert, theme.Error, "The last push failed", st.Err, nil)
		case !st.LastPush.IsZero():
			ui.Text(c, "Pushed "+textfmt.Age(st.LastPush, now)+" · "+shortCommit(st.Commit)+" · "+textfmt.Plural(st.Files, "1 file", strconv.Itoa(st.Files)+" files")).
				FontSize(12).TextColor(theme.TextSoft)
		case !st.LastRun.IsZero():
			ui.Text(c, "Looked "+textfmt.Age(st.LastRun, now)+": nothing new to push").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
		if st.On && !st.Next.IsZero() && st.Here {
			ui.Text(c, "Next "+textfmt.Until(st.Next, now)).FontSize(theme.TextLabel).TextColor(theme.Muted)
		}
		ui.Row(c).Gap(8).Children(func() {
			if st.BranchURL != "" && widget.GhostButton(c, "Open the branch").Clicked() {
				go func() { _ = mygo.Shell.OpenExternal(st.BranchURL) }()
			}
			if st.PRURL != "" && widget.GhostButton(c, "Open the pull request").Clicked() {
				go func() { _ = mygo.Shell.OpenExternal(st.PRURL) }()
			}
		})
	})
}

// savePush records the auto push settings typed, on the host shown.
func (a *App) savePush(f *linksForm) {
	cl := a.client
	if cl == nil {
		return
	}
	every, err := parseEvery(f.pushText.interval)
	if err != nil {
		f.err = err.Error()
		return
	}
	if every < projectlinks.MinPushInterval {
		f.err = "push at most every minute"
		return
	}
	s := f.push.AutopushSettings
	s.Interval, s.Branch, s.Remote, s.Base = int(every/time.Second), f.pushText.branch, f.pushText.remote, f.pushText.base
	s.Name = f.project.Name
	if s.Host == a.daemonInfo.ID || s.Host == "" {
		s.Path = f.project.Path
	}
	go func() {
		st, err := cl.SetAutopush(s)
		a.onMain(func() {
			if err != nil {
				f.err = client.Explain(err)
				f.push.On = false
				return
			}
			f.err, f.push = "", st
			a.pushChanged(st)
		})
	}()
}

// pushNow pushes the project now, from the host shown.
func (a *App) pushNow(f *linksForm) {
	cl := a.client
	if cl == nil {
		return
	}
	f.busy["push"] = "pushing"
	dir := f.project.Path
	go func() {
		st, err := cl.AutopushNow(dir)
		a.onMain(func() {
			delete(f.busy, "push")
			if err != nil && st.Project == "" {
				f.err = client.Explain(err)
				return
			}
			f.push = st
			if err != nil {
				f.push.Err = client.Explain(err)
			}
		})
	}()
}

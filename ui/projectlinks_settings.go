package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/core/projectlinks"
	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// linkFields are a link's settings as the user types them.
type linkFields struct {
	interval, include, exclude, bandwidth, largeMB string
}

func linkFieldsOf(l proto.ProjectLink) linkFields {
	f := linkFields{include: strings.Join(l.Include, ", "), exclude: strings.Join(l.Exclude, ", ")}
	every := defaultLinkInterval
	if l.Interval > 0 {
		every = time.Duration(l.Interval) * time.Second
	}
	f.interval = formatEvery(every)
	if l.Bandwidth > 0 {
		f.bandwidth = strconv.Itoa(l.Bandwidth)
	}
	if l.LargeMB > 0 {
		f.largeMB = strconv.Itoa(l.LargeMB)
	}
	return f
}

// linkSettings edits the settings of the link f.editing.
func (a *App) linkSettings(c *ui.Context, f *linksForm) {
	d, src, dst := &f.draft, f.draft.Source.Host, f.draft.Target.Host
	ui.Column(c).FillWidth().Gap(14).Padding(12, 0, 0, 0).BorderWidth(1, 0, 0, 0).BorderColor(theme.Border).Children(func() {
		widget.Field(c, "Direction", func() {
			dirs := []string{proto.SyncOneWay, proto.SyncTwoWay, proto.SyncReverse}
			cur := 1
			for i, x := range dirs {
				if x == d.Direction {
					cur = i
				}
			}
			if i := widget.Segmented(c, []string{src + " → " + dst, "Both ways", dst + " → " + src}, cur, nil); i >= 0 {
				d.Direction = dirs[i]
			}
		})
		widget.Field(c, "When", func() {
			ui.Column(c).FillWidth().Gap(6).Children(func() {
				cur := 0
				if d.Realtime {
					cur = 1
				}
				if i := widget.Segmented(c, []string{"At an interval", "As files change"}, cur, nil); i >= 0 {
					d.Realtime = i == 1
				}
				if !d.Realtime {
					widget.Input(c, &f.fields.interval, "5m: 30s, 10m, 1h").FillWidth()
				}
			})
		})
		widget.Field(c, "Files", func() {
			ui.Column(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
				widget.Input(c, &f.fields.include, "Only these (empty: all), such as src, docs/*.md").FillWidth()
				widget.Input(c, &f.fields.exclude, "Never these, such as *.log, build, data/**").FillWidth()
				follow := !d.All
				if widget.Checkbox(c, &follow, "Follow .gitignore") {
					d.All = !follow
				}
				ui.Text(c, "node_modules, target, dist, .venv and other dependencies' folders never go.").FontSize(theme.TextLabel).TextColor(theme.Muted)
				widget.Checkbox(c, &d.Env, "Include .env files")
			})
		})
		widget.Field(c, "Deleted files", func() {
			cur := 0
			if d.Deletes == proto.DeletesNever {
				cur = 1
			}
			if i := widget.Segmented(c, []string{"Deleted on the other side too", "Never deleted there"}, cur, nil); i >= 0 {
				d.Deletes = []string{proto.DeletesMirror, proto.DeletesNever}[i]
			}
		})
		widget.Field(c, "Large and binary files", func() {
			ui.Row(c).FillWidth().Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Large from").FontSize(theme.TextLabel).TextColor(theme.Muted)
				widget.Input(c, &f.fields.largeMB, fmt.Sprint(projectlinks.DefaultLargeMB)).Width(64)
				ui.Text(c, "MB:").FontSize(theme.TextLabel).TextColor(theme.Muted)
				cur := 0
				if d.Large == proto.LargeSource {
					cur = 1
				}
				if i := widget.Segmented(c, []string{"Skip", "Always the source's"}, cur, nil); i >= 0 {
					d.Large = []string{proto.LargeSkip, proto.LargeSource}[i]
				}
			})
		})
		widget.Field(c, "Pace", func() {
			ui.Column(c).FillWidth().Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
					widget.Input(c, &f.fields.bandwidth, "no cap").Width(90)
					ui.Text(c, "KB/s at most").FontSize(theme.TextLabel).TextColor(theme.Muted)
				})
				widget.Checkbox(c, &d.PauseBusy, "Wait while a host is busy (CPU over 85%)")
				widget.Checkbox(c, &d.PauseBattery, "Wait while this computer runs on battery")
			})
		})
		ui.Row(c).FillWidth().Gap(8).Children(func() {
			ui.Spacer(c)
			if widget.GhostButton(c, "Cancel").Clicked() {
				f.editing = ""
			}
			if widget.PrimaryButton(c, icon.Check, "Save", "").Clicked() {
				if err := f.fields.apply(d); err != nil {
					f.err = err.Error()
					return
				}
				a.saveLink(f, *d)
				f.editing = ""
			}
		})
	})
}

// apply sets l's settings from the fields typed.
func (lf linkFields) apply(l *proto.ProjectLink) error {
	if !l.Realtime {
		every, err := parseEvery(lf.interval)
		if err != nil {
			return err
		}
		if every < 5*time.Second {
			return fmt.Errorf("sync at most every 5 seconds, or as files change")
		}
		l.Interval = int(every / time.Second)
	}
	l.Include, l.Exclude = splitList(lf.include), splitList(lf.exclude)
	var err error
	if l.Bandwidth, err = optionalInt(lf.bandwidth, "the bandwidth"); err != nil {
		return err
	}
	l.LargeMB, err = optionalInt(lf.largeMB, "the size of large files")
	return err
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == '\n' }) {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func optionalInt(s, what string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s is a number: %q", what, s)
	}
	return n, nil
}

// parseEvery reads an interval: "90s", "5m", "1h", "1h30m", or seconds.
func parseEvery(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("an interval such as 30s, 5m or 1h, not %q", s)
	}
	return d, nil
}

// formatEvery spells an interval as parseEvery reads it.
func formatEvery(d time.Duration) string {
	switch {
	case d%time.Hour == 0:
		return fmt.Sprintf("%dh", d/time.Hour)
	case d%time.Minute == 0:
		return fmt.Sprintf("%dm", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}

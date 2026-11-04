package canvasview

import (
	"fmt"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Sharing a design to the community library: a design's menu opens the form, on
// a draft the host has checked and packed. The form says what goes public and
// what stays, and sending waits for the prompt's approval and the rights
// statement. Nothing shows when the host's cloud has no library.

// libraryCategories are the choices a form has: value, then how it reads.
var libraryCategories = [][2]string{{"component", "Component"}, {"dashboard", "Dashboard"}, {"landing-page", "Landing page"}, {"mobile", "Mobile"},
	{"onboarding", "Onboarding"}, {"motion", "Motion"}, {"diagram", "Diagram"}, {"icon-illustration", "Icon and illustration"}, {"other", "Other"}}

// libraryState is what the view holds of the library: whether it's there, and
// the dialog open.
type libraryState struct {
	asked  bool
	state  proto.LibraryState
	form   *libraryForm
	status *libraryStatus
	signIn bool
	menu   widget.ContextMenu
}

// libraryForm is the publish form.
type libraryForm struct {
	open    bool
	busy    bool
	saved   bool
	err     string
	canvas  string
	frame   string
	noPhone bool
	prep    *proto.LibraryPrepared
	f       proto.LibraryForm
	tag     string
	accept  bool
}

// libraryStatus is the dialog saying how a shared design stands.
type libraryStatus struct {
	open   bool
	busy   bool
	err    string
	design proto.LibraryDesign
}

// loadLibrary asks once, as the page opens, whether there's a library.
func (v *View) loadLibrary(o Options) {
	if v.library.asked || o.LibraryStatus == nil {
		return
	}
	v.library.asked = true
	v.reloadLibrary(o)
}

func (v *View) reloadLibrary(o Options) {
	if o.LibraryStatus == nil {
		return
	}
	o.LibraryStatus(func(st proto.LibraryState, err error) {
		if err == nil {
			v.library.state = st
		}
	})
}

// designOf is how design frame of canvas stands in the library, if it does.
func (v *View) designOf(canvas, frame string) (proto.LibraryDesign, bool) {
	list := v.library.state.Designs
	i := slices.IndexFunc(list, func(d proto.LibraryDesign) bool { return d.Canvas == canvas && d.Frame == frame && d.ItemID != "" })
	if i < 0 {
		return proto.LibraryDesign{}, false
	}
	return list[i], true
}

// libraryItem is a design's entry in its menu: the form, how it stands once
// shared, or signing in first.
func (v *View) libraryItem(c *ui.Context, o Options, canvas string, f proto.CanvasFrame, done func()) {
	if !v.library.state.On || o.LibraryPrepare == nil {
		return
	}
	widget.MenuSeparator(c)
	d, shared := v.designOf(canvas, f.ID)
	label := "Share to Community Library"
	if shared {
		label = "In the Community Library…"
	}
	if widget.MenuItem(c.Key("library"), icon.Globe, label, false) {
		switch {
		case !v.library.state.SignedIn:
			v.library.signIn = true
		case shared:
			v.library.status = &libraryStatus{open: true, design: d}
		default:
			v.openLibraryForm(o, canvas, f.ID)
		}
		done()
	}
}

// openLibraryForm opens the form while the host checks and packs the design.
func (v *View) openLibraryForm(o Options, canvas, frame string) {
	form := &libraryForm{open: true, busy: true, canvas: canvas, frame: frame}
	v.library.form = form
	v.prepareLibrary(o, form)
}

func (v *View) prepareLibrary(o Options, form *libraryForm) {
	form.busy, form.err = true, ""
	o.LibraryPrepare(proto.LibraryPrepareParams{Canvas: form.canvas, Frame: form.frame, NoPhone: form.noPhone}, func(p proto.LibraryPrepared, err error) {
		form.busy = false
		if err != nil {
			form.err = err.Error()
			return
		}
		form.prep = &p
		if form.f.Title == "" {
			form.f = p.Form
		}
	})
}

func libraryCategoryName(value string) string {
	for _, c := range libraryCategories {
		if c[0] == value {
			return c[1]
		}
	}
	return "Other"
}

func (v *View) libraryDialogs(c *ui.Context, o Options) {
	v.librarySignIn(c, o)
	v.libraryFormDialog(c, o)
	v.libraryStatusDialog(c, o)
}

func (v *View) librarySignIn(c *ui.Context, o Options) {
	if !v.library.signIn {
		return
	}
	widget.FormSheet(c, &v.library.signIn, 440, "Sign in to share", "The library uses your GitHub account through enowX Cloud, the same sign-in as your team. No new password.", func() {}, func() {
		if widget.Button(c, widget.Primary, nil, "Sign in", "").Clicked() {
			v.library.signIn = false
			if o.CloudSignIn != nil {
				o.CloudSignIn()
			}
		}
	})
}

// libraryFormDialog is the form: what's said of the design on the left, what
// becomes public and what the check found on the right, in columns of a width
// the dialog gives and one over the other when it's narrow.
func (v *View) libraryFormDialog(c *ui.Context, o Options) {
	form := v.library.form
	if form == nil {
		return
	}
	if !form.open {
		v.library.form = nil
		return
	}
	w, _ := c.Size()
	widget.FormSheet(c, &form.open, 920, "Share to Community Library", "This design only: "+v.libraryFormTitle(form), func() {
		if form.prep == nil {
			if form.err != "" {
				widget.Banner(c, theme.Error, form.err, nil)
			} else {
				ui.Text(c, "Checking the design and making its picture…").FontSize(theme.TextUI).TextColor(theme.TextSoft)
			}
			return
		}
		if form.err != "" {
			widget.Banner(c, theme.Error, form.err, nil)
		}
		said := func() { v.libraryFields(c.Key("said"), o, form) }
		shown := func() { v.librarySide(c.Key("shown"), o, form) }
		if w < 760 { // one over the other: each as tall as what it holds
			ui.Column(c).FillWidth().Gap(theme.S4).Children(func() {
				ui.Column(c.Key("said")).FillWidth().MinWidth(0).Gap(theme.S3).Children(said)
				ui.Column(c.Key("shown")).FillWidth().MinWidth(0).Gap(theme.S3).Children(shown)
			})
			return
		}
		ui.Row(c).FillWidth().Gap(theme.S5).AlignItems(ui.Start).Children(func() {
			ui.Column(c.Key("said")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S3).Children(said)
			ui.Column(c.Key("shown")).Grow(1).Basis(0).MinWidth(0).Gap(theme.S3).Children(shown)
		})
	}, func() { v.libraryButtons(c, o, form) })
}

func (v *View) libraryFormTitle(form *libraryForm) string {
	if form.prep == nil {
		return "being prepared"
	}
	return fmt.Sprintf("%s, %d×%d", form.prep.FrameName, form.prep.Width, form.prep.Height)
}

func (v *View) libraryFields(c *ui.Context, o Options, form *libraryForm) {
	f := &form.f
	widget.Field(c, "Type of item", func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			if widget.Pick(c, nil, "Full page", "Prompt, reference or skill. No code copy", f.Kind == "page", nil).Clicked() {
				f.Kind = "page"
			}
			if widget.Pick(c, nil, "Component", "A button, a card. HTML and CSS can be copied", f.Kind == "component", nil).Clicked() {
				f.Kind = "component"
			}
		})
	})
	widget.Field(c, "Title", func() { widget.Input(c, &f.Title, "A name for the design").Label("Design title") })
	widget.Field(c, "Description", func() {
		widget.Well(ui.TextArea(c, &f.Description).Label("Design description").FillWidth().TextColor(theme.Text).Height(72).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).FontSize(theme.TextUI), false)
	})
	widget.Field(c, "Category", func() {
		b := widget.MenuButton(c, widget.Secondary, libraryCategoryName(f.Category)).FillWidth()
		m := &v.library.menu
		m.OpenBelow(b, "library-category", theme.ControlHeight)
		m.Show(c, b, "library-category", func(done func()) {
			for _, cat := range libraryCategories {
				if widget.MenuItem(c.Key("cat:"+cat[0]), nil, cat[1], false) {
					f.Category = cat[0]
					done()
				}
			}
		})
	})
	widget.Field(c, "Tags", func() {
		if len(f.Tags) > 0 {
			ui.Row(c).FillWidth().Wrap().Gap(theme.S1).Children(func() {
				for _, tag := range f.Tags {
					b := ui.ButtonBase(c.Key("tag:" + tag)).Height(24).PaddingX(theme.S2).Radius(theme.RadiusFull).Background(theme.Raised).Gap(theme.S1).Label("Remove " + tag).Cursor(ui.CursorPointer).Shrink(0)
					b.Children(func() {
						ui.Text(c, tag).FontSize(theme.TextLabel).TextColor(theme.Text).SingleLine()
						ui.Icon(c, icon.Close).FontSize(10)
					})
					if b.Clicked() {
						f.Tags = slices.DeleteFunc(slices.Clone(f.Tags), func(t string) bool { return t == tag })
					}
				}
			})
		}
		if len(f.Tags) < 8 && widget.Input(c, &form.tag, "Add a tag, then Enter").Label("Add a tag").Submitted() {
			if t := strings.ToLower(strings.TrimSpace(form.tag)); t != "" && !slices.Contains(f.Tags, t) {
				f.Tags = append(f.Tags, t)
			}
			form.tag = ""
		}
	})
	widget.Field(c, "Source link (optional)", func() { widget.Input(c, &f.SourceURL, "https://github.com/you/design").Label("Source link") })
	widget.Field(c, "License", func() {
		ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
			for _, l := range [][3]string{{"mit", "MIT", "Use and change it, keep the notice"}, {"cc-by-4.0", "CC BY 4.0", "Use and change it, credit the creator. Default"}, {"view-only", "View only", "People can look. No prompt, code or copy"}} {
				if widget.Pick(c, nil, l[1], l[2], f.License == l[0], nil).Clicked() {
					f.License = l[0]
				}
			}
		})
	})
}

func (v *View) librarySide(c *ui.Context, o Options, form *libraryForm) {
	f, p := &form.f, form.prep
	if f.License != "view-only" {
		widget.Field(c, "Prompt others can copy", func() {
			widget.Well(ui.TextArea(c.Key("prompt"), &f.Prompt).Label("Prompt others can copy").FillWidth().TextColor(theme.Text).Height(132).Padding(theme.S2, theme.S3).Radius(theme.RadiusCard).FontSize(theme.TextUI), false)
			ui.Text(c, "Written from the design alone. Your conversations with agents are never included.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		})
	} else {
		f.PromptApproved = true
	}
	if f.License != "view-only" {
		widget.Checkbox(c.Key("approve"), &f.PromptApproved, "I read this prompt and approve it")
	}
	widget.Checkbox(c.Key("rights"), &f.Rights, "I have the right to share this design")
	ui.Text(c, "You made it or may share it, and it copies no one else's work.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	if p.PhoneOffered {
		on := !form.noPhone
		if widget.Checkbox(c.Key("phone"), &on, "Also show the phone design of this page") {
			form.noPhone = !on
			v.prepareLibrary(o, form)
		}
	}
	list := func(title string, lines []string) {
		widget.Field(c.Key(title), title, func() {
			ui.Column(c).FillWidth().Gap(theme.S1).Children(func() {
				for i, line := range lines {
					widget.Running(c.Key(fmt.Sprint(title, i)), line, widget.RunStyle{Size: theme.TextLabel})
				}
			})
		})
	}
	public := []string{fmt.Sprintf("This design only: %s, %d×%d", p.FrameName, p.Width, p.Height)}
	if p.Phone != "" {
		public = append(public, fmt.Sprintf("The phone design: %s, %d×%d", p.PhoneName, p.PhoneWidth, p.PhoneHeight))
	}
	public = append(public, fmt.Sprintf("HTML, CSS and %d %s in it", p.Files, plural(p.Files, "file", "files")), "Title, description, tags, license, prompt", "Your GitHub name and picture")
	if p.MadeWith == "agent" {
		public = append(public, "A \"made with an agent\" label")
	}
	if p.SiteURL != "" {
		public = append(public, strings.TrimSuffix(p.SiteURL, "/")+"/d/<id>/…")
	}
	list("Becomes public", public)
	list("Stays private", []string{fmt.Sprintf("The other %d %s on this canvas", p.OtherDesigns, plural(p.OtherDesigns, "design", "designs")), "Comments, versions and the brief", "Your project, sessions and conversations"})
	ui.Text(c, "Anyone who can open a preview can read its code in their browser, view only or not.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	v.libraryCheck(c, form)
	if p.Trust == "new" {
		ui.Text(c, "New account: an admin reviews this first. After your first approval, designs go live directly.").FontSize(theme.TextLabel).TextColor(theme.TextSoft)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// libraryCheck says what the check found: where and of what kind, no value.
func (v *View) libraryCheck(c *ui.Context, form *libraryForm) {
	refused, warned := libraryCounts(form.prep.Findings)
	switch {
	case refused > 0:
		widget.Banner(c, theme.Error, fmt.Sprintf("Secret check: %d %s to fix first", refused, plural(refused, "thing", "things")), nil)
	case warned > 0:
		widget.Banner(c, theme.Warning, fmt.Sprintf("Secret check: %d %s to look at", warned, plural(warned, "thing", "things")), nil)
	default:
		widget.Banner(c, theme.Done, "Secret check: nothing found", nil)
	}
	for i, f := range form.prep.Findings {
		ui.Column(c.Key(fmt.Sprint("found", i))).FillWidth().MinWidth(0).Children(func() {
			widget.Running(c, f.Where+": "+f.Kind, widget.RunStyle{Size: theme.TextLabel, Color: libraryFindingColor(f)})
		})
	}
	if refused == 0 && warned > 0 {
		widget.Checkbox(c.Key("accept"), &form.accept, "I looked at these, and they can be public")
	}
}

func libraryFindingColor(f proto.LibraryFinding) ui.Color {
	if f.Severity == "refuse" {
		return theme.Error
	}
	return theme.Warning
}

func libraryCounts(found []proto.LibraryFinding) (refused, warned int) {
	for _, f := range found {
		if f.Severity == "refuse" {
			refused++
		} else {
			warned++
		}
	}
	return refused, warned
}

func (v *View) libraryButtons(c *ui.Context, o Options, form *libraryForm) {
	if form.prep == nil {
		return
	}
	id := form.prep.DraftID
	if widget.GhostButton(c, "Save draft").Clicked() && !form.busy {
		form.busy, form.err = true, ""
		o.LibrarySaveDraft(proto.LibraryDraftParams{DraftID: id, Form: form.f}, func(err error) {
			form.busy = false
			if err != nil {
				form.err = err.Error()
				return
			}
			form.open = false
			v.reloadLibrary(o)
		})
	}
	refused, warned := libraryCounts(form.prep.Findings)
	label := "Submit for review"
	if form.prep.Trust == "trusted" {
		label = "Publish"
	}
	if refused > 0 {
		label = fmt.Sprintf("Fix %d %s to submit", refused, plural(refused, "thing", "things"))
	}
	ready := refused == 0 && !form.busy && form.f.Rights && form.f.PromptApproved && strings.TrimSpace(form.f.Title) != "" && (warned == 0 || form.accept)
	b := widget.Button(c, widget.Primary, nil, label, "").Disabled(!ready)
	if ready && b.Clicked() {
		form.busy, form.err = true, ""
		o.LibraryPublish(proto.LibraryPublishParams{DraftID: id, Form: form.f, AcceptWarnings: form.accept}, func(d proto.LibraryDesign, err error) {
			form.busy = false
			if err != nil {
				form.err = err.Error()
				if len(d.Findings) > 0 {
					form.prep.Findings = d.Findings
				}
				return
			}
			form.open = false
			v.library.status = &libraryStatus{open: true, design: d}
			v.reloadLibrary(o)
		})
	}
}

// libraryStatusDialog says how a shared design stands, and what can be done.
func (v *View) libraryStatusDialog(c *ui.Context, o Options) {
	st := v.library.status
	if st == nil {
		return
	}
	if !st.open {
		v.library.status = nil
		return
	}
	d := st.design
	widget.DetailSheet(c, &st.open, 480, d.Title, libraryStatusName(d.Status), func() {
		if st.err != "" {
			widget.Banner(c, theme.Error, st.err, nil)
		}
		ui.Text(c, libraryStatusSay(d.Status)).FontSize(theme.TextUI).TextColor(theme.TextSoft)
		if d.Status == "rejected" && d.RejectReason != "" {
			widget.Banner(c, theme.Error, "The reason from the admin: "+d.RejectReason, nil)
		}
		if d.URL != "" && d.Status == "live" {
			ui.Column(c).FillWidth().MinWidth(0).Children(func() {
				widget.Running(c, d.URL, widget.RunStyle{Size: theme.TextLabel, Mono: true})
			})
		}
		if d.Version > 0 {
			ui.Text(c, fmt.Sprintf("Version %d", d.Version)).FontSize(theme.TextLabel).TextColor(theme.TextSoft)
		}
	}, func() {
		if d.URL != "" && d.Status == "live" && o.OpenURL != nil && widget.GhostButton(c, "Open").Clicked() {
			o.OpenURL(d.URL)
		}
		if d.ItemID != "" && (d.Status == "live" || d.Status == "review") && o.LibraryWithdraw != nil {
			word := "Withdraw"
			if d.Status == "live" {
				word = "Unpublish"
			}
			if widget.Button(c, widget.Danger, nil, word, "").Clicked() {
				o.Confirm(word+" “"+d.Title+"”?", "Its address stops working for everyone, and it's deleted from the library after 30 days. Copies people already made stay theirs, with your credit.", word, func() {
					st.busy = true
					o.LibraryWithdraw(d.ItemID, func(err error) {
						st.busy = false
						if err != nil {
							st.err = err.Error()
							return
						}
						st.open = false
						v.reloadLibrary(o)
					})
				})
			}
		}
		if d.Canvas != "" && d.Frame != "" && d.Status != "unpublished" && d.Status != "removed" && d.Status != "withdrawn" {
			word := "Publish an update"
			if d.Status == "rejected" {
				word = "Edit and submit again"
			}
			if widget.Button(c, widget.Primary, nil, word, "").Clicked() {
				st.open = false
				v.openLibraryForm(o, d.Canvas, d.Frame)
			}
		}
	})
}

func libraryStatusName(status string) string {
	switch status {
	case proto.LibraryStatusDraft:
		return "Draft"
	case "review":
		return "In review"
	case "live":
		return "Live"
	case "rejected":
		return "Rejected"
	case "removed":
		return "Taken down"
	case "unpublished", "withdrawn":
		return "Unpublished"
	}
	return status
}

func libraryStatusSay(status string) string {
	switch status {
	case proto.LibraryStatusDraft:
		return "Saved here, not on the library. Finish the rights statement, then submit it."
	case "review":
		return "An admin reviews a new account's first designs. You'll see the result here, and you can withdraw it until then."
	case "live":
		return "Anyone can see it, copy its prompt if its license lets them, and report it."
	case "rejected":
		return "It isn't public. Change what the reason says, then submit it again."
	case "removed":
		return "A moderator took it down. Its address says so."
	}
	return "It isn't public any more. Copies people already made stay theirs, with your credit."
}

package canvasview

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

func libraryOptions(published *proto.LibraryPublishParams) Options {
	return Options{
		Confirm: func(title, msg, action string, do func()) { do() },
		LibraryStatus: func(done func(proto.LibraryState, error)) {
			done(proto.LibraryState{On: true, SignedIn: true}, nil)
		},
		LibraryPrepare: func(p proto.LibraryPrepareParams, done func(proto.LibraryPrepared, error)) {
			done(proto.LibraryPrepared{DraftID: "0123456789abcdef", Canvas: p.Canvas, Frame: p.Frame, FrameName: "Landing", Width: 1440, Height: 900, Files: 1, OtherDesigns: 2,
				Form: proto.LibraryForm{Kind: "page", License: "cc-by-4.0", Title: "Landing", Category: "other", Prompt: "Build a landing page"}, MadeWith: "agent", Trust: "new", SiteURL: "https://ui.example.com"}, nil)
		},
		LibrarySaveDraft: func(p proto.LibraryDraftParams, done func(error)) { done(nil) },
		LibraryPublish: func(p proto.LibraryPublishParams, done func(proto.LibraryDesign, error)) {
			*published = p
			done(proto.LibraryDesign{ItemID: "item", Title: p.Form.Title, Status: "review", Version: 1}, nil)
		},
		LibraryWithdraw: func(id string, done func(error)) { done(nil) },
	}
}

func libraryTester(t *testing.T, o Options, width int) (*View, *ui.Tester) {
	t.Helper()
	v := &View{}
	return v, ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		ui.Column(c).Grow(1).FillWidth().TextColor(theme.Text).Children(func() {
			v.loadLibrary(o)
			v.libraryDialogs(c, o)
		})
	}, width, 900)
}

func shows(tt *ui.Tester, text string) bool {
	return strings.Contains(strings.Join(tt.Texts(), "\n"), text)
}

// The form says what goes public and what stays, and sending waits for the
// prompt's approval and the rights statement.
func TestLibraryFormWaitsForApprovalAndRights(t *testing.T) {
	var published proto.LibraryPublishParams
	o := libraryOptions(&published)
	v, tt := libraryTester(t, o, 1440)
	tt.Frame()
	v.openLibraryForm(o, "c", "f")
	tt.Frame()
	tt.Frame()
	for _, want := range []string{"Share to Community Library", "Becomes public", "Stays private", "The other 2 designs on this canvas", "Comments, versions and the brief",
		"Your project, sessions and conversations", "A \"made with an agent\" label", "Secret check: nothing found", "New account: an admin reviews this first"} {
		if !shows(tt, want) {
			t.Errorf("no %q in %q", want, tt.Texts())
		}
	}
	shot(t, tt, "library-form.png")
	if err := tt.Click("Other"); err != nil {
		t.Fatalf("no category menu: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Dashboard"); err != nil {
		t.Fatalf("no category to pick: %v in %q", err, tt.Texts())
	}
	tt.Frame()
	if f := v.library.form.f.Category; f != "dashboard" {
		t.Fatalf("category: %q", f)
	}
	// Not approved: the button does nothing.
	tt.Click("Submit for review")
	tt.Frame()
	if published.DraftID != "" {
		t.Fatal("sent before its prompt was approved")
	}
	form := v.library.form
	form.f.PromptApproved, form.f.Rights = true, true
	tt.Frame()
	if err := tt.Click("Submit for review"); err != nil {
		t.Fatalf("no button: %q", tt.Texts())
	}
	tt.Frame()
	tt.Frame()
	if published.DraftID == "" || !published.Form.Rights {
		t.Fatalf("not sent: %+v", published)
	}
	if !shows(tt, "In review") {
		t.Fatalf("no status: %q", tt.Texts())
	}
}

// What the check refuses stops the button, and no value shows.
func TestLibraryFormStopsAtWhatTheCheckRefuses(t *testing.T) {
	var published proto.LibraryPublishParams
	o := libraryOptions(&published)
	v, tt := libraryTester(t, o, 390)
	tt.Frame()
	v.library.form = &libraryForm{open: true, canvas: "c", frame: "f", prep: &proto.LibraryPrepared{DraftID: "0123456789abcdef", FrameName: "Landing", Width: 1, Height: 1,
		Findings: []proto.LibraryFinding{{Severity: "refuse", Kind: "a GitHub token", Where: "the design, line 12"}}},
		f: proto.LibraryForm{Kind: "page", License: "mit", Title: "x", PromptApproved: true, Rights: true}}
	tt.Frame()
	if !shows(tt, "Fix 1 thing to submit") || !shows(tt, "the design, line 12: a GitHub token") {
		t.Fatalf("texts: %q", tt.Texts())
	}
	shot(t, tt, "library-form-phone.png")
	for _, want := range []string{"Title", "License"} {
		box, ok := tt.Find(want)
		if !ok || box.W <= 0 || box.H <= 0 {
			t.Errorf("%s doesn't show at phone width: %+v %q", want, box, tt.Texts())
		}
		if box.X < 0 || box.X+box.W > 390 {
			t.Errorf("%s sticks out at phone width: %+v", want, box)
		}
	}
	tt.Click("Fix 1 thing to submit")
	tt.Frame()
	if published.DraftID != "" {
		t.Fatal("sent with something refused")
	}
}

// Unpublishing asks first, saying what happens to the copies.
func TestLibraryUnpublishConfirms(t *testing.T) {
	var published proto.LibraryPublishParams
	o := libraryOptions(&published)
	asked := ""
	withdrawn := false
	o.Confirm = func(title, msg, action string, do func()) { asked = msg }
	o.LibraryWithdraw = func(id string, done func(error)) { withdrawn = true; done(nil) }
	v, tt := libraryTester(t, o, 1440)
	tt.Frame()
	v.library.status = &libraryStatus{open: true, design: proto.LibraryDesign{ItemID: "i", Title: "Mine", Status: "live", URL: "https://ui.example.com/d/i/mine", Version: 1}}
	tt.Frame()
	if err := tt.Click("Unpublish"); err != nil {
		t.Fatalf("no button: %q", tt.Texts())
	}
	tt.Frame()
	if withdrawn {
		t.Fatal("unpublished without asking")
	}
	if !strings.Contains(asked, "Copies people already made stay theirs") {
		t.Fatalf("asked: %q", asked)
	}
}

// Without a library on the cloud the view doesn't offer one.
func TestLibraryMenuOnlyWhenTheCloudHasIt(t *testing.T) {
	o := Options{LibraryPrepare: func(proto.LibraryPrepareParams, func(proto.LibraryPrepared, error)) {}}
	v := &View{}
	tt := ui.NewTester(func(c *ui.Context) {
		v.libraryItem(c, o, "c", proto.CanvasFrame{ID: "f"}, func() {})
	}, 400, 400)
	tt.Frame()
	if shows(tt, "Share to Community Library") {
		t.Fatal("offered with no library")
	}
	v.library.state = proto.LibraryState{On: true, SignedIn: true}
	tt.Frame()
	if !shows(tt, "Share to Community Library") {
		t.Fatalf("not offered: %q", tt.Texts())
	}
}

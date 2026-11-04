package canvasview

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
)

// The list leads to the community library only when the cloud has one.
func TestCommunityEntryOnlyWhenTheCloudHasIt(t *testing.T) {
	opened := false
	o := Options{OpenCommunity: func() { opened = true }}
	v := &View{}
	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(theme.Window())
		v.communityEntry(c, o)
	}, 400, 400)
	tt.Frame()
	if shows(tt, "Community library") {
		t.Fatal("offered with no library")
	}
	v.library.state = proto.LibraryState{On: true}
	tt.Frame()
	if err := tt.Click("Community library"); err != nil {
		t.Fatalf("not offered: %q", tt.Texts())
	}
	tt.Frame()
	if !opened {
		t.Fatal("it doesn't lead there")
	}
}

// What an agent asked for opens the form and nothing else: nothing is sent, and
// sending still waits for the user's approval and rights statement.
func TestAnAgentsAskingOnlyOpensTheForm(t *testing.T) {
	var published proto.LibraryPublishParams
	o := libraryOptions(&published)
	v, tt := libraryTester(t, o, 1440)
	tt.Frame()
	v.ShareDesign(o, "c", "f")
	tt.Frame()
	tt.Frame()
	if !shows(tt, "Share to Community Library") || !shows(tt, "Becomes public") {
		t.Fatalf("no form: %q", tt.Texts())
	}
	if f := v.library.form.f; f.PromptApproved || f.Rights {
		t.Fatalf("approved for the user: %+v", f)
	}
	tt.Click("Submit for review")
	tt.Frame()
	if published.DraftID != "" {
		t.Fatal("sent without the user's word")
	}
}

// Signed out, it asks to sign in rather than opening a form.
func TestAnAgentsAskingSignedOut(t *testing.T) {
	var published proto.LibraryPublishParams
	o := libraryOptions(&published)
	o.LibraryStatus = func(done func(proto.LibraryState, error)) { done(proto.LibraryState{On: true}, nil) }
	v, tt := libraryTester(t, o, 1440)
	tt.Frame()
	v.ShareDesign(o, "c", "f")
	tt.Frame()
	tt.Frame()
	if !shows(tt, "Sign in to share") || v.library.form != nil {
		t.Fatalf("signed out: %q", tt.Texts())
	}
}

package canvasview

import (
	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/widget"
)

// communityEntry leads from the list to the community library, when the host's
// cloud has one. Without one nothing shows.
func (v *View) communityEntry(c *ui.Context, o Options) {
	v.loadLibrary(o)
	if !v.library.state.On || o.OpenCommunity == nil {
		return
	}
	if widget.SideEntry(c.Key("community"), icon.Globe, "Community library", false).Clicked() {
		o.OpenCommunity()
	}
}

// ShareDesign does what a design's menu does, for an agent's or a command's
// asking that the user followed: the form when it can be shared, how it stands
// when it already is, signing in when they aren't. The form is all it opens, with
// nothing filled in by whoever asked.
func (v *View) ShareDesign(o Options, canvas, frame string) {
	if o.LibraryStatus == nil {
		return
	}
	o.LibraryStatus(func(st proto.LibraryState, err error) {
		if err != nil || !st.On {
			return
		}
		v.library.state = st
		d, shared := v.designOf(canvas, frame)
		switch {
		case !st.SignedIn:
			v.library.signIn = true
		case shared:
			v.library.status = &libraryStatus{open: true, design: d}
		default:
			v.openLibraryForm(o, canvas, frame)
		}
	})
}

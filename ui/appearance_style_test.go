package ui

import (
	"testing"

	"github.com/enowx/enowx/ui/theme"
)

// A background image and its colors wait in a style with no glass, and are back on
// returning to Liquid Glass; a style a later version doesn't have loads the default.
func TestBackgroundWaitsInFlatStyles(t *testing.T) {
	st := appState{BackgroundImage: "/some/image.png", FollowBackground: true, derived: &theme.Derived{Accent: theme.Default.Accent}}
	if !st.followsImage() {
		t.Fatal("Liquid Glass doesn't follow the image")
	}
	for _, s := range theme.Styles {
		st.UIStyle = s.ID
		if s.Flat == st.followsImage() {
			t.Errorf("%s: follows the image %v", s.ID, st.followsImage())
		}
	}
	st.UIStyle = "gone"
	if st.flatStyle() {
		t.Fatal("an unknown style isn't Liquid Glass")
	}
}

package ui

import (
	"encoding/json"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// A team deleted by its owner leaves the rail as the host drops its space;
// a card in the window's corner says so, until the user saw it, as the
// host keeps saying it (proto.SpaceGone).

// hostSpacesGone is the connected host's notices of teams deleted. It
// belongs to the main thread.
type hostSpacesGone struct {
	host string
	list []proto.SpaceGone
}

func (a *App) spacesGoneOf() *hostSpacesGone {
	if a.goneList == nil || a.goneList.host != a.host.ID {
		a.goneList = &hostSpacesGone{host: a.host.ID}
		hs, cl := a.goneList, a.client
		if cl != nil {
			go func() {
				list, err := cl.SpacesGone()
				if err != nil {
					return
				}
				a.onMain(func() { hs.list = list })
			}()
		}
	}
	return a.goneList
}

// spacesGoneEvent takes the host's notices as they changed.
func (a *App) spacesGoneEvent(data json.RawMessage) func() {
	var list []proto.SpaceGone
	if json.Unmarshal(data, &list) != nil {
		return nil
	}
	return func() {
		if hs := a.goneList; hs != nil && hs.host == a.host.ID {
			hs.list = list
		}
	}
}

// spacesGoneShown says a corner card tells of a team deleted.
func (a *App) spacesGoneShown() bool { return a.client != nil && len(a.spacesGoneOf().list) > 0 }

// spacesGoneCards are the corner's cards of teams deleted, each until its
// OK.
func (a *App) spacesGoneCards(c *ui.Context) {
	if !a.spacesGoneShown() {
		return
	}
	hs := a.spacesGoneOf()
	for _, g := range hs.list {
		widget.Float(ui.Column(c.Key("gone:"+g.Team)).FillWidth().Padding(theme.S3, theme.S3, theme.S3, theme.S4).Gap(theme.S2).Radius(theme.RadiusCard).
			Background(theme.Dialog)).Transition(widget.OverlayMotion).Children(func() {
			ui.Column(c.Key("what")).FillWidth().Shrink(0).Gap(theme.S1).Children(func() {
				widget.Running(c, g.Name+" was deleted", widget.RunStyle{Weight: 500})
				widget.CardNote(c.Key("detail"), spaceGoneDetail(g, a.host.Name), 0)
			})
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if widget.PrimaryButton(c, nil, "OK", "").Clicked() {
					a.spaceGoneSeen(g.Team)
				}
			})
		})
	}
}

// spaceGoneDetail says what went with team g from host.
func spaceGoneDetail(g proto.SpaceGone, host string) string {
	s := "Its owner deleted the team: its chat, notes and canvases are gone from " + host + ", your own stay."
	if g.Shared {
		s += " " + host + " is no longer shared with it."
	}
	return s
}

// spaceGoneSeen takes team's card away, here and on the host.
func (a *App) spaceGoneSeen(team string) {
	hs := a.spacesGoneOf()
	for i, g := range hs.list {
		if g.Team == team {
			hs.list = append(hs.list[:i:i], hs.list[i+1:]...)
			break
		}
	}
	if cl := a.client; cl != nil {
		go cl.SpaceGoneSeen(team)
	}
}

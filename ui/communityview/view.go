// Package communityview is enowX UI, the community library, as a page of the
// app: search, a category and a kind to narrow it, a grid of pictures, a sheet
// for one design with what it allows. Everything comes through the host's daemon,
// from the cloud the user set. Other people's HTML doesn't run here: only their
// pictures show, and a link opens the design's page in the browser.
package communityview

import (
	"slices"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/icon"
	"github.com/enowx/enowx/ui/theme"
	"github.com/enowx/enowx/ui/widget"
)

// Options connect the view to the app. Each done runs on the UI's goroutine.
type Options struct {
	Search func(p proto.LibrarySearchParams, done func(proto.LibraryResults, error))
	// Prompt is what Copy prompt copies, with its credit.
	Prompt func(id string, done func(string, error))
	// Picture is a design's picture, decoded (DecodePicture) off the UI's goroutine.
	Picture func(it proto.LibraryListing, done func(*ui.Bitmap, error))
	// OpenURL opens an address in the browser.
	OpenURL func(url string)
	// Back goes to the Canvas page.
	Back func()
}

// searchQuiet is how long typing rests before it searches.
const searchQuiet = 350 * time.Millisecond

// narrowRoom is the width under which the side list becomes a row above the grid.
const narrowRoom = 760

// filterWidth is the side list's width.
const filterWidth = 216

// cellWidth is the least a card takes, and mostColumns the most cards in a row.
const (
	cellWidth   = 264
	mostColumns = 5
)

// categories are the choices of a filter: value, then how it reads.
var categories = [][2]string{{"", "All"}, {"component", "Component"}, {"dashboard", "Dashboard"}, {"landing-page", "Landing page"}, {"mobile", "Mobile"},
	{"onboarding", "Onboarding"}, {"motion", "Motion"}, {"diagram", "Diagram"}, {"icon-illustration", "Icon and illustration"}, {"other", "Other"}}

// kinds are the choices of a kind: value, then how it reads.
var kinds = [][2]string{{"", "Any"}, {"component", "Component"}, {"page", "Full page"}}

// View is the page. The UI's goroutine owns it: every answer comes back there.
type View struct {
	asked    bool
	known    bool // an answer came, so On says something
	on       bool
	busy     bool
	err      string
	turn     int // the search whose answer counts
	query    string
	sent     string
	typed    time.Time
	category string
	kind     string
	items    []proto.LibraryListing
	next     string
	more     bool // the next page is on its way
	pictures pictures
	detail   *detail
	menu     widget.ContextMenu
}

func (v *View) params() proto.LibrarySearchParams {
	return proto.LibrarySearchParams{Query: v.sent, Category: v.category, Kind: v.kind}
}

// search asks anew, from the first page. An answer overtaken by another is dropped.
func (v *View) search(o Options) {
	if o.Search == nil {
		return
	}
	v.asked, v.busy, v.err, v.more = true, true, "", false
	v.sent = v.query
	v.turn++
	turn := v.turn
	o.Search(v.params(), func(res proto.LibraryResults, err error) {
		if turn != v.turn {
			return
		}
		v.busy = false
		if err != nil {
			v.err = err.Error()
			return
		}
		v.known, v.on, v.items, v.next = true, res.On, res.Items, res.Next
	})
}

// showMore adds the next page.
func (v *View) showMore(o Options) {
	if o.Search == nil || v.next == "" || v.more {
		return
	}
	v.more = true
	turn := v.turn
	p := v.params()
	p.Cursor = v.next
	o.Search(p, func(res proto.LibraryResults, err error) {
		if turn != v.turn {
			return
		}
		v.more = false
		if err != nil {
			v.err = err.Error()
			return
		}
		// Pages count from the top, so what went live meanwhile can repeat an item.
		for _, it := range res.Items {
			if !slices.ContainsFunc(v.items, func(x proto.LibraryListing) bool { return x.ItemID == it.ItemID }) {
				v.items = append(v.items, it)
			}
		}
		v.next = res.Next
	})
}

// keepSearching asks the first time, and once typing has rested.
func (v *View) keepSearching(c *ui.Context, o Options) {
	if !v.asked {
		v.search(o)
		return
	}
	if v.query == v.sent {
		return
	}
	if v.typed.IsZero() {
		v.typed = c.Now()
	}
	rest := searchQuiet - c.Now().Sub(v.typed)
	if rest > 0 {
		c.After(rest)
		return
	}
	v.typed = time.Time{}
	v.search(o)
}

// Draw fills the room it's given. With no library on the host's cloud there's
// nothing to browse: it says so, and offers nothing else.
func (v *View) Draw(c *ui.Context, o Options) {
	v.keepSearching(c, o)
	widget.PageHeader(c, "Community library", "", func() {
		if widget.Button(c, widget.Secondary, icon.ArrowLeft, "Canvas", "").Clicked() && o.Back != nil {
			o.Back()
		}
	})
	if v.known && !v.on {
		ui.Column(c.Key("no-library")).Grow(1).FillWidth().Center().Padding(theme.S5).Children(func() {
			ui.Text(c, "This host's cloud has no community library").FontSize(theme.TextUI).TextColor(theme.TextSoft)
		})
		return
	}
	room, _ := c.Size()
	narrow := room < narrowRoom
	ui.Row(c.Key("page")).Grow(1).FillWidth().MinHeight(0).Children(func() {
		if !narrow {
			v.filters(c, o)
		}
		ui.Column(c.Key("results")).Grow(1).Basis(0).MinWidth(0).FillHeight().Children(func() {
			ui.Column(c.Key("top")).FillWidth().Shrink(0).Padding(theme.S3, theme.S4, theme.S2, theme.S4).Gap(theme.S2).Children(func() {
				widget.Breadcrumb(c, "Canvas", "Community library")
				widget.SearchField(c.Key("search"), &v.query, "Search designs", icon.Search)
				if narrow {
					v.filtersInARow(c, o)
				}
			})
			v.grid(c, o)
		})
	})
	v.detailSheet(c, o)
}

// filters is the side list: the kinds, then the categories.
func (v *View) filters(c *ui.Context, o Options) {
	ui.Column(c.Key("filters")).Width(filterWidth).Shrink(0).FillHeight().BorderWidth(0, theme.Rule, 0, 0).BorderColor(theme.Border).Children(func() {
		ui.Scroll(c).Grow(1).FillWidth().MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(theme.S2, widget.ScrollGutter, theme.S2, theme.S2).Gap(theme.S1).Children(func() {
				widget.SideLabel(c, "Type of item")
				// Each its own column: Component is in both.
				ui.Column(c.Key("kinds")).FillWidth().Gap(theme.S1).Children(func() {
					for _, k := range kinds {
						if widget.SideEntry(c, nil, k[1], v.kind == k[0]).Clicked() && v.kind != k[0] {
							v.kind = k[0]
							v.search(o)
						}
					}
				})
				widget.SideLabel(c, "Category")
				ui.Column(c.Key("categories")).FillWidth().Gap(theme.S1).Children(func() {
					for _, cat := range categories {
						if widget.SideEntry(c, nil, cat[1], v.category == cat[0]).Clicked() && v.category != cat[0] {
							v.category = cat[0]
							v.search(o)
						}
					}
				})
			})
		})
	})
}

// filtersInARow is the same choices where there's no room for a side list.
func (v *View) filtersInARow(c *ui.Context, o Options) {
	ui.Column(c.Key("row-filters")).FillWidth().Gap(theme.S2).Children(func() {
		labels := make([]string, len(kinds))
		now := 0
		for i, k := range kinds {
			labels[i] = k[1]
			if v.kind == k[0] {
				now = i
			}
		}
		if i := widget.SegmentedFill(c.Key("kinds"), labels, now); i >= 0 && i != now {
			v.kind = kinds[i][0]
			v.search(o)
		}
		b := widget.MenuButton(c.Key("categories"), widget.Secondary, categoryName(v.category)).FillWidth().Label("Category")
		v.menu.OpenBelow(b, "community-category", theme.ControlHeight)
		v.menu.Show(c, b, "community-category", func(done func()) {
			for _, cat := range categories {
				if widget.MenuItem(c.Key("cat:"+cat[0]), nil, cat[1], v.category == cat[0]) {
					if v.category != cat[0] {
						v.category = cat[0]
						v.search(o)
					}
					done()
				}
			}
		})
	})
}

func categoryName(value string) string {
	for _, cat := range categories {
		if cat[0] == value {
			return cat[1]
		}
	}
	return "Other"
}

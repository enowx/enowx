package widget

import (
	"hash/fnv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// teamColors tell teams apart, one each, picked by the team's ID so every
// device colors a team alike.
var teamColors = []ui.Color{
	ui.RGB(0xE5, 0x48, 0x4D), ui.RGB(0xF7, 0x6B, 0x15), ui.RGB(0x46, 0xA7, 0x58), ui.RGB(0x12, 0xA5, 0x94),
	ui.RGB(0x00, 0x90, 0xFF), ui.RGB(0x8E, 0x4E, 0xC6), ui.RGB(0xD6, 0x40, 0x9F), ui.RGB(0xAD, 0x7F, 0x58),
}

// TeamColor is team's color.
func TeamColor(team string) ui.Color {
	h := fnv.New32a()
	h.Write([]byte(team))
	return teamColors[h.Sum32()%uint32(len(teamColors))]
}

// TeamMark is a team's mark wherever its items show: the first letter of
// its name on its color, size square.
func TeamMark(c *ui.Context, team, name string, size float32) ui.Element {
	initial := "?"
	if f := strings.Fields(name); len(f) > 0 {
		initial = strings.ToUpper(string([]rune(f[0])[0]))
	}
	col := TeamColor(team)
	return ui.Box(c).Size(size, size).Radius(size/4 + 1).Shrink(0).Background(col).Center().Children(func() {
		ui.Text(c, initial).FontSize(size * 0.56).FontWeight(700).TextColor(ui.RGB(255, 255, 255))
	})
}

// TeamTag is a team's mark with its name, after an item of its space.
func TeamTag(c *ui.Context, team, name string) {
	ui.Row(c).Gap(5).Height(18).Padding(0, 7, 0, 5).Radius(theme.RadiusSmall).Background(theme.Fade(TeamColor(team), 0.14)).AlignItems(ui.Center).Shrink(0).Children(func() {
		TeamMark(c, team, name, 11)
		ui.Text(c, name).FontSize(theme.TextLabel).TextColor(theme.TextSoft).SingleLine()
	})
}

package tokenview

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/proto"
	"github.com/enowx/enowx/ui/framecheck"
)

// Token tables with hundreds of rows, and a name of a megabyte among them, stay quick.
func TestLongTokenTablesStayQuick(t *testing.T) {
	line := framecheck.Line()
	counts := proto.TokenCounts{Input: 1000, Output: 2000, Calls: 3}
	groups := []proto.TokenGroup{{Key: line, Label: line, TokenCounts: counts}}
	for i := range 300 {
		groups = append(groups, proto.TokenGroup{Key: fmt.Sprint("/work/project-", i), TokenCounts: counts})
	}
	summary := proto.TokensSummary{Groups: groups}
	list := []HostTotal{{Name: line, Live: true, On: true, Total: counts}}
	data := Data{Coverage: []proto.TokenCoverage{{Agent: "claude", Name: "Claude Code", Layer: proto.TokenLayerRecords, Records: line, Where: line}}}
	v := &View{}
	for i, s := range splits {
		if s.by == proto.TokensByProject {
			v.Split = i
		}
	}
	tt := framecheck.New(t, "tokens", func(c *ui.Context) {
		c.After(time.Millisecond)
		ui.Scroll(c).Width(1000).Height(700).Children(func() {
			ui.Column(c).FillWidth().Children(func() {
				v.whereTheyWent(c, summary, Options{})
				hosts(c, list)
				v.coverage(c, data)
			})
		})
	}, 1000, 700)
	framecheck.Quick(t, "tokens", tt)
}

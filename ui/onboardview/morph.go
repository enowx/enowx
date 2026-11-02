package onboardview

import "github.com/egoist/mygo/ui"

// morph is the change from one style to another. For now a style just changes.
type morph struct{}

func (m *morph) running() bool { return false }

// start changes to style id.
func (m *morph) start(o Options, id string) { o.SetStyle(id) }

func (m *morph) geometry(g geometry) geometry { return g }

func (m *morph) color(c ui.Color) ui.Color { return c }

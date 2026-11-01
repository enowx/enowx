package cli

import "fmt"

var designProp = mcpProp{"string", "The design: its number (from 1), name or ID"}

// canvasTools are the canvases as MCP tools, their designs' included.
var canvasTools = append(append(append(append(canvasCoreTools, designTools...), chatTools...), gridTools...), buildTools...)

// designTools are a canvas's designs as MCP tools.
var designTools = []mcpTool{
	{name: "canvas_designs", description: "A canvas's designs (its frames: variations, pages): number, name, page × variant, favorite, winner, and a canvas:// link to each.",
		props: map[string]mcpProp{"canvas": canvasProp}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return []string{"designs", str(a, "canvas")} }},
	{name: "canvas_new_design", description: "Add a design to a canvas (one of several variations or pages), or a copy of design `from` as a new variant; answers its number, to write it with frame N.",
		props: map[string]mcpProp{"canvas": canvasProp, "title": {"string", "Its name"}, "description": {"string", "What it tries, in a line"},
			"page": {"string", "The page it is of (Home, Login…)"}, "variant": {"string", "Its variant (A, B…)"}, "size": {"string", "WIDTHxHEIGHT (default: the others')"},
			"from": designProp},
		required: []string{"canvas"},
		args: func(a map[string]any) []string {
			args := []string{"new-design", str(a, "canvas")}
			for _, k := range []string{"title", "description", "page", "variant", "size", "from"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "canvas_design", description: "Rename or describe a design, or set its page and variant.",
		props: map[string]mcpProp{"canvas": canvasProp, "design": designProp, "title": {"string", "A new name"}, "description": {"string", "What it tries"},
			"page": {"string", "Its page"}, "variant": {"string", "Its variant"}},
		required: []string{"canvas", "design"},
		args: func(a map[string]any) []string {
			args := []string{"design", str(a, "canvas"), str(a, "design")}
			for _, k := range []string{"title", "description", "page", "variant"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "canvas_goto", description: "Point the user to a design, with a card they may click to go there; it doesn't move them.",
		props: map[string]mcpProp{"canvas": canvasProp, "design": designProp}, required: []string{"canvas", "design"},
		args: func(a map[string]any) []string { return []string{"goto", str(a, "canvas"), str(a, "design")} }},
	{name: "canvas_compare", description: "Answer what differs between 2 to 4 designs, with a card the user may click to see them side by side; it doesn't move them.",
		props: map[string]mcpProp{"canvas": canvasProp, "designs": {"array", "2 to 4 designs: numbers or names"}}, required: []string{"canvas", "designs"},
		args: func(a map[string]any) []string {
			args := []string{"compare", str(a, "canvas")}
			list, _ := a["designs"].([]any)
			for _, d := range list {
				args = append(args, fmt.Sprint(d))
			}
			return args
		}},
	{name: "canvas_pick", description: "Make a design the winner of a canvas (design \"none\": no winner).",
		props: map[string]mcpProp{"canvas": canvasProp, "design": designProp}, required: []string{"canvas", "design"},
		args: func(a map[string]any) []string { return []string{"pick", str(a, "canvas"), str(a, "design")} }},
	{name: "canvas_favorite", description: "Star a design, or unstar it (off: true).",
		props: map[string]mcpProp{"canvas": canvasProp, "design": designProp, "off": {"boolean", "Unstar it"}}, required: []string{"canvas", "design"},
		args: func(a map[string]any) []string {
			return withFlag([]string{"favorite", str(a, "canvas"), str(a, "design")}, a, "off", "off")
		}},
	{name: "canvas_move_design", description: "Put a design before another (before), or last.",
		props: map[string]mcpProp{"canvas": canvasProp, "design": designProp, "before": designProp}, required: []string{"canvas", "design"},
		args: func(a map[string]any) []string {
			return withFlag([]string{"move-design", str(a, "canvas"), str(a, "design")}, a, "before", "before")
		}},
	{name: "canvas_rm_design", description: "Delete a design of a canvas.",
		props: map[string]mcpProp{"canvas": canvasProp, "design": designProp}, required: []string{"canvas", "design"},
		args: func(a map[string]any) []string { return []string{"rm-design", str(a, "canvas"), str(a, "design")} }},
}

// gridTools are a design's layout grid and mockups' icons as MCP tools.
var gridTools = []mcpTool{
	{name: "canvas_grid", description: "A design's layout grid (columns, rows, gaps, margin, named areas spanning cells): shown to the user as guides, shapes snap to it; answers its CSS. Give it before laying a design out; apply: true lays the design's HTML out on it, an empty element per area to fill.",
		props: map[string]mcpProp{"canvas": canvasProp, "frame": designProp, "cols": {"number", "Columns (12 for pages)"}, "rows": {"number", "Rows"},
			"gap": {"number", "Gap between columns and rows"}, "colGap": {"number", "Gap between columns"}, "rowGap": {"number", "Gap between rows"},
			"margin": {"number", "Margin around the grid"}, "areas": {"array", "Areas, each NAME:COL,ROW,COLSPAN,ROWSPAN (header:1,1,12,1)"},
			"noSnap": {"boolean", "Shapes don't snap to it"}, "apply": {"boolean", "Lay the design's HTML out on it"}, "off": {"boolean", "Take the grid away"}},
		required: []string{"canvas"},
		args: func(a map[string]any) []string {
			args := []string{"grid", str(a, "canvas")}
			for k, flag := range map[string]string{"frame": "frame", "cols": "cols", "rows": "rows", "gap": "gap", "colGap": "col-gap", "rowGap": "row-gap",
				"margin": "margin", "noSnap": "no-snap", "apply": "apply", "off": "off"} {
				args = withFlag(args, a, k, flag)
			}
			list, _ := a["areas"].([]any)
			for _, ar := range list {
				args = append(args, "--area", fmt.Sprint(ar))
			}
			return args
		}},
	{name: "canvas_icon", description: "An icon (lucide, tabler or phosphor; icons_search finds names) as inline SVG to put in a mockup, in the text's color; works offline.",
		props: map[string]mcpProp{"icon": {"string", "lib:name, as lucide:rocket"}, "size": {"number", "Its size (24)"}}, required: []string{"icon"},
		args: func(a map[string]any) []string { return withFlag([]string{"icon", str(a, "icon")}, a, "size", "size") }},
}

package cli

import "fmt"

var (
	canvasProp = mcpProp{"string", "The canvas: its ID or title"}
	frameProp  = mcpProp{"string", "The frame: its ID or name (the first when left out)"}
)

// canvasArgs is a canvas command with the canvas and the flags given.
func canvasArgs(a map[string]any, cmd string, flags ...string) []string {
	args := []string{cmd, str(a, "canvas")}
	for _, k := range flags {
		args = withFlag(args, a, k, k)
	}
	return args
}

// canvasCoreTools are the canvases as MCP tools, but for their designs.
var canvasCoreTools = []mcpTool{
	{name: "canvas_guide", description: "How to design on an enowX canvas (mockups in HTML, wireframes, diagrams): the steps, rules, components and layouts. Read it before designing. topic motion: animation in depth (timing, easing, choreography, three.js, checking it).",
		props: map[string]mcpProp{"topic": {"string", "motion, for animation in depth"}},
		args:  func(a map[string]any) []string { return []string{"guide", str(a, "topic")} }},
	{name: "canvas_list", description: "The host's canvases: ID, kind and title.",
		args: func(map[string]any) []string { return []string{"list"} }},
	{name: "canvas_new", description: "A new canvas, its ID answered. canvas_open tells the user it's there.",
		props: map[string]mcpProp{"title": {"string", "Its title"}, "kind": {"string", "mockup (HTML), wireframe, diagram or board"},
			"frames": {"array", "Its frames: desktop, tablet, mobile (their sizes), or NAME:WxH"}, "group": {"string", "A group, as notes have"},
			"project": {"string", "The project folder it belongs to"}},
		required: []string{"title"},
		args: func(a map[string]any) []string {
			args := []string{"new", str(a, "title")}
			args = withFlag(args, a, "kind", "kind")
			args = withFlag(args, a, "group", "group")
			args = withFlag(args, a, "project", "project")
			list, _ := a["frames"].([]any)
			for _, f := range list {
				args = append(args, "--frame", fmt.Sprint(f))
			}
			return args
		}},
	{name: "canvas_open", description: "Tell the user in the enowX app about a canvas, with a card they may click to go there; it doesn't move them.",
		props: map[string]mcpProp{"canvas": canvasProp}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return canvasArgs(a, "open") }},
	{name: "canvas_show", description: "A canvas's frames, their HTML elements (by id) and shapes, and how many comments are open; html: true answers a frame's HTML.",
		props: map[string]mcpProp{"canvas": canvasProp, "frame": frameProp, "html": {"boolean", "The frame's HTML instead"}}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return canvasArgs(a, "show", "frame", "html") }},
	{name: "canvas_write", description: "A frame's whole HTML (a mockup): the skeleton and its <style>, regions with ids, filled later with canvas_patch.",
		props: map[string]mcpProp{"canvas": canvasProp, "html": {"string", "The HTML"}, "frame": frameProp}, required: []string{"canvas", "html"},
		args: func(a map[string]any) []string { return canvasArgs(a, "write", "html", "frame") }},
	{name: "canvas_patch", description: "Change one element of a frame's HTML: html replaces the element with id `element`, or is added at the end of `into` (or the body) when none has it; text sets what it holds (or a shape's text).",
		props: map[string]mcpProp{"canvas": canvasProp, "element": {"string", "The element's id"}, "html": {"string", "Its new outer HTML, with the id"},
			"into": {"string", "Where a new element goes: its parent's id"}, "text": {"string", "What it holds, as HTML"}, "frame": frameProp},
		required: []string{"canvas", "element"},
		args: func(a map[string]any) []string {
			return canvasArgs(a, "patch", "element", "html", "into", "text", "frame")
		}},
	{name: "canvas_shape", description: "Add a shape to a frame (wireframes, diagrams, boards), or change shape `id` (only what is given). Arrows and lines join shapes with from and to.",
		props: map[string]mcpProp{"canvas": canvasProp, "frame": frameProp, "id": {"string", "Its id: a new one, or one to change"},
			"type": {"string", "rect, ellipse, diamond, text, sticky, line, arrow or image"},
			"x":    {"number", "Left"}, "y": {"number", "Top"}, "w": {"number", "Width"}, "h": {"number", "Height"},
			"text": {"string", "What it says"}, "fill": {"string", "#RRGGBB, or none"}, "stroke": {"string", "#RRGGBB, or none"}, "color": {"string", "Its text's #RRGGBB"},
			"size": {"number", "Its text's size"}, "bold": {"boolean", "Bold text"}, "radius": {"number", "Corner radius"},
			"from": {"string", "An arrow's start: a shape's id"}, "to": {"string", "An arrow's end: a shape's id"}, "src": {"string", "An image's asset or URL"}},
		required: []string{"canvas"},
		args: func(a map[string]any) []string {
			return canvasArgs(a, "shape", "frame", "id", "type", "x", "y", "w", "h", "text", "fill", "stroke", "color", "size", "bold", "radius", "from", "to", "src")
		}},
	{name: "canvas_remove", description: "Remove an HTML element or a shape from a frame.",
		props: map[string]mcpProp{"canvas": canvasProp, "element": {"string", "Its id"}, "frame": frameProp}, required: []string{"canvas", "element"},
		args: func(a map[string]any) []string { return canvasArgs(a, "remove", "element", "frame") }},
	{name: "canvas_frame", description: "Add a frame (action add, name), change one (set: name, size WxH, background), or remove one (rm).",
		props: map[string]mcpProp{"canvas": canvasProp, "action": {"string", "add, set or rm"}, "frame": {"string", "The frame to add (its name), set or remove"},
			"name": {"string", "A new name (set)"}, "size": {"string", "WIDTHxHEIGHT"}, "background": {"string", "#RRGGBB"}},
		required: []string{"canvas", "action", "frame"},
		args: func(a map[string]any) []string {
			args := []string{"frame", str(a, "canvas"), str(a, "action"), str(a, "frame")}
			for _, k := range []string{"name", "size", "background"} {
				args = withFlag(args, a, k, k)
			}
			return args
		}},
	{name: "canvas_screenshot", description: "A frame as an image, to look at what you made (or saved where path says); frame desktop, tablet or mobile shows the first frame at that width.",
		props: map[string]mcpProp{"canvas": canvasProp, "frame": frameProp, "path": {"string", "Where to write it (optional)"}}, required: []string{"canvas"}, image: true,
		args: func(a map[string]any) []string {
			return withFlag([]string{"screenshot", str(a, "canvas"), str(a, "path")}, a, "frame", "frame")
		}},
	{name: "canvas_export", description: "Write a canvas to a file: png, pdf or svg of a frame, html (without frame: the whole canvas as one read-only page to share), or mp4 of a design that moves, rendered frame by frame by the app, which can take minutes.",
		props: map[string]mcpProp{"canvas": canvasProp, "format": {"string", "png, pdf, svg, html or mp4"}, "frame": frameProp, "out": {"string", "The file (default: its title, in the current folder; an mp4 stays where the app runs without it)"},
			"fps": {"number", "mp4: 30 or 60 (default 30)"}, "width": {"number", "mp4: its width (default the frame's)"}, "from-ms": {"number", "mp4: where it starts, in ms"}, "to-ms": {"number", "mp4: where it ends, in ms (default its end; one cycle of a loop)"}},
		required: []string{"canvas", "format"},
		args: func(a map[string]any) []string {
			return canvasArgs(a, "export", "format", "frame", "out", "fps", "width", "from-ms", "to-ms")
		}},
	{name: "canvas_comments", description: "The user's comments on a canvas's elements, to work on; all: true also those resolved.",
		props: map[string]mcpProp{"canvas": canvasProp, "all": {"boolean", "The resolved ones too"}}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return canvasArgs(a, "comments", "all") }},
	{name: "canvas_comment", description: "Comment on a canvas's element, as a question or a note for the user.",
		props:    map[string]mcpProp{"canvas": canvasProp, "text": {"string", "What to say"}, "element": {"string", "The element's id"}, "frame": frameProp},
		required: []string{"canvas", "text"},
		args: func(a map[string]any) []string {
			return append(canvasArgs(a, "comment", "element", "frame"), str(a, "text"))
		}},
	{name: "canvas_resolve", description: "Mark a comment done, once its change is made.",
		props: map[string]mcpProp{"canvas": canvasProp, "comment": {"string", "The comment's ID"}}, required: []string{"canvas", "comment"},
		args: func(a map[string]any) []string { return []string{"resolve", str(a, "canvas"), str(a, "comment")} }},
	{name: "canvas_unslop", description: "What makes a mockup's design look AI-made, by pass in the order to fix them (truth, structure, spacing, type, color, surfaces, icons, copy, data, motion, access, mobile), each with its fix, and a score from 100. Run it before saying a design is done, and again after each pass of a redesign.",
		props: map[string]mcpProp{"canvas": canvasProp, "frame": {"string", "The design: its number, name or ID (the first without)"},
			"pass":   {"string", "In a redesign the app started: the pass you begin, 1 to 12, or done"},
			"region": {"string", "As a region's agent of that redesign: your region, as its pass's reporter"},
			"fanout": {"string", "As the redesign's agent: regions (\"#a,#b\") to start an agent on each, for passes 6 to 10; wait for them with the CLI's canvas unslop CANVAS --wait"}}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return canvasArgs(a, "unslop", "frame", "pass", "region", "fanout") }},
	{name: "canvas_brief", description: "What a canvas's designs are for: product, users, tone as decisions, what the first screen is for, peers, what to remove, what is fixed. Without brief it prints it (or the sections to write); with brief it keeps it. Write it before designing or redesigning.",
		props: map[string]mcpProp{"canvas": canvasProp, "brief": {"string", "The brief as Markdown, a ## section a field"}}, required: []string{"canvas"},
		args: func(a map[string]any) []string {
			return withFlag([]string{"brief", str(a, "canvas")}, a, "brief", "set")
		}},
	{name: "canvas_history", description: "A canvas's versions: each change, by the user or an agent; * the one shown.",
		props: map[string]mcpProp{"canvas": canvasProp}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return canvasArgs(a, "history") }},
	{name: "canvas_undo", description: "Show a canvas's version before the one shown.",
		props: map[string]mcpProp{"canvas": canvasProp}, required: []string{"canvas"},
		args: func(a map[string]any) []string { return canvasArgs(a, "undo") }},
	{name: "canvas_restore", description: "Make a canvas as it was at version n its newest version.",
		props: map[string]mcpProp{"canvas": canvasProp, "n": {"number", "The version, from canvas_history"}}, required: []string{"canvas", "n"},
		args: func(a map[string]any) []string { return []string{"restore", str(a, "canvas"), str(a, "n")} }},
}

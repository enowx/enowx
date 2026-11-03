package proto

// Unslop: a mockup's design read for what makes it look made by AI
// without a designer's care (core/unslop), found by passes in the order
// they're best fixed, and, asked to start, the canvas's agent redesigning
// a copy of it pass by pass.
const (
	MethodCanvasUnslop = "canvas.unslop" // CanvasUnslopParams → CanvasUnslop
	// A redesign's steps, to play them back, compare and export them; and
	// one left out, as a new design.
	MethodCanvasUnslopSteps    = "canvas.unslopSteps"    // UnslopStepsParams → UnslopStory
	MethodCanvasUnslopLeaveOut = "canvas.unslopLeaveOut" // UnslopLeaveOutParams → UnslopLeftOut
)

// UnslopGroup starts the session group (SessionInfo.Group) of a fanned-out
// unslop's region agents, by design: canvas agents, not its conversation.
const UnslopGroup = "unslop:"

// EventCanvasUnslop carries a CanvasUnslopRun as it goes: started, a pass
// begun, the design read again, done.
const EventCanvasUnslop = "canvas.unslop"

// CanvasUnslopParams reads design Frame of canvas ID (its first when
// empty). With Start the design is copied, the copy read, and Ask is what
// to tell the canvas's agent to redesign it. Pass, by the agent
// redesigning, says the pass it begins (its number or name; "done" when
// all are), the design read again with it; with Region, an agent of that
// region says it. Regions, from the redesign's agent (Session), fan the
// rest out: an agent each, as tabs of Session. Status answers the run
// alone, the design not read.
type CanvasUnslopParams struct {
	ID      string   `json:"id"`
	Frame   string   `json:"frame,omitempty"`
	Start   bool     `json:"start,omitempty"`
	Pass    string   `json:"pass,omitempty"`
	Region  string   `json:"region,omitempty"`
	Regions []string `json:"regions,omitempty"`
	Session string   `json:"session,omitempty"`
	Status  bool     `json:"status,omitempty"`
	// Only, with Start, are the passes to run (numbers or names), the
	// others skipped; empty runs them all.
	Only []string `json:"only,omitempty"`
}

// UnslopStep is what a redesign changed from one pass's start to the
// next's: Passes those it covers (several when region agents did some at
// once), Version the canvas's version it ended at, Changes measured in the
// design as rendered (Measured once they are).
type UnslopStep struct {
	Passes   []string       `json:"passes"`
	Version  int            `json:"version,omitempty"`
	Changes  []UnslopChange `json:"changes,omitempty"`
	Measured bool           `json:"measured,omitempty"`
}

// UnslopChange is an element a step changed: its properties as rendered
// before and after, and where it is after (X, Y, W, H in the design's
// pixels); or Added, or Removed.
type UnslopChange struct {
	Element string       `json:"element"`
	Props   []UnslopProp `json:"props,omitempty"`
	X       int          `json:"x"`
	Y       int          `json:"y"`
	W       int          `json:"w"`
	H       int          `json:"h"`
	Added   bool         `json:"added,omitempty"`
	Removed bool         `json:"removed,omitempty"`
}

// UnslopProp is a property's value before (From) and after (To) a step:
// font, size, weight, line height, tracking, color, background, padding,
// gap, radius, shadow, border.
type UnslopProp struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// UnslopStepsParams names a redesign by its design: Frame of canvas ID.
type UnslopStepsParams struct {
	ID    string `json:"id"`
	Frame string `json:"frame"`
}

// UnslopStory is a redesign to play back: the design's documents, the
// one it started from first then each step's end, and the steps between
// them (len(Docs) is len(Steps)+1), at the design's size; From and Score
// its score before and after.
type UnslopStory struct {
	Frame  string       `json:"frame"`
	Name   string       `json:"name,omitempty"`
	Width  int          `json:"width"`
	Height int          `json:"height"`
	Docs   []string     `json:"docs"`
	Steps  []UnslopStep `json:"steps"`
	From   int          `json:"from,omitempty"`
	Score  int          `json:"score,omitempty"`
}

// UnslopLeaveOutParams leaves step Step (1 the first) of the redesign of
// design Frame of canvas ID out, in a copy.
type UnslopLeaveOutParams struct {
	ID    string `json:"id"`
	Frame string `json:"frame"`
	Step  int    `json:"step"`
}

// UnslopLeftOut is the copy made (Frame), the elements put back as before
// the step (Back) and those kept as they are, changed again since (Kept).
type UnslopLeftOut struct {
	Frame string   `json:"frame"`
	Back  []string `json:"back,omitempty"`
	Kept  []string `json:"kept,omitempty"`
}

// UnslopRegion is a part of a design an agent redesigns, at once with
// others: element ID and all in it, the only part its Session may change;
// the pass it is at, and whether it is done.
type UnslopRegion struct {
	ID      string `json:"id"`
	Session string `json:"session"`
	Agent   string `json:"agent,omitempty"`
	Pass    string `json:"pass,omitempty"`
	Done    bool   `json:"done,omitempty"`
}

// CanvasUnslopRun is a redesign of canvas ID's design Frame as it goes, for
// the app to show: the score it started from and the latest, the pass
// under way (a name of core/unslop's passes), those done, the latest
// findings, and whether it is Done. The daemon keeps it while it runs.
type CanvasUnslopRun struct {
	ID       string   `json:"id"`
	Frame    string   `json:"frame"`
	From     int      `json:"from"`
	Score    int      `json:"score"`
	Pass     string   `json:"pass,omitempty"`
	Passes   []string `json:"passes"` // every pass, in order
	Finished []string `json:"finished,omitempty"`
	// Passed is what each step changed, as passes end.
	Passed   []UnslopStep    `json:"passed,omitempty"`
	Findings []UnslopFinding `json:"findings"`
	Scans    int             `json:"scans"` // times read, the first 1
	// Theme is the original's, dark or light ("" unknown): the redesign
	// keeps it.
	Theme string `json:"theme,omitempty"`
	// Regions are the parts agents redesign at once, once fanned out;
	// Lead the session of the redesign's agent, told when they're done.
	Regions []UnslopRegion `json:"regions,omitempty"`
	Lead    string         `json:"lead,omitempty"`
	// Steps are what a run does before its first read is in (reading the
	// page at each size, making a branch…), Step the one under way: all
	// done when it is len(Steps).
	Steps []string `json:"steps,omitempty"`
	Step  int      `json:"step,omitempty"`
	// Original is the design a canvas's run redesigns a copy of, to
	// compare with; Shots how many widths a Browser's run has before and
	// after screenshots of (browser.unslopShots), once done.
	Original string `json:"original,omitempty"`
	Shots    int    `json:"shots,omitempty"`
	// Branch is the branch a redesign of a project's code is made on, Base
	// the one it started from: going back to Base undoes it all.
	Branch string `json:"branch,omitempty"`
	Base   string `json:"base,omitempty"`
	// Brief is what the design or page is for, as the agent wrote it
	// before the first pass.
	Brief *DesignBrief `json:"brief,omitempty"`
	// Review is the independent review of the redesign once done, the
	// latest round.
	Review *DesignReview `json:"review,omitempty"`
	Done   bool          `json:"done,omitempty"`
}

// CanvasUnslop is what reading a design found: Score from 100 (nothing
// found) down, and the findings by pass. Frame is the design read (the
// copy, when started); Ask what to tell the canvas's agent.
type CanvasUnslop struct {
	Frame    string          `json:"frame"`
	Score    int             `json:"score"`
	Findings []UnslopFinding `json:"findings"`
	Ask      string          `json:"ask,omitempty"`
	// Run is the redesign under way it is part of, if any.
	Run *CanvasUnslopRun `json:"run,omitempty"`
	// Walk is what a walk through the design's states reached, measured anew.
	Walk *UnslopWalk `json:"walk,omitempty"`
}

// UnslopFinding is a tell found in a design: Rule its ID, Pass the pass
// fixing it ("01 Spacing"), Title what it is, Count how often, Examples a
// few of them, Fix how to fix it, Targets where.
type UnslopFinding struct {
	Rule     string   `json:"rule"`
	Pass     string   `json:"pass"`
	Title    string   `json:"title"`
	Severity string   `json:"severity"` // high, medium, low
	Count    int      `json:"count"`
	Examples []string `json:"examples,omitempty"`
	Fix      string   `json:"fix"`
	// Targets are the design's elements it is about, as CSS selectors.
	Targets []string `json:"targets,omitempty"`
	// State is the state of an interactive design it was found in, "" for the
	// first screen, and Steps the clicks from that screen that reach it.
	State string   `json:"state,omitempty"`
	Steps []string `json:"steps,omitempty"`
}

// UnslopWalk is what a walk through a design's states reached: Tried controls
// clicked, Left seen and not tried, Limit what stopped it early ("states",
// "time", "tries"), in Millis.
type UnslopWalk struct {
	States []UnslopWalkState `json:"states"`
	Tried  int               `json:"tried"`
	Left   int               `json:"left,omitempty"`
	Limit  string            `json:"limit,omitempty"`
	Millis int               `json:"millis,omitempty"`
}

// UnslopWalkState is a state reached, how, and what its lint found there.
type UnslopWalkState struct {
	Name     string   `json:"name"`
	Steps    []string `json:"steps"`
	Findings int      `json:"findings"`
}

// Severities of an UnslopFinding.
const (
	UnslopHigh   = "high"
	UnslopMedium = "medium"
	UnslopLow    = "low"
)

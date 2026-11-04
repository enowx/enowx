package api

// Filters and search of the library's items: GET items takes q, category, kind,
// license, tag, sort, and these.

// SortRelevant ranks a search's results by how well they match, what a search
// does when no sort is asked for.
const SortRelevant = "relevant"

// What an item may say of itself in filters. A filter with another value finds
// nothing.
var (
	// LibraryStyles are what a creator may pick for a design.
	LibraryStyles = []string{"minimal", "editorial", "dense", "playful"}
	// LibraryThemes and LibraryDevices the server reads from the design.
	LibraryThemes  = []string{"dark", "light"}
	LibraryDevices = []string{"desktop", "phone"}
	// LibraryColors are the colors the server finds a design's dominant ones among.
	LibraryColors = []string{"red", "yellow", "green", "teal", "blue", "violet", "black", "paper"}
)

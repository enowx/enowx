// Package wrapcheck is for tests: it finds elements sticking out of the one
// that wraps them, the owner's rule for every surface. It reads MyGo's element
// tree by reflection, so nothing but tests may import it.
package wrapcheck

import (
	"fmt"
	"reflect"
	"unsafe"

	"github.com/egoist/mygo/ui"
)

// The owner's rule: every element is wrapped by its parent, a text inside its
// button, a button inside its container, and so on up. MyGo's Tester says
// nothing of parents, so this reads its element tree. The flags are MyGo's, of
// the version in go.mod.
const (
	wrapScrollX   = 1 << 4
	wrapScrollY   = 1 << 5
	wrapClipX     = 1 << 6
	wrapClipY     = 1 << 7
	wrapAbsolute  = 1 << 8
	wrapInvisible = 1 << 18
	wrapSlack     = 1.5 // what rounding leaves
)

// field reads an unexported field.
func field(v reflect.Value, name string) reflect.Value {
	f := v.FieldByName(name)
	return reflect.NewAt(f.Type(), unsafe.Pointer(f.UnsafeAddr())).Elem()
}

// nodeBox is an element's box as last laid out, ok false if it has none.
func nodeBox(n reflect.Value) (x, y, w, h float32, ok bool) {
	st := field(n, "st")
	if st.IsNil() {
		return
	}
	s := st.Elem()
	get := func(name string) float32 { return float32(field(s, name).Float()) }
	return get("x"), get("y"), get("w"), get("h"), true
}

// nodeName says which element it is, to find it.
func nodeName(n reflect.Value) string {
	for _, f := range []string{"label", "text"} {
		if s := field(n, f).String(); s != "" {
			if len(s) > 30 {
				s = s[:30] + "…"
			}
			return fmt.Sprintf("%q", s)
		}
	}
	for k := n.FieldByName("first"); k.IsValid() && !k.IsNil(); k = field(k.Elem(), "first") {
		for _, f := range []string{"label", "text"} {
			if s := field(k.Elem(), f).String(); s != "" {
				if len(s) > 30 {
					s = s[:30] + "…"
				}
				return fmt.Sprintf("in %q", s)
			}
		}
	}
	return "an element"
}

// OutOfParents are the elements sticking out of the one that wraps them. What
// a parent scrolls or clips on purpose doesn't count, nor what floats over it
// (absolute, attached).
func OutOfParents(tt *ui.Tester) []string {
	rt := field(reflect.ValueOf(tt).Elem(), "rt").Elem()
	ctx := field(rt, "c")
	if ctx.Kind() == reflect.Pointer {
		ctx = ctx.Elem()
	}
	root := field(ctx, "root")
	var out []string
	seen := map[string]bool{}
	var walk func(n reflect.Value)
	walk = func(n reflect.Value) {
		pf := uint32(field(n, "flags").Uint())
		px, py, pw, ph, pok := nodeBox(n)
		for k := field(n, "first"); !k.IsNil(); k = field(k.Elem(), "next") {
			ch := k.Elem()
			f := uint32(field(ch, "flags").Uint())
			if f&wrapInvisible != 0 || field(ch, "leaving").Uint() != 0 {
				continue
			}
			floats := f&wrapAbsolute != 0 || field(ch, "attach").Uint() != 0
			x, y, w, h, ok := nodeBox(ch)
			if !floats && ok && pok && w > 0 && h > 0 && pw > 0 && ph > 0 {
				over := ""
				if pf&(wrapScrollX|wrapClipX) == 0 && (x < px-wrapSlack || x+w > px+pw+wrapSlack) {
					over = fmt.Sprintf("%.0f..%.0f across, its parent %.0f..%.0f", x, x+w, px, px+pw)
				}
				if pf&(wrapScrollY|wrapClipY) == 0 && (y < py-wrapSlack || y+h > py+ph+wrapSlack) {
					over = fmt.Sprintf("%.0f..%.0f down, its parent %.0f..%.0f", y, y+h, py, py+ph)
				}
				if over != "" {
					line := nodeName(ch) + " out of " + nodeName(n) + ": " + over
					if !seen[line] {
						seen[line] = true
						out = append(out, line)
					}
				}
			}
			walk(ch)
		}
	}
	if !root.IsNil() {
		walk(root.Elem())
	}
	return out
}

// FullWidth is how wide the element labelled or reading text is as laid out,
// the widest if several are. The Tester's rectangle is cut by what scrolls or
// clips an element; this isn't. 0 if there is none.
func FullWidth(tt *ui.Tester, text string) float32 {
	rt := field(reflect.ValueOf(tt).Elem(), "rt").Elem()
	ctx := field(rt, "c")
	if ctx.Kind() == reflect.Pointer {
		ctx = ctx.Elem()
	}
	var widest float32
	var walk func(n reflect.Value)
	walk = func(n reflect.Value) {
		if field(n, "label").String() == text || field(n, "text").String() == text {
			if _, _, w, _, ok := nodeBox(n); ok {
				widest = max(widest, w)
			}
		}
		for k := field(n, "first"); !k.IsNil(); k = field(k.Elem(), "next") {
			walk(k.Elem())
		}
	}
	if root := field(ctx, "root"); !root.IsNil() {
		walk(root.Elem())
	}
	return widest
}

// How far in from a scroller's right edge MyGo draws its bar, at the width
// ui/theme gives it: 3 of margin and 8 of thumb.
const wrapBarReach = 11

// ScrollbarOverlaps are what would lie under the bar of a vertical scroller,
// whether or not its bar shows now: words, controls and painted backgrounds
// that reach past the gutter. Plain layout boxes paint nothing there, and what
// floats or scrolls sideways on its own is judged by its own box.
func ScrollbarOverlaps(tt *ui.Tester) []string {
	rt := field(reflect.ValueOf(tt).Elem(), "rt").Elem()
	ctx := field(rt, "c")
	if ctx.Kind() == reflect.Pointer {
		ctx = ctx.Elem()
	}
	var out []string
	seen := map[string]bool{}
	var under func(scroller, n reflect.Value, edge float32)
	under = func(scroller, n reflect.Value, edge float32) {
		for k := field(n, "first"); !k.IsNil(); k = field(k.Elem(), "next") {
			ch := k.Elem()
			f := uint32(field(ch, "flags").Uint())
			if f&(wrapInvisible|wrapAbsolute) != 0 || field(ch, "leaving").Uint() != 0 || field(ch, "attach").Uint() != 0 {
				continue
			}
			x, _, w, h, ok := nodeBox(ch)
			shows := field(ch, "label").String() != "" || field(ch, "text").String() != "" || field(ch, "bg").FieldByName("A").Uint() != 0
			if ok && shows && w > 0 && h > 0 && x+w > edge+wrapSlack {
				line := nodeName(ch) + " under the bar of " + nodeName(scroller) + fmt.Sprintf(": ends at %.0f, the gutter at %.0f", x+w, edge)
				if !seen[line] {
					seen[line] = true
					out = append(out, line)
				}
			}
			if f&(wrapScrollX|wrapClipX) == 0 {
				under(scroller, ch, edge)
			}
		}
	}
	var walk func(n reflect.Value)
	walk = func(n reflect.Value) {
		f := uint32(field(n, "flags").Uint())
		if f&wrapScrollY != 0 && f&wrapInvisible == 0 {
			if x, _, w, _, ok := nodeBox(n); ok && w > 0 {
				under(n, n, x+w-wrapBarReach-1)
			}
		}
		for k := field(n, "first"); !k.IsNil(); k = field(k.Elem(), "next") {
			walk(k.Elem())
		}
	}
	if root := field(ctx, "root"); !root.IsNil() {
		walk(root.Elem())
	}
	return out
}

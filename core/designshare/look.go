package designshare

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// What a design looks like, read from its page and shapes, for the library's
// filters. It's read without drawing anything: a page's ground counts most, then
// other backgrounds, then the rest, and the page's scripts aren't read.

// Look is what a frame shows.
type Look struct {
	// Theme is dark or light, by the page's ground.
	Theme string
	// Device is phone for a narrow design, desktop otherwise.
	Device string
	// Colors are its dominant colors, the most shown first.
	Colors []LookColor
}

// LookColor is a color, one of the library's few, and its share out of 100.
type LookColor struct {
	Name  string
	Share int
}

// mostResolved is the most text a page's CSS variables may expand to.
const mostResolved = 4 << 20

// phoneWidest is the widest design counted as made for a phone.
const phoneWidest = 600

// A color needs this share of what was counted to be dominant, and a design
// keeps this many.
const (
	leastShare = 12
	mostColors = 3
)

// How much a color counts by where it's used.
const (
	groundWeight     = 24.0
	backgroundWeight = 6.0
	fillWeight       = 3.0
	textWeight       = 2.0
	edgeWeight       = 1.0
)

var (
	scriptBlock = regexp.MustCompile(`(?is)<script\b.*?</script\s*>`)
	htmlComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	cssComment  = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssVar      = regexp.MustCompile(`--([\w-]+)\s*:\s*([^;{}]+)`)
	varUse      = regexp.MustCompile(`var\(\s*--([\w-]+)[^)]*\)`)
	declaration = regexp.MustCompile(`(?i)(?:^|[\s;{"'])(background-color|background|color|fill|stroke|border-color|border|outline-color|outline)\s*:\s*([^;{}"']+)`)
	colorAttr   = regexp.MustCompile(`(?i)\s(bgcolor|fill|stroke)\s*=\s*["']([^"']+)["']`)
	colorToken  = regexp.MustCompile(`(?i)#[0-9a-f]{3,8}\b|\b(?:rgb|hsl)a?\([^)]*\)|\b(?:white|black|red|orange|yellow|green|teal|blue|navy|purple|violet|pink|gray|grey)\b`)
	groundRule  = regexp.MustCompile(`(?is)(?:^|[\s,}>])(?:html|body|:root)\b[^{}]*\{([^{}]*)\}`)
	groundTag   = regexp.MustCompile(`(?is)<(?:html|body)\b[^>]*>`)
	styleAttr   = regexp.MustCompile(`(?is)\sstyle\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	classAttr   = regexp.MustCompile(`(?is)\sclass\s*=\s*(?:"([^"]*)"|'([^']*)')`)
	utility     = regexp.MustCompile(`(?:^|[\s:])(bg|from|via|to|fill|text|border|stroke)-(slate|gray|zinc|neutral|stone|red|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink|rose|black|white)(?:-(\d{2,3}))?(?:\s|$|/)`)
)

// utilityColors are the utility classes' color families, by the bright shade's
// color: enough to tell which of the library's colors one is.
var utilityColors = map[string]string{
	"red": "#ef4444", "orange": "#f97316", "amber": "#f59e0b", "yellow": "#eab308", "lime": "#84cc16", "green": "#22c55e", "emerald": "#10b981",
	"teal": "#14b8a6", "cyan": "#06b6d4", "sky": "#0ea5e9", "blue": "#3b82f6", "indigo": "#6366f1", "violet": "#8b5cf6", "purple": "#a855f7",
	"fuchsia": "#d946ef", "pink": "#ec4899", "rose": "#f43f5e", "slate": "#64748b", "gray": "#6b7280", "zinc": "#71717a", "neutral": "#737373", "stone": "#78716c",
	"black": "#000000", "white": "#ffffff",
}

var namedColors = map[string]string{
	"white": "#ffffff", "black": "#000000", "red": "#ff0000", "orange": "#ffa500", "yellow": "#ffff00", "green": "#008000", "teal": "#008080",
	"blue": "#0000ff", "navy": "#000080", "purple": "#800080", "violet": "#ee82ee", "pink": "#ffc0cb", "gray": "#808080", "grey": "#808080",
}

type rgb struct{ r, g, b float64 }

// LookOf is what f shows.
func LookOf(f Frame) Look {
	page := cssComment.ReplaceAllString(htmlComment.ReplaceAllString(base64Data.ReplaceAllString(scriptBlock.ReplaceAllString(f.HTML, ""), ""), ""), "")
	vars := map[string]string{}
	for _, m := range cssVar.FindAllStringSubmatch(page, -1) {
		if _, had := vars[m[1]]; !had { // the first is :root's, and what a page's dark switch overrides later is unknown here
			vars[m[1]] = m[2]
		}
	}
	// Variables naming variables multiply: what they may add up to is shared by the
	// whole page, and past it the look is read from what's left.
	left := mostResolved
	resolve := func(v string) string {
		for range 3 {
			if !strings.Contains(v, "var(") {
				break
			}
			v = varUse.ReplaceAllStringFunc(v, func(use string) string {
				value := vars[varUse.FindStringSubmatch(use)[1]]
				if len(value) > left {
					return ""
				}
				left -= len(value)
				return value
			})
		}
		return v
	}

	counts := map[string]float64{}
	add := func(value string, weight float64) {
		colors := colorsIn(resolve(value))
		for _, c := range colors {
			if name := swatchOf(c); name != "" {
				counts[name] += weight / float64(len(colors))
			}
		}
	}
	for _, m := range declaration.FindAllStringSubmatch(page, -1) {
		add(m[2], weightOf(strings.ToLower(m[1])))
	}
	for _, m := range colorAttr.FindAllStringSubmatch(page, -1) {
		add(m[2], fillWeight)
	}
	for _, m := range classAttr.FindAllStringSubmatch(page, -1) {
		for _, u := range utility.FindAllStringSubmatch(m[1]+m[2], -1) {
			if c, ok := utilityOf(u[2], u[3]); ok {
				counts[swatchOf(c)] += weightOf(utilityProperty(u[1]))
			}
		}
	}
	for _, e := range f.Elements {
		add(e.Fill, fillWeight)
		add(e.Stroke, edgeWeight)
		add(e.Color, textWeight)
	}

	ground, known := groundOf(f, page, resolve)
	if !known {
		ground = rgb{1, 1, 1} // what a browser draws under a page that says nothing
	}
	if name := swatchOf(ground); name != "" {
		counts[name] += groundWeight
	}
	delete(counts, "")

	look := Look{Theme: "light", Device: "desktop"}
	if luminance(ground) < 0.45 { // middling grounds carry light text
		look.Theme = "dark"
	}
	if f.Width > 0 && f.Width <= phoneWidest {
		look.Device = "phone"
	}
	look.Colors = dominant(counts)
	return look
}

func weightOf(property string) float64 {
	switch property {
	case "background", "background-color":
		return backgroundWeight
	case "fill", "stroke":
		return fillWeight
	case "color":
		return textWeight
	}
	return edgeWeight
}

func utilityProperty(prefix string) string {
	switch prefix {
	case "bg", "from", "via", "to":
		return "background"
	case "fill":
		return "fill"
	case "text":
		return "color"
	}
	return "border"
}

// utilityOf is the color of a utility class's family and shade. Its lightness
// follows the shade, as a dark blue is blue but a near-black gray is black.
func utilityOf(family, shade string) (rgb, bool) {
	base, ok := parseColor(utilityColors[family])
	if !ok {
		return rgb{}, false
	}
	n, err := strconv.Atoi(shade)
	if err != nil {
		return base, true
	}
	mix := func(to float64, by float64) rgb {
		return rgb{base.r + (to-base.r)*by, base.g + (to-base.g)*by, base.b + (to-base.b)*by}
	}
	switch {
	case n < 500:
		return mix(1, float64(500-n)/500), true
	case n > 500:
		return mix(0, float64(n-500)/500), true
	}
	return base, true
}

// groundOf is the page's ground: the frame's background unless the page's html or
// body says otherwise, or failing all of those the first background of the page,
// usually the block filling it.
func groundOf(f Frame, page string, resolve func(string) string) (rgb, bool) {
	first := func(css string) (rgb, bool) {
		for _, m := range declaration.FindAllStringSubmatch(css, -1) {
			if p := strings.ToLower(m[1]); p != "background" && p != "background-color" {
				continue
			}
			if colors := colorsIn(resolve(m[2])); len(colors) > 0 {
				return colors[0], true
			}
		}
		return rgb{}, false
	}
	var found rgb
	ok := false
	for _, m := range groundRule.FindAllStringSubmatch(page, -1) {
		if c, has := first(m[1]); has {
			found, ok = c, true
		}
	}
	for _, tag := range groundTag.FindAllString(page, -1) {
		if m := styleAttr.FindStringSubmatch(tag); m != nil {
			if c, has := first(m[1] + m[2]); has {
				found, ok = c, true
			}
		}
		if m := classAttr.FindStringSubmatch(tag); m != nil {
			for _, u := range utility.FindAllStringSubmatch(m[1]+m[2], -1) {
				if u[1] == "bg" {
					if c, has := utilityOf(u[2], u[3]); has {
						found, ok = c, true
					}
				}
			}
		}
	}
	if ok {
		return found, true
	}
	if c, has := parseColor(f.Background); has {
		return c, true
	}
	if c, has := first(page); has {
		return c, true
	}
	return rgb{}, false
}

func dominant(counts map[string]float64) []LookColor {
	total := 0.0
	for _, w := range counts {
		total += w
	}
	out := []LookColor{}
	for name, w := range counts {
		if share := int(math.Round(100 * w / total)); share >= leastShare {
			out = append(out, LookColor{name, share})
		}
	}
	slices.SortFunc(out, func(a, b LookColor) int {
		return cmp.Or(cmp.Compare(b.Share, a.Share), cmp.Compare(a.Name, b.Name))
	})
	return out[:min(mostColors, len(out))]
}

// colorsIn are the opaque enough colors value names.
func colorsIn(value string) []rgb {
	var out []rgb
	for _, tok := range colorToken.FindAllString(value, -1) {
		if c, ok := parseColor(tok); ok {
			out = append(out, c)
		}
	}
	return out
}

// faintest is the least opacity a color counts from: below it, it tints what's
// under it rather than shows.
const faintest = 0.3

// parseColor reads a CSS color: a name, hex, rgb() or hsl().
func parseColor(s string) (rgb, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if named, ok := namedColors[s]; ok {
		s = named
	}
	if hex, ok := strings.CutPrefix(s, "#"); ok {
		return parseHex(hex)
	}
	open, close := strings.IndexByte(s, '('), strings.LastIndexByte(s, ')')
	if open < 0 || close < open {
		return rgb{}, false
	}
	parts := strings.FieldsFunc(s[open+1:close], func(r rune) bool { return r == ',' || r == ' ' || r == '/' })
	if len(parts) < 3 {
		return rgb{}, false
	}
	if len(parts) > 3 {
		if a, ok := number(parts[3], 1); !ok || a < faintest {
			return rgb{}, false
		}
	}
	if strings.HasPrefix(s, "rgb") {
		r, ok1 := number(parts[0], 255)
		g, ok2 := number(parts[1], 255)
		b, ok3 := number(parts[2], 255)
		if !ok1 || !ok2 || !ok3 {
			return rgb{}, false
		}
		return rgb{clamp(r / 255), clamp(g / 255), clamp(b / 255)}, true
	}
	h, err := strconv.ParseFloat(strings.TrimSuffix(parts[0], "deg"), 64)
	sat, ok2 := number(parts[1], 1)
	light, ok3 := number(parts[2], 1)
	if err != nil || !ok2 || !ok3 {
		return rgb{}, false
	}
	return fromHSL(h, clamp(sat), clamp(light)), true
}

// number reads a CSS number, a percentage of whole.
func number(s string, whole float64) (float64, bool) {
	pct := strings.HasSuffix(s, "%")
	n, err := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if err != nil {
		return 0, false
	}
	if pct {
		n = n / 100 * whole
	}
	return n, true
}

func clamp(x float64) float64 { return math.Max(0, math.Min(1, x)) }

func parseHex(hex string) (rgb, bool) {
	switch len(hex) {
	case 3, 4:
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]}) + strings.Repeat(hex[3:], 2)
	case 6, 8:
	default:
		return rgb{}, false
	}
	v, err := strconv.ParseUint(hex, 16, 64)
	if err != nil {
		return rgb{}, false
	}
	if len(hex) == 8 {
		if float64(v&0xff)/255 < faintest {
			return rgb{}, false
		}
		v >>= 8
	}
	return rgb{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}, true
}

func fromHSL(h, s, l float64) rgb {
	h = math.Mod(math.Mod(h, 360)+360, 360) / 60
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h, 2)-1))
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g = c, x
	case 1:
		r, g = x, c
	case 2:
		g, b = c, x
	case 3:
		g, b = x, c
	case 4:
		r, b = x, c
	default:
		r, b = c, x
	}
	m := l - c/2
	return rgb{r + m, g + m, b + m}
}

func toHSL(c rgb) (h, s, l float64) {
	high, low := math.Max(c.r, math.Max(c.g, c.b)), math.Min(c.r, math.Min(c.g, c.b))
	l = (high + low) / 2
	d := high - low
	if d == 0 {
		return 0, 0, l
	}
	s = d / (1 - math.Abs(2*l-1))
	switch high {
	case c.r:
		h = math.Mod((c.g-c.b)/d, 6)
	case c.g:
		h = (c.b-c.r)/d + 2
	default:
		h = (c.r-c.g)/d + 4
	}
	return math.Mod(h*60+360, 360), s, l
}

func luminance(c rgb) float64 { return 0.2126*c.r + 0.7152*c.g + 0.0722*c.b }

// swatchOf is which of the library's colors c reads as, "" for a middling gray,
// which reads as none.
func swatchOf(c rgb) string {
	h, s, l := toHSL(c)
	if l < 0.14 {
		return "black"
	}
	if l > 0.88 || s < 0.14 {
		switch {
		case l >= 0.72:
			return "paper"
		case l <= 0.3:
			return "black"
		}
		return ""
	}
	if s < 0.3 && l < 0.22 {
		return "black"
	}
	switch {
	case h < 25 || h >= 330:
		return "red"
	case h < 70:
		return "yellow"
	case h < 165:
		return "green"
	case h < 200:
		return "teal"
	case h < 240:
		return "blue"
	}
	return "violet"
}

package onboardview

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/enowx/enowx/ui/theme"
)

// fadeColor is c at this much of its own alpha.
func fadeColor(c ui.Color, a float32) ui.Color {
	return theme.Fade(c, float64(a)*float64(c.A)/255)
}

// Colors cross in Oklab, so that nothing goes muddy halfway.
func linear(v uint8) float64 {
	x := float64(v) / 255
	if x <= .04045 {
		return x / 12.92
	}
	return math.Pow((x+.055)/1.055, 2.4)
}

func gamma(v float64) uint8 {
	v = math.Max(0, math.Min(v, 1))
	if v <= .0031308 {
		v *= 12.92
	} else {
		v = 1.055*math.Pow(v, 1/2.4) - .055
	}
	return uint8(math.Round(v * 255))
}

func oklab(c ui.Color) [3]float64 {
	r, g, b := linear(c.R), linear(c.G), linear(c.B)
	l := math.Cbrt(.4122*r + .5363*g + .0514*b)
	m := math.Cbrt(.2119*r + .6807*g + .1074*b)
	s := math.Cbrt(.0883*r + .2817*g + .6300*b)
	return [3]float64{.2105*l + .7936*m - .0041*s, 1.978*l - 2.4286*m + .4506*s, .0259*l + .7828*m - .8087*s}
}

// mixColor is a and b part of the way, in Oklab.
func mixColor(a, b ui.Color, x float32) ui.Color {
	if x <= 0 {
		return a
	}
	if x >= 1 {
		return b
	}
	A, B, t := oklab(a), oklab(b), float64(x)
	L, p, q := A[0]+(B[0]-A[0])*t, A[1]+(B[1]-A[1])*t, A[2]+(B[2]-A[2])*t
	l := math.Pow(L+.3963*p+.2158*q, 3)
	m := math.Pow(L-.1056*p-.0639*q, 3)
	s := math.Pow(L-.0895*p-1.2915*q, 3)
	return ui.RGBA(gamma(4.0767*l-3.3077*m+.2310*s), gamma(-1.2684*l+2.6098*m-.3413*s), gamma(-.0042*l-.7034*m+1.7076*s), (float32(a.A)+(float32(b.A)-float32(a.A))*x)/255)
}

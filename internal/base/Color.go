/*
    Source: https://github.com/gerow/go-color/blob/master/color.go
	Package color implements some simple RGB/HSL color conversions for golang.
	By Brandon Thomson
	Adapted from
	http://code.google.com/p/closure-library/source/browse/trunk/closure/goog/color/color.js
	and algorithms on easyrgb.com.
	To maintain accuracy between conversions we use floats in the color types.
	If you are storing lots of colors and care about memory use you might want
	to use something based on byte types instead.
	Also, color types don't verify their validity before converting. If you do
	something like RGB{10,20,30}.ToHSL() the results will be undefined. All
	values must be between 0 and 1.
*/

package base

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
)

type Color3b struct {
	R, G, B uint8
}
type Color3f struct {
	R, G, B float64
}

func NewColor3b(r, g, b uint8) Color3b   { return Color3b{R: r, G: g, B: b} }
func NewColor3f(r, g, b float64) Color3f { return Color3f{R: r, G: g, B: b} }

func (x *Color3b) Broadcast(v uint8) {
	x.R = v
	x.G = v
	x.B = v
}
func (x *Color3f) Broadcast(v float64) {
	x.R = v
	x.G = v
	x.B = v
}

const maxUint8f = float64(math.MaxUint8)
const maxUint8OO = 1.0 / maxUint8f

func unquantizeColor(b uint8) float64 { return float64(b) * maxUint8OO }
func quantizeColor(f float64) uint8   { return uint8(f * maxUint8f) }

func (x Color3b) Unquantize(srgb bool) (result Color3f) {
	result = Color3f{
		R: unquantizeColor(x.R),
		G: unquantizeColor(x.G),
		B: unquantizeColor(x.B),
	}
	if srgb {
		result = result.SrgbToLinear()
	}
	return
}
func (x Color3f) Quantize(srgb bool) (result Color3b) {
	if srgb {
		x = x.LinearToSrgb()
	}
	result = Color3b{
		R: quantizeColor(x.R),
		G: quantizeColor(x.G),
		B: quantizeColor(x.B),
	}
	return
}

func (x Color3b) Ansi(fg bool) string {
	if !enableAnsiColor {
		return ""
	}
	ansiFmt := ANSI_BG_TRUECOLOR_FMT
	if fg {
		ansiFmt = ANSI_FG_TRUECOLOR_FMT
	}
	return fmt.Sprintf(ansiFmt.String(), uint(x.R), uint(x.G), uint(x.B))
}
func (x Color3b) Lerp(o Color3b, f float64) Color3b {
	return x.Unquantize(false).Lerp(o.Unquantize(false), f).Quantize(false)
}

var colorHslHot = Color3b{R: 255, G: 210, B: 128}.Unquantize(true)
var colorHslCold = Color3b{R: 103, G: 79, B: 73}.Unquantize(true)

func NewColdHotColor(f float64) Color3f {
	color := colorHslCold.Lerp(colorHslHot, f)
	return color.Brightness(0.45 + 0.40*f*f)
}

func NewHeatmapColor(f float64) Color3f {
	f = Saturate(f)
	x1 := [4]float64{1, f, f * f, f * f * f}   // 1 x x2 x3
	x2 := [2]float64{x1[3] * f, x1[3] * f * f} // x4 x5
	return Color3f{
		R: Saturate(Dot(x1[:], []float64{-0.027780558, +1.228188385, +0.278906882, +3.892783760}) + Dot(x2[:], []float64{-8.490712758, +4.069046086})),
		G: Saturate(Dot(x1[:], []float64{+0.014065206, +0.015360518, +1.605395918, -4.821108251}) + Dot(x2[:], []float64{+8.389314011, -4.193858954})),
		B: Saturate(Dot(x1[:], []float64{-0.019628385, +3.122510347, -5.893222355, +2.798380308}) + Dot(x2[:], []float64{-3.608884658, +4.324996022})),
	}
}
func NewPastelizerColor(f float64) Color3f {
	_, h := math.Modf(Saturate(f) + 0.92620819117478)
	h = math.Abs(h) * 6.2831853071796
	cocg_x, cocg_y := 0.25*math.Cos(h), 0.25*math.Sin(h)
	br_x, br_y := -cocg_x-cocg_y, cocg_x-cocg_y
	c_x, c_y, c_z := 0.729+br_y, 0.729+cocg_y, 0.729+br_x
	return Color3f{
		R: Saturate(c_x * c_x),
		G: Saturate(c_y * c_y),
		B: Saturate(c_z * c_z),
	}
}

func nextFloat01(r *rand.Rand) float64 {
	// see official comment in func (r *Rand) Float64() float64
	return float64(r.Int63n(1<<53)) / (1 << 53)
}

func NewColorFromHash(h uint64) Color3f {
	rnd := rand.New(rand.NewSource(int64(h)))
	return NewPastelizerColor(nextFloat01(rnd))
}
func NewColorFromStringHash(s string) Color3f {
	hasher := fnv.New64a()
	hasher.Write(UnsafeBytesFromString(s))
	return NewColorFromHash(hasher.Sum64())
}

func (x Color3f) Brightness(f float64) Color3f {
	brightness := math.Exp2((f*2.0 - 1.0) * 4.0)
	return Color3f{
		R: Saturate(x.R * brightness),
		G: Saturate(x.G * brightness),
		B: Saturate(x.B * brightness),
	}
}
func (x Color3f) Lerp(o Color3f, f float64) Color3f {
	return Color3f{
		R: Lerp(x.R, o.R, f),
		G: Lerp(x.G, o.G, f),
		B: Lerp(x.B, o.B, f),
	}
}
func (x Color3f) SrgbToLinear() Color3f {
	return Color3f{
		R: linearizeColor(x.R),
		G: linearizeColor(x.G),
		B: linearizeColor(x.B),
	}
}
func (x Color3f) LinearToSrgb() Color3f {
	return Color3f{
		R: delinearizeColor(x.R),
		G: delinearizeColor(x.G),
		B: delinearizeColor(x.B),
	}
}

func linearizeColor(c float64) float64 {
	if c <= 0.04045 {
		return c / 12.92
	} else {
		return math.Pow(float64((c+0.055)/1.055), 2.4)
	}
}
func delinearizeColor(c float64) float64 {
	if c <= 0.0031308 {
		return 12.92 * c
	} else {
		return 1.055*math.Pow(float64(c), 1/2.4) - 0.055
	}
}

func (x Color3f) RgbToOklab() Color3f {
	l := 0.4122214708*x.R + 0.5363325362*x.G + 0.0514459929*x.B
	m := 0.2119034982*x.R + 0.6806995451*x.G + 0.1073969566*x.B
	s := 0.0883024619*x.R + 0.2817188376*x.G + 0.6299787005*x.B

	l = math.Cbrt(l)
	m = math.Cbrt(m)
	s = math.Cbrt(s)

	return Color3f{
		R: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		G: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		B: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

func (x Color3f) OklabToRgb() Color3f {
	l := x.R + 0.3963377774*x.G + 0.2158037573*x.B
	m := x.R - 0.1055613458*x.G - 0.0638541728*x.B
	s := x.R - 0.0894841775*x.G - 1.2914855480*x.B

	l = l * l * l
	m = m * m * m
	s = s * s * s

	return Color3f{
		R: 4.0767416621*l - 3.3077115913*m + 0.2309699292*s,
		G: -1.2684380046*l + 2.6097574011*m - 0.3413193965*s,
		B: -0.0041960863*l - 0.7034186147*m + 1.7076147010*s,
	}
}

// func (x Color3f) RgbToHSL() Color3f {
// 	var h, s, l float64

// 	r := x.R
// 	g := x.G
// 	b := x.B

// 	max := math.Max(math.Max(r, g), b)
// 	min := math.Min(math.Min(r, g), b)

// 	// Luminosity is the average of the max and min rgb color intensities.
// 	l = (max + min) / 2

// 	// saturation
// 	delta := max - min
// 	if delta == 0 {
// 		// it's gray
// 		return Color3f{0, 0, l}
// 	}

// 	// it's not gray
// 	if l < 0.5 {
// 		s = delta / (max + min)
// 	} else {
// 		s = delta / (2 - max - min)
// 	}

// 	// hue
// 	r2 := (((max - r) / 6) + (delta / 2)) / delta
// 	g2 := (((max - g) / 6) + (delta / 2)) / delta
// 	b2 := (((max - b) / 6) + (delta / 2)) / delta
// 	switch {
// 	case r == max:
// 		h = b2 - g2
// 	case g == max:
// 		h = (1.0 / 3.0) + r2 - b2
// 	case b == max:
// 		h = (2.0 / 3.0) + g2 - r2
// 	}

// 	// fix wraparounds
// 	switch {
// 	case h < 0:
// 		h++
// 	case h > 1:
// 		h--
// 	}

// 	return Color3f{h, s, l}
// }

// func hueToRgb(v1, v2, h float64) float64 {
// 	if h < 0 {
// 		h++
// 	}
// 	if h > 1 {
// 		h--
// 	}
// 	switch {
// 	case 6*h < 1:
// 		return (v1 + (v2-v1)*6*h)
// 	case 2*h < 1:
// 		return v2
// 	case 3*h < 2:
// 		return v1 + (v2-v1)*((2.0/3.0)-h)*6
// 	}
// 	return v1
// }

// func (x Color3f) HslToRgb() Color3f {
// 	h := x.R
// 	s := x.G
// 	l := x.B

// 	if s == 0 {
// 		// it's gray
// 		return Color3f{l, l, l}
// 	}

// 	var v1, v2 float64
// 	if l < 0.5 {
// 		v2 = l * (1 + s)
// 	} else {
// 		v2 = (l + s) - (s * l)
// 	}

// 	v1 = 2*l - v2

// 	r := hueToRgb(v1, v2, h+(1.0/3.0))
// 	g := hueToRgb(v1, v2, h)
// 	b := hueToRgb(v1, v2, h-(1.0/3.0))

// 	return Color3f{r, g, b}
// }

// A nudge to make truncation round to nearest number instead of flooring

func (x Color3b) ToHTML(alpha uint8) string {
	return fmt.Sprintf("#%02x%02x%02x%02x", x.R, x.G, x.B, alpha)
}
